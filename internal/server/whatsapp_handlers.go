package server

import (
	"net/http"

	"github.com/anthdm/superkit/kit"
	"github.com/jackc/pgx/v5"

	"whatsappconverty/internal/auth"
	"whatsappconverty/internal/shops"
	"whatsappconverty/internal/whatsapp"
	viewshared "whatsappconverty/web/views/components"
	vsettings "whatsappconverty/web/views/settings"
)

// handleWhatsappSettings renders the WhatsApp onboarding/connect page. When a
// shop already has credentials it shows the connected state plus the templates
// available on its messaging account.
func (a *App) handleWhatsappSettings(k *kit.Kit) error {
	principal := auth.FromKit(k)
	shop, err := a.Shops.GetByID(k.Request.Context(), principal.User.ShopID)
	if err != nil && err != shops.ErrNotFound {
		return err
	}

	page := viewshared.Page{
		Title:    "Settings",
		Active:   "settings",
		ShopName: shop.Name,
		UserName: principal.User.Name,
	}

	integ, err := a.WhatsApp.Integration(k.Request.Context(), principal.User.ShopID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return k.Render(vsettings.Settings(page, nil, nil))
		}
		return err
	}

	token, err := a.WhatsApp.DecryptToken(integ.AccessTokenEncrypted)
	if err != nil {
		return err
	}

	templates, err := a.WhatsApp.ListTemplates(k.Request.Context(), token, integ.MessagingAccountID)
	if err != nil {
		a.Log.Warn("whatsapp: template list failed on settings page", "error", err)
	}

	return k.Render(vsettings.Settings(page, &integ, templates))
}

// handleWhatsappConnect stores the shop's Meta WhatsApp credentials after a
// live sanity check of the token and ids against the Graph API.
func (a *App) handleWhatsappConnect(k *kit.Kit) error {
	principal := auth.FromKit(k)

	ctx := k.Request.Context()
	token := k.Request.FormValue("token")
	phoneNumberID := k.Request.FormValue("phone_number_id")
	messagingAccountID := k.Request.FormValue("messaging_account_id")
	phoneNumber := k.Request.FormValue("phone_number")

	if token == "" || phoneNumberID == "" || messagingAccountID == "" {
		a.Log.Warn("whatsapp connect rejected: missing fields")
		return k.Redirect(http.StatusSeeOther, "/settings?connect=error")
	}

	// Verify the credentials are live before persisting anything.
	pn, err := a.WhatsApp.GetPhoneNumber(ctx, token, phoneNumberID)
	if err != nil {
		a.Log.Warn("whatsapp connect rejected: phone number check failed", "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings?connect=error")
	}
	if phoneNumber == "" {
		phoneNumber = pn.DisplayPhoneNumber
	}

	if err := a.WhatsApp.SaveIntegration(ctx, principal.User.ShopID, token, whatsapp.Integration{
		ShopID:             principal.User.ShopID,
		WaacID:             phoneNumberID,
		PhoneNumberID:      phoneNumberID,
		MessagingAccountID: messagingAccountID,
		PhoneNumber:        phoneNumber,
		Status:             "connected",
	}); err != nil {
		a.Log.Error("whatsapp connect failed to save", "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings?connect=error")
	}

	a.Log.Info("whatsapp connected",
		"shop_id", principal.User.ShopID,
		"phone_number_id", phoneNumberID,
		"messaging_account_id", messagingAccountID,
		"phone", phoneNumber,
		"verified_name", pn.VerifiedName)
	return k.Redirect(http.StatusSeeOther, "/settings?connect=success")
}

// handleWhatsappDisconnect removes the shop's WhatsApp credentials.
func (a *App) handleWhatsappDisconnect(k *kit.Kit) error {
	principal := auth.FromKit(k)

	if err := a.WhatsApp.DeleteIntegration(k.Request.Context(), principal.User.ShopID); err != nil {
		a.Log.Error("whatsapp disconnect failed", "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings?connect=error")
	}
	a.Log.Info("whatsapp disconnected", "shop_id", principal.User.ShopID)
	return k.Redirect(http.StatusSeeOther, "/settings?connect=disconnected")
}

// handleWhatsappOnboard renders the WhatsApp enable page: the merchant opts
// into the platform's shared number, records a contact phone, and accepts the
// service terms (which include the customer opt-in obligation).
func (a *App) handleWhatsappOnboard(k *kit.Kit) error {
	principal := auth.FromKit(k)

	shop, err := a.Shops.GetByID(k.Request.Context(), principal.User.ShopID)
	if err != nil && err != shops.ErrNotFound {
		return err
	}

	page := viewshared.Page{
		Title:    "Enable WhatsApp",
		Active:   "settings",
		ShopName: shop.Name,
		UserName: principal.User.Name,
	}
	return k.Render(vsettings.Onboarding(page, shop))
}

// handleWhatsappOnboardPost records the merchant's opt-in: terms accepted,
// service enabled, contact phone saved. No Meta credentials are handled here —
// sending happens through the platform's centrally owned number.
func (a *App) handleWhatsappOnboardPost(k *kit.Kit) error {
	principal := auth.FromKit(k)

	if k.Request.FormValue("accept_terms") != "1" {
		return k.Redirect(http.StatusSeeOther, "/whatsapp/onboard?error=terms")
	}
	phone := k.Request.FormValue("shop_phone")

	if err := a.Shops.EnableWhatsApp(k.Request.Context(), principal.User.ShopID, phone); err != nil {
		a.Log.Error("whatsapp onboard enable failed", "shop_id", principal.User.ShopID, "error", err)
		return k.Redirect(http.StatusSeeOther, "/whatsapp/onboard?error=enable")
	}

	a.Log.Info("whatsapp service enabled", "shop_id", principal.User.ShopID, "phone", phone)
	return k.Redirect(http.StatusSeeOther, "/settings?connect=success")
}
