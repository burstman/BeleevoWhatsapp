package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"whatsappconverty/internal/whatsapp"
)

// handleTemplateDelete removes a template the operator no longer wants. The
// template is soft-deleted (lifecycle state "deleted") after a best-effort
// delete on the messaging account that owns it, so it can never be sent again.
// A template still referenced by an automation is refused: the operator must
// detach it first, so no automation is silently broken.
func (a *App) handleTemplateDelete(k *kit.Kit) error {
	if _, err := a.shopsFor(k); err != nil {
		return err
	}

	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/templates?flash=notfound")
	}

	tpl, err := a.WhatsApp.TemplateByID(k.Request.Context(), id)
	if err != nil {
		a.Log.Error("template delete lookup failed", "template_id", id, "error", err.Error())
		return k.Redirect(http.StatusSeeOther, "/templates?flash=error")
	}
	if tpl.ID == uuid.Nil || tpl.ApprovalStatus == "deleted" {
		return k.Redirect(http.StatusSeeOther, "/templates?flash=notfound")
	}

	users, err := a.Automations.ListAll(k.Request.Context())
	if err != nil {
		a.Log.Error("template delete: automation lookup failed", "error", err.Error())
		return k.Redirect(http.StatusSeeOther, "/templates?flash=error")
	}
	var attached []string
	for _, au := range users {
		if au.TemplateID == tpl.ID {
			attached = append(attached, au.Name)
		}
	}
	if len(attached) > 0 {
		return k.Redirect(http.StatusSeeOther,
			"/templates?flash=inuse&names="+url.QueryEscape(strings.Join(attached, ", ")))
	}

	if err := a.WhatsApp.DeleteByMerchant(k.Request.Context(), tpl); err != nil {
		a.Log.Warn("template delete failed", "template_id", tpl.ID, "name", tpl.Name, "error", err)
		return k.Redirect(http.StatusSeeOther, "/templates?flash=error")
	}

	a.Log.Info("template deleted by operator", "template_id", tpl.ID, "name", tpl.Name, "language", tpl.Language)
	return k.Redirect(http.StatusSeeOther, "/templates?flash=deleted")
}

// handleTemplateRefresh re-syncs the merchant's template approval statuses
// from Meta's review engine and returns to the templates page.
func (a *App) handleTemplateRefresh(k *kit.Kit) error {
	if _, err := a.shopsFor(k); err != nil {
		return err
	}
	owner, err := a.sharedOwnerShop(k.Request.Context())
	if err != nil {
		return err
	}
	shopID := owner.ID

	if purged, err := a.WhatsApp.PurgeExpiredMarketing(k.Request.Context()); err != nil {
		a.Log.Warn("marketing purge check failed", "error", err.Error())
	} else if purged > 0 {
		a.Log.Info("expired marketing templates purged", "count", purged)
	}

	updated, err := a.WhatsApp.SyncTemplates(k.Request.Context(), shopID)
	if err != nil {
		a.Log.Warn("template refresh failed", "error", err.Error())
		return k.Redirect(http.StatusSeeOther, "/templates?flash=refresh")
	}
	a.Log.Info("templates refreshed from meta", "updated", updated)
	return k.Redirect(http.StatusSeeOther, "/templates?flash=synced")
}

