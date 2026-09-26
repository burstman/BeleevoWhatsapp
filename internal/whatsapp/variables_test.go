package whatsapp

import "testing"

func TestTokenAllowedBySource(t *testing.T) {
	if TokenAllowed(SourceConverty, TokenDriverName) || TokenAllowed(SourceConverty, TokenDriverPhone) {
		t.Fatal("driver variables must not be allowed in a Converty template: the event carries none")
	}
	for _, k := range []TokenKey{TokenCustomerName, TokenCustomerPhone, TokenOrderID, TokenOrderStatus, TokenTrackingCode} {
		if !TokenAllowed(SourceConverty, k) {
			t.Fatalf("converty token %s should be allowed", k)
		}
	}
	if !TokenAllowed(SourceDelivery, TokenDriverName) || !TokenAllowed(SourceDelivery, TokenDriverPhone) {
		t.Fatal("driver variables must be allowed in a delivery template")
	}
}

func TestVariableChipSources(t *testing.T) {
	seen := 0
	for _, c := range VariableChips() {
		switch c.Value {
		case string(TokenDriverName), string(TokenDriverPhone):
			seen++
			if c.Sources() != "delivery,any" {
				t.Fatalf("chip %s should be limited to delivery,any, got %q", c.Value, c.Sources())
			}
		default:
			if c.Sources() != "converty,delivery,any" {
				t.Fatalf("chip %s should be available everywhere, got %q", c.Value, c.Sources())
			}
		}
	}
	if seen != 2 {
		t.Fatalf("expected 2 delivery-only chips, found %d", seen)
	}
}

func TestNormalizeSource(t *testing.T) {
	for _, s := range []string{SourceConverty, SourceDelivery, SourceAny} {
		if NormalizeSource(s) != s {
			t.Fatalf("NormalizeSource(%q) = %q", s, NormalizeSource(s))
		}
	}
	for _, bad := range []string{"", "converty ", "unknown", "DELIVERY"} {
		if NormalizeSource(bad) != SourceAny {
			t.Fatalf("NormalizeSource(%q) should fall back to any, got %q", bad, NormalizeSource(bad))
		}
	}
}

func TestTokenValueResolvesDriverFields(t *testing.T) {
	vals := TemplateVariableValues{
		CustomerName: "Karima",
		DriverName:   "Ali Mansour",
		DriverPhone:  "+216 98 111 222",
	}
	if got := TokenValue(TokenDriverName, vals); got != "Ali Mansour" {
		t.Fatalf("driver name = %q", got)
	}
	if got := TokenValue(TokenDriverPhone, vals); got != "+216 98 111 222" {
		t.Fatalf("driver phone = %q", got)
	}
	// A Converty event leaves the driver empty; callers must handle that rather
	// than send a blank to the customer.
	if got := TokenValue(TokenDriverName, TemplateVariableValues{}); got != "" {
		t.Fatalf("missing driver should resolve empty, got %q", got)
	}
}

func TestValidateTemplateBody(t *testing.T) {
	ok := []string{
		"Bonjour Mr {{1}}: votre commande arrive. Merci!",
		"Votre livreur est {{1}} et appelez le {{2}}. Merci!",
		"Bonjour\n{{1}}\nVotre commande arrive",
		"Votre commande. Livreur: {{1}},",
		"Votre commande. Livreur: {{1}}: Merci",
		"Bonjour Monsieur {{1}} merci",
	}
	for _, body := range ok {
		if err := ValidateTemplateBody(body); err != nil {
			t.Errorf("body %q should be accepted: %v", body, err)
		}
	}

	bad := []string{
		"Votre commande. Livreur: {{1}}",
		"Votre commande. Livreur: {{1}}.",
		"Votre commande. Livreur: {{1}}!",
		"Votre commande. Livreur: {{1}}?",
		"Votre commande. Livreur: {{1}}..",
		"Votre commande. Livreur: {{1}} ",
		"Votre commande. Livreur: {{1}}.\n",
		"{{1}}, votre commande arrive",
		"Bonjour {{1}} merci",
		"Livreur: {{1}} Tel: {{2}}",
		"Bonjour {{1}}, livreur {{2}}, tel {{3}}",
	}
	for _, body := range bad {
		if err := ValidateTemplateBody(body); err == nil {
			t.Errorf("body %q should be rejected", body)
		}
	}

	if err := ValidateTemplateBody("Bienvenue dans notre boutique"); err != nil {
		t.Errorf("a static body without variables is valid: %v", err)
	}
}

func TestExplainTemplateRejection(t *testing.T) {
	for _, sc := range []struct {
		sub  int
		want string
	}{
		{2388293, "more words around the variables"},
		{2388299, "cannot be the first or last thing"},
	} {
		got := ExplainTemplateRejection(&APIError{Code: 100, SubCode: sc.sub})
		if got == "" || !contains(got, sc.want) {
			t.Errorf("subcode %d: got %q, want it to mention %q", sc.sub, got, sc.want)
		}
	}
	if got := ExplainTemplateRejection(&APIError{Code: 100, SubCode: 1}); got != "" {
		t.Errorf("unknown subcode should have no hint, got %q", got)
	}
}

func contains(hay, needle string) bool {
	return len(needle) == 0 || len(hay) >= len(needle) && (hay == needle || indexOf(hay, needle) >= 0)
}

func indexOf(hay, needle string) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
