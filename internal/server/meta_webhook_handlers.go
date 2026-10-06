package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/anthdm/superkit/kit"
	"github.com/jackc/pgx/v5"

	"whatsappconverty/internal/whatsapp"
)

const maxMetaWebhookBody = 2 << 20

// handleMetaWebhookVerify answers Meta's GET subscription handshake. The
// configured hub.verify_token must match, otherwise Meta's subscribe call
// fails and nothing is registered.
func (a *App) handleMetaWebhookVerify(k *kit.Kit) error {
	q := k.Request.URL.Query()
	mode := q.Get("hub.mode")
	token := q.Get("hub.verify_token")
	challenge := q.Get("hub.challenge")

	if !whatsapp.VerifyWebhookChallenge(a.Cfg.MetaWebhookVerifyToken, mode, token, challenge) {
		a.Log.Warn("meta webhook verify challenge rejected", "mode", mode)
		return k.Text(http.StatusForbidden, "verification failed")
	}
	return k.Text(http.StatusOK, challenge)
}

// handleMetaWebhook is the POST target Meta delivers webhook events to: status
// updates for outbound messages and inbound customer messages. Only deliveries
// carrying a valid X-Hub-Signature-256 are processed; raw payloads are never
// returned to any merchant. Status updates are idempotent and only ever move a
// message forward; inbound messages are deduped by Meta message id.
func (a *App) handleMetaWebhook(k *kit.Kit) error {
	body, err := io.ReadAll(io.LimitReader(k.Request.Body, maxMetaWebhookBody))
	if err != nil {
		return err
	}

	ctx := k.Request.Context()

	if a.Cfg.MetaAppSecret == "" {
		a.WhatsApp.RecordWebhookReceipt(ctx, whatsapp.WebhookReceipt{
			Kind:   "config_missing",
			Detail: "META_APP_SECRET empty: every Meta delivery is rejected here",
		})
		a.Log.Error("meta webhook: META_APP_SECRET not configured")
		return k.Text(http.StatusInternalServerError, "not configured")
	}
	sig := k.Request.Header.Get("X-Hub-Signature-256")
	if !whatsapp.VerifyWebhookSignature(a.Cfg.MetaAppSecret, sig, body) {
		a.exposeRemoteIP(k)
		a.WhatsApp.RecordWebhookReceipt(ctx, whatsapp.WebhookReceipt{
			Kind:   "auth_failed",
			Detail: "X-Hub-Signature-256 rejected",
		})
		a.Log.Warn("meta webhook signature verification failed")
		return k.Text(http.StatusForbidden, "forbidden")
	}

	delivery, err := whatsapp.ParseWebhook(body)
	if err != nil {
		a.WhatsApp.RecordWebhookReceipt(ctx, whatsapp.WebhookReceipt{
			Kind:   "parse_error",
			Detail: err.Error(),
		})
		a.Log.Warn("meta webhook parse failed", "error", err.Error())
		return k.Text(http.StatusOK, "ok") // acknowledge; nothing actionable here
	}

	for _, u := range delivery.Statuses {
		detail := u.Status
		if len(u.Errors) > 0 {
			detail = fmt.Sprintf("%s (code %d): %s", u.Status, u.Errors[0].Code, u.Errors[0].Title)
		}
		a.WhatsApp.RecordWebhookReceipt(ctx, whatsapp.WebhookReceipt{
			Kind:          "status",
			MetaMessageID: u.MetaMessageID,
			Detail:        detail,
		})
		applyErr := a.WhatsApp.ApplyStatusUpdate(ctx, u)
		if applyErr != nil && !errors.Is(applyErr, pgx.ErrNoRows) {
			a.Log.Warn("meta webhook status apply failed",
				"meta_message_id", u.MetaMessageID, "status", u.Status, "error", applyErr.Error())
			continue
		}
		// ErrNoRows means the id belongs to a free-form chat reply, not the
		// template ledger; the chat update below owns that row.
		if chatErr := a.WhatsApp.ApplyChatStatusUpdate(ctx, u); chatErr != nil {
			a.Log.Warn("meta webhook chat status apply failed",
				"meta_message_id", u.MetaMessageID, "status", u.Status, "error", chatErr.Error())
		}
		a.Log.Info("meta webhook status applied",
			"meta_message_id", u.MetaMessageID, "status", u.Status,
			"errors", len(u.Errors))
	}

	for _, m := range delivery.Inbound {
		if m.MetaMessageID == "" || m.From == "" {
			continue
		}
		shopID, err := a.WhatsApp.ShopByPhoneNumberID(ctx, m.PhoneNumberID)
		if err != nil {
			a.WhatsApp.RecordWebhookReceipt(ctx, whatsapp.WebhookReceipt{
				Kind:          "dropped",
				MetaMessageID: m.MetaMessageID,
				FromPhone:     m.From,
				PhoneNumberID: m.PhoneNumberID,
				Detail:        "phone_number_id not mapped to a shop: " + err.Error(),
			})
			a.Log.Warn("meta webhook inbound: number not mapped to a shop",
				"phone_number_id", m.PhoneNumberID, "error", err.Error())
			continue
		}
		a.WhatsApp.RecordWebhookReceipt(ctx, whatsapp.WebhookReceipt{
			Kind:          "inbound",
			MetaMessageID: m.MetaMessageID,
			FromPhone:     m.From,
			PhoneNumberID: m.PhoneNumberID,
			ShopID:        &shopID,
		})
		if ingestErr := a.WhatsApp.UpsertInbound(ctx, shopID, m); ingestErr != nil {
			a.Log.Warn("meta webhook inbound ingest failed",
				"meta_message_id", m.MetaMessageID, "from", m.From, "error", ingestErr.Error())
			continue
		}
		a.Log.Info("meta webhook inbound ingested",
			"meta_message_id", m.MetaMessageID, "from", m.From, "shop_id", shopID)
	}

	return k.Text(http.StatusOK, "EVENT_RECEIVED")
}

// exposeRemoteIP logs the caller's IP for abuse watching without echoing the
// payload (Raw Meta webhook content is never echoed back to callers).
func (a *App) exposeRemoteIP(k *kit.Kit) {
	a.Log.Warn("meta webhook consumer ip", "remote", k.Request.RemoteAddr)
}
