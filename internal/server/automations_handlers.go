package server

import (
	"errors"
	"net/http"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"whatsappconverty/internal/automations"
	"whatsappconverty/internal/whatsapp"
	vdashboard "whatsappconverty/web/views/dashboard"
)

// handleAutomations renders the order-event automation page: every configured
// trigger (Converty order events and delivery provider events) mapped to an
// approved message template.
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
	approved := make([]whatsapp.MerchantTemplate, 0, len(templates))
	templateNames := make(map[uuid.UUID]string, len(templates))
	for _, t := range templates {
		templateNames[t.ID] = t.Name
		if t.ApprovalStatus == "approved" && !t.MarketingFlagged {
			approved = append(approved, t)
		}
	}

	flash := vdashboard.AutomationFlash{}
	switch k.Request.URL.Query().Get("flash") {
	case "":
	case "created":
		flash.Info = "Automation created. It fires on matching order or delivery events from now on."
	case "duplicate":
		flash.Error = "An automation for this trigger already exists — edit it instead."
	case "missing":
		flash.Error = "Event source, trigger event and template are required."
	case "badtime":
		flash.Error = "Invalid send time or timezone — use a HH:MM send time and a valid IANA timezone."
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
	return k.Render(vdashboard.AutomationsPage(page, list, approved, templateNames,
		automations.DeliveryStatuses(), flash))
}

// handleAutomationCreate records a new order/delivery event → template mapping.
func (a *App) handleAutomationCreate(k *kit.Kit) error {
	active, err := a.requireShop(k)
	if err != nil {
		return err
	}

	ctx := k.Request.Context()
	source := k.Request.FormValue("event_source")
	status := k.Request.FormValue("event_status")
	templateID, pErr := uuid.Parse(k.Request.FormValue("template_id"))
	if (source != automations.SourceConverty && source != automations.SourceDelivery) ||
		status == "" || pErr != nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=missing")
	}

	schedule, sErr := automations.ParseSchedule(
		k.Request.FormValue("send_mode"),
		k.Request.FormValue("send_time"),
		k.Request.FormValue("send_timezone"),
	)
	if sErr != nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=badtime")
	}

	if err := a.Automations.Create(ctx, active.ID, source, status, templateID, schedule); err != nil {
		if errors.Is(err, automations.ErrDuplicate) {
			return k.Redirect(http.StatusSeeOther, "/automations?flash=duplicate")
		}
		a.Log.Error("automation create failed", "shop_id", active.ID, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations?flash=error")
	}

	a.Log.Info("automation created", "shop_id", active.ID, "source", source, "status", status)
	return k.Redirect(http.StatusSeeOther, "/automations?flash=created")
}

// handleAutomationToggle enables or disables an automation.
func (a *App) handleAutomationToggle(k *kit.Kit) error {
	active, err := a.requireShop(k)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
	}

	enabled := k.Request.FormValue("enabled") == "1"
	if err := a.Automations.SetEnabled(k.Request.Context(), active.ID, id, enabled); err != nil {
		a.Log.Error("automation toggle failed", "shop_id", active.ID, "id", id, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations?flash=error")
	}
	return k.Redirect(http.StatusSeeOther, "/automations?flash=toggled")
}

// handleAutomationDelete removes an automation.
func (a *App) handleAutomationDelete(k *kit.Kit) error {
	active, err := a.requireShop(k)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
	}

	if err := a.Automations.Delete(k.Request.Context(), active.ID, id); err != nil {
		a.Log.Error("automation delete failed", "shop_id", active.ID, "id", id, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations?flash=error")
	}
	return k.Redirect(http.StatusSeeOther, "/automations?flash=deleted")
}