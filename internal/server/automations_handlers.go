package server

import (
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
// prefilled edit form for one automation.
func (a *App) handleAutomationEdit(k *kit.Kit) error {
	active, all, err := a.activeShops(k)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return k.Redirect(http.StatusSeeOther, "/integrations")
	}

	ctx := k.Request.Context()
	templates, err := a.WhatsApp.Templates(ctx, active.ID)
	if err != nil {
		return err
	}
	approved, names, bodies := catalogFromTemplates(templates)

	var automation *automations.Automation
	if idParam := chi.URLParam(k.Request, "id"); idParam != "" {
		id, pErr := uuid.Parse(idParam)
		if pErr != nil {
			return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
		}
		automation, err = a.Automations.Automation(ctx, active.ID, id)
		if err != nil {
			a.Log.Error("automation edit lookup failed", "shop_id", active.ID, "id", id, "error", err)
			return err
		}
		if automation == nil {
			return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
		}
	}

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
	return k.Render(vdashboard.AutomationFormPage(page, automation, approved, names, bodies,
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

// handleAutomationCreate records a new event → template mapping.
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

	schedule, sErr := scheduleFromForm(k)
	if sErr != nil {
		a.Log.Warn("automation create rejected", "shop_id", active.ID, "error", sErr)
		return k.Redirect(http.StatusSeeOther, "/automations/new?flash=badschedule")
	}

	if err := a.Automations.Create(ctx, active.ID, source, status, templateID, schedule, name, description); err != nil {
		if errors.Is(err, automations.ErrDuplicate) {
			return k.Redirect(http.StatusSeeOther, "/automations/new?flash=duplicate")
		}
		a.Log.Error("automation create failed", "shop_id", active.ID, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations/new?flash=error")
	}

	a.Log.Info("automation created", "shop_id", active.ID, "source", source, "status", status)
	return k.Redirect(http.StatusSeeOther, "/automations?flash=created")
}

// handleAutomationUpdate saves edits to an existing automation.
func (a *App) handleAutomationUpdate(k *kit.Kit) error {
	active, err := a.requireShop(k)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
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
		return k.Redirect(http.StatusSeeOther, "/automations/"+id.String()+"/edit?flash=missing")
	}

	schedule, sErr := scheduleFromForm(k)
	if sErr != nil {
		a.Log.Warn("automation update rejected", "shop_id", active.ID, "error", sErr)
		return k.Redirect(http.StatusSeeOther, "/automations/"+id.String()+"/edit?flash=badschedule")
	}

	if err := a.Automations.Update(ctx, active.ID, id, source, status, templateID, schedule, name, description); err != nil {
		if errors.Is(err, automations.ErrDuplicate) {
			return k.Redirect(http.StatusSeeOther, "/automations/"+id.String()+"/edit?flash=duplicate")
		}
		a.Log.Error("automation update failed", "shop_id", active.ID, "id", id, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations/"+id.String()+"/edit?flash=error")
	}

	a.Log.Info("automation updated", "shop_id", active.ID, "id", id)
	return k.Redirect(http.StatusSeeOther, "/automations?flash=updated")
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
