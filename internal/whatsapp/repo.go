package whatsapp

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Integration is a shop's WhatsApp connection. Per the new Meta account
// model, waac_id (phone-number side) and messaging_account_id (template and
// billing side) are stored alongside the phone_number_id used for sending —
// existing IDs keep working through Phase 1, which is when sending targets
// the WAAC id instead.
type Integration struct {
	ID                   uuid.UUID
	ShopID               uuid.UUID
	WaacID               string
	PhoneNumberID        string
	MessagingAccountID   string
	BusinessPortfolioID  string
	PhoneNumber          string
	AccessTokenEncrypted string
	Status               string
}

// SaveIntegration creates or replaces the shop's WhatsApp integration with
// fresh credentials.
func (s *Service) SaveIntegration(ctx context.Context, shopID uuid.UUID, accessToken string, integ Integration) error {
	accessEnc, err := s.cipher.Encrypt(accessToken)
	if err != nil {
		return err
	}
	integ.AccessTokenEncrypted = accessEnc
	if integ.Status == "" {
		integ.Status = "connected"
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO whatsapp_integrations (
			shop_id, waac_id, phone_number_id, messaging_account_id,
			business_portfolio_id, phone_number, access_token_encrypted, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (shop_id) DO UPDATE SET
			waac_id                = EXCLUDED.waac_id,
			phone_number_id        = EXCLUDED.phone_number_id,
			messaging_account_id   = EXCLUDED.messaging_account_id,
			business_portfolio_id  = EXCLUDED.business_portfolio_id,
			phone_number           = EXCLUDED.phone_number,
			access_token_encrypted = EXCLUDED.access_token_encrypted,
			status                 = EXCLUDED.status,
			updated_at             = now()`,
		shopID, integ.WaacID, integ.PhoneNumberID, integ.MessagingAccountID,
		integ.BusinessPortfolioID, integ.PhoneNumber, accessEnc, integ.Status,
	)
	return err
}

// Integration returns the shop's WhatsApp connection.
func (s *Service) Integration(ctx context.Context, shopID uuid.UUID) (Integration, error) {
	var i Integration
	err := s.pool.QueryRow(ctx, `
		SELECT id, shop_id, waac_id, phone_number_id, messaging_account_id,
		       business_portfolio_id, phone_number, access_token_encrypted, status
		FROM whatsapp_integrations
		WHERE shop_id = $1`,
		shopID,
	).Scan(
		&i.ID, &i.ShopID, &i.WaacID, &i.PhoneNumberID, &i.MessagingAccountID,
		&i.BusinessPortfolioID, &i.PhoneNumber, &i.AccessTokenEncrypted, &i.Status,
	)
	if err != nil {
		return i, err
	}
	return i, nil
}

// RecordMessage inserts one message send into the messages table for audit
// and delivery-status tracking.
func (s *Service) RecordMessage(ctx context.Context, shopID uuid.UUID, integrationID uuid.UUID, orderID, recipientPhone, templateName, metaMessageID string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO messages (
			shop_id, whatsapp_integration_id, converty_order_id, recipient_phone,
			meta_message_id, status, sent_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 'sent', $6, $6, $6)`,
		shopID, integrationID, orderID, recipientPhone, metaMessageID, now,
	)
	return err
}

// DeleteIntegration removes the shop's WhatsApp connection (tokens included).
// A missing row is not an error.
func (s *Service) DeleteIntegration(ctx context.Context, shopID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM whatsapp_integrations
		WHERE shop_id = $1`,
		shopID,
	)
	return err
}
