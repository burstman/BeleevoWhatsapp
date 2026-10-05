package whatsapp

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// WebhookReceipt is one row of the delivery trail: a Meta webhook POST the
// platform saw and what happened to it. The trail lives on its own table so the
// settings page can answer "did Meta reach us?" without touching message data,
// and it is the first place to look when a send or an inbox message "does
// nothing".
type WebhookReceipt struct {
	ReceivedAt    time.Time
	Kind          string // config_missing|auth_failed|parse_error|status|inbound|dropped
	MetaMessageID string
	FromPhone     string
	PhoneNumberID string
	ShopID        *uuid.UUID
	Detail        string
}

// RecordWebhookReceipt appends one delivery to the trail. It never blocks or
// fails a webhook response: if the trail write fails, the delivery still gets
// its normal reply.
func (s *Service) RecordWebhookReceipt(ctx context.Context, r WebhookReceipt) {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhook_receipts
			(received_at, kind, meta_message_id, from_phone, phone_number_id, shop_id, detail)
		VALUES (now(), $1, $2, $3, $4, $5, $6)`,
		r.Kind, r.MetaMessageID, r.FromPhone, r.PhoneNumberID, r.ShopID, r.Detail)
	if err != nil {
		s.log.Warn("webhook receipt record failed", "kind", r.Kind, "error", err.Error())
		return
	}
	// The trail is diagnostic only; age it out so it cannot grow forever.
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM webhook_receipts WHERE received_at < now() - interval '30 days'`); err != nil {
		s.log.Warn("webhook receipt purge failed", "error", err.Error())
	}
}

// RecentWebhookReceipts returns the most recent deliveries, newest first, for
// the settings page trail.
func (s *Service) RecentWebhookReceipts(ctx context.Context, limit int) ([]WebhookReceipt, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT received_at, kind, meta_message_id, from_phone, phone_number_id, shop_id, detail
		FROM webhook_receipts
		ORDER BY received_at DESC, id DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]WebhookReceipt, 0, limit)
	for rows.Next() {
		var r WebhookReceipt
		if err := rows.Scan(
			&r.ReceivedAt, &r.Kind, &r.MetaMessageID,
			&r.FromPhone, &r.PhoneNumberID, &r.ShopID, &r.Detail,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}