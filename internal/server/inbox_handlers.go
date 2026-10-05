package server

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"whatsappconverty/internal/whatsapp"
	"whatsappconverty/web/views/components"
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

var inboxThreadPathRe = regexp.MustCompile(`/inbox/([0-9a-fA-F-]{36})`)

// activeInboxID recovers which conversation is open from htmx's current URL
// header, so the polling sidebar can keep that row highlighted.
func activeInboxID(k *kit.Kit) uuid.UUID {
	m := inboxThreadPathRe.FindStringSubmatch(k.Request.Header.Get("HX-Current-URL"))
	if len(m) < 2 {
		return uuid.Nil
	}
	id, err := uuid.Parse(m[1])
	if err != nil {
		return uuid.Nil
	}
	return id
}

// inboxThreads loads the operator's conversations and the shop name map used
// by the sidebar rows, and marks the page's unread badge.
func (a *App) inboxThreads(k *kit.Kit, page *components.Page) ([]whatsapp.Conversation, map[uuid.UUID]string, error) {
	threads, err := a.WhatsApp.Threads(k.Request.Context())
	if err != nil {
		return nil, nil, err
	}
	shopNames := make(map[uuid.UUID]string)
	if list, err := a.Shops.List(k.Request.Context()); err == nil {
		for _, s := range list {
			shopNames[s.ID] = s.Name
		}
	}
	page.UnreadInbox = totalUnread(threads)
	return threads, shopNames, nil
}

func totalUnread(threads []whatsapp.Conversation) int {
	n := 0
	for _, t := range threads {
		n += t.UnreadCount
	}
	return n
}

// handleInbox lists the operator's customer conversations on the left and (if
// one is open) shows it on the right. The Inbox is gated on a connected
// WhatsApp number: without one there is no number to receive messages, so the
// page explains what to do instead of crashing.
func (a *App) handleInbox(k *kit.Kit) error {
	all, err := a.pageShops(k)
	if err != nil {
		return err
	}
	page := a.dashboardPage(k, "Inbox", "inbox", all)
	if !page.WhatsAppConnected {
		return k.Render(vdashboard.InboxPage(page, nil, nil, nil, ""))
	}

	threads, shopNames, err := a.inboxThreads(k, &page)
	if err != nil {
		a.Log.Error("inbox threads failed", "error", err.Error())
		return k.Render(vdashboard.InboxPage(page, nil, nil, nil, "Could not load conversations."))
	}

	return k.Render(vdashboard.InboxPage(page, threads, shopNames, nil, inboxFlash(k.Request.URL.Query().Get("flash"))))
}

// handleInboxSidebar is the polling response for the conversation list: the
// sidebar rows plus the nav unread badge. Empty when WhatsApp is disconnected.
func (a *App) handleInboxSidebar(k *kit.Kit) error {
	all, err := a.pageShops(k)
	if err != nil {
		return err
	}
	page := a.dashboardPage(k, "Inbox", "inbox", all)
	if !page.WhatsAppConnected {
		return k.Text(http.StatusOK, "")
	}

	threads, shopNames, err := a.inboxThreads(k, &page)
	if err != nil {
		return k.Text(http.StatusOK, "")
	}
	return k.Render(vdashboard.InboxSidebar(threads, activeInboxID(k), shopNames))
}

// handleInboxThread shows one conversation with its messages and reply box. On
// an htmx request only the thread card (plus out-of-band sidebar and badge
// updates) is returned, so opening a conversation from the list is a single
// swap; a plain request renders the full two-pane page.
func (a *App) handleInboxThread(k *kit.Kit) error {
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
	}

	all, err2 := a.pageShops(k)
	if err2 != nil {
		return err2
	}
	page := a.dashboardPage(k, "Inbox", "inbox", all)
	if !page.WhatsAppConnected {
		return k.Render(vdashboard.InboxPage(page, nil, nil, nil, ""))
	}

	conv, msgs, err := a.WhatsApp.Thread(k.Request.Context(), id)
	if err != nil {
		if errors.Is(err, whatsapp.ErrConversationNotFound) {
			return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
		}
		a.Log.Error("inbox thread failed", "id", id, "error", err.Error())
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=error")
	}

	if err := a.WhatsApp.MarkThreadRead(k.Request.Context(), id); err != nil {
		a.Log.Warn("inbox mark read failed", "id", id, "error", err.Error())
	}

	threads, shopNames, err := a.inboxThreads(k, &page)
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=error")
	}

	ok := ""
	if k.Request.URL.Query().Get("flash") == "sent" {
		ok = "Reply sent."
	}

	if k.Request.Header.Get("HX-Request") == "true" {
		return k.Render(vdashboard.InboxThreadSwap(page, conv, msgs, "", ok, threads, id, shopNames))
	}

	return k.Render(vdashboard.InboxPage(page, threads, shopNames, &vdashboard.ActiveInboxThread{Conv: conv, Msgs: msgs, Ok: ok}, ""))
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

	body := k.Request.FormValue("body")

	conv, msgs, err := a.WhatsApp.Thread(k.Request.Context(), id)
	if err != nil {
		if errors.Is(err, whatsapp.ErrConversationNotFound) {
			return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
		}
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=error")
	}

	errMsg, okMsg := "", ""
	switch _, err := a.WhatsApp.SendReply(k.Request.Context(), id, body); {
	case err == nil:
		okMsg = "Reply sent."
	case errors.Is(err, whatsapp.ErrReplyWindowClosed):
		errMsg = explanationForReply(err)
		// Re-fetch so a recorded failed bubble (e.g. a meta rejection) shows.
		conv, msgs, _ = a.WhatsApp.Thread(k.Request.Context(), id)
	case errors.Is(err, whatsapp.ErrConversationNotFound):
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
	default:
		errMsg = explanationForReply(err)
		conv, msgs, _ = a.WhatsApp.Thread(k.Request.Context(), id)
	}

	if k.Request.Header.Get("HX-Request") == "" {
		if errMsg == "" {
			return k.Redirect(http.StatusSeeOther, "/inbox/"+id.String()+"?flash=sent")
		}
		// Non-htmx fallback: render the two-pane page with the error inline.
		threads, shopNames, terr := a.inboxThreads(k, &page)
		if terr != nil {
			return k.Redirect(http.StatusSeeOther, "/inbox?flash=error")
		}
		return k.Render(vdashboard.InboxPage(page, threads, shopNames, &vdashboard.ActiveInboxThread{Conv: conv, Msgs: msgs, Err: errMsg, Ok: okMsg}, ""))
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