package server

import (
	"net/http"

	"github.com/anthdm/superkit/kit"
	"github.com/jackc/pgx/v5"

	"whatsappconverty/internal/whatsapp"
	vsettings "whatsappconverty/web/views/settings"
)

// handleWhatsappSettings renders the WhatsApp settings page. This is a legacy
// view that predates the shared single-WABA model: template listings belong to
// the shop-scoped /templates page only. A shop must never see the platform's
// account-level templates (other merchants' content), so no template list is
// rendered here.
func (a *App) handleWhatsappSettings(k *kit.Kit) error {
	active, all, err := a.activeShops(k)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return k.Redirect(http.StatusSeeOther, "/integrations")
	}

	page := a.dashboardPage(k, "Settings", "settings", active, all)

	integ, err := a.WhatsApp.Integration(k.Request.Context(), active.ID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return k.Render(vsettings.Settings(page, nil, nil))
		}
		return err
	}

	return k.Render(vsettings.Settings(page, &integ, nil))
}

// handleWhatsappConnect stores the shop's Meta WhatsApp credentials after a
// live sanity check of the token and ids against the Graph API.
func (a *App) handleWhatsappConnect(k *kit.Kit) error {
	active, err := a.requireShop(k)
	if err != nil {
		return err
	}

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

	if err := a.WhatsApp.SaveIntegration(ctx, active.ID, token, whatsapp.Integration{
		ShopID:             active.ID,
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
		"shop_id", active.ID,
		"phone_number_id", phoneNumberID,
		"messaging_account_id", messagingAccountID,
		"phone", phoneNumber,
		"verified_name", pn.VerifiedName)
	return k.Redirect(http.StatusSeeOther, "/settings?connect=success")
}

// handleWhatsappDisconnect removes the shop's WhatsApp credentials.
func (a *App) handleWhatsappDisconnect(k *kit.Kit) error {
	active, err := a.requireShop(k)
	if err != nil {
		return err
	}

	if err := a.WhatsApp.DeleteIntegration(k.Request.Context(), active.ID); err != nil {
		a.Log.Error("whatsapp disconnect failed", "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings?connect=error")
	}
	a.Log.Info("whatsapp disconnected", "shop_id", active.ID)
	return k.Redirect(http.StatusSeeOther, "/settings?connect=disconnected")
}

// handleWhatsappOnboard renders the WhatsApp enable page: the merchant opts
// into the platform's shared number, records a contact phone, and accepts the
// service terms (which include the customer opt-in obligation).
func (a *App) handleWhatsappOnboard(k *kit.Kit) error {
	active, all, err := a.activeShops(k)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return k.Redirect(http.StatusSeeOther, "/integrations")
	}

	page := a.dashboardPage(k, "Enable WhatsApp", "settings", active, all)
	return k.Render(vsettings.Onboarding(page, active))
}

// handleWhatsappOnboardPost records the merchant's opt-in: terms accepted,
// service enabled, contact phone saved. No Meta credentials are handled here —
// sending happens through the platform's centrally owned number.
func (a *App) handleWhatsappOnboardPost(k *kit.Kit) error {
	active, err := a.requireShop(k)
	if err != nil {
		return err
	}

	if k.Request.FormValue("accept_terms") != "1" {
		return k.Redirect(http.StatusSeeOther, "/whatsapp/onboard?error=terms")
	}
	phone := k.Request.FormValue("shop_phone")

	if err := a.Shops.EnableWhatsApp(k.Request.Context(), active.ID, phone); err != nil {
		a.Log.Error("whatsapp onboard enable failed", "shop_id", active.ID, "error", err)
		return k.Redirect(http.StatusSeeOther, "/whatsapp/onboard?error=enable")
	}

	a.Log.Info("whatsapp service enabled", "shop_id", active.ID, "phone", phone)
	return k.Redirect(http.StatusSeeOther, "/settings?connect=success")
}
