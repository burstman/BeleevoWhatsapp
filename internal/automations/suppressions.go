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
	OrderID        string
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
			 trigger_label, tracking_code, order_id, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (automation_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`,
		in.ShopID, in.AutomationID, in.CustomerID, in.TemplateID,
		reasonMissingVariables, encoded, in.StatusLabel, in.TrackingCode, in.OrderID, in.IdempotencyKey,
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
		       COALESCE(s.trigger_label, ''), COALESCE(s.tracking_code, ''), COALESCE(s.order_id, ''),
		       COALESCE(s.idempotency_key, ''), s.created_at, COALESCE(c.name, ''),
		       COALESCE(t.meta_template_name, '')
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
			&s.TriggerLabel, &s.TrackingCode, &s.OrderID, &s.IdempotencyKey,
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

// RetryResult reports what a "send the held-back ones now" attempt did.
type RetryResult struct {
	Attempted int
	Sent      int
	StillHeld int
}

// RetrySuppressions re-runs the sends this automation held back, and is the
// reason the suppression row carries the order, tracking code and label rather
// than only the reason: a retry is self-contained, so it does not depend on
// replaying a webhook that arrived days ago.
//
// Everything that could have changed since is re-read rather than assumed. The
// driver is the whole point — a carrier attaches it after the fact, which is
// the usual reason a send was held back in the first place.
//
// The original idempotency key is reused, so an event that turns out to have
// been sent already is recorded and skipped instead of double-messaging the
// customer. A row that still cannot send stays on the list.
func (p *Processor) RetrySuppressions(ctx context.Context, automationID uuid.UUID) (RetryResult, error) {
	var res RetryResult

	automation, err := p.AutomationByID(ctx, automationID)
	if err != nil {
		return res, err
	}
	if automation == nil {
		return res, nil
	}

	held, err := SuppressionsForAutomation(ctx, p.pool, automationID, 500)
	if err != nil {
		return res, err
	}
	res.Attempted = len(held)

	// A merchant pressing this wants the messages out now, so the automation's
	// own window is deliberately bypassed: the event's moment has long passed.
	instant := fireRule{timezone: "UTC", days: AllDays()}

	for _, s := range held {
		in := SendInput{
			ShopID:         s.ShopID,
			AutomationID:   automationID,
			CustomerID:     s.CustomerID,
			TemplateID:     automation.TemplateID,
			OrderID:        s.OrderID,
			StatusLabel:    s.TriggerLabel,
			TrackingCode:   s.TrackingCode,
			IdempotencyKey: s.IdempotencyKey,
			fire:           instant,
		}
		if c, cErr := p.whatsapp.Customer(ctx, s.ShopID, s.CustomerID); cErr == nil {
			in.CustomerName, in.CustomerPhone = c.Name, c.Phone
		}
		if p.delivery != nil && s.TrackingCode != "" {
			tracked, tErr := p.delivery.TrackedByBarcode(ctx, s.ShopID, s.TrackingCode)
			if tErr == nil && tracked.ID != uuid.Nil {
				in.DriverName, in.DriverPhone = tracked.DriverName, tracked.DriverPhone
				if in.CustomerName == "" {
					in.CustomerName = tracked.CustomerName
				}
				if in.CustomerPhone == "" {
					in.CustomerPhone = tracked.CustomerPhone
				}
				if in.CustomerID == uuid.Nil {
					in.CustomerID = tracked.CustomerID
				}
			}
		}

		if sErr := p.send(ctx, in); sErr != nil {
			p.log.Warn("automation: retry of held-back send failed",
				"automation_id", automationID, "tracking_code", s.TrackingCode, "error", sErr)
			res.StillHeld++
			continue
		}

		// The send may still have been declined, in which case the row is
		// refreshed in place and the next attempt sees the new reason.
		landed, cErr := p.whatsapp.MessageExistsForIdempotencyKey(ctx, s.ShopID, s.IdempotencyKey)
		if cErr != nil {
			p.log.Warn("automation: could not confirm retried send",
				"automation_id", automationID, "error", cErr)
			continue
		}
		if !landed {
			res.StillHeld++
			continue
		}
		res.Sent++
		p.clearSuppression(ctx, automationID, s.IdempotencyKey)
	}

	return res, nil
}

// clearSuppression drops a row once its message exists. Keeping it would leave
// the merchant staring at a problem that is already solved, and the message
// itself is the durable record that the send happened.
func (p *Processor) clearSuppression(ctx context.Context, automationID uuid.UUID, idempotencyKey string) {
	if idempotencyKey == "" {
		return
	}
	if _, err := p.pool.Exec(ctx, `
		DELETE FROM automation_suppressions
		WHERE automation_id = $1 AND idempotency_key = $2`,
		automationID, idempotencyKey,
	); err != nil {
		p.log.Warn("automation: could not clear retried suppression",
			"automation_id", automationID, "error", err)
	}
}
