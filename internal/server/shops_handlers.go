package server

import (
	"context"
	"net/http"

	"github.com/anthdm/superkit/kit"

	"whatsappconverty/internal/auth"
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
func (a *App) dashboardPage(k *kit.Kit, title, activeSection string, all []shops.Shop) viewshared.Page {
	principal := auth.FromKit(k)
	return viewshared.Page{
		Title:    title,
		Active:   activeSection,
		UserName: principal.User.Name,
		Shops:    all,
	}
}
