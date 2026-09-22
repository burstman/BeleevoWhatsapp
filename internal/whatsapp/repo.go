package whatsapp

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Number is one rentable WhatsApp number from the platform's pool. Under the
// BSP model the platform owns the number (and the messaging account it sits
// on); a shop picks one during onboarding and the connection is provisioned
// with the platform's system-user token rather than the client's.
type Number struct {
	ID                 uuid.UUID
	DisplayPhoneNumber string
	PhoneNumberID      string
	MessagingAccountID string
	WaacID             string
	VerifiedName       string
	Status             string
	ShopID             *uuid.UUID
}

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

// AvailableNumbers returns the rentable numbers that are not yet assigned to
// a shop, for the onboarding wizard.
func (s *Service) AvailableNumbers(ctx context.Context) ([]Number, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, display_phone_number, phone_number_id, messaging_account_id,
		       waac_id, verified_name, status, shop_id
		FROM whatsapp_numbers
		WHERE status = 'available'
		ORDER BY created_at`)

	return s.scanNumbers(rows, err)
}

// Provision assigns a numbered pool number to a shop: it stores the
// shop's integration (using the platform's system-user token) and marks the
// number as assigned. Runs in a transaction so a torn assignment can not
// leave an orphaned number flagged to a shop that has no credentials.
func (s *Service) Provision(ctx context.Context, shopID uuid.UUID, numberID uuid.UUID, platformToken string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var n Number
	err = tx.QueryRow(ctx, `
		SELECT id, display_phone_number, phone_number_id, messaging_account_id,
		       waac_id, verified_name, status, shop_id
		FROM whatsapp_numbers
		WHERE id = $1 AND status = 'available'
		FOR UPDATE`,
		numberID,
	).Scan(&n.ID, &n.DisplayPhoneNumber, &n.PhoneNumberID, &n.MessagingAccountID,
		&n.WaacID, &n.VerifiedName, &n.Status, &n.ShopID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return errors.New("whatsapp: requested number is not available")
		}
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO whatsapp_integrations (
			shop_id, waac_id, phone_number_id, messaging_account_id,
			business_portfolio_id, phone_number, access_token_encrypted, status
		) VALUES ($1, $2, $3, $4, '', $5, $6, 'connected')
		ON CONFLICT (shop_id) DO UPDATE SET
			waac_id                = EXCLUDED.waac_id,
			phone_number_id        = EXCLUDED.phone_number_id,
			messaging_account_id   = EXCLUDED.messaging_account_id,
			phone_number           = EXCLUDED.phone_number,
			access_token_encrypted = EXCLUDED.access_token_encrypted,
			status                 = 'connected',
			updated_at             = now()`,
		shopID, n.WaacID, n.PhoneNumberID, n.MessagingAccountID, n.DisplayPhoneNumber,
		s.encryptOrEmpty(platformToken),
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE whatsapp_numbers
		SET status = 'assigned', shop_id = $2, updated_at = now()
		WHERE id = $1`,
		numberID, shopID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Service) encryptOrEmpty(plain string) string {
	if plain == "" || s.cipher == nil {
		return ""
	}
	enc, err := s.cipher.Encrypt(plain)
	if err != nil {
		s.log.Error("whatsapp: token encryption failed during provision", "error", err)
		return ""
	}
	return enc
}

func (s *Service) scanNumbers(rows pgx.Rows, err error) ([]Number, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var numbers []Number
	for rows.Next() {
		var n Number
		if err := rows.Scan(&n.ID, &n.DisplayPhoneNumber, &n.PhoneNumberID, &n.MessagingAccountID,
			&n.WaacID, &n.VerifiedName, &n.Status, &n.ShopID); err != nil {
			return nil, err
		}
		numbers = append(numbers, n)
	}
	return numbers, rows.Err()
}
