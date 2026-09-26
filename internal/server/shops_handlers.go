package server

import (
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

// primaryShop returns the canonical owner for shared settings (WhatsApp
// connection, delivery token, template sync): the oldest integrated shop. The
// callers have already ensured at least one integrated shop exists.
func primaryShop(all []shops.Shop) shops.Shop {
	if len(all) == 0 {
		return shops.Shop{}
	}
	return all[0]
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
