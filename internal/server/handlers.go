package server

import (
	"errors"
	"net/http"

	"github.com/anthdm/superkit/kit"

	"whatsappconverty/internal/auth"
	"whatsappconverty/internal/database"
	"whatsappconverty/internal/shops"
	"whatsappconverty/internal/whatsapp"
	viewshared "whatsappconverty/web/views/components"
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
	return k.Render(vlanding.Index())
}

// handlePrivacyPage renders the public Privacy Policy, required by Meta for
// live apps. It intentionally skips auth so the URL can be published to the
// WhatsApp review and marketing materials.
func (a *App) handlePrivacyPage(k *kit.Kit) error {
	return k.Render(vlegal.Privacy())
}

func (a *App) handleOverview(k *kit.Kit) error {
	principal := auth.FromKit(k)
	shop, err := a.Shops.GetByID(k.Request.Context(), principal.User.ShopID)
	if err != nil && !errors.Is(err, shops.ErrNotFound) {
		return err
	}

	stats, err := a.Dashboard.Stats(k.Request.Context(), principal.User.ShopID)
	if err != nil {
		return err
	}

	page := viewshared.Page{
		Title:    "Overview",
		ShopName: shop.Name,
		UserName: principal.User.Name,
	}

	return k.Render(vdashboard.Overview(page, stats))
}

// handleTemplates renders the merchant's WhatsApp templates with their Meta
// approval status.
func (a *App) handleTemplates(k *kit.Kit) error {
	principal := auth.FromKit(k)
	shop, err := a.Shops.GetByID(k.Request.Context(), principal.User.ShopID)
	if err != nil && !errors.Is(err, shops.ErrNotFound) {
		return err
	}

	templates, err := a.WhatsApp.Templates(k.Request.Context(), principal.User.ShopID)
	if err != nil {
		return err
	}

	flash := vdashboard.TemplateFlash{}
	switch k.Request.URL.Query().Get("flash") {
	case "created":
		flash.Info = "Template submitted to Meta for review. Status refreshes here once decided."
	case "rejected":
		flash.Error = "Meta rejected the submission — check the rejection reason on the row below."
	case "synced":
		flash.Info = "Approval statuses refreshed from Meta."
	case "missing":
		flash.Error = "Name, language and body are required."
	case "novariables":
		flash.Error = "The body must contain at least one {{N}} placeholder."
	case "example":
		flash.Error = "Provide an example value for every {{N}} placeholder."
	case "error", "internal", "refresh":
		flash.Error = "Something went wrong — try again."
	}

	page := viewshared.Page{
		Title:    "Templates",
		ShopName: shop.Name,
		UserName: principal.User.Name,
	}
	return k.Render(vdashboard.TemplatesPage(page, templates, flash))
}

// handleMessages renders the merchant's message history with delivery status
// plus the approved templates available for a test send.
func (a *App) handleMessages(k *kit.Kit) error {
	principal := auth.FromKit(k)
	shop, err := a.Shops.GetByID(k.Request.Context(), principal.User.ShopID)
	if err != nil && !errors.Is(err, shops.ErrNotFound) {
		return err
	}

	messages, err := a.WhatsApp.Messages(k.Request.Context(), principal.User.ShopID)
	if err != nil {
		return err
	}

	templates, err := a.WhatsApp.Templates(k.Request.Context(), principal.User.ShopID)
	if err != nil {
		return err
	}
	approved := make([]whatsapp.MerchantTemplate, 0, len(templates))
	for _, t := range templates {
		if t.ApprovalStatus == "approved" {
			approved = append(approved, t)
		}
	}

	page := viewshared.Page{
		Title:    "Messages",
		ShopName: shop.Name,
		UserName: principal.User.Name,
	}
	return k.Render(vdashboard.MessagesPage(page, messages, approved))
}

// handlePlaceholder renders a shell page for features that arrive in later phases.
func (a *App) handlePlaceholder(section string) func(*kit.Kit) error {
	return func(k *kit.Kit) error {
		principal := auth.FromKit(k)
		shop, err := a.Shops.GetByID(k.Request.Context(), principal.User.ShopID)
		if err != nil && !errors.Is(err, shops.ErrNotFound) {
			return err
		}
		page := viewshared.Page{
			Title:    sectionTitle(section),
			ShopName: shop.Name,
			UserName: principal.User.Name,
		}
		return k.Render(vdashboard.Placeholder(page, section))
	}
}

func sectionTitle(section string) string {
	switch section {
	case "automations":
		return "Automations"
	case "templates":
		return "Templates"
	case "messages":
		return "Messages"
	case "settings":
		return "Settings"
	default:
		return "Dashboard"
	}
}
