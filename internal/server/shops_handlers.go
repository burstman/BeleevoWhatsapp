package server

import (
	"net/http"
	"strings"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"whatsappconverty/internal/auth"
	"whatsappconverty/internal/shops"
	viewshared "whatsappconverty/web/views/components"
	vdashboard "whatsappconverty/web/views/dashboard"
)

// ActiveShopCookieName remembers the operator's selected shop across requests.
const ActiveShopCookieName = "active_shop"

// activeShops resolves the operator's full shop list plus the currently
// selected shop (from a cookie), falling back to the first shop when the
// cookie is missing or stale.
func (a *App) activeShops(k *kit.Kit) (shops.Shop, []shops.Shop, error) {
	all, err := a.Shops.List(k.Request.Context())
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

// requireShop resolves the selected shop, redirecting to /shops when the
// operator has no shop yet.
func (a *App) requireShop(k *kit.Kit) (shops.Shop, error) {
	active, all, err := a.activeShops(k)
	if err != nil {
		return shops.Shop{}, err
	}
	if len(all) == 0 {
		return shops.Shop{}, k.Redirect(http.StatusSeeOther, "/shops")
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

// handleShops renders the operator's shop list with a create form.
func (a *App) handleShops(k *kit.Kit) error {
	active, all, err := a.activeShops(k)
	if err != nil {
		return err
	}

	flash := vdashboard.ShopFlash{}
	switch k.Request.URL.Query().Get("flash") {
	case "created":
		flash.Info = "Shop created."
	case "updated":
		flash.Info = "Shop renamed."
	case "missing":
		flash.Error = "A shop name is required."
	}

	page := a.dashboardPage(k, "Shops", "shops", active, all)
	return k.Render(vdashboard.ShopsPage(page, all, active.ID, flash))
}

// handleShopsCreate adds a new shop to the operator's portfolio and selects it.
func (a *App) handleShopsCreate(k *kit.Kit) error {
	name := strings.TrimSpace(k.Request.FormValue("name"))
	if name == "" {
		return k.Redirect(http.StatusSeeOther, "/shops?flash=missing")
	}

	shop, err := a.Shops.Create(k.Request.Context(), a.Pool, name)
	if err != nil {
		return err
	}

	setActiveShopCookie(k, shop.ID)
	a.Log.Info("shop created", "shop_id", shop.ID, "name", name)
	return k.Redirect(http.StatusSeeOther, "/dashboard")
}

// handleShopsSelect pins the active shop via cookie and returns to the dashboard.
func (a *App) handleShopsSelect(k *kit.Kit) error {
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/shops")
	}
	if _, err := a.Shops.GetByID(k.Request.Context(), id); err != nil {
		return k.Redirect(http.StatusSeeOther, "/shops")
	}
	setActiveShopCookie(k, id)
	return k.Redirect(http.StatusSeeOther, "/dashboard")
}

// handleShopUpdate renames a shop.
func (a *App) handleShopUpdate(k *kit.Kit) error {
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/shops?flash=missing")
	}
	name := strings.TrimSpace(k.Request.FormValue("name"))
	if name == "" {
		return k.Redirect(http.StatusSeeOther, "/shops?flash=missing")
	}
	if err := a.Shops.Update(k.Request.Context(), id, name); err != nil {
		return err
	}
	return k.Redirect(http.StatusSeeOther, "/shops?flash=updated")
}
