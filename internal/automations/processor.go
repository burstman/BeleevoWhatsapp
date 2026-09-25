package automations

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/delivery"
	"whatsappconverty/internal/whatsapp"
)

// Processor evaluates order and delivery events against the shop's automations
// and turns a match into a WhatsApp send (queued, or inline when Redis is
// absent). It is the single entry point used by the webhook handler and the
// delivery reconcile ticker.
type Processor struct {
	cfg      config.Config
	pool     *pgxpool.Pool
	log      *slog.Logger
	whatsapp *whatsapp.Service
	delivery *delivery.Service
}

func NewProcessor(cfg config.Config, pool *pgxpool.Pool, log *slog.Logger, wa *whatsapp.Service, del *delivery.Service) *Processor {
	return &Processor{
		cfg:      cfg,
		pool:     pool,
		log:      log,
		whatsapp: wa,
		delivery: del,
	}
}

// ConvertyEvent is the normalized subset of an order webhook that automation
// needs. Barcode is a tracking reference embedded in the payload, when Converty
// already created the delivery parcel.
type ConvertyEvent struct {
	ShopID        uuid.UUID
	OrderStatus   string
	OrderID       string
	CustomerName  string
	CustomerPhone string
	Barcode       string
}

// OnConvertyEvent feeds one captured Converty order event through the
// automation pipeline: any tracking barcode is registered (so future delivery
// events can trigger), and the order-status automation fires if configured.
func (p *Processor) OnConvertyEvent(ctx context.Context, e ConvertyEvent) error {
	if e.ShopID == uuid.Nil {
		return nil
	}

	// Learn the customer and, when the payload carries a tracking barcode, keep
	// an eye on that parcel for delivery-event automations.
	cid := uuid.Nil
	if e.CustomerPhone != "" {
		c, err := p.whatsapp.UpsertCustomer(ctx, e.ShopID, e.CustomerName, e.CustomerPhone)
		if err != nil {
			p.log.Warn("automation: customer upsert failed", "shop_id", e.ShopID, "error", err)
		} else {
			cid = c
		}
	}
	if p.delivery != nil && e.Barcode != "" {
		if err := p.delivery.UpsertTracked(ctx, e.ShopID, e.Barcode, e.OrderID, cid, e.CustomerName, e.CustomerPhone); err != nil {
			p.log.Warn("automation: tracking barcode upsert failed",
				"shop_id", e.ShopID, "barcode", e.Barcode, "error", err)
		} else {
			p.log.Info("automation: tracking barcode registered",
				"shop_id", e.ShopID, "barcode", e.Barcode, "order_id", e.OrderID)
		}
	}

	automation, err := p.match(ctx, e.ShopID, SourceConverty, e.OrderStatus)
	if err != nil {
		return err
	}
	if automation.ID == uuid.Nil {
		return nil
	}

	if cid == uuid.Nil {
		p.log.Debug("automation: event carries no customer phone, nothing to send",
			"shop_id", e.ShopID, "status", e.OrderStatus)
		return nil
	}

	return p.send(ctx, SendInput{
		ShopID:          e.ShopID,
		CustomerID:      cid,
		TemplateID:      automation.TemplateID,
		CustomerName:    e.CustomerName,
		CustomerPhone:   e.CustomerPhone,
		OrderID:         e.OrderID,
		StatusLabel:     e.OrderStatus,
		IdempotencyKey:  "conv:" + e.ShopID.String() + ":" + e.OrderStatus + ":" + e.OrderID,
	})
}

// OnDeliveryChange feeds one observed delivery status transition through the
// automation pipeline. Called by the reconcile ticker.
func (p *Processor) OnDeliveryChange(ctx context.Context, change delivery.StatusChange) error {
	automation, err := p.match(ctx, change.ShopID, SourceDelivery, change.Status)
	if err != nil {
		return err
	}
	if automation.ID == uuid.Nil {
		return nil
	}
	if p.delivery == nil {
		return nil
	}

	tracked, err := p.delivery.TrackedByBarcode(ctx, change.ShopID, change.Barcode)
	if err != nil || tracked.ID == uuid.Nil {
		p.log.Warn("automation: delivery change for untracked parcel",
			"shop_id", change.ShopID, "barcode", change.Barcode, "error", err)
		return nil
	}

	cid := tracked.CustomerID
	if cid == uuid.Nil && tracked.CustomerPhone != "" {
		if c, err := p.whatsapp.UpsertCustomer(ctx, change.ShopID, tracked.CustomerName, tracked.CustomerPhone); err == nil {
			cid = c
			_ = p.delivery.UpsertTracked(ctx, change.ShopID, change.Barcode,
				tracked.OrderID, cid, tracked.CustomerName, tracked.CustomerPhone)
		} else {
			p.log.Warn("automation: delivery customer upsert failed", "barcode", change.Barcode, "error", err)
		}
	}

	orderID := tracked.OrderID
	if change.OrderID != "" {
		orderID = change.OrderID
	}

	return p.send(ctx, SendInput{
		ShopID:         change.ShopID,
		CustomerID:     cid,
		TemplateID:     automation.TemplateID,
		CustomerName:   tracked.CustomerName,
		CustomerPhone:  tracked.CustomerPhone,
		OrderID:        orderID,
		StatusLabel:    change.Label,
		IdempotencyKey: "msc:" + change.ShopID.String() + ":" + change.Status + ":" + change.Barcode,
	})
}

