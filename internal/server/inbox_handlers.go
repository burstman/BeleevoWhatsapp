package server

import (
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"whatsappconverty/internal/whatsapp"
	"whatsappconverty/web/views/components"
	vdashboard "whatsappconverty/web/views/dashboard"
)

// maxSendAudioBytes is the Cloud API ceiling for outbound audio (16 MB). The
// parser is allowed one extra megabyte to fit the multipart overhead.
const maxSendAudioBytes = 16 << 20

// maxSendImageBytes is the Cloud API ceiling for outbound images (5 MB).
const maxSendImageBytes = 5 << 20

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
	return k.Render(vdashboard.BubblesList(id, msgs))
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

// handleInboxReply sends a free-form reply inside the service window. On
// success it responds with just the new bubble as an OOB swap so htmx appends
// it to #thread-bubbles without rebuilding the chat; errors re-render the whole
// card so the recorded failed bubble / banner shows.
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
		// Re-fetch so the outgoing bubble shows up in the swapped card right away
		// instead of waiting for the fragment poll.
		conv, msgs, _ = a.WhatsApp.Thread(k.Request.Context(), id)
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
	if errMsg == "" && len(msgs) > 0 && msgs[len(msgs)-1].Direction == "outbound" {
		return k.Render(vdashboard.OOBNewBubble(conv.ID, msgs[len(msgs)-1]))
	}
	return k.Render(vdashboard.ThreadCard(page, conv, msgs, errMsg, okMsg))
}

// handleInboxReplyAudio sends an operator audio recording or file to the
// customer. The web composer POSTs a multipart form; the new voice-note bubble
// is returned as an OOB swap so the chat does not reload.
func (a *App) handleInboxReplyAudio(k *kit.Kit) error {
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

	_, _, err = a.WhatsApp.Thread(k.Request.Context(), id)
	if err != nil {
		if errors.Is(err, whatsapp.ErrConversationNotFound) {
			return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
		}
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=error")
	}

	k.Request.Body = http.MaxBytesReader(k.Response, k.Request.Body, maxSendAudioBytes+1<<20)
	if err := k.Request.ParseMultipartForm(maxSendAudioBytes + 1<<20); err != nil {
		return k.Text(http.StatusBadRequest, "Audio upload too large (max 16 MB).")
	}

	file, hdr, err := k.Request.FormFile("audio")
	if err != nil {
		return k.Text(http.StatusBadRequest, "Missing audio file.")
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return k.Text(http.StatusBadRequest, "Could not read audio file.")
	}
	if len(data) == 0 || len(data) > maxSendAudioBytes {
		return k.Text(http.StatusBadRequest, "Audio file is empty or larger than 16 MB.")
	}
	mime := canonicalSendAudioMime(hdr.Header.Get("Content-Type"))
	if mime == "" {
		return k.Text(http.StatusBadRequest,
			"WhatsApp does not accept this audio format (WebM is not supported). Use MP3, M4A, AAC, AMR or OGG.")
	}

	durationMS, _ := strconv.Atoi(k.Request.FormValue("duration_ms"))
	if durationMS < 0 || durationMS > 24*60*60*1000 {
		durationMS = 0
	}

	errMsg, okMsg := "", ""
	switch _, serr := a.WhatsApp.SendAudioReply(k.Request.Context(), id, mime, audioFilename(hdr.Filename, mime), data, durationMS); {
	case serr == nil:
		okMsg = "Voice note sent."
	case errors.Is(serr, whatsapp.ErrReplyWindowClosed):
		errMsg = explanationForReply(serr)
	case errors.Is(serr, whatsapp.ErrConversationNotFound):
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
	default:
		errMsg = explanationForReply(serr)
	}

	conv, msgs, _ := a.WhatsApp.Thread(k.Request.Context(), id)
	if errMsg == "" && len(msgs) > 0 && msgs[len(msgs)-1].Direction == "outbound" {
		return k.Render(vdashboard.OOBNewBubble(conv.ID, msgs[len(msgs)-1]))
	}
	return k.Render(vdashboard.ThreadCard(page, conv, msgs, errMsg, okMsg))
}

