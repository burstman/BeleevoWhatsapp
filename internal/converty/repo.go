package converty

import (
	"context"
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
	err := s.pool.QueryRow(ctx, `
		SELECT shop_id, converty_store_id, store_name, store_slug, store_domain,
		       store_currency, store_country, scopes,
		       access_token_encrypted, refresh_token_encrypted,
		       access_token_expires_at, status
		FROM converty_integrations
		WHERE shop_id = $1`,
		shopID,
	).Scan(
		&i.ShopID, &i.ConvertyStoreID, &i.StoreName, &i.StoreSlug, &i.StoreDomain,
		&i.StoreCurrency, &i.StoreCountry, &i.Scopes,
		&i.AccessTokenEncrypted, &i.RefreshTokenEncrypted,
		&i.AccessTokenExpiresAt, &i.Status,
	)
	return i, err
}
