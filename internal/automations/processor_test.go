package automations

import (
	"testing"
	"time"

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

func TestResolveSendAtInstant(t *testing.T) {
	if at := resolveSendAt(nil, "UTC", time.Now()); !at.IsZero() {
		t.Fatalf("instant automation should send now, got %v", at)
	}
}

func TestResolveSendAtBeforeTimeWaits(t *testing.T) {
	now := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC) // 09:00 UTC
	minute := 10 * 60                                   // 10:00
	at := resolveSendAt(&minute, "UTC", now)
	want := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	if !at.Equal(want) {
		t.Fatalf("at = %v, want %v", at.UTC(), want.UTC())
	}
}

func TestResolveSendAtAfterTimeFiresImmediately(t *testing.T) {
	now := time.Date(2026, 9, 25, 11, 15, 0, 0, time.UTC) // 11:15 > 10:00
	minute := 10 * 60
	if at := resolveSendAt(&minute, "UTC", now); !at.IsZero() {
		t.Fatalf("event after the daily time should go now, got %v", at.UTC())
	}
}

func TestResolveSendAtExactTimeFiresImmediately(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC) // == the daily time
	minute := 10 * 60
	if at := resolveSendAt(&minute, "UTC", now); !at.IsZero() {
		t.Fatalf("event exactly at the daily time should go now, got %v", at.UTC())
	}
}

func TestResolveSendAtUsesTimezone(t *testing.T) {
	now := time.Date(2026, 9, 25, 8, 30, 0, 0, time.UTC) // 09:30 in Africa/Tunis (UTC+1, no DST)
	minute := 10 * 60                                    // 10:00 Africa/Tunis = 09:00 UTC
	at := resolveSendAt(&minute, "Africa/Tunis", now)
	want := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	if !at.Equal(want) {
		t.Fatalf("at = %v, want %v", at.UTC(), want.UTC())
	}
}

func TestResolveSendAtFallsBackToUTC(t *testing.T) {
	now := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	minute := 10 * 60
	at := resolveSendAt(&minute, "Not/AZone", now)
	want := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	if !at.Equal(want) {
		t.Fatalf("invalid timezone should fall back to UTC, at = %v", at.UTC())
	}
}

func TestParseSchedule(t *testing.T) {
	if s, err := ParseSchedule("instant", "10:00", "UTC"); err != nil || s != nil {
		t.Fatalf("instant mode should yield nil schedule, got %v err %v", s, err)
	}
	s, err := ParseSchedule("fixed", "10:00", "Africa/Tunis")
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	if s == nil || s.SendMinute == nil || *s.SendMinute != 10*60 {
		t.Fatalf("wrong schedule: %+v", s)
	}
	if s.Timezone != "Africa/Tunis" {
		t.Fatalf("wrong timezone: %q", s.Timezone)
	}
	if _, err := ParseSchedule("fixed", "25:99", "UTC"); err == nil {
		t.Fatal("expected error for invalid time")
	}
	if _, err := ParseSchedule("fixed", "10:00", "Mars/Olympus"); err == nil {
		t.Fatal("expected error for invalid timezone")
	}
	if s, err := ParseSchedule("fixed", "10:00", ""); err != nil || s == nil || s.Timezone != "UTC" {
		t.Fatalf("empty timezone should default to UTC, got %+v err %v", s, err)
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