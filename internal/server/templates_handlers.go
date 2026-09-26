package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/anthdm/superkit/kit"

	"whatsappconverty/internal/whatsapp"
)

// handleTemplateRefresh re-syncs the merchant's template approval statuses
// from Meta's review engine and returns to the templates page.
func (a *App) handleTemplateRefresh(k *kit.Kit) error {
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID

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
	all, err := a.shopsFor(k)
	if err != nil {
		return err
	}
	shopID := primaryShop(all).ID

	name := strings.TrimSpace(k.Request.FormValue("name"))
	language := strings.TrimSpace(k.Request.FormValue("language"))
	body := strings.TrimSpace(k.Request.FormValue("body"))
	if name == "" || language == "" || body == "" {
		return k.Redirect(http.StatusSeeOther, "/templates?flash=missing")
	}

	numbers := whatsapp.PlaceholderNumbers(body)
	if len(numbers) == 0 {
		return k.Redirect(http.StatusSeeOther, "/templates?flash=novariables")
	}

	examples := make([]string, 0, len(numbers))
	for _, n := range numbers {
		v := strings.TrimSpace(k.Request.FormValue("example_" + strconv.Itoa(n)))
		if v == "" {
			return k.Redirect(http.StatusSeeOther, "/templates?flash=example")
		}
		examples = append(examples, v)
	}

	components, err := json.Marshal([]map[string]any{
		{
			"type": "BODY",
			"text": body,
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
	})
	if err != nil {
		var rej *whatsapp.SendRejection
		if errors.As(err, &rej) {
			// Meta refused the submission at submit time — no review wait. The
			// row is stored rejected with the reason; surface that reason now.
			a.Log.Warn("template rejected by meta at submit",
				"shop_id", shopID, "name", name, "language", language, "reason", rej.Reason)
			return k.Redirect(http.StatusSeeOther, "/templates?flash=rejected&reason="+url.QueryEscape(rej.Reason))
		}
		a.Log.Error("template create internal failure", "error", err.Error())
		return k.Redirect(http.StatusSeeOther, "/templates?flash=internal")
	}

	if tmpl.ApprovalStatus == "rejected" {
		a.Log.Warn("template rejected by meta",
			"shop_id", shopID, "name", tmpl.Name, "language", tmpl.Language, "reason", tmpl.RejectionReason)
		return k.Redirect(http.StatusSeeOther, "/templates?flash=rejected&reason="+url.QueryEscape(tmpl.RejectionReason))
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
