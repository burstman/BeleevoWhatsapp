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
