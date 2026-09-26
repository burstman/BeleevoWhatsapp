package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"whatsappconverty/internal/automations"
	"whatsappconverty/internal/whatsapp"
	vdashboard "whatsappconverty/web/views/dashboard"
)

// handleAutomations renders the automation page: every configured trigger shown
// as a card (naming, trigger, template, delivery rule), plus the create button.
func (a *App) handleAutomations(k *kit.Kit) error {
	active, all, err := a.activeShops(k)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return k.Redirect(http.StatusSeeOther, "/integrations")
	}

	ctx := k.Request.Context()
	list, err := a.Automations.ListByShop(ctx, active.ID)
	if err != nil {
		return err
	}

	templates, err := a.WhatsApp.Templates(ctx, active.ID)
	if err != nil {
		return err
	}
	approved, names, bodies := catalogFromTemplates(templates)

	flash := vdashboard.AutomationFlash{}
	switch k.Request.URL.Query().Get("flash") {
	case "":
	case "created":
		flash.Info = "Automation created. It fires on matching order or delivery events from now on."
	case "updated":
		flash.Info = "Automation updated."
	case "duplicate":
		flash.Error = "An automation for this trigger already exists."
	case "missing":
		flash.Error = "A name, trigger and template are required."
	case "badschedule":
		flash.Error = "The send rule is invalid — check the time, timezone, days or delay."
	case "notemplate":
		flash.Error = "Create and get an approved template first."
	case "toggled":
		flash.Info = "Automation updated."
	case "deleted":
		flash.Info = "Automation removed."
	case "notfound":
		flash.Error = "That automation no longer exists."
	case "error", "internal":
		flash.Error = "Something went wrong — try again."
	}

	page := a.dashboardPage(k, "Automations", "automations", active, all)
	return k.Render(vdashboard.AutomationsPage(page, list, approved, names, bodies,
		automations.DeliveryStatuses(), flash))
}

// handleAutomationEdit renders the create form when no id is given, or the
// prefilled edit form for one automation. The form lets the operator choose the
// target shop, so the edited automation resolves to its own shop, not the
// operator's active one.
func (a *App) handleAutomationEdit(k *kit.Kit) error {
	active, all, err := a.activeShops(k)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return k.Redirect(http.StatusSeeOther, "/integrations")
	}

	ctx := k.Request.Context()
	var automation *automations.Automation
	shopID := active.ID
	if idParam := chi.URLParam(k.Request, "id"); idParam != "" {
		id, pErr := uuid.Parse(idParam)
		if pErr != nil {
			return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
		}
		automation, err = a.Automations.AutomationByID(ctx, id)
		if err != nil {
			a.Log.Error("automation edit lookup failed", "id", id, "error", err)
			return err
		}
		if automation == nil {
			return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
		}
		shopID = automation.ShopID
	}

	templates, err := a.WhatsApp.Templates(ctx, shopID)
	if err != nil {
		return err
	}
	approved, _, _ := catalogFromTemplates(templates)

	flash := vdashboard.AutomationFlash{}
	switch k.Request.URL.Query().Get("flash") {
	case "", "created":
	case "missing":
		flash.Error = "A name, trigger and template are required."
	case "badschedule":
		flash.Error = "The send rule is invalid — check the time, timezone, days or delay."
	case "duplicate":
		flash.Error = "An automation for this trigger already exists."
	case "notemplate":
		flash.Error = "Create and get an approved template first."
	case "notfound":
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
	case "error", "internal":
		flash.Error = "Something went wrong — try again."
	}

	page := a.dashboardPage(k, "Automation", "automations", active, all)
	return k.Render(vdashboard.AutomationFormPage(page, automation, approved,
		automations.DeliveryStatuses(), automation != nil, flash))
}

// scheduleFromForm validates the send-rule fields into a Schedule.
func scheduleFromForm(k *kit.Kit) (*automations.Schedule, error) {
	if err := k.Request.ParseForm(); err != nil {
		return nil, err
	}
	delayMinutes := 0
	if v := strings.TrimSpace(k.Request.FormValue("send_delay_minutes")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, errors.New("invalid delay")
		}
		delayMinutes = n
	}
	return automations.ParseSchedule(
		k.Request.FormValue("send_mode"),
		strings.TrimSpace(k.Request.FormValue("send_time")),
		strings.TrimSpace(k.Request.FormValue("send_timezone")),
		k.Request.Form["send_days"],
		delayMinutes,
	)
}

