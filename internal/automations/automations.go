package automations

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"whatsappconverty/internal/delivery"
)

// Event source identifiers stored on every automation row.
const (
	SourceConverty = "converty"
	SourceDelivery = "delivery"
)

// ErrDuplicate is returned when a (shop, source, status) automation already
// exists (the unique constraint mirrors the UI rule "one template per trigger").
var ErrDuplicate = errors.New("automation already exists for this trigger")

// Automation maps a trigger (an order or delivery event) to a message template.
// EventSource disambiguates which origin the trigger key belongs to, so the same
// status string can be automated independently for Converty and for delivery.
type Automation struct {
	ID          uuid.UUID
	ShopID      uuid.UUID
	EventSource string
	OrderStatus string
	TemplateID  uuid.UUID
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ListByShop returns the automations configured for a shop.
func (p *Processor) ListByShop(ctx context.Context, shopID uuid.UUID) ([]Automation, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, shop_id, event_source, order_status, template_id, enabled, created_at, updated_at
		FROM automations
		WHERE shop_id = $1
		ORDER BY created_at`,
		shopID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Automation
	for rows.Next() {
		var a Automation
		if err := rows.Scan(&a.ID, &a.ShopID, &a.EventSource, &a.OrderStatus, &a.TemplateID,
			&a.Enabled, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Create records a new automation. The source must be one of the known event
// sources; the pair (shop, source, status) is unique.
func (p *Processor) Create(ctx context.Context, shopID uuid.UUID, source, status string, templateID uuid.UUID) error {
	if source != SourceConverty && source != SourceDelivery {
		return errors.New("unknown automation event source")
	}
	if status == "" {
		return errors.New("automation trigger is required")
	}
	if templateID == uuid.Nil {
		return errors.New("automation template is required")
	}
	tag, err := p.pool.Exec(ctx, `
		INSERT INTO automations (shop_id, event_source, order_status, template_id, enabled)
		VALUES ($1, $2, $3, $4, true)
		ON CONFLICT (shop_id, event_source, order_status) DO NOTHING`,
		shopID, source, status, templateID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDuplicate
	}
	return nil
}

// SetEnabled toggles an automation on or off.
func (p *Processor) SetEnabled(ctx context.Context, shopID, id uuid.UUID, enabled bool) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE automations
		SET enabled = $3, updated_at = now()
		WHERE id = $1 AND shop_id = $2`,
		id, shopID, enabled,
	)
	return err
}

// Delete removes an automation.
func (p *Processor) Delete(ctx context.Context, shopID, id uuid.UUID) error {
	_, err := p.pool.Exec(ctx, `
		DELETE FROM automations
		WHERE id = $1 AND shop_id = $2`,
		id, shopID,
	)
	return err
}

// match returns the enabled automation for a trigger, or an empty automation
// when none (or one whose template was deleted) is configured.
func (p *Processor) match(ctx context.Context, shopID uuid.UUID, source, status string) (Automation, error) {
	var a Automation
	err := p.pool.QueryRow(ctx, `
		SELECT a.id, a.shop_id, a.event_source, a.order_status, a.template_id, a.enabled, a.created_at, a.updated_at
		FROM automations a
		JOIN templates t ON t.id = a.template_id
		WHERE a.shop_id = $1 AND a.event_source = $2 AND a.order_status = $3
		  AND a.enabled AND t.approval_status = 'approved' AND t.marketing_flagged = false
		LIMIT 1`,
		shopID, source, status,
	).Scan(&a.ID, &a.ShopID, &a.EventSource, &a.OrderStatus, &a.TemplateID,
		&a.Enabled, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Automation{}, nil
	}
	return a, err
}

// DeliveryStatuses returns the delivery trigger statuses to present in the
// automation creation form.
func DeliveryStatuses() []string {
	return delivery.KnownStatuses()
}

// SourceLabel is a human label for the event-source select.
func SourceLabel(source string) string {
	switch source {
	case SourceConverty:
		return "Order event (Converty)"
	case SourceDelivery:
		return "Delivery event (provider)"
	}
	return source
}