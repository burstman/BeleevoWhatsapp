package server

import (
	"context"
	"net/http"

	"github.com/anthdm/superkit/kit"
	"github.com/jackc/pgx/v5"

	"whatsappconverty/internal/whatsapp"
	vsettings "whatsappconverty/web/views/settings"
)

// handleWhatsappSettings renders the WhatsApp settings page. The WhatsApp
// connection is shared across the operator's shops: the page shows the single
// connected number (whatever shop holds it) and connecting/disconnecting
// manages it in place.
func (a *App) handleWhatsappSettings(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}

	page := a.dashboardPage(k, "Settings", "settings", all)

	integ, err := a.whatsappIntegrationAnyShop(k.Request.Context())
	if err == pgx.ErrNoRows {
		return k.Render(vsettings.Settings(page, nil, nil))
	}
	if err != nil {
		return err
	}
	return k.Render(vsettings.Settings(page, &integ, nil))
}

// whatsappIntegrationAnyShop finds the operator's WhatsApp connection wherever
// it lives. The number is shared across shops, so the row under any shop is the
// same connection. ErrNoRows is returned when nothing is connected yet.
func (a *App) whatsappIntegrationAnyShop(ctx context.Context) (whatsapp.Integration, error) {
	all, err := a.Shops.ListIntegrated(ctx)
	if err != nil {
		return whatsapp.Integration{}, err
	}
	var errs error
	for _, s := range all {
		integ, err := a.WhatsApp.Integration(ctx, s.ID)
		if err == nil {
			return integ, nil
		}
		if err != pgx.ErrNoRows {
			errs = err
		}
	}
	if errs != nil {
		return whatsapp.Integration{}, errs
	}
	return whatsapp.Integration{}, pgx.ErrNoRows
}

// handleWhatsappConnect stores the shared Meta WhatsApp credentials after a
// live sanity check of the token and ids against the Graph API. The connection
// is kept under the primary shop, which is also where the client's templates
// are synced, so sends resolve both from that shop.
func (a *App) handleWhatsappConnect(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID

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

	if err := a.WhatsApp.SaveIntegration(ctx, shopID, token, whatsapp.Integration{
		ShopID:             shopID,
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
		"shop_id", shopID,
		"phone_number_id", phoneNumberID,
		"messaging_account_id", messagingAccountID,
		"phone", phoneNumber,
		"verified_name", pn.VerifiedName)
	return k.Redirect(http.StatusSeeOther, "/settings?connect=success")
}

// handleWhatsappDisconnect removes the shared WhatsApp credentials.
func (a *App) handleWhatsappDisconnect(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID

	if err := a.WhatsApp.DeleteIntegration(k.Request.Context(), shopID); err != nil {
		a.Log.Error("whatsapp disconnect failed", "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings?connect=error")
	}
	a.Log.Info("whatsapp disconnected", "shop_id", shopID)
	return k.Redirect(http.StatusSeeOther, "/settings?connect=disconnected")
}

// handleWhatsappOnboard renders the WhatsApp enable page: the operator opts
// into the platform's shared number, records a contact phone, and accepts the
// service terms (which include the customer opt-in obligation).
func (a *App) handleWhatsappOnboard(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}

	page := a.dashboardPage(k, "Enable WhatsApp", "settings", all)
	return k.Render(vsettings.Onboarding(page, primaryShop(all)))
}

// handleWhatsappOnboardPost records the operator's opt-in: terms accepted,
// service enabled, contact phone saved. No Meta credentials are handled here —
// sending happens through the platform's centrally owned number.
func (a *App) handleWhatsappOnboardPost(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID

	if k.Request.FormValue("accept_terms") != "1" {
		return k.Redirect(http.StatusSeeOther, "/whatsapp/onboard?error=terms")
	}
	phone := k.Request.FormValue("shop_phone")

	if err := a.Shops.EnableWhatsApp(k.Request.Context(), shopID, phone); err != nil {
		a.Log.Error("whatsapp onboard enable failed", "shop_id", shopID, "error", err)
		return k.Redirect(http.StatusSeeOther, "/whatsapp/onboard?error=enable")
	}

	a.Log.Info("whatsapp service enabled", "shop_id", shopID, "phone", phone)
	return k.Redirect(http.StatusSeeOther, "/settings?connect=success")
}
