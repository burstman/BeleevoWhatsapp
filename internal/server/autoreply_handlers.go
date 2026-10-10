package server

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/anthdm/superkit/kit"

	"whatsappconverty/internal/whatsapp"
	vsettings "whatsappconverty/web/views/settings"
)

// handleAutoreplySettings renders the first-contact auto-reply page: the
// greeting configured on the shop that owns the WhatsApp number.
func (a *App) handleAutoreplySettings(k *kit.Kit) error {
	all, err := a.pageShops(k)
	if err != nil {
		return err
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

	page := a.dashboardPage(k, "autoreply", "autoreply", all)
	dict := page.I18N

	flash := vsettings.AutoreplyFlash{}
	switch k.Request.URL.Query().Get("flash") {
	case "saved":
		flash.Info = dict.T("far.flashSaved")
	case "missingtext":
		flash.Error = dict.T("far.flashMissingText")
	case "missingfile":
		flash.Error = dict.T("far.flashMissingFile")
	case "toolarge":
		flash.Error = dict.T("far.flashTooLarge")
	case "badformat":
		flash.Error = dict.T("far.flashBadFormat")
	case "error", "internal":
		flash.Error = dict.T("far.flashError")
	}

	return k.Render(vsettings.Autoreply(page, dict, cfg, flash))
}

// handleAutoreplySave stores the greeting. It is a multipart form because the
// image/audio kinds carry a file; the text kind is validated to be non-empty
// when the greeting is enabled.
func (a *App) handleAutoreplySave(k *kit.Kit) error {
	if _, err := a.pageShops(k); err != nil {
		return err
	}
	ctx := k.Request.Context()
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
		return k.Redirect(http.StatusSeeOther, "/settings/autoreply?flash=error")
	}

	cfg := whatsapp.FirstContactAutoreply{ShopID: shopID, Enabled: enabled, Kind: kind}

	// Parse the multipart body once, bounded by the largest upload we accept
	// (audio) plus one MB of overhead; the per-kind checks below enforce the
	// tighter cap. FormValue and FormFile then read the parsed form.
	k.Request.Body = http.MaxBytesReader(k.Response, k.Request.Body, maxSendAudioBytes+1<<20)
	if err := k.Request.ParseMultipartForm(maxSendAudioBytes + 1<<20); err != nil {
		if strings.Contains(err.Error(), "too large") {
			return k.Redirect(http.StatusSeeOther, "/settings/autoreply?flash=toolarge")
		}
		a.Log.Error("autoreply form parse failed", "shop_id", shopID, "error", err.Error())
		return k.Redirect(http.StatusSeeOther, "/settings/autoreply?flash=error")
	}

	switch kind {
	case whatsapp.AutoreplyImage:
		cfg.Caption = strings.TrimSpace(k.Request.FormValue("caption"))
		mime, data, filename, hasFile, reason := readAutoreplyUpload(k, "image")
		if reason != "" {
			return k.Redirect(http.StatusSeeOther, "/settings/autoreply?flash="+reason)
		}
		if hasFile {
			cfg.MediaMime, cfg.MediaBytes, cfg.MediaFilename = mime, data, filename
		} else if !(old.Kind == whatsapp.AutoreplyImage && old.MediaMime != "") {
			return k.Redirect(http.StatusSeeOther, "/settings/autoreply?flash=missingfile")
		}
	case whatsapp.AutoreplyAudio:
		mime, data, filename, hasFile, reason := readAutoreplyUpload(k, "audio")
		if reason != "" {
			return k.Redirect(http.StatusSeeOther, "/settings/autoreply?flash="+reason)
		}
		if hasFile {
			cfg.MediaMime, cfg.MediaBytes, cfg.MediaFilename = mime, data, filename
			if ms, err := strconv.Atoi(k.Request.FormValue("duration_ms")); err == nil && ms > 0 && ms <= 24*60*60*1000 {
				cfg.MediaDurationMS = ms
			}
		} else if !(old.Kind == whatsapp.AutoreplyAudio && old.MediaMime != "") {
			return k.Redirect(http.StatusSeeOther, "/settings/autoreply?flash=missingfile")
		}
	default:
		cfg.TextBody = strings.TrimSpace(k.Request.FormValue("text_body"))
		if enabled && cfg.TextBody == "" {
			return k.Redirect(http.StatusSeeOther, "/settings/autoreply?flash=missingtext")
		}
	}

	if err := a.WhatsApp.SaveFirstContactAutoreply(ctx, cfg); err != nil {
		a.Log.Error("autoreply save failed", "shop_id", shopID, "error", err.Error())
		return k.Redirect(http.StatusSeeOther, "/settings/autoreply?flash=error")
	}
	a.Log.Info("first-contact autoreply saved", "shop_id", shopID, "kind", kind, "enabled", enabled)
	return k.Redirect(http.StatusSeeOther, "/settings/autoreply?flash=saved")
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
