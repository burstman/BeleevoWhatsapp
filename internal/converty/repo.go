package converty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("converty integration not found")

// Integration is one Converty store connection belonging to a shop. A shop
// account can connect several stores, so integration-scoped operations are
// keyed by ID and always re-checked against the owning shop. Every row carries
// its own Converty OAuth app credentials, because each merchant's Converty app
// is registered inside their own store.
type Integration struct {
	ID                    uuid.UUID
	Active                bool
	ShopID                uuid.UUID
	ConvertyStoreID       string
	StoreName             string
	StoreSlug             string
	StoreDomain           string
	StoreCurrency         string
	StoreCountry          string
	Scopes                string
	ConvertyClientID      string
	CSecretEncrypted      string
	AccessTokenEncrypted  string
	RefreshTokenEncrypted string
	AccessTokenExpiresAt  *time.Time
	Status                string
	// WebhookSubscriptions maps webhook event -> Converty hook id, so hooks
	// can be removed upstream when the shop disconnects.
	WebhookSubscriptions map[string]string
}

// CreatePendingIntegration records a shop's Converty OAuth app credentials
// before the OAuth round-trip, returning the id the callback will complete.
// The client secret is encrypted at rest. A pending row never has tokens until
// the authorization code is exchanged.
func (s *Service) CreatePendingIntegration(ctx context.Context, shopID uuid.UUID, clientID, clientSecretEncrypted string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO converty_integrations (
			shop_id, converty_client_id, converty_client_secret_encrypted, status, active
		) VALUES ($1, $2, $3, 'pending', true)
		RETURNING id`,
		shopID, clientID, clientSecretEncrypted,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// ClientCredentials returns the integration's own Converty OAuth client id and
// decrypted client secret, scoped to its owning shop. The OAuth authorize,
// code-exchange and refresh calls all authenticate with these.
func (s *Service) ClientCredentials(ctx context.Context, shopID, id uuid.UUID) (clientID, clientSecret string, err error) {
	var encrypted string
	err = s.pool.QueryRow(ctx, `
		SELECT converty_client_id, converty_client_secret_encrypted
		FROM converty_integrations
		WHERE id = $1 AND shop_id = $2`,
		id, shopID,
	).Scan(&clientID, &encrypted)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	clientSecret, err = s.cipher.Decrypt(encrypted)
	if err != nil {
		return "", "", fmt.Errorf("decrypt client secret: %w", err)
	}
	return clientID, clientSecret, nil
}

// CompleteIntegration fills the pending row created at connect time with the
// exchanged store details, scopes and tokens and marks it connected.
func (s *Service) CompleteIntegration(ctx context.Context, shopID, id uuid.UUID, store Store, scopes string, tok Token, now time.Time) error {
	accessEnc, err := s.cipher.Encrypt(tok.AccessToken)
	if err != nil {
		return err
	}
	refreshEnc, err := s.cipher.Encrypt(tok.RefreshToken)
	if err != nil {
		return err
	}

	currency, country := string(store.Currency), string(store.Country)

	tag, err := s.pool.Exec(ctx, `
		UPDATE converty_integrations
		SET converty_store_id         = $3,
		    store_name                = $4,
		    store_slug                = $5,
		    store_domain              = $6,
		    store_currency            = $7,
		    store_country             = $8,
		    scopes                    = $9,
		    access_token_encrypted    = $10,
		    refresh_token_encrypted   = $11,
		    access_token_expires_at   = $12,
		    status                    = 'connected',
		    active                    = true,
		    updated_at                = now()
		WHERE id = $1 AND shop_id = $2`,
		id, shopID, store.ID, store.Name, store.Slug, store.Domain, currency, country,
		scopes, accessEnc, refreshEnc, tok.ExpiresAt(now),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// IntegrationByID returns one of a shop's Converty connections, scoped to the
// owning shop so a tenant can never read another shop's row.
func (s *Service) IntegrationByID(ctx context.Context, shopID, id uuid.UUID) (Integration, error) {
	var i Integration
	var subs json.RawMessage
	err := s.pool.QueryRow(ctx, `
		SELECT id, active, shop_id, converty_store_id, store_name, store_slug, store_domain,
		       store_currency, store_country, scopes,
		       converty_client_id, converty_client_secret_encrypted,
		       access_token_encrypted, refresh_token_encrypted,
		       access_token_expires_at, status, webhook_subscriptions
		FROM converty_integrations
		WHERE id = $1 AND shop_id = $2`,
		id, shopID,
	).Scan(
		&i.ID, &i.Active, &i.ShopID, &i.ConvertyStoreID, &i.StoreName, &i.StoreSlug, &i.StoreDomain,
		&i.StoreCurrency, &i.StoreCountry, &i.Scopes,
		&i.ConvertyClientID, &i.CSecretEncrypted,
		&i.AccessTokenEncrypted, &i.RefreshTokenEncrypted,
		&i.AccessTokenExpiresAt, &i.Status, &subs,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return i, ErrNotFound
	}
	if err != nil {
		return i, err
	}
	if len(subs) > 0 && string(subs) != "{}" {
		if err := json.Unmarshal(subs, &i.WebhookSubscriptions); err != nil {
			return i, err
		}
	}
	return i, nil
}

// Integrations lists every Converty store connection belonging to the shop,
// newest first.
func (s *Service) Integrations(ctx context.Context, shopID uuid.UUID) ([]Integration, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, active, shop_id, converty_store_id, store_name, store_slug, store_domain,
		       store_currency, store_country, scopes,
		       converty_client_id, converty_client_secret_encrypted,
		       access_token_encrypted, refresh_token_encrypted,
		       access_token_expires_at, status, webhook_subscriptions
		FROM converty_integrations
		WHERE shop_id = $1
		ORDER BY created_at DESC`,
		shopID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Integration
	for rows.Next() {
		var i Integration
		var subs json.RawMessage
		if err := rows.Scan(
			&i.ID, &i.Active, &i.ShopID, &i.ConvertyStoreID, &i.StoreName, &i.StoreSlug, &i.StoreDomain,
			&i.StoreCurrency, &i.StoreCountry, &i.Scopes,
			&i.ConvertyClientID, &i.CSecretEncrypted,
			&i.AccessTokenEncrypted, &i.RefreshTokenEncrypted,
			&i.AccessTokenExpiresAt, &i.Status, &subs,
		); err != nil {
			return nil, err
		}
		if len(subs) > 0 && string(subs) != "{}" {
			if err := json.Unmarshal(subs, &i.WebhookSubscriptions); err != nil {
				return nil, err
			}
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// IntegrationsAll lists every Converty store connection on the platform,
// newest first. On this single-client deployment all stores belong to the
// operator, so the Shop Integration page shows them in one place.
func (s *Service) IntegrationsAll(ctx context.Context) ([]Integration, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, active, shop_id, converty_store_id, store_name, store_slug, store_domain,
		       store_currency, store_country, scopes,
		       converty_client_id, converty_client_secret_encrypted,
		       access_token_encrypted, refresh_token_encrypted,
		       access_token_expires_at, status, webhook_subscriptions
		FROM converty_integrations
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Integration
	for rows.Next() {
		var i Integration
		var subs json.RawMessage
		if err := rows.Scan(
			&i.ID, &i.Active, &i.ShopID, &i.ConvertyStoreID, &i.StoreName, &i.StoreSlug, &i.StoreDomain,
			&i.StoreCurrency, &i.StoreCountry, &i.Scopes,
			&i.ConvertyClientID, &i.CSecretEncrypted,
			&i.AccessTokenEncrypted, &i.RefreshTokenEncrypted,
			&i.AccessTokenExpiresAt, &i.Status, &subs,
		); err != nil {
			return nil, err
		}
		if len(subs) > 0 && string(subs) != "{}" {
			if err := json.Unmarshal(subs, &i.WebhookSubscriptions); err != nil {
				return nil, err
			}
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// IntegrationByIDAnyShop returns one Converty connection regardless of which
// shop owns it, so cross-shop management actions can resolve the owning shop.
func (s *Service) IntegrationByIDAnyShop(ctx context.Context, id uuid.UUID) (Integration, error) {
	var i Integration
	var subs json.RawMessage
	err := s.pool.QueryRow(ctx, `
		SELECT id, active, shop_id, converty_store_id, store_name, store_slug, store_domain,
		       store_currency, store_country, scopes,
		       converty_client_id, converty_client_secret_encrypted,
		       access_token_encrypted, refresh_token_encrypted,
		       access_token_expires_at, status, webhook_subscriptions
		FROM converty_integrations
		WHERE id = $1`,
		id,
	).Scan(
		&i.ID, &i.Active, &i.ShopID, &i.ConvertyStoreID, &i.StoreName, &i.StoreSlug, &i.StoreDomain,
		&i.StoreCurrency, &i.StoreCountry, &i.Scopes,
		&i.ConvertyClientID, &i.CSecretEncrypted,
		&i.AccessTokenEncrypted, &i.RefreshTokenEncrypted,
		&i.AccessTokenExpiresAt, &i.Status, &subs,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return i, ErrNotFound
	}
	if err != nil {
		return i, err
	}
	if len(subs) > 0 && string(subs) != "{}" {
		if err := json.Unmarshal(subs, &i.WebhookSubscriptions); err != nil {
			return i, err
		}
	}
	return i, nil
}

// CountIntegrations returns how many Converty connections a shop currently
// has. Used to detect orphaned placeholder shops (created at connect time)
// after an abandoned authorization.
func (s *Service) CountIntegrations(ctx context.Context, shopID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM converty_integrations WHERE shop_id = $1`, shopID,
	).Scan(&n)
	return n, err
}

// UpdateIntegrationInfo edits the display details (name, domain) a shop
// client sees on the integration list. Tokens are never touched.
func (s *Service) UpdateIntegrationInfo(ctx context.Context, shopID, id uuid.UUID, name, domain string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE converty_integrations
		SET store_name = $3, store_domain = $4, updated_at = now()
		WHERE id = $1 AND shop_id = $2`,
		id, shopID, name, domain,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetIntegrationActive pauses (active=false) or resumes an integration.
// Paused integrations keep their tokens and webhooks but are excluded from
// new order-to-message routing.
func (s *Service) SetIntegrationActive(ctx context.Context, shopID, id uuid.UUID, active bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE converty_integrations
		SET active = $3, updated_at = now()
		WHERE id = $1 AND shop_id = $2`,
		id, shopID, active,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetIntegrationStatus records the last connectivity result (connected /
// error / offline) surfaced on the integration list.
func (s *Service) SetIntegrationStatus(ctx context.Context, shopID, id uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE converty_integrations
		SET status = $3, updated_at = now()
		WHERE id = $1 AND shop_id = $2`,
		id, shopID, status,
	)
	return err
}

// SaveTokens refreshes just the token columns after an access-token
// rotation; the rest of the integration (store info, subscriptions) is
// left untouched.
func (s *Service) SaveTokens(ctx context.Context, shopID, id uuid.UUID, tok Token, now time.Time) error {
	accessEnc, err := s.cipher.Encrypt(tok.AccessToken)
	if err != nil {
		return err
	}
	refreshEnc, err := s.cipher.Encrypt(tok.RefreshToken)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE converty_integrations
		SET access_token_encrypted = $3,
		    refresh_token_encrypted = $4,
		    access_token_expires_at = $5,
		    status = 'connected',
		    updated_at = now()
		WHERE id = $1 AND shop_id = $2`,
		id, shopID, accessEnc, refreshEnc, tok.ExpiresAt(now),
	)
	return err
}

// SaveWebhookSubscriptions replaces one integration's stored Converty hook
// map (event -> hook id) with subs.
func (s *Service) SaveWebhookSubscriptions(ctx context.Context, shopID, id uuid.UUID, subs map[string]string) error {
	raw, err := json.Marshal(subs)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE converty_integrations
		SET webhook_subscriptions = $3::jsonb, updated_at = now()
		WHERE id = $1 AND shop_id = $2`,
		id, shopID, raw,
	)
	return err
}

// DeleteIntegration removes one of a shop's Converty connections (tokens
// included) after a disconnect. A missing row is not an error.
func (s *Service) DeleteIntegration(ctx context.Context, shopID, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM converty_integrations
		WHERE id = $1 AND shop_id = $2`,
		id, shopID,
	)
	return err
}
