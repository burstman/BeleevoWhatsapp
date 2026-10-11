package server

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/anthdm/superkit/kit"

	"whatsappconverty/internal/i18n"
	"whatsappconverty/internal/whatsapp"
	vsettings "whatsappconverty/web/views/settings"
)

// handleAutoreplySettings renders the inbox auto-reply modal body. It is a
// fragment-only endpoint: the inbox button loads it via htmx. A plain GET
// (someone pasting the URL) is bounced back to the inbox.
func (a *App) handleAutoreplySettings(k *kit.Kit) error {
	if k.Request.Header.Get("HX-Request") != "true" {
		return k.Redirect(http.StatusSeeOther, "/inbox")
	}
	ctx := k.Request.Context()
	owner, err := a.sharedOwnerShop(ctx)
	if err != nil {
		return err
	}
	cfg, err := a.WhatsApp.GetFirstContactAutoreply(ctx, owner.ID)
	if err != nil {
		return err
	}
	dict := i18n.New(a.publicLang(k))
	return k.Render(vsettings.AutoreplyModal(dict, cfg, vsettings.AutoreplyFlash{}))
}

// handleAutoreplySave stores the greeting. It is a multipart form because the
// image/audio kinds carry a file; the text kind is validated to be non-empty
// when the greeting is enabled. On success an HX-Request gets an HX-Trigger
// event the inbox listens for to close the modal; on failure the modal body is
// re-rendered in place with the error.
func (a *App) handleAutoreplySave(k *kit.Kit) error {
	isHX := k.Request.Header.Get("HX-Request") == "true"
	ctx := k.Request.Context()
	dict := i18n.New(a.publicLang(k))
	owner, err := a.sharedOwnerShop(ctx)
	if err != nil {
		return err
	}
	shopID := owner.ID

	kind := strings.TrimSpace(k.Request.FormValue("kind"))
	switch kind {
	case whatsapp.AutoreplyImage, whatsapp.AutoreplyAudio:
	default:
		kind = whatsapp.AutoreplyText
	}
	enabled := k.Request.FormValue("enabled") == "1" || k.Request.FormValue("enabled") == "on"

	old, err := a.WhatsApp.GetFirstContactAutoreply(ctx, shopID)
	if err != nil {
		a.Log.Error("autoreply load failed", "shop_id", shopID, "error", err.Error())
		return a.autoreplyFail(k, dict, whatsapp.FirstContactAutoreply{ShopID: shopID, Enabled: enabled, Kind: kind}, "error", isHX)
	}

	cfg := whatsapp.FirstContactAutoreply{ShopID: shopID, Enabled: enabled, Kind: kind}

	// Parse the multipart body once, bounded by the largest upload we accept
	// (audio) plus one MB of overhead; the per-kind checks below enforce the
	// tighter cap. FormValue and FormFile then read the parsed form.
	k.Request.Body = http.MaxBytesReader(k.Response, k.Request.Body, maxSendAudioBytes+1<<20)
	if err := k.Request.ParseMultipartForm(maxSendAudioBytes + 1<<20); err != nil {
		if strings.Contains(err.Error(), "too large") {
			return a.autoreplyFail(k, dict, cfg, "toolarge", isHX)
		}
		a.Log.Error("autoreply form parse failed", "shop_id", shopID, "error", err.Error())
		return a.autoreplyFail(k, dict, cfg, "error", isHX)
	}

	cfg.TextBody = strings.TrimSpace(k.Request.FormValue("text_body"))
	cfg.Caption = strings.TrimSpace(k.Request.FormValue("caption"))

	switch kind {
	case whatsapp.AutoreplyImage:
		mime, data, filename, hasFile, reason := readAutoreplyUpload(k, "image")
		if reason != "" {
			return a.autoreplyFail(k, dict, cfg, reason, isHX)
		}
		if hasFile {
			cfg.MediaMime, cfg.MediaBytes, cfg.MediaFilename = mime, data, filename
		} else if !(old.Kind == whatsapp.AutoreplyImage && old.MediaMime != "") {
			return a.autoreplyFail(k, dict, cfg, "missingfile", isHX)
		}
	case whatsapp.AutoreplyAudio:
		mime, data, filename, hasFile, reason := readAutoreplyUpload(k, "audio")
		if reason != "" {
			return a.autoreplyFail(k, dict, cfg, reason, isHX)
		}
		if hasFile {
			cfg.MediaMime, cfg.MediaBytes, cfg.MediaFilename = mime, data, filename
			if ms, err := strconv.Atoi(k.Request.FormValue("duration_ms")); err == nil && ms > 0 && ms <= 24*60*60*1000 {
				cfg.MediaDurationMS = ms
			}
		} else if !(old.Kind == whatsapp.AutoreplyAudio && old.MediaMime != "") {
			return a.autoreplyFail(k, dict, cfg, "missingfile", isHX)
		}
	default:
		if enabled && cfg.TextBody == "" {
			return a.autoreplyFail(k, dict, cfg, "missingtext", isHX)
		}
	}

	if err := a.WhatsApp.SaveFirstContactAutoreply(ctx, cfg); err != nil {
		a.Log.Error("autoreply save failed", "shop_id", shopID, "error", err.Error())
		return a.autoreplyFail(k, dict, cfg, "error", isHX)
	}
	a.Log.Info("first-contact autoreply saved", "shop_id", shopID, "kind", kind, "enabled", enabled)

	if isHX {
		k.Response.Header().Set("HX-Trigger", "autoreply-saved")
		return k.Text(http.StatusOK, "")
	}
	return k.Redirect(http.StatusSeeOther, "/inbox")
}

