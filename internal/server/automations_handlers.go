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
	"whatsappconverty/internal/shops"
	"whatsappconverty/internal/whatsapp"
	vdashboard "whatsappconverty/web/views/dashboard"
)

// handleAutomations renders the automation page: every configured trigger shown
// as a card (naming, trigger, template, delivery rule), plus the create button.
func (a *App) handleAutomations(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}

	ctx := k.Request.Context()
	list, err := a.Automations.ListAll(ctx)
	if err != nil {
		return err
	}

	// Templates belong to the client, so an automation may reference a
	// template synced under another shop; names/bodies come from the full
	// catalog.
	templates, err := a.WhatsApp.TemplatesAll(ctx)
	if err != nil {
		return err
	}
	approved, names, bodies := catalogFromTemplates(templates)
	shopNames := shopNameMap(all)

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
	case "tested":
		flash.Info = "Test message sent. Check the number in WhatsApp."
	case "testfailed":
		flash.Error = "Test message failed — the sender must be healthy: use a valid E.164 number that is in the WhatsApp test phone list or has an open 24h conversation."
	case "testnophone":
		flash.Error = "Enter a phone number to test the message."
	case "toggled":
		flash.Info = "Automation updated."
	case "deleted":
		flash.Info = "Automation removed."
	case "notfound":
		flash.Error = "That automation no longer exists."
	case "error", "internal":
		flash.Error = "Something went wrong — try again."
	}

	page := a.dashboardPage(k, "Automations", "automations", all)
	return k.Render(vdashboard.AutomationsPage(page, list, approved, names, bodies, shopNames,
		automations.DeliveryStatuses(), flash))
}

