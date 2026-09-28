package server

import (
	"testing"

	"github.com/google/uuid"

	"whatsappconverty/internal/shops"
	"whatsappconverty/internal/whatsapp"
)

func TestTestVariablesUsesSemanticMap(t *testing.T) {
	vars := testVariables(whatsapp.MerchantTemplate{
		NumVariables: 4,
		Variables: []whatsapp.TokenKey{
			whatsapp.TokenCustomerName,
			whatsapp.TokenTrackingCode,
			whatsapp.TokenDriverName,
			whatsapp.TokenDriverPhone,
		},
	})

	want := map[string]string{
		"1": "Karima",
		"2": "1234567890113",
		"3": "Ali Mansour",
		"4": "+216 98 111 222",
	}
	for slot, value := range want {
		if got := vars[slot]; got != value {
			t.Fatalf("slot %s = %q, want %q", slot, got, value)
		}
	}
}

func TestTestVariablesFallsBackForPositionalTemplates(t *testing.T) {
	vars := testVariables(whatsapp.MerchantTemplate{NumVariables: 5})

	if got := vars["1"]; got != "Hamed" {
		t.Fatalf("slot 1 = %q, want the sample name", got)
	}
	if got := vars["5"]; got != "Ali Mansour" {
		t.Fatalf("slot 5 = %q, want the sample driver name", got)
	}
	if _, ok := vars["6"]; ok {
		t.Fatal("slot 6 must stay absent when the template has five placeholders")
	}
}

func TestTestVariablesPadsUnknownSlots(t *testing.T) {
	vars := testVariables(whatsapp.MerchantTemplate{
		NumVariables: 8,
		Variables:    []whatsapp.TokenKey{whatsapp.TokenCustomerName},
	})

	if got := vars["1"]; got != "Karima" {
		t.Fatalf("slot 1 = %q, want the chip example", got)
	}
	for _, slot := range []string{"7", "8"} {
		if got := vars[slot]; got != "—" {
			t.Fatalf("slot %s = %q, want the em dash pad", slot, got)
		}
	}
}

// A guessed automation id must not reach another shop's send history. The
// membership check is the only thing standing between a valid id and rows the
// operator has no business seeing, so it is worth pinning.
func TestOperatorSeesShop(t *testing.T) {
	visible := []shops.Shop{
		{ID: uuid.New()},
		{ID: uuid.New()},
	}
	if !operatorSeesShop(visible, visible[1].ID) {
		t.Fatal("a shop in the operator's list must be visible")
	}
	if operatorSeesShop(visible, uuid.New()) {
		t.Fatal("a shop outside the operator's list must not be visible")
	}
	if operatorSeesShop(nil, uuid.New()) {
		t.Fatal("an empty shop list must not authorise anything")
	}
}
