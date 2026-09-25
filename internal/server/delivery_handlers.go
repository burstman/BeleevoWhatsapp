package server

import (
	"net/http"
	"strings"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"

	"whatsappconverty/internal/delivery"
	vsettings "whatsappconverty/web/views/settings"
)

// handleDeliverySettings renders the Delivery settings page: the Mes Colis
// connection (where the access token is entered) plus every parcel currently
// being watched for delivery events.
func (a *App) handleDeliverySettings(k *kit.Kit) error {
	active, all, err := a.activeShops(k)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return k.Redirect(http.StatusSeeOther, "/integrations")
	}

	ctx := k.Request.Context()
	integ, err := a.Delivery.Integration(ctx, active.ID, delivery.ProviderMescolis)
	if err != nil && err != delivery.ErrNotConfigured {
		return err
	}
	var integPtr *delivery.Integration
	if err == nil {
		integPtr = &integ
	}

	tracked, err := a.Delivery.Tracked(ctx, active.ID)
	if err != nil {
		return err
	}

	flash := vsettings.DeliveryFlash{}
	switch k.Request.URL.Query().Get("flash") {
	case "connected":
		flash.Info = "Mes Colis connected — parcel statuses are now polled."
	case "testfailed":
		flash.Error = "Connection test failed — check that the access token is valid for Mes Colis Express."
	case "missing":
		flash.Error = "The access token is required."
	case "error", "internal":
		flash.Error = "Something went wrong — try again."
	case "disconnected":
		flash.Info = "Delivery provider disconnected."
	case "removed":
		flash.Info = "Parcel removed from tracking."
	}

	page := a.dashboardPage(k, "Delivery", "delivery", active, all)
	return k.Render(vsettings.Delivery(page, integPtr, tracked, flash))
}

// handleDeliveryConnect stores the shop's Mes Colis access token after a live
// sanity check that the token reaches the API.
func (a *App) handleDeliveryConnect(k *kit.Kit) error {
	active, err := a.requireShop(k)
	if err != nil {
		return err
	}

	ctx := k.Request.Context()
	token := strings.TrimSpace(k.Request.FormValue("access_token"))
	accountCode := strings.TrimSpace(k.Request.FormValue("account_code"))
	allowSubAccount := k.Request.FormValue("allow_sub_account") == "1"
	if token == "" {
		return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=missing")
	}

	// Verify the token reaches Mes Colis before persisting anything.
	client := delivery.NewMescolisClient(token, allowSubAccount, accountCode, a.Cfg, nil)
	if err := client.Probe(ctx); err != nil {
		a.Log.Warn("delivery connect rejected: token check failed", "shop_id", active.ID, "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=testfailed")
	}

	if err := a.Delivery.SaveIntegration(ctx, active.ID, delivery.ProviderMescolis, token, accountCode, allowSubAccount); err != nil {
		a.Log.Error("delivery connect failed to save", "shop_id", active.ID, "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=error")
	}

	a.Log.Info("delivery provider connected", "shop_id", active.ID, "provider", delivery.ProviderMescolis)
	return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=connected")
}

// handleDeliveryDisconnect removes the shop's delivery provider connection.
func (a *App) handleDeliveryDisconnect(k *kit.Kit) error {
	active, err := a.requireShop(k)
	if err != nil {
		return err
	}
	if err := a.Delivery.DeleteIntegration(k.Request.Context(), active.ID, delivery.ProviderMescolis); err != nil {
		a.Log.Error("delivery disconnect failed", "shop_id", active.ID, "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=error")
	}
	a.Log.Info("delivery provider disconnected", "shop_id", active.ID, "provider", delivery.ProviderMescolis)
	return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=disconnected")
}

// handleDeliveryTrackRemove stops watching a parcel.
func (a *App) handleDeliveryTrackRemove(k *kit.Kit) error {
	active, err := a.requireShop(k)
	if err != nil {
		return err
	}
	barcode := chi.URLParam(k.Request, "barcode")
	if err := a.Delivery.RemoveTracked(k.Request.Context(), active.ID, barcode); err != nil {
		a.Log.Error("delivery untrack failed", "shop_id", active.ID, "barcode", barcode, "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=error")
	}
	return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=removed")
}