// handleTemplateCreate submits a merchant-authored template to Meta's review
// through the platform's central account. Only UTILITY-category templates are
// accepted here; marketing templates are outside this platform's scope. The
// stored status (pending/rejected/approved) is always Meta's verdict.
func (a *App) handleTemplateCreate(k *kit.Kit) error {
	if _, err := a.shopsFor(k); err != nil {
		return err
	}
	owner, err := a.sharedOwnerShop(k.Request.Context())
	if err != nil {
		return err
	}
	shopID := owner.ID

	name := strings.TrimSpace(k.Request.FormValue("name"))
	language := strings.TrimSpace(k.Request.FormValue("language"))
	body := strings.TrimSpace(k.Request.FormValue("body"))
	source := whatsapp.NormalizeSource(strings.TrimSpace(k.Request.FormValue("source")))
	if name == "" || language == "" || body == "" {
		return k.Redirect(http.StatusSeeOther, "/templates?flash=missing")
	}

	positional, tokens := whatsapp.TokenizeTemplateBody(body)
	if utf8.RuneCountInString(positional) > whatsapp.MaxTemplateBodyChars {
		return k.Redirect(http.StatusSeeOther, "/templates?flash=toolong")
	}
	if err := whatsapp.ValidateTemplateBody(positional); err != nil {
		return k.Redirect(http.StatusSeeOther, "/templates?flash=invalidbody&msg="+url.QueryEscape(err.Error()))
	}

	numbers := whatsapp.PlaceholderNumbers(positional)
	if len(numbers) == 0 {
		return k.Redirect(http.StatusSeeOther, "/templates?flash=novariables")
	}

	// Examples arrive keyed by placeholder. Semantic bodies ({{token}} chips)
	// are keyed by token; legacy {{N}} bodies keep the numeric keys.
	var variables []whatsapp.TokenKey
	examples := make([]string, 0, len(numbers))
	if len(tokens) > 0 {
		variables = tokens
		for _, t := range tokens {
			v := strings.TrimSpace(k.Request.FormValue("example_" + string(t)))
			if v == "" {
				return k.Redirect(http.StatusSeeOther, "/templates?flash=example")
			}
			examples = append(examples, v)
		}
	} else {
		for _, n := range numbers {
			v := strings.TrimSpace(k.Request.FormValue("example_" + strconv.Itoa(n)))
			if v == "" {
				return k.Redirect(http.StatusSeeOther, "/templates?flash=example")
			}
			examples = append(examples, v)
		}
	}

	// A Converty event carries no deliveryman, so a driver variable there would
	// reach the customer as an empty slot. Refuse it at creation instead.
	for _, t := range tokens {
		if !whatsapp.TokenAllowed(source, t) {
			return k.Redirect(http.StatusSeeOther, "/templates?flash=badsource")
		}
	}

	components, err := json.Marshal([]map[string]any{
		{
			"type": "BODY",
			"text": positional,
			"example": map[string]any{
				"body_text": [][]string{examples},
			},
		},
	})
	if err != nil {
		return err
	}

	tmpl, err := a.WhatsApp.CreateTemplate(k.Request.Context(), shopID, whatsapp.TemplateDraft{
		Name:       name,
		Language:   language,
		Category:   "UTILITY",
		Components: components,
		Variables:  variables,
		Source:     source,
	})
	if err != nil {
		var rej *whatsapp.SendRejection
		if errors.As(err, &rej) {
			// Meta refused the submission at submit time — no review wait. The
			// row is stored rejected with the reason; surface that reason now.
			a.Log.Warn("template rejected by meta at submit",
				"shop_id", shopID, "name", name, "language", language, "reason", rej.Reason)
			return k.Redirect(http.StatusSeeOther, "/templates?flash=rejected&reason="+url.QueryEscape(explainRejection(err, rej.Reason)))
		}
		a.Log.Error("template create internal failure", "error", err.Error())
		return k.Redirect(http.StatusSeeOther, "/templates?flash=internal")
	}

	if tmpl.ApprovalStatus == "rejected" {
		a.Log.Warn("template rejected by meta",
			"shop_id", shopID, "name", tmpl.Name, "language", tmpl.Language, "reason", tmpl.RejectionReason)
		return k.Redirect(http.StatusSeeOther, "/templates?flash=rejected&reason="+url.QueryEscape(explainRejection(err, tmpl.RejectionReason)))
	}
	if tmpl.MarketingFlagged {
		a.Log.Warn("template flagged as marketing by meta",
			"shop_id", shopID, "name", tmpl.Name, "warning", tmpl.MetaWarnings)
		return k.Redirect(http.StatusSeeOther, "/templates?flash=marketing")
	}
	a.Log.Info("template submitted for review",
		"shop_id", shopID, "name", tmpl.Name, "language", tmpl.Language)
	return k.Redirect(http.StatusSeeOther, "/templates?flash=created")
}

// explainRejection appends Meta's subcode translation to a stored reason so the
// operator sees what to change instead of "Invalid parameter (subcode 2388299)".
func explainRejection(err error, reason string) string {
	hint := whatsapp.ExplainTemplateRejection(err)
	if hint == "" || reason == "" {
		return reason
	}
	return reason + " — " + hint
}
