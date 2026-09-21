package converty

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Integration is a shop's Converty connection.
type Integration struct {
	ShopID                uuid.UUID
	ConvertyStoreID       string
	StoreName             string
	StoreSlug             string
	StoreDomain           string
	StoreCurrency         string
	StoreCountry          string
	Scopes                string
	AccessTokenEncrypted  string
	RefreshTokenEncrypted string
	AccessTokenExpiresAt  *time.Time
	Status                string
	// WebhookSubscriptions maps webhook event -> Converty hook id, so hooks
	// can be removed upstream when the shop disconnects.
	WebhookSubscriptions map[string]string
}

// SaveIntegration upserts the shop's Converty integration with freshly
// exchanged tokens.
func (s *Service) SaveIntegration(ctx context.Context, shopID uuid.UUID, store Store, scopes string, tok Token, now time.Time) error {
	accessEnc, err := s.cipher.Encrypt(tok.AccessToken)
	if err != nil {
		return err
	}
	refreshEnc, err := s.cipher.Encrypt(tok.RefreshToken)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO converty_integrations (
			shop_id, converty_store_id, store_name, store_slug, store_domain,
			store_currency, store_country, scopes,
			access_token_encrypted, refresh_token_encrypted,
			access_token_expires_at, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (shop_id) DO UPDATE SET
			converty_store_id       = EXCLUDED.converty_store_id,
			store_name              = EXCLUDED.store_name,
			store_slug              = EXCLUDED.store_slug,
			store_domain            = EXCLUDED.store_domain,
			store_currency          = EXCLUDED.store_currency,
			store_country           = EXCLUDED.store_country,
			scopes                  = EXCLUDED.scopes,
			access_token_encrypted  = EXCLUDED.access_token_encrypted,
			refresh_token_encrypted = EXCLUDED.refresh_token_encrypted,
			access_token_expires_at = EXCLUDED.access_token_expires_at,
			status                  = EXCLUDED.status,
			updated_at              = now()`,
		shopID, store.ID, store.Name, store.Slug, store.Domain, store.Currency, store.Country,
		scopes, accessEnc, refreshEnc, tok.ExpiresAt(now), "connected",
	)
	return err
}

// Integration returns the shop's Converty connection.
func (s *Service) Integration(ctx context.Context, shopID uuid.UUID) (Integration, error) {
	var i Integration
	var subs json.RawMessage
	err := s.pool.QueryRow(ctx, `
		SELECT shop_id, converty_store_id, store_name, store_slug, store_domain,
		       store_currency, store_country, scopes,
		       access_token_encrypted, refresh_token_encrypted,
		       access_token_expires_at, status, webhook_subscriptions
		FROM converty_integrations
		WHERE shop_id = $1`,
		shopID,
	).Scan(
		&i.ShopID, &i.ConvertyStoreID, &i.StoreName, &i.StoreSlug, &i.StoreDomain,
		&i.StoreCurrency, &i.StoreCountry, &i.Scopes,
		&i.AccessTokenEncrypted, &i.RefreshTokenEncrypted,
		&i.AccessTokenExpiresAt, &i.Status, &subs,
	)
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

// SaveWebhookSubscriptions replaces the shop's stored Converty hook map
// (event -> hook id) with subs.
func (s *Service) SaveWebhookSubscriptions(ctx context.Context, shopID uuid.UUID, subs map[string]string) error {
	raw, err := json.Marshal(subs)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE converty_integrations
		SET webhook_subscriptions = $2::jsonb, updated_at = now()
		WHERE shop_id = $1`,
		shopID, raw,
	)
	return err
}

// DeleteIntegration removes the shop's Converty connection (tokens included)
// after a disconnect. A missing row is not an error.
func (s *Service) DeleteIntegration(ctx context.Context, shopID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM converty_integrations
		WHERE shop_id = $1`,
		shopID,
	)
	return err
}
