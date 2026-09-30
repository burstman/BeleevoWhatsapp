package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"whatsappconverty/internal/delivery"
	vsettings "whatsappconverty/web/views/settings"
)

// handleDeliverySettings renders the Delivery settings page: the shared Mes
// Colis connection (where the access token is entered) plus every parcel
// currently being watched across the operator's shops.
func (a *App) handleDeliverySettings(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}

	ctx := k.Request.Context()
	owner, err := a.sharedOwnerShop(ctx)
	if err != nil {
		return err
	}
	shopID := owner.ID
	integ, err := a.Delivery.Integration(ctx, shopID, delivery.ProviderMescolis)
	if err != nil && err != delivery.ErrNotConfigured {
		return err
	}
	var integPtr *delivery.Integration
	if err == nil {
		integPtr = &integ
	}

	tracked := []delivery.TrackedOrder{}
	allShops, err := a.Shops.List(ctx)
	if err != nil {
		return err
	}
	for _, s := range allShops {
		rows, tErr := a.Delivery.Tracked(ctx, s.ID)
		if tErr != nil {
			return tErr
		}
		for _, t := range rows {
			if !delivery.StatusTerminal(t.LastStatus) {
				tracked = append(tracked, t)
			}
		}
	}

	flash := vsettings.DeliveryFlash{}
	switch k.Request.URL.Query().Get("flash") {
	case "connected":
		flash.Info = "Mes Colis connected — parcel statuses are now polled."
	case "testfailed":
		if msg := k.Request.URL.Query().Get("error"); msg != "" {
			flash.Error = "Connection test failed: " + msg
		} else {
			flash.Error = "Connection test failed — check that the access token is valid for Mes Colis Express."
		}
	case "missing":
		flash.Error = "The access token is required."
	case "error", "internal":
		flash.Error = "Something went wrong — try again."
	case "disconnected":
		flash.Info = "Delivery provider disconnected."
	case "removed":
		flash.Info = "Parcel removed from tracking."
	case "polled":
		changes, _ := strconv.Atoi(k.Request.URL.Query().Get("changes"))
		checked, _ := strconv.Atoi(k.Request.URL.Query().Get("checked"))
		if changes == 0 {
			flash.Info = fmt.Sprintf("Checked %d parcel(s) — no new status changes.", checked)
		} else {
			flash.Info = fmt.Sprintf("Checked %d parcel(s) — %d status change(s) found.", checked, changes)
		}
	}

	shopNames := shopNameMap(all)
	page := a.dashboardPage(k, "Delivery", "delivery", all)
	return k.Render(vsettings.Delivery(page, integPtr, tracked, shopNames, flash))
}

// handleDeliveryConnect stores the shared Mes Colis access token after a live
// sanity check that the token reaches the API. The connection lives on the
// shop that owns the shared resources (whichever shop already holds the
// WhatsApp row, else the oldest).
func (a *App) handleDeliveryConnect(k *kit.Kit) error {
	if _, err := a.shopsFor(k); err != nil {
		return err
	}
	owner, err := a.sharedOwnerShop(k.Request.Context())
	if err != nil {
		return err
	}
	shopID := owner.ID

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
		a.Log.Warn("delivery connect rejected: token check failed",
			"shop_id", shopID,
			"token_len", len(token),
			"token_shape", maskedToken(token),
			"error", err)
		msg := strings.TrimSpace(err.Error())
		if msg == "" {
			msg = "token rejected"
		}
		return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=testfailed&error="+url.QueryEscape(msg))
	}

	if err := a.Delivery.SaveIntegration(ctx, shopID, delivery.ProviderMescolis, token, accountCode, allowSubAccount); err != nil {
		a.Log.Error("delivery connect failed to save", "shop_id", shopID, "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=error")
	}

	a.Log.Info("delivery provider connected", "shop_id", shopID, "provider", delivery.ProviderMescolis)
	return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=connected")
}

// handleDeliveryDisconnect removes the shared delivery provider connection.
func (a *App) handleDeliveryDisconnect(k *kit.Kit) error {
	if _, err := a.shopsFor(k); err != nil {
		return err
	}
	owner, err := a.sharedOwnerShop(k.Request.Context())
	if err != nil {
		return err
	}
	shopID := owner.ID

	if err := a.Delivery.DeleteIntegration(k.Request.Context(), shopID, delivery.ProviderMescolis); err != nil {
		a.Log.Error("delivery disconnect failed", "shop_id", shopID, "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=error")
	}
	a.Log.Info("delivery provider disconnected", "shop_id", shopID, "provider", delivery.ProviderMescolis)
	return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=disconnected")
}

func maskedToken(t string) string {
	if t == "" {
		return "<empty>"
	}
	if len(t) <= 8 {
		return "<short>"
	}
	return t[:4] + "..." + t[len(t)-4:]
}

// handleDeliveryPoll runs the delivery sweep on demand. It is the same call the
// two-minute ticker makes, automations included, so an operator can prove the
// poller reaches Mes Colis instead of waiting to see whether a message arrives.
func (a *App) handleDeliveryPoll(k *kit.Kit) error {
	ctx := k.Request.Context()

	allShops, err := a.Shops.List(ctx)
	if err != nil {
		return err
	}
	checked := 0
	for _, s := range allShops {
		rows, tErr := a.Delivery.Tracked(ctx, s.ID)
		if tErr != nil {
			return tErr
		}
		for _, t := range rows {
			if !delivery.StatusTerminal(t.LastStatus) {
				checked++
			}
		}
	}

	changes, err := a.Automations.SweepDelivery(ctx)
	if err != nil {
		a.Log.Error("delivery manual sweep failed", "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=error")
	}
	a.Log.Info("delivery manual sweep", "parcels_checked", checked, "status_changes", changes)
	return k.Redirect(http.StatusSeeOther,
		"/settings/delivery?flash=polled&changes="+strconv.Itoa(changes)+"&checked="+strconv.Itoa(checked))
}

// handleDeliveryTrackRemove stops watching a parcel, resolving the shop that
// tracks it.
func (a *App) handleDeliveryTrackRemove(k *kit.Kit) error {
	barcode := chi.URLParam(k.Request, "barcode")

	all, err := a.Shops.List(k.Request.Context())
	if err != nil {
		return err
	}

	var target uuid.UUID
	found := false
	for _, s := range all {
		rows, tErr := a.Delivery.Tracked(k.Request.Context(), s.ID)
		if tErr != nil {
			return tErr
		}
		for _, t := range rows {
			if t.Barcode == barcode {
				target = s.ID
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		a.Log.Debug("delivery untrack: barcode not found", "barcode", barcode)
		return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=removed")
	}

	if err := a.Delivery.RemoveTracked(k.Request.Context(), target, barcode); err != nil {
		a.Log.Error("delivery untrack failed", "shop_id", target, "barcode", barcode, "error", err)
		return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=error")
	}
	return k.Redirect(http.StatusSeeOther, "/settings/delivery?flash=removed")
}
