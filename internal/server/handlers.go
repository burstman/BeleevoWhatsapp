package server

import (
	"net/http"

	"github.com/anthdm/superkit/kit"

	"whatsappconverty/internal/auth"
	"whatsappconverty/internal/database"
	"whatsappconverty/internal/i18n"
	"whatsappconverty/internal/whatsapp"
	vdashboard "whatsappconverty/web/views/dashboard"
	vlanding "whatsappconverty/web/views/landing"
	vlegal "whatsappconverty/web/views/legal"
)

func (a *App) handleLiveness(k *kit.Kit) error {
	return k.Text(http.StatusOK, "ok")
}

func (a *App) handleReadiness(k *kit.Kit) error {
	if err := database.Ping(k.Request.Context(), a.Pool); err != nil {
		return err
	}
	return k.Text(http.StatusOK, "ready")
}

func (a *App) handleIndex(k *kit.Kit) error {
	if authenticated := auth.FromKit(k); authenticated.LoggedIn {
		return k.Redirect(http.StatusSeeOther, "/dashboard")
	}
	return k.Render(vlanding.Index(i18n.New(a.publicLang(k))))
}

// handlePrivacyPage renders the public Privacy Policy, required by Meta for
// live apps. It intentionally skips auth so the URL can be published to the
// WhatsApp review and marketing materials.
func (a *App) handlePrivacyPage(k *kit.Kit) error {
	return k.Render(vlegal.Privacy(a.Cfg.SupportEmail, i18n.New(a.publicLang(k))))
}

// handleDataDeletionPage renders the public data-deletion instructions, the
// URL Meta's app review asks for alongside the privacy policy. Like the
// policy it is unauthenticated so reviewers and customers can open it
// directly.
func (a *App) handleDataDeletionPage(k *kit.Kit) error {
	return k.Render(vlegal.DataDeletion(a.Cfg.SupportEmail, i18n.New(a.publicLang(k))))
}

func (a *App) handleOverview(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil || len(all) == 0 {
		return err
	}

	stats, err := a.Dashboard.StatsAll(k.Request.Context())
	if err != nil {
		return err
	}

	page := a.dashboardPage(k, "overview", "overview", all)
	return k.Render(vdashboard.Overview(page, stats))
}

// handleTemplates renders the merchant's WhatsApp templates with their Meta
// approval status.
func (a *App) handleTemplates(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil || len(all) == 0 {
		return err
	}

	purged := 0
	if n, err := a.WhatsApp.PurgeExpiredMarketing(k.Request.Context()); err != nil {
		a.Log.Warn("marketing purge check failed", "error", err.Error())
	} else if n > 0 {
		purged = n
		a.Log.Info("expired marketing templates deleted", "count", n)
	}

	// Templates belong to the client and are shared across shops, so the page
	// lists the full catalog (an approved template under another store still
	// shows here), not just the active shop's.
	templates, err := a.WhatsApp.TemplatesAll(k.Request.Context())
	if err != nil {
		return err
	}
	shopNames := shopNameMap(all)

	page := a.dashboardPage(k, "templates", "templates", all)
	dict := page.I18N

	flash := vdashboard.TemplateFlash{}
	if purged > 0 {
		if purged == 1 {
			flash.Info = dict.Tf("tpl.flashPurgedOne", purged)
		} else {
			flash.Info = dict.Tf("tpl.flashPurgedMany", purged)
		}
	}
	switch k.Request.URL.Query().Get("flash") {
	case "created":
		flash.Info = dict.T("tpl.flashCreated")
	case "marketing":
		flash.Error = dict.T("tpl.flashMarketing")
	case "rejected":
		flash.Error = dict.T("tpl.flashRejected")
		if reason := k.Request.URL.Query().Get("reason"); reason != "" {
			flash.Error = dict.T("tpl.flashRejectedReason") + " " + reason
		}
	case "synced":
		flash.Info = dict.T("tpl.flashSynced")
	case "missing":
		flash.Error = dict.T("tpl.flashMissing")
	case "novariables":
		flash.Error = dict.T("tpl.flashNoVariables")
	case "example":
		flash.Error = dict.T("tpl.flashExample")
	case "toolong":
		flash.Error = dict.Tf("tpl.flashTooLong", whatsapp.MaxTemplateBodyChars)
	case "invalidbody":
		flash.Error = k.Request.URL.Query().Get("msg")
	case "deleted":
		flash.Info = dict.T("tpl.flashDeleted")
	case "notfound":
		flash.Error = dict.T("tpl.flashNotFound")
	case "inuse":
		flash.Error = dict.T("tpl.flashInUse") + " " +
			k.Request.URL.Query().Get("names")
	case "error", "internal", "refresh":
		flash.Error = dict.T("error.genericBody")
	}

	return k.Render(vdashboard.TemplatesPage(page, templates, shopNames, flash))
}

// handlePlaceholder renders a shell page for features that arrive in later phases.
func (a *App) handlePlaceholder(section string) func(*kit.Kit) error {
	return func(k *kit.Kit) error {
		all, err := a.shopsFor(k)
		if err != nil || len(all) == 0 {
			return err
		}
		page := a.dashboardPage(k, sectionTitle(section), section, all)
		return k.Render(vdashboard.Placeholder(page, section))
	}
}

func sectionTitle(section string) string {
	switch section {
	case "automations":
		return "automations"
	case "templates":
		return "templates"
	case "settings":
		return "settings"
	default:
		return "overview"
	}
}