// ReconcileDelivery runs one sweep of the delivery poller. It is safe to call
// repeatedly and is invoked on a ticker by the worker process.
func (p *Processor) ReconcileDelivery(ctx context.Context) error {
	if p.delivery == nil {
		return nil
	}
	return p.delivery.Reconcile(ctx, func(ctx context.Context, ch delivery.StatusChange) error {
		return p.OnDeliveryChange(ctx, ch)
	})
}

// SendInput is the resolved, ready-to-send automation input.
type SendInput struct {
	ShopID          uuid.UUID
	CustomerID      uuid.UUID
	TemplateID      uuid.UUID
	CustomerName    string
	CustomerPhone   string
	OrderID         string
	StatusLabel     string
	IdempotencyKey  string
}

// send executes one automation: it resolves the template (driving the variable
// set and the declared purpose), ensures opt-in consent, and enqueues the send
// — falling back to an inline send when no queue is available.
func (p *Processor) send(ctx context.Context, in SendInput) error {
	if in.CustomerID == uuid.Nil {
		p.log.Debug("automation send skipped: no customer", "shop_id", in.ShopID)
		return nil
	}

	t, err := p.whatsapp.Template(ctx, in.ShopID, in.TemplateID)
	if err != nil {
		p.log.Warn("automation: template lookup failed", "template_id", in.TemplateID, "error", err)
		return err
	}
	if t.ID == uuid.Nil {
		p.log.Warn("automation: template no longer exists", "template_id", in.TemplateID)
		return nil
	}

	purpose := t.Category
	if err := p.whatsapp.GrantConsent(ctx, in.ShopID, in.CustomerID, purpose, "automation", in.OrderID); err != nil {
		p.log.Warn("automation: consent grant failed", "shop_id", in.ShopID, "error", err)
	}

	job := whatsapp.SendWhatsAppTemplateJob{
		ShopID:          in.ShopID,
		CustomerID:      in.CustomerID,
		TemplateID:      in.TemplateID,
		ConvertyOrderID: in.OrderID,
		Purpose:         purpose,
		Variables:       buildVariables(t, in.CustomerName, in.OrderID, in.StatusLabel),
		IdempotencyKey:  in.IdempotencyKey,
	}

	queued, err := p.whatsapp.EnqueueSend(ctx, job)
	if err != nil {
		p.log.Warn("automation: enqueue failed, sending inline", "error", err)
		queued = false
	}
	if queued {
		p.log.Info("automation send queued",
			"shop_id", in.ShopID, "customer_id", in.CustomerID,
			"template_id", in.TemplateID, "trigger", in.StatusLabel)
		return nil
	}

	if _, err := p.whatsapp.SendTemplateMessage(ctx, toSendRequest(job)); err != nil {
		var rej *whatsapp.SendRejection
		if errors.As(err, &rej) {
			p.log.Debug("automation inline send rejected",
				"shop_id", in.ShopID, "code", rej.Code, "reason", rej.Reason)
			return nil
		}
		p.log.Warn("automation inline send failed", "shop_id", in.ShopID, "error", err)
		return err
	}
	return nil
}

// buildVariables fills every template placeholder positionally. The lexical
// order is documented in the Automations UI: {{1}}=customer, {{2}}=order,
// {{3}}=status. Unused positions get a neutral em dash so the send gate's
// variable-count check still passes.
func buildVariables(t whatsapp.MerchantTemplate, customerName, orderID, statusLabel string) map[string]string {
	vocab := []string{customerName, orderID, statusLabel}
	vars := make(map[string]string, t.NumVariables)
	for i := 0; i < t.NumVariables; i++ {
		v := "—"
		if i < len(vocab) && vocab[i] != "" {
			v = vocab[i]
		}
		vars[strconv.Itoa(i+1)] = v
	}
	return vars
}

func toSendRequest(job whatsapp.SendWhatsAppTemplateJob) whatsapp.SendRequest {
	return whatsapp.SendRequest{
		ShopID:          job.ShopID,
		CustomerID:      job.CustomerID,
		TemplateID:      job.TemplateID,
		ConvertyOrderID: job.ConvertyOrderID,
		Purpose:         job.Purpose,
		Variables:       job.Variables,
		IdempotencyKey:  job.IdempotencyKey,
	}
}