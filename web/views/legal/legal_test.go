package viewslegal_test

import (
	"whatsappconverty/internal/i18n"
	"context"
	"strings"
	"testing"

	legal "whatsappconverty/web/views/legal"
)

func TestPrivacyPagePublishesContactAndDeletionLink(t *testing.T) {
	ctx := context.Background()

	var sb strings.Builder
	if err := legal.Privacy("support@example.com", i18n.New(i18n.En)).Render(ctx, &sb); err != nil {
		t.Fatalf("render privacy: %v", err)
	}
	html := sb.String()

	for _, want := range []string{
		`href="mailto:support@example.com"`, // a reachable contact, or Meta rejects it
		"support@example.com",
		`href="/data-deletion"`, // link to the deletion instructions
		`href="/privacy"`,
		"Meta Platforms, Inc.", // sub-processors are named
		"Converty",
		"Mes Colis Express",
		"Privacy Policy",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("privacy page missing %q", want)
		}
	}
	// The page previously shipped a literal "update this before going live"
	// placeholder, which is exactly what a reviewer flags.
	if strings.Contains(html, "Update this section") {
		t.Error("privacy page still contains the pre-launch placeholder")
	}
	// A mailto with no address is worse than none at all.
	if strings.Contains(html, `href="mailto:"`) {
		t.Error("privacy page renders an empty mailto link")
	}
}

func TestDataDeletionPagePublishesContactAndInventory(t *testing.T) {
	ctx := context.Background()

	var sb strings.Builder
	if err := legal.DataDeletion("support@example.com", i18n.New(i18n.En)).Render(ctx, &sb); err != nil {
		t.Fatalf("render data deletion: %v", err)
	}
	html := sb.String()

	for _, want := range []string{
		`href="mailto:support@example.com"`,
		"Data Deletion Instructions",
		"Account data",               // what is held
		"Message records",            //
		"Order and delivery records", //
		"What we delete",             // what is actioned
		"What we keep, and why",      // what is retained
		`href="/privacy"`,            // back to the policy
		"stop all sending",           // deletion implies opt-out
	} {
		if !strings.Contains(html, want) {
			t.Errorf("data deletion page missing %q", want)
		}
	}
	if strings.Contains(html, `href="mailto:"`) {
		t.Error("data deletion page renders an empty mailto link")
	}
}

// An unset SUPPORT_EMAIL must still render a coherent page rather than an
// empty mailto or a placeholder address.
func TestLegalPagesWithoutSupportEmail(t *testing.T) {
	ctx := context.Background()

	var privacy strings.Builder
	if err := legal.Privacy("", i18n.New(i18n.En)).Render(ctx, &privacy); err != nil {
		t.Fatalf("render privacy: %v", err)
	}
	if strings.Contains(privacy.String(), "mailto:") {
		t.Error("privacy page renders a mailto link with no address configured")
	}

	var deletion strings.Builder
	if err := legal.DataDeletion("", i18n.New(i18n.En)).Render(ctx, &deletion); err != nil {
		t.Fatalf("render data deletion: %v", err)
	}
	if strings.Contains(deletion.String(), "mailto:") {
		t.Error("data deletion page renders a mailto link with no address configured")
	}
	if !strings.Contains(deletion.String(), "our support team") {
		t.Error("data deletion page should fall back to plain support wording")
	}
}