// handleInboxReplyImage sends an attached photo to the customer. The web
// composer POSTs a multipart form; the new image bubble is returned as an OOB
// swap so the chat does not reload.
func (a *App) handleInboxReplyImage(k *kit.Kit) error {
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

	_, _, err = a.WhatsApp.Thread(k.Request.Context(), id)
	if err != nil {
		if errors.Is(err, whatsapp.ErrConversationNotFound) {
			return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
		}
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=error")
	}

	k.Request.Body = http.MaxBytesReader(k.Response, k.Request.Body, maxSendImageBytes+1<<20)
	if err := k.Request.ParseMultipartForm(maxSendImageBytes + 1<<20); err != nil {
		return k.Text(http.StatusBadRequest, "Image upload too large (max 5 MB).")
	}

	file, hdr, err := k.Request.FormFile("image")
	if err != nil {
		return k.Text(http.StatusBadRequest, "Missing image file.")
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return k.Text(http.StatusBadRequest, "Could not read image file.")
	}
	if len(data) == 0 || len(data) > maxSendImageBytes {
		return k.Text(http.StatusBadRequest, "Image file is empty or larger than 5 MB.")
	}
	mime := canonicalSendImageMime(hdr.Header.Get("Content-Type"))
	if mime == "" {
		return k.Text(http.StatusBadRequest,
			"WhatsApp does not accept this image format. Use JPG, PNG or WEBP.")
	}

	errMsg, okMsg := "", ""
	switch _, serr := a.WhatsApp.SendImageReply(k.Request.Context(), id, mime, imageFilename(mime), data); {
	case serr == nil:
		okMsg = "Image sent."
	case errors.Is(serr, whatsapp.ErrReplyWindowClosed):
		errMsg = explanationForReply(serr)
	case errors.Is(serr, whatsapp.ErrConversationNotFound):
		return k.Redirect(http.StatusSeeOther, "/inbox?flash=notfound")
	default:
		errMsg = explanationForReply(serr)
	}

	conv, msgs, _ := a.WhatsApp.Thread(k.Request.Context(), id)
	if errMsg == "" && len(msgs) > 0 && msgs[len(msgs)-1].Direction == "outbound" {
		return k.Render(vdashboard.OOBNewBubble(conv.ID, msgs[len(msgs)-1]))
	}
	return k.Render(vdashboard.ThreadCard(page, conv, msgs, errMsg, okMsg))
}

// canonicalSendImageMime maps any upload content type onto the Cloud API's
// supported image set (GIF is NOT accepted), or "" when the format cannot be
// sent to WhatsApp.
func canonicalSendImageMime(mime string) string {
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch base {
	case "image/jpg", "image/jpeg", "image/pjpeg":
		return "image/jpeg"
	case "image/png":
		return "image/png"
	case "image/webp":
		return "image/webp"
	default:
		return ""
	}
}

// imageFilename keeps a safe, correctly-extended upload name for Meta.
func imageFilename(mime string) string {
	exts := map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}
	return "photo." + exts[mime]
}

// canonicalSendAudioMime maps any upload content type onto the Cloud API's
// supported audio set, or "" when the format cannot be sent to WhatsApp.
func canonicalSendAudioMime(mime string) string {
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch base {
	case "audio/ogg", "audio/opus":
		return "audio/ogg"
	case "audio/mp4", "audio/m4a", "audio/x-m4a", "audio/m4b":
		return "audio/mp4"
	case "audio/mpeg", "audio/mp3", "audio/mpga":
		return "audio/mpeg"
	case "audio/aac":
		return "audio/aac"
	case "audio/amr", "audio/amr-nb", "audio/x-amr":
		return "audio/amr"
	default:
		return ""
	}
}

// audioFilename keeps a safe, correctly-extended upload name for Meta.
func audioFilename(original, mime string) string {
	exts := map[string]string{"audio/ogg": "ogg", "audio/mp4": "m4a", "audio/mpeg": "mp3", "audio/aac": "aac", "audio/amr": "amr"}
	name := "voice." + exts[mime]
	if original != "" {
		orig := regexp.MustCompile(`[^A-Za-z0-9._-]`).ReplaceAllString(original, "_")
		if len(orig) > 80 {
			orig = orig[:80]
		}
		if orig != "" {
			name = orig
		}
	}
	return name
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

// handleChatMedia streams a stored inbound voice note back to the inbox. The
// message must belong to the conversation in the URL, and its 30-day retention
// may have already cleared the bytes.
func (a *App) handleChatMedia(k *kit.Kit) error {
	convID, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Text(http.StatusNotFound, "")
	}
	msgID, err := uuid.Parse(chi.URLParam(k.Request, "msg"))
	if err != nil {
		return k.Text(http.StatusNotFound, "")
	}
	data, mime, err := a.WhatsApp.MediaForMessage(k.Request.Context(), convID, msgID)
	if err != nil || len(data) == 0 {
		return k.Text(http.StatusNotFound, "")
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	k.Response.Header().Set("Content-Type", mime)
	k.Response.Header().Set("X-Content-Type-Options", "nosniff")
	k.Response.Header().Set("Cache-Control", "private, no-store")
	k.Response.WriteHeader(http.StatusOK)
	_, err = k.Response.Write(data)
	return err
}