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
	if all, err := a.shopsFor(k); err != nil || len(all) == 0 {
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
	if all, err := a.shopsFor(k); err != nil || len(all) == 0 {
		return err
	}
	owner, err := a.sharedOwnerShop(k.Request.Context())
	if err != nil {
		return err
	}
	shopID := owner.ID

	if purged, err := a.WhatsApp.PurgeExpiredNegative(k.Request.Context()); err != nil {
		a.Log.Warn("negative purge check failed", "error", err.Error())
	} else if purged > 0 {
		a.Log.Info("expired negative templates purged", "count", purged)
	}

	updated, err := a.WhatsApp.SyncTemplates(k.Request.Context(), shopID)
	if err != nil {
		a.Log.Warn("template refresh failed", "error", err.Error())
		return k.Redirect(http.StatusSeeOther, "/templates?flash=refresh")
	}
	a.Log.Info("templates refreshed from meta", "updated", updated)
	return k.Redirect(http.StatusSeeOther, "/templates?flash=synced")
}

// parseTemplateForm reads the shared template editor fields (name, language,
// body, source and per-placeholder examples) and builds the positional
// components Meta expects. It returns a non-empty flash code when the input is
// not usable; flash=="internal" means an unexpected marshal failure. Identity
// fields may be empty here — create validates them, edit reads them from the
// stored row.
func parseTemplateForm(r *http.Request) (draft whatsapp.TemplateDraft, flash, msg string) {
	name := strings.TrimSpace(r.FormValue("name"))
	language := strings.TrimSpace(r.FormValue("language"))
	body := strings.TrimSpace(r.FormValue("body"))
	source := whatsapp.NormalizeSource(strings.TrimSpace(r.FormValue("source")))

	positional, tokens := whatsapp.TokenizeTemplateBody(body)
	if utf8.RuneCountInString(positional) > whatsapp.MaxTemplateBodyChars {
		return whatsapp.TemplateDraft{}, "toolong", ""
	}
	if err := whatsapp.ValidateTemplateBody(positional); err != nil {
		return whatsapp.TemplateDraft{}, "invalidbody", err.Error()
	}

	numbers := whatsapp.PlaceholderNumbers(positional)
	if len(numbers) == 0 {
		return whatsapp.TemplateDraft{}, "novariables", ""
	}

	// Examples arrive keyed by placeholder. Semantic bodies ({{token}} chips)
	// are keyed by token; legacy {{N}} bodies keep the numeric keys.
	var variables []whatsapp.TokenKey
	examples := make([]string, 0, len(numbers))
	if len(tokens) > 0 {
		variables = tokens
		for _, t := range tokens {
			v := strings.TrimSpace(r.FormValue("example_" + string(t)))
			if v == "" {
				return whatsapp.TemplateDraft{}, "example", ""
			}
			examples = append(examples, v)
		}
	} else {
		for _, n := range numbers {
			v := strings.TrimSpace(r.FormValue("example_" + strconv.Itoa(n)))
			if v == "" {
				return whatsapp.TemplateDraft{}, "example", ""
			}
			examples = append(examples, v)
		}
	}

	// A Converty event carries no deliveryman, so a driver variable there would
	// reach the customer as an empty slot. Refuse it at creation instead.
	for _, t := range tokens {
		if !whatsapp.TokenAllowed(source, t) {
			return whatsapp.TemplateDraft{}, "badsource", ""
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
		return whatsapp.TemplateDraft{}, "internal", ""
	}

	return whatsapp.TemplateDraft{
		Name:       name,
		Language:   language,
		Category:   "UTILITY",
		Components: components,
		Variables:  variables,
		Source:     source,
	}, "", ""
}

// handleTemplateCreate submits a merchant-authored template to Meta's review
// through the platform's central account. Only UTILITY-category templates are
// accepted here; marketing templates are outside this platform's scope. The
// stored status (pending/rejected/approved) is always Meta's verdict.
func (a *App) handleTemplateCreate(k *kit.Kit) error {
	if all, err := a.shopsFor(k); err != nil || len(all) == 0 {
		return err
	}
	owner, err := a.sharedOwnerShop(k.Request.Context())
	if err != nil {
		return err
	}
	shopID := owner.ID

	name := strings.TrimSpace(k.Request.FormValue("name"))
	language := strings.TrimSpace(k.Request.FormValue("language"))
	if name == "" || language == "" {
		return k.Redirect(http.StatusSeeOther, "/templates?flash=missing")
	}

	draft, flash, msg := parseTemplateForm(k.Request)
	if flash != "" {
		return redirectTemplateFlash(k, flash, msg)
	}

	tmpl, err := a.WhatsApp.CreateTemplate(k.Request.Context(), shopID, draft)
	if err != nil {
		var rej *whatsapp.SendRejection
		if errors.As(err, &rej) {
			// Meta refused the submission at submit time — no review wait. The
			// row is stored rejected with the reason; surface that reason now.
			a.Log.Warn("template rejected by meta at submit",
				"shop_id", shopID, "name", draft.Name, "language", draft.Language, "reason", rej.Reason)
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

// handleTemplateUpdate edits a negative template in place at Meta inside its
// grace window and resubmits it for review. Name, language and category are
// locked by Meta, so only the body and source change.
func (a *App) handleTemplateUpdate(k *kit.Kit) error {
	if all, err := a.shopsFor(k); err != nil || len(all) == 0 {
		return err
	}

	id, err := uuid.Parse(chi.URLParam(k.Request, "id"))
	if err != nil {
		return k.Redirect(http.StatusSeeOther, "/templates?flash=notfound")
	}

	draft, flash, msg := parseTemplateForm(k.Request)
	if flash != "" {
		return redirectTemplateFlash(k, flash, msg)
	}

	tmpl, err := a.WhatsApp.UpdateTemplate(k.Request.Context(), id, draft)
	if err != nil {
		if errors.Is(err, whatsapp.ErrTemplateNotEditable) {
			return k.Redirect(http.StatusSeeOther, "/templates?flash=editnotallowed")
		}
		var rej *whatsapp.SendRejection
		if errors.As(err, &rej) {
			a.Log.Warn("template edit rejected by meta", "template_id", id, "reason", rej.Reason)
			return k.Redirect(http.StatusSeeOther, "/templates?flash=editrejected&reason="+url.QueryEscape(explainRejection(err, rej.Reason)))
		}
		a.Log.Error("template update failed", "template_id", id, "error", err.Error())
		return k.Redirect(http.StatusSeeOther, "/templates?flash=internal")
	}

	a.Log.Info("template edited and resubmitted", "template_id", tmpl.ID, "name", tmpl.Name, "language", tmpl.Language)
	return k.Redirect(http.StatusSeeOther, "/templates?flash=edited")
}

// redirectTemplateFlash sends the operator back to the templates page with a
// flash code and, when present, a detail message.
func redirectTemplateFlash(k *kit.Kit, flash, msg string) error {
	target := "/templates?flash=" + flash
	if msg != "" {
		target += "&msg=" + url.QueryEscape(msg)
	}
	return k.Redirect(http.StatusSeeOther, target)
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
