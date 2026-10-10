package server

import (
	"context"
	"net/http"

	"github.com/anthdm/superkit/kit"

	"whatsappconverty/internal/auth"
	"whatsappconverty/internal/i18n"
	"whatsappconverty/internal/shops"
	viewshared "whatsappconverty/web/views/components"
)

// shopsFor resolves the operator's integrated shop list, redirecting to the
// integration page (the entry point) when no store has been connected yet.
// There is no per-request "active shop": data is cross-shop, and per-shop
// resources carry their own shop id.
func (a *App) shopsFor(k *kit.Kit) ([]shops.Shop, error) {
	all, err := a.Shops.ListIntegrated(k.Request.Context())
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, k.Redirect(http.StatusSeeOther, "/integrations")
	}
	return all, nil
}

// pageShops lists every shop for settings pages that must render before any
// Converty store is connected: the shared WhatsApp connection, delivery
// tracking and onboarding all keep working regardless of a store's OAuth
// status, because their connections sit on the shop row, not the integration.
func (a *App) pageShops(k *kit.Kit) ([]shops.Shop, error) {
	return a.Shops.List(k.Request.Context())
}

// sharedOwnerShop returns the shop that owns the operator's shared resources:
// whichever shop holds the WhatsApp connection (the number and its approved
// templates co-locate there, and sends resolve through that shop's creds).
// Falls back to the oldest shop when nothing is connected yet. It scans every
// shop, not only the integrated ones, because some shops may hold shared
// resources without a Converty integration in the same row set.
func (a *App) sharedOwnerShop(ctx context.Context) (shops.Shop, error) {
	all, err := a.Shops.List(ctx)
	if err != nil {
		return shops.Shop{}, err
	}
	if len(all) == 0 {
		return shops.Shop{}, shops.ErrNotFound
	}
	for _, s := range all {
		if _, err := a.WhatsApp.Integration(ctx, s.ID); err == nil {
			return s, nil
		}
	}
	return all[0], nil
}

// dashboardPage builds the page scaffolding every dashboard page shares.
// titleKey is the "title.*" translation key for the page's heading, not the
// display text. StoreConnected reflects the Converty integration state, not the
// raw shop list: settings pages pass every shop (WhatsApp/delivery connections
// are shared and standalone), so the sidebar flags Overview, Automations and
// Templates as locked until an actual store is connected.
func (a *App) dashboardPage(k *kit.Kit, titleKey, activeSection string, all []shops.Shop) viewshared.Page {
	principal := auth.FromKit(k)
	storeConnected := len(all) > 0
	if integrated, err := a.Shops.ListIntegrated(k.Request.Context()); err == nil {
		storeConnected = len(integrated) > 0
	}
	waConnected := false
	if _, err := a.whatsappIntegrationAnyShop(k.Request.Context()); err == nil {
		waConnected = true
	}
	dict := i18n.New(i18n.Parse(principal.User.Lang))
	return viewshared.Page{
		Title:             dict.T("title." + titleKey),
		Active:            activeSection,
		UserName:          principal.User.Name,
		Shops:             all,
		StoreConnected:    storeConnected,
		WhatsAppConnected: waConnected,
		I18N:              dict,
	}
}

// publicLang resolves the interface language for unauthenticated pages
// (landing, login, legal). It prefers the durable cookie the language switch
// writes so a user who just chose French sees the login screen in French too.
func (a *App) publicLang(k *kit.Kit) i18n.Lang {
	if c, err := k.Request.Cookie("lang"); err == nil {
		return i18n.Parse(c.Value)
	}
	return i18n.En
}