// autoreplyFail reports a save/load problem. For the inbox modal it re-renders
// the modal body in place with a localized message; for a plain post it simply
// bounces back to the inbox.
func (a *App) autoreplyFail(k *kit.Kit, dict *i18n.Dict, cfg whatsapp.FirstContactAutoreply, code string, isHX bool) error {
	if !isHX {
		return k.Redirect(http.StatusSeeOther, "/inbox")
	}
	return k.Render(vsettings.AutoreplyModal(dict, cfg, autoreplyFlash(dict, code)))
}

// autoreplyFlash maps a save/load problem code to the localized banner shown in
// the modal.
func autoreplyFlash(dict *i18n.Dict, code string) vsettings.AutoreplyFlash {
	switch code {
	case "saved":
		return vsettings.AutoreplyFlash{Info: dict.T("far.flashSaved")}
	case "missingtext":
		return vsettings.AutoreplyFlash{Error: dict.T("far.flashMissingText")}
	case "missingfile":
		return vsettings.AutoreplyFlash{Error: dict.T("far.flashMissingFile")}
	case "toolarge":
		return vsettings.AutoreplyFlash{Error: dict.T("far.flashTooLarge")}
	case "badformat":
		return vsettings.AutoreplyFlash{Error: dict.T("far.flashBadFormat")}
	default:
		return vsettings.AutoreplyFlash{Error: dict.T("far.flashError")}
	}
}

// handleAutoreplyMedia streams the stored greeting media for the preview. It is
// what an <img>/<audio> tag on the settings page points at.
func (a *App) handleAutoreplyMedia(k *kit.Kit) error {
	ctx := k.Request.Context()
	owner, err := a.sharedOwnerShop(ctx)
	if err != nil {
		return k.Text(http.StatusNotFound, "")
	}
	data, mime, err := a.WhatsApp.FirstContactAutoreplyMedia(ctx, owner.ID)
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

// readAutoreplyUpload reads the optional media part for a greeting. The form
// must already be parsed. hasFile is false when no file was chosen (the stored
// one is kept). reason is empty on success, else a flash code: "toolarge",
// "badformat", "empty" or "error".
func readAutoreplyUpload(k *kit.Kit, field string) (mime string, data []byte, filename string, hasFile bool, reason string) {
	limit := maxSendImageBytes
	if field == "audio" {
		limit = maxSendAudioBytes
	}
	file, hdr, err := k.Request.FormFile(field)
	if err == http.ErrMissingFile {
		return "", nil, "", false, ""
	}
	if err != nil {
		return "", nil, "", false, "error"
	}
	defer file.Close()

	body, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return "", nil, "", false, "error"
	}
	if len(body) == 0 {
		return "", nil, "", false, "empty"
	}
	if len(body) > limit {
		return "", nil, "", false, "toolarge"
	}
	if field == "audio" {
		ct := canonicalSendAudioMime(hdr.Header.Get("Content-Type"))
		if ct == "" {
			return "", nil, "", false, "badformat"
		}
		return ct, body, audioFilename("", ct), true, ""
	}
	ct := canonicalSendImageMime(hdr.Header.Get("Content-Type"))
	if ct == "" {
		return "", nil, "", false, "badformat"
	}
	return ct, body, imageFilename(ct), true, ""
}