// handleAutomationEdit renders the create form when no id is given, or the
// prefilled edit form for one automation. The form lets the operator choose the
// target shop, so the edited automation resolves to its own shop, not the
// operator's active one.
func (a *App) handleAutomationEdit(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}

	ctx := k.Request.Context()
	var automation *automations.Automation
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
	}

	templates, err := a.WhatsApp.TemplatesAll(ctx)
	if err != nil {
		return err
	}
	approved, _, _ := catalogFromTemplates(templates)
	templateShops := shopNameMap(all)

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

	page := a.dashboardPage(k, "Automation", "automations", all)
	return k.Render(vdashboard.AutomationFormPage(page, automation, approved, templateShops,
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
// shops and returns the target shop id (falling back to the first shop when no
// selection was made — the form's dropdown preselects it).
func (a *App) resolveShopID(k *kit.Kit) (uuid.UUID, error) {
	integrated, err := a.Shops.ListIntegrated(k.Request.Context())
	if err != nil {
		return uuid.Nil, err
	}
	if len(integrated) == 0 {
		return uuid.Nil, errors.New("no shop integrated")
	}
	raw := strings.TrimSpace(k.Request.FormValue("shop_id"))
	if raw == "" {
		return integrated[0].ID, nil
	}
	shopID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errors.New("invalid shop")
	}
	for _, s := range integrated {
		if s.ID == shopID {
			return shopID, nil
		}
	}
	return uuid.Nil, errors.New("shop not integrated")
}

// shopHasTemplate reports whether a template with the given id exists anywhere
// in the client's catalog (templates are shared across shops).
func (a *App) shopHasTemplate(ctx context.Context, templateID uuid.UUID) (bool, error) {
	templates, err := a.WhatsApp.TemplatesAll(ctx)
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

// shopNameMap maps shop ids to display names for the shared template dropdown.
func shopNameMap(shops []shops.Shop) map[uuid.UUID]string {
	out := make(map[uuid.UUID]string, len(shops))
	for _, s := range shops {
		out[s.ID] = s.Name
	}
	return out
}

// handleAutomationCreate records a new event → template mapping for the chosen
// shop (the form's Shop dropdown).
func (a *App) handleAutomationCreate(k *kit.Kit) error {
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

	shopID, sErr := a.resolveShopID(k)
	if sErr != nil {
		a.Log.Warn("automation create rejected (shop)", "error", sErr)
		return k.Redirect(http.StatusSeeOther, "/automations/new?flash=error")
	}

	owns, tErr := a.shopHasTemplate(ctx, templateID)
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

	a.Log.Info("automation created", "shop_id", shopID, "source", source, "status", status)
	return k.Redirect(http.StatusSeeOther, "/automations?flash=created")
}

// handleAutomationUpdate saves edits to an existing automation, keeping the
// automation's own shop.
func (a *App) handleAutomationUpdate(k *kit.Kit) error {
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

	owns, tErr := a.shopHasTemplate(ctx, templateID)
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

	a.Log.Info("automation updated", "shop_id", shopID, "id", id)
	return k.Redirect(http.StatusSeeOther, "/automations?flash=updated")
}

// handleAutomationToggle enables or disables an automation.
func (a *App) handleAutomationToggle(k *kit.Kit) error {
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

// handleAutomationTest sends the automation's template to a number the user
// fills in, so the message can be previewed live before it fires on real
// events. Sample variables stand in for the real order data.
func (a *App) handleAutomationTest(k *kit.Kit) error {
	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
	}

	ctx := k.Request.Context()
	automation, err := a.Automations.AutomationByID(ctx, id)
	if err != nil {
		a.Log.Error("automation test lookup failed", "id", id, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations?flash=error")
	}
	if automation == nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notfound")
	}
	if automation.TemplateID == uuid.Nil {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notemplate")
	}

	to := strings.TrimSpace(k.Request.FormValue("phone"))
	if to == "" {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=testnophone")
	}

	templates, err := a.WhatsApp.TemplatesAll(ctx)
	if err != nil {
		return err
	}
	var numVariables int
	found := false
	for _, t := range templates {
		if t.ID == automation.TemplateID {
			numVariables = t.NumVariables
			found = true
			break
		}
	}
	if !found {
		return k.Redirect(http.StatusSeeOther, "/automations?flash=notemplate")
	}

	if _, err := a.WhatsApp.SendTemplateTest(ctx, whatsapp.TestTemplateRequest{
		ShopID:     automation.ShopID,
		TemplateID: automation.TemplateID,
		To:         to,
		Variables:  testVariables(numVariables),
	}); err != nil {
		var rej *whatsapp.SendRejection
		if errors.As(err, &rej) {
			a.Log.Warn("automation test send rejected",
				"automation_id", id, "code", rej.Code, "reason", rej.Reason)
			return k.Redirect(http.StatusSeeOther, "/automations?flash=testfailed")
		}
		a.Log.Error("automation test send failed", "automation_id", id, "error", err)
		return k.Redirect(http.StatusSeeOther, "/automations?flash=error")
	}

	a.Log.Info("automation test message sent", "automation_id", id, "to", to)
	return k.Redirect(http.StatusSeeOther, "/automations?flash=tested")
}

// testVariables fills every template slot positionally with sample values so
// the send gate's variable-count check passes; unused slots get an em dash.
func testVariables(n int) map[string]string {
	vocab := []string{"Hamed", "CVY-TEST", "Sample status"}
	vars := make(map[string]string, n)
	for i := 0; i < n; i++ {
		v := "—"
		if i < len(vocab) && vocab[i] != "" {
			v = vocab[i]
		}
		vars[strconv.Itoa(i+1)] = v
	}
	return vars
}

// handleAutomationDelete removes an automation.
func (a *App) handleAutomationDelete(k *kit.Kit) error {
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

// handleShopTemplates returns the approved templates the operator can attach
// to an automation of one shop. Templates belong to the client and are shared
// across shops, so the full catalog is returned with each template's shop
// name, letting the form rebuild the dropdown when the shop changes.
func (a *App) handleShopTemplates(k *kit.Kit) error {
	if _, err := a.shopsFor(k); err != nil {
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
	shopNames := shopNameMap(integrated)

	type item struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Language string `json:"language"`
		Body     string `json:"body"`
		Shop     string `json:"shop"`
	}
	items := []item{}
	templates, err := a.WhatsApp.TemplatesAll(ctx)
	if err != nil {
		return err
	}
	for _, t := range templates {
		if t.ApprovalStatus == "approved" && !t.MarketingFlagged {
			items = append(items, item{
				ID: t.ID.String(), Name: t.Name, Language: t.Language,
				Body: whatsapp.TemplateBody(t), Shop: shopNames[t.ShopID],
			})
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