// resolveShopID validates the form's shop_id against the operator's integrated
// shops and returns the target shop id (falling back to the active shop).
func (a *App) resolveShopID(k *kit.Kit, fallback uuid.UUID) (uuid.UUID, error) {
	raw := strings.TrimSpace(k.Request.FormValue("shop_id"))
	if raw == "" {
		return fallback, nil
	}
	shopID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errors.New("invalid shop")
	}
	integrated, err := a.Shops.ListIntegrated(k.Request.Context())
	if err != nil {
		return uuid.Nil, err
	}
	for _, s := range integrated {
		if s.ID == shopID {
			return shopID, nil
		}
	}
	return uuid.Nil, errors.New("shop not integrated")
}

// shopHasTemplate reports whether the shop owns a template with the given id.
func (a *App) shopHasTemplate(ctx context.Context, shopID uuid.UUID, templateID uuid.UUID) (bool, error) {
	templates, err := a.WhatsApp.Templates(ctx, shopID)
	if err != nil {
		return false, err
	}
	for _, t := range templates {
		if t.ID == templateID {
			return true, nil
		}
	}
	return false, nil
}

// handleAutomationCreate records a new event → template mapping for the chosen
// shop (defaulting to the active one).
func (a *App) handleAutomationCreate(k *kit.Kit) error {
	active, err := a.requireShop(k)
	if err != nil {
		return err
	}

	ctx := k.Request.Context()
	if err := k.Request.ParseForm(); err != nil {
		return err
	}
	name := strings.TrimSpace(k.Request.FormValue("name"))
	description := strings.TrimSpace(k.Request.FormValue("description"))
	source := k.Request.FormValue("event_source")
	status := k.Request.FormValue("event_status")
	templateID, pErr := uuid.Parse(k.Request.FormValue("template_id"))
	if name == "" || (source != automations.SourceConverty && source != automations.SourceDelivery) ||
		status == "" || pErr != nil {
		return k.Redirect(http.StatusSeeOther, "/automations/new?flash=missing")
	}

	shopID, sErr := a.resolveShopID(k, active.ID)
	if sErr != nil {
		a.Log.Warn("automation create rejected (shop)", "shop_id", active.ID, "error", sErr)
		return k.Redirect(http.StatusSeeOther, "/automations/new?flash=error")
	}

	owns, tErr := a.shopHasTemplate(ctx, shopID, templateID)
	if tErr != nil || !owns {
		a.Log.Warn("automation create rejected (template)", "shop_id", shopID, "template_id", templateID, "error", tErr)
		return k.Redirect(http.StatusSeeOther, "/automations/new?flash=notemplate")
	}

	schedule, sErr2 := scheduleFromForm(k)
	if sErr2 != nil {
		a.Log.Warn("automation create rejected", "shop_id", shopID, "error", sErr2)
		return k.Redirect(http.StatusSeeOther, "/automations/new?flash=badschedule")
	}

	if err := a.Automations.Create(ctx, shopID, source, status, templateID, schedule, name, description); err != nil {
		if errors.Is(err, automations.ErrDuplicate) {
			return k.Redirect(http.StatusSeeOther, "/automations/new?flash=duplicate")
		}
		a.Log.Error("automation create failed", "shop_id", shopID, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations/new?flash=error")
	}

	setActiveShopCookie(k, shopID)
	a.Log.Info("automation created", "shop_id", shopID, "source", source, "status", status)
	return k.Redirect(http.StatusSeeOther, "/automations?flash=created")
}

// handleAutomationUpdate saves edits to an existing automation, keeping the
// automation's own shop regardless of the operator's active shop.
func (a *App) handleAutomationUpdate(k *kit.Kit) error {
	_, err := a.requireShop(k)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
	}

	ctx := k.Request.Context()
	automation, err := a.Automations.AutomationByID(ctx, id)
	if err != nil {
		a.Log.Error("automation update lookup failed", "id", id, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations?flash=error")
	}
	if automation == nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
	}
	shopID := automation.ShopID

	if err := k.Request.ParseForm(); err != nil {
		return err
	}
	name := strings.TrimSpace(k.Request.FormValue("name"))
	description := strings.TrimSpace(k.Request.FormValue("description"))
	source := k.Request.FormValue("event_source")
	status := k.Request.FormValue("event_status")
	templateID, pErr := uuid.Parse(k.Request.FormValue("template_id"))
	if name == "" || (source != automations.SourceConverty && source != automations.SourceDelivery) ||
		status == "" || pErr != nil {
		return k.Redirect(http.StatusSeeOther, "/automations/"+id.String()+"/edit?flash=missing")
	}

	owns, tErr := a.shopHasTemplate(ctx, shopID, templateID)
	if tErr != nil || !owns {
		a.Log.Warn("automation update rejected (template)", "shop_id", shopID, "template_id", templateID, "error", tErr)
		return k.Redirect(http.StatusSeeOther, "/automations/"+id.String()+"/edit?flash=notemplate")
	}

	schedule, sErr := scheduleFromForm(k)
	if sErr != nil {
		a.Log.Warn("automation update rejected", "shop_id", shopID, "error", sErr)
		return k.Redirect(http.StatusSeeOther, "/automations/"+id.String()+"/edit?flash=badschedule")
	}

	if err := a.Automations.Update(ctx, shopID, id, source, status, templateID, schedule, name, description); err != nil {
		if errors.Is(err, automations.ErrDuplicate) {
			return k.Redirect(http.StatusSeeOther, "/automations/"+id.String()+"/edit?flash=duplicate")
		}
		a.Log.Error("automation update failed", "shop_id", shopID, "id", id, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations/"+id.String()+"/edit?flash=error")
	}

	setActiveShopCookie(k, shopID)
	a.Log.Info("automation updated", "shop_id", shopID, "id", id)
	return k.Redirect(http.StatusSeeOther, "/automations?flash=updated")
}

// handleAutomationToggle enables or disables an automation.
func (a *App) handleAutomationToggle(k *kit.Kit) error {
	_, err := a.requireShop(k)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
	}

	ctx := k.Request.Context()
	automation, err := a.Automations.AutomationByID(ctx, id)
	if err != nil {
		a.Log.Error("automation toggle lookup failed", "id", id, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations?flash=error")
	}
	if automation == nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
	}

	enabled := k.Request.FormValue("enabled") == "1"
	if err := a.Automations.SetEnabled(ctx, automation.ShopID, id, enabled); err != nil {
		a.Log.Error("automation toggle failed", "shop_id", automation.ShopID, "id", id, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations?flash=error")
	}
	return k.Redirect(http.StatusSeeOther, "/automations?flash=toggled")
}

// handleAutomationDelete removes an automation.
func (a *App) handleAutomationDelete(k *kit.Kit) error {
	_, err := a.requireShop(k)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
	}

	ctx := k.Request.Context()
	automation, err := a.Automations.AutomationByID(ctx, id)
	if err != nil {
		a.Log.Error("automation delete lookup failed", "id", id, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations?flash=error")
	}
	if automation == nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
	}

	if err := a.Automations.Delete(ctx, automation.ShopID, id); err != nil {
		a.Log.Error("automation delete failed", "shop_id", automation.ShopID, "id", id, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations?flash=error")
	}
	return k.Redirect(http.StatusSeeOther, "/automations?flash=deleted")
}

// handleShopTemplates returns the approved templates of a shop as JSON so the
// automation form can reload the message dropdown when the shop changes.
func (a *App) handleShopTemplates(k *kit.Kit) error {
	if _, err := a.requireShop(k); err != nil {
		return err
	}
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		http.Error(k.Response, "invalid shop", http.StatusBadRequest)
		return nil
	}

	ctx := k.Request.Context()
	integrated, err := a.Shops.ListIntegrated(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, s := range integrated {
		if s.ID == id {
			found = true
			break
		}
	}

	type item struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Language string `json:"language"`
		Body     string `json:"body"`
	}
	items := []item{}
	if found {
		templates, err := a.WhatsApp.Templates(ctx, id)
		if err != nil {
			return err
		}
		for _, t := range templates {
			if t.ApprovalStatus == "approved" && !t.MarketingFlagged {
				items = append(items, item{t.ID.String(), t.Name, t.Language, whatsapp.TemplateBody(t)})
			}
		}
	}

	k.Response.Header().Set("Content-Type", "application/json; charset=utf-8")
	return k.JSON(http.StatusOK, map[string]any{"shop_id": id, "templates": items})
}

// catalogFromTemplates splits a shop's templates into the approved sendable
// list and id → name / id → body maps used by the cards and preview.
func catalogFromTemplates(templates []whatsapp.MerchantTemplate) (approved []whatsapp.MerchantTemplate, names map[uuid.UUID]string, bodies map[uuid.UUID]string) {
	names = make(map[uuid.UUID]string, len(templates))
	bodies = make(map[uuid.UUID]string, len(templates))
	approved = make([]whatsapp.MerchantTemplate, 0, len(templates))
	for _, t := range templates {
		names[t.ID] = t.Name
		bodies[t.ID] = whatsapp.TemplateBody(t)
		if t.ApprovalStatus == "approved" && !t.MarketingFlagged {
			approved = append(approved, t)
		}
	}
	return approved, names, bodies
}
