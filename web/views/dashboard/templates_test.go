package viewsdashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/whatsapp"
	"whatsappconverty/web/views/components"
)

func renderTemplates(t *testing.T, tpl whatsapp.MerchantTemplate, grace time.Duration) string {
	t.Helper()
	now := time.Now()
	tpl.NegativeAt = &now
	var sb strings.Builder
	page := components.Page{Title: "Templates", Active: "templates"}
	if err := TemplatesPage(page, []whatsapp.MerchantTemplate{tpl}, nil, TemplateFlash{}, grace).Render(t.Context(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

func baseTemplate(status string) whatsapp.MerchantTemplate {
	return whatsapp.MerchantTemplate{
		ID:             uuid.New(),
		Name:           "order_confirmed",
		Language:       "ar",
		ApprovalStatus: status,
		Components:     []byte(`[{"type":"BODY","text":"Bonjour {{1}}"}]`),
		NumVariables:   1,
	}
}

func TestTemplatesPageShowsEditButtonForRejectedWithinGrace(t *testing.T) {
	html := renderTemplates(t, baseTemplate("rejected"), 15*time.Minute)
	if !strings.Contains(html, "startTemplateEdit(this)") {
		t.Error("a rejected template inside its grace window must offer an Edit button")
	}
	if !strings.Contains(html, "data-edit=") {
		t.Error("the edit button must carry its template payload in data-edit")
	}
	// The delete action must remain available alongside edit.
	if !strings.Contains(html, "/delete") {
		t.Error("the delete form must still be rendered")
	}
}

func TestTemplatesPageHidesEditWhenGraceExpired(t *testing.T) {
	tpl := whatsapp.MerchantTemplate{
		ID:             uuid.New(),
		Name:           "old",
		Language:       "fr",
		ApprovalStatus: "rejected",
	}
	past := time.Now().Add(-time.Hour)
	tpl.NegativeAt = &past

	var sb strings.Builder
	page := components.Page{Title: "Templates", Active: "templates"}
	if err := TemplatesPage(page, []whatsapp.MerchantTemplate{tpl}, nil, TemplateFlash{}, 15*time.Minute).Render(t.Context(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(sb.String(), "startTemplateEdit(this)") {
		t.Fatal("an expired grace window must not offer an edit")
	}
}

func TestTemplatesPageNoEditForApproved(t *testing.T) {
	html := renderTemplates(t, whatsapp.MerchantTemplate{
		ID:             uuid.New(),
		Name:           "ok",
		Language:       "ar",
		ApprovalStatus: "approved",
	}, 15*time.Minute)
	if strings.Contains(html, "startTemplateEdit(this)") {
		t.Fatal("an approved template must not offer an edit")
	}
}
