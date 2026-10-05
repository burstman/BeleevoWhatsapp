package server

import (
	"errors"
	"net/http"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"whatsappconverty/internal/whatsapp"
	vdashboard "whatsappconverty/web/views/dashboard"
)

// inboxFlash maps the ?flash= query value onto a user-facing line for the
// inbox list page.
func inboxFlash(q string) string {
	switch q {
	case "sent":
		return "Reply sent."
	case "notfound":
		return "Conversation not found."
	case "error":
		return "Something went wrong; try again."
	}
	return ""
}

// handleInbox lists the operator's customer conversations. The Inbox is gated
// on a connected WhatsApp number: without one there is no number to receive
// messages, so the page explains what to do instead of crashing.
func (a *App) handleInbox(k *kit.Kit) error {
	all, err := a.pageShops(k)
	if err != nil {
		return err
	}
	page := a.dashboardPage(k, "Inbox", "inbox", all)
	if !page.WhatsAppConnected {
		return k.Render(vdashboard.InboxPage(page, nil, nil, ""))
	}

	ctx := k.Request.Context()
	threads, err := a.WhatsApp.Threads(ctx)
	if err != nil {
		a.Log.Error("inbox threads failed", "error", err.Error())
		return k.Render(vdashboard.InboxPage(page, nil, nil, "Could not load conversations."))
	}

	shopNames := make(map[uuid.UUID]string)
	if list, err := a.Shops.List(ctx); err == nil {
		for _, s := range list {
			shopNames[s.ID] = s.Name
		}
	}

	return k.Render(vdashboard.InboxPage(page, threads, shopNames, inboxFlash(k.Request.URL.Query().Get("flash"))))
}

// handleInboxThread shows one conversation with its messages and reply box.
func (a *App) handleInboxThread(k *kit.Kit) error {
	all, err := a.pageShops(k)
	if err != nil {
		return err
	}
	page := a.dashboardPage(k, "Inbox", "inbox", all)
	if !page.WhatsAppConnected {
		return k.Render(vdashboard.InboxPage(page, nil, nil, ""))
	}

	ctx := k.Request.Context()
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
	}

	conv, msgs, err := a.WhatsApp.Thread(ctx, id)
	if err != nil {
		if errors.Is(err, whatsapp.ErrConversationNotFound) {
			return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
		}
		a.Log.Error("inbox thread failed", "id", id, "error", err.Error())
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=error")
	}

	if err := a.WhatsApp.MarkThreadRead(ctx, id); err != nil {
		a.Log.Warn("inbox mark read failed", "id", id, "error", err.Error())
	}
	ok := ""
	if k.Request.URL.Query().Get("flash") == "sent" {
		ok = "Reply sent."
	}
	return k.Render(vdashboard.InboxThreadPage(page, conv, msgs, "", ok))
}

// handleInboxFragment is the polling endpoint: the thread's bubbles only, so
// the open reply form keeps its focus while new inbound appears.
func (a *App) handleInboxFragment(k *kit.Kit) error {
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Text(http.StatusOK, "")
	}
	_, msgs, err := a.WhatsApp.Thread(k.Request.Context(), id)
	if err != nil {
		return k.Text(http.StatusOK, "")
	}
	return k.Render(vdashboard.BubblesList(msgs))
}

// explanationForReply turns a failed reply into a line the operator can act
// on, mirroring the template-rejection explanations.
func explanationForReply(err error) string {
	switch {
	case errors.Is(err, whatsapp.ErrReplyWindowClosed):
		return "The 24h customer service window has closed; message this customer with an approved template instead."
	case errors.Is(err, whatsapp.ErrNotConfigured):
		return "WhatsApp is not configured. Connect your number in Settings first."
	default:
		return "The reply could not be sent: " + err.Error()
	}
}

// handleInboxReply sends a free-form reply inside the service window. It
// re-renders the thread card so htmx swaps the new bubble in place.
func (a *App) handleInboxReply(k *kit.Kit) error {
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
	}

	all, err := a.pageShops(k)
	if err != nil {
		return err
	}
	page := a.dashboardPage(k, "Inbox", "inbox", all)
	if !page.WhatsAppConnected {
		return k.Redirect(http.StatusSeeOther, "/inbox")
	}

	ctx := k.Request.Context()
	body := k.Request.FormValue("body")

	conv, msgs, err := a.WhatsApp.Thread(ctx, id)
	if err != nil {
		if errors.Is(err, whatsapp.ErrConversationNotFound) {
			return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
		}
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=error")
	}

	errMsg, okMsg := "", ""
	switch _, err := a.WhatsApp.SendReply(ctx, id, body); {
	case err == nil:
		okMsg = "Reply sent."
	case errors.Is(err, whatsapp.ErrReplyWindowClosed):
		errMsg = explanationForReply(err)
		// Re-fetch so a recorded failed bubble (e.g. a meta rejection) shows.
		conv, msgs, _ = a.WhatsApp.Thread(ctx, id)
	case errors.Is(err, whatsapp.ErrConversationNotFound):
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
	default:
		errMsg = explanationForReply(err)
		conv, msgs, _ = a.WhatsApp.Thread(ctx, id)
	}

	if k.Request.Header.Get("HX-Request") == "" {
		if errMsg == "" {
			return k.Redirect(http.StatusSeeOther, "/inbox/"+id.String()+"?flash=sent")
		}
		// Non-htmx fallback: render the thread page with the error inline.
		return k.Render(vdashboard.InboxThreadPage(page, conv, msgs, errMsg, okMsg))
	}
	return k.Render(vdashboard.ThreadCard(page, conv, msgs, errMsg, okMsg))
}

// handleInboxRead marks an open thread read.
func (a *App) handleInboxRead(k *kit.Kit) error {
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Text(http.StatusOK, "")
	}
	if err := a.WhatsApp.MarkThreadRead(k.Request.Context(), id); err != nil {
		a.Log.Warn("inbox mark read failed", "id", id, "error", err.Error())
	}
	return k.Text(http.StatusOK, "")
}
