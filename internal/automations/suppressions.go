package automations

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/database"
	"whatsappconverty/internal/whatsapp"
)

// Suppression is one event an automation matched but deliberately did not send.
// A semantic template places its variables on purpose, so a chip that resolves
// to nothing (a driver Mescolis has not attached yet) must not become a blank
// line in the customer's message. Dropping the send is right; dropping it
// silently is not, because the merchant's history then looks identical to an
// automation that never fired.
type Suppression struct {
	ID             uuid.UUID
	ShopID         uuid.UUID
	AutomationID   uuid.UUID
	CustomerID     uuid.UUID
	CustomerName   string
	TemplateName   string
	Reason         string
	Missing        []whatsapp.TokenKey
	TriggerLabel   string
	TrackingCode   string
	IdempotencyKey string
	CreatedAt      time.Time
}

// reasonMissingVariables explains a suppression in the merchant's terms rather
// than leaking token keys.
const reasonMissingVariables = "a variable the template places has no value for this order yet"

// RecordSuppression stores one dropped event. It is best effort from the
// caller's point of view: the send has already been declined on purpose, so a
// failed audit write must not turn a deliberate skip into a pipeline error that
// gets retried.
func (p *Processor) RecordSuppression(ctx context.Context, in SendInput, missing []whatsapp.TokenKey) {
	if len(missing) == 0 {
		return
	}
	encoded, err := json.Marshal(missing)
	if err != nil {
		encoded = []byte("[]")
	}
	if _, err := p.pool.Exec(ctx, `
		INSERT INTO automation_suppressions
			(shop_id, automation_id, customer_id, template_id, reason, missing_variables,
			 trigger_label, tracking_code, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (automation_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`,
		in.ShopID, in.AutomationID, in.CustomerID, in.TemplateID,
		reasonMissingVariables, encoded, in.StatusLabel, in.TrackingCode, in.IdempotencyKey,
	); err != nil {
		p.log.Warn("automation: could not record suppressed send",
			"shop_id", in.ShopID, "automation_id", in.AutomationID, "error", err)
	}
}

// SuppressionsForAutomation lists the events an automation matched but did not
// send, newest first. The names are joined in rather than fetched per row, for
// the same reason the message history does it: this list runs to hundreds of
// rows on a busy shop.
func SuppressionsForAutomation(ctx context.Context, db database.Querier, automationID uuid.UUID, limit int) ([]Suppression, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := db.Query(ctx, `
		SELECT s.id, s.shop_id, s.automation_id, s.customer_id, s.reason, s.missing_variables,
		       COALESCE(s.trigger_label, ''), COALESCE(s.tracking_code, ''), COALESCE(s.idempotency_key, ''),
		       s.created_at, COALESCE(c.name, ''), COALESCE(t.meta_template_name, '')
		FROM automation_suppressions s
		LEFT JOIN customers c ON c.id = s.customer_id
		LEFT JOIN templates t ON t.id = s.template_id
		WHERE s.automation_id = $1
		ORDER BY s.created_at DESC
		LIMIT $2`,
		automationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Suppression{}
	for rows.Next() {
		var s Suppression
		var missing []byte
		if err := rows.Scan(
			&s.ID, &s.ShopID, &s.AutomationID, &s.CustomerID, &s.Reason, &missing,
			&s.TriggerLabel, &s.TrackingCode, &s.IdempotencyKey,
			&s.CreatedAt, &s.CustomerName, &s.TemplateName,
		); err != nil {
			return nil, err
		}
		if len(missing) > 0 {
			_ = json.Unmarshal(missing, &s.Missing)
		}
		if s.Missing == nil {
			s.Missing = []whatsapp.TokenKey{}
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
