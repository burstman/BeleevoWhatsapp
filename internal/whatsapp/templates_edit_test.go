package whatsapp

import (
	"fmt"
	"testing"
	"time"
)

func TestNegativeReview(t *testing.T) {
	cases := []struct {
		approval  string
		marketing bool
		want      bool
	}{
		{"pending", false, false},
		{"approved", false, false},
		{"approved", true, true}, // Meta approved the mechanics but flagged it marketing
		{"rejected", false, true},
		{"paused", false, true},
		{"deleted", false, false},
	}
	for _, tc := range cases {
		if got := NegativeReview(tc.approval, tc.marketing); got != tc.want {
			t.Errorf("NegativeReview(%q, %v) = %v, want %v", tc.approval, tc.marketing, got, tc.want)
		}
	}
}

func TestSemanticBodyForEditRoundTrip(t *testing.T) {
	semantic := "Bonjour {{customer_name}}, votre commande {{order_id}} est prete."
	positional, tokens := TokenizeTemplateBody(semantic)
	if positional == semantic || len(tokens) != 2 {
		t.Fatalf("tokenize failed: positional=%q tokens=%v", positional, tokens)
	}

	tpl := MerchantTemplate{
		Components: []byte(`[{"type":"BODY","text":` + fmt.Sprintf("%q", positional) + `,"example":{"body_text":[["Karima","ORD-1"]]}}]`),
		Variables:  tokens,
	}
	if got := SemanticBodyForEdit(tpl); got != semantic {
		t.Fatalf("SemanticBodyForEdit round-trip:\n got %q\nwant %q", got, semantic)
	}
}

func TestSemanticBodyForEditFallsBackToLegacyPositional(t *testing.T) {
	// A legacy template has no stored token map, so the default positional
	// mapping is used to make the placeholders editable again.
	tpl := MerchantTemplate{
		Components: []byte(`[{"type":"BODY","text":"Bonjour {{1}}, commande {{2}}."}]`),
	}
	want := "Bonjour {{customer_name}}, commande {{order_id}}."
	if got := SemanticBodyForEdit(tpl); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTemplateExamples(t *testing.T) {
	tpl := MerchantTemplate{
		Components: []byte(`[{"type":"BODY","text":"Hi {{1}} order {{2}}","example":{"body_text":[["Karima","ORD-123"]]}}]`),
		Variables:  []TokenKey{TokenCustomerName, TokenOrderID},
	}
	got := TemplateExamples(tpl)
	if got[string(TokenCustomerName)] != "Karima" || got[string(TokenOrderID)] != "ORD-123" {
		t.Fatalf("examples = %#v", got)
	}
}

func TestTemplateGraceRemaining(t *testing.T) {
	now := time.Now()
	grace := 15 * time.Minute

	// A negative template with no recorded episode treats the full window as
	// open (legacy rows).
	legacy := MerchantTemplate{ApprovalStatus: "rejected"}
	if remaining, ok := TemplateGraceRemaining(legacy, grace, now); !ok || remaining != grace {
		t.Fatalf("legacy negative row: remaining=%v ok=%v, want full grace", remaining, ok)
	}

	// A rejected template inside its window is editable with time remaining.
	fiveMinAgo := now.Add(-5 * time.Minute)
	inside := MerchantTemplate{ApprovalStatus: "rejected", NegativeAt: &fiveMinAgo}
	remaining, ok := TemplateGraceRemaining(inside, grace, now)
	if !ok || remaining <= 0 || remaining > 10*time.Minute+time.Second {
		t.Fatalf("inside window: remaining=%v ok=%v", remaining, ok)
	}

	// A marketing-flagged approved template counts as negative.
	flagged := MerchantTemplate{ApprovalStatus: "approved", MarketingFlagged: true, NegativeAt: &fiveMinAgo}
	if _, ok := TemplateGraceRemaining(flagged, grace, now); !ok {
		t.Fatal("marketing-flagged approved template should be editable")
	}

	// Once the window closes it is no longer editable.
	old := now.Add(-16 * time.Minute)
	expired := MerchantTemplate{ApprovalStatus: "rejected", NegativeAt: &old}
	if _, ok := TemplateGraceRemaining(expired, grace, now); ok {
		t.Fatal("expired template must not be editable")
	}

	// A clean approved template is never editable here.
	clean := MerchantTemplate{ApprovalStatus: "approved", NegativeAt: &fiveMinAgo}
	if _, ok := TemplateGraceRemaining(clean, grace, now); ok {
		t.Fatal("approved non-marketing template should not be editable")
	}
}
