package automations

import (
	"testing"

	"github.com/google/uuid"

	"whatsappconverty/internal/whatsapp"
)

func TestBuildVariablesFillsVocabularyInOrder(t *testing.T) {
	tpl := whatsapp.MerchantTemplate{NumVariables: 3}
	vars := buildVariables(tpl, "Ahmed", "ORD-42", "Delivered")

	want := map[string]string{
		"1": "Ahmed",
		"2": "ORD-42",
		"3": "Delivered",
	}
	for k, v := range want {
		if vars[k] != v {
			t.Fatalf("vars[%s] = %q, want %q", k, vars[k], v)
		}
	}
}

func TestBuildVariablesPadsToTemplateCount(t *testing.T) {
	tpl := whatsapp.MerchantTemplate{NumVariables: 5}
	vars := buildVariables(tpl, "Ahmed", "ORD-42", "Delivered")

	if len(vars) != 5 {
		t.Fatalf("len(vars) = %d, want 5 (must equal template count)", len(vars))
	}
	for i := 1; i <= 5; i++ {
		if vars[string(rune('0'+i))] == "" {
			t.Fatalf("vars[%d] should never be empty", i)
		}
	}
	if vars["4"] != "—" {
		t.Fatalf("vars[4] = %q, want em dash", vars["4"])
	}
}

func TestBuildVariablesNoTemplateVars(t *testing.T) {
	vars := buildVariables(whatsapp.MerchantTemplate{NumVariables: 0}, "A", "B", "C")
	if len(vars) != 0 {
		t.Fatalf("expected empty vars, got %v", vars)
	}
}

func TestToSendRequestRoundTrip(t *testing.T) {
	job := whatsapp.SendWhatsAppTemplateJob{
		ShopID:          mustUUID(t),
		CustomerID:      mustUUID(t),
		TemplateID:      mustUUID(t),
		ConvertyOrderID: "ORD-7",
		Purpose:         "UTILITY",
		IdempotencyKey:  "msc:test",
		Variables:       map[string]string{"1": "x"},
	}
	req := toSendRequest(job)
	if req.ShopID != job.ShopID || req.CustomerID != job.CustomerID || req.TemplateID != job.TemplateID {
		t.Fatal("IDs differ after round trip")
	}
	if req.IdempotencyKey != job.IdempotencyKey || req.Purpose != job.Purpose {
		t.Fatal("send request fields lost after round trip")
	}
	if req.Variables["1"] != "x" {
		t.Fatal("variables lost after round trip")
	}
}

func mustUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewRandom()
	if err != nil {
		t.Fatal(err)
	}
	return id
}