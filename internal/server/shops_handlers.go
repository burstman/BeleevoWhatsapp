package server

import (
	"net/http"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"whatsappconverty/internal/auth"
	"whatsappconverty/internal/shops"
	viewshared "whatsappconverty/web/views/components"
)

// ActiveShopCookieName remembers the operator's selected shop across requests.
const ActiveShopCookieName = "active_shop"

// activeShops resolves the integrated shop list (a shop only exists once its
// Converty store is connected) plus the currently selected shop (from a
// cookie), falling back to the first shop when the cookie is missing/stale.
func (a *App) activeShops(k *kit.Kit) (shops.Shop, []shops.Shop, error) {
	all, err := a.Shops.ListIntegrated(k.Request.Context())
	if err != nil {
		return shops.Shop{}, nil, err
	}
	if len(all) == 0 {
		return shops.Shop{}, all, nil
	}

	active := all[0]
	if c, cerr := k.Request.Cookie(ActiveShopCookieName); cerr == nil {
		if id, perr := uuid.Parse(c.Value); perr == nil {
			for _, s := range all {
				if s.ID == id {
					active = s
					break
				}
			}
		}
	}
	return active, all, nil
}

// requireShop resolves the selected shop, redirecting to the integration page
// (the entry point) when no store has been connected yet.
func (a *App) requireShop(k *kit.Kit) (shops.Shop, error) {
	active, all, err := a.activeShops(k)
	if err != nil {
		return shops.Shop{}, err
	}
	if len(all) == 0 {
		return shops.Shop{}, k.Redirect(http.StatusSeeOther, "/integrations")
	}
	return active, nil
}

// setActiveShopCookie pins the selected shop for the operator.
func setActiveShopCookie(k *kit.Kit, id uuid.UUID) {
	cookie := &http.Cookie{
		Name:     ActiveShopCookieName,
		Value:    id.String(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(k.Response, cookie)
}

// dashboardPage builds the page scaffolding every dashboard page shares.
func (a *App) dashboardPage(k *kit.Kit, title, activeSection string, active shops.Shop, all []shops.Shop) viewshared.Page {
	principal := auth.FromKit(k)
	return viewshared.Page{
		Title:        title,
		Active:       activeSection,
		ShopName:     active.Name,
		UserName:     principal.User.Name,
		Shops:        all,
		ActiveShopID: active.ID,
	}
}

// handleShopsSelect pins the active shop via cookie and returns to the
// dashboard. The shop must be integrated to be selectable.
func (a *App) handleShopsSelect(k *kit.Kit) error {
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/integrations")
	}
	if _, err := a.Shops.GetByID(k.Request.Context(), id); err != nil {
		return k.Redirect(http.StatusSeeOther, "/integrations")
	}
	setActiveShopCookie(k, id)
	return k.Redirect(http.StatusSeeOther, "/dashboard")
}
