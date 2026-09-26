package automations

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/whatsapp"
)

func TestBuildVariablesFillsVocabularyInOrder(t *testing.T) {
	tpl := whatsapp.MerchantTemplate{NumVariables: 3}
	vars := buildVariables(tpl, SendInput{CustomerName: "Ahmed", OrderID: "ORD-42", StatusLabel: "Delivered"})

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
	vars := buildVariables(tpl, SendInput{CustomerName: "Ahmed", OrderID: "ORD-42", StatusLabel: "Delivered"})

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
	vars := buildVariables(whatsapp.MerchantTemplate{NumVariables: 0}, SendInput{})
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

func TestScheduledFireInstant(t *testing.T) {
	r := ruleFromSchedule(nil)
	if at := scheduledFire(r, time.Now()); !at.IsZero() {
		t.Fatalf("instant automation should send now, got %v", at)
	}
}

func TestScheduledFireBeforeTimeWaits(t *testing.T) {
	now := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC) // Friday 09:00 UTC
	rule := fireRule{minute: intPtr(10 * 60), timezone: "UTC", days: AllDays()}
	at := scheduledFire(rule, now)
	want := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	if !at.Equal(want) {
		t.Fatalf("at = %v, want %v", at.UTC(), want.UTC())
	}
}

func TestScheduledFireAfterTimeFiresImmediately(t *testing.T) {
	now := time.Date(2026, 9, 25, 11, 15, 0, 0, time.UTC) // 11:15 > 10:00, Friday allowed
	rule := fireRule{minute: intPtr(10 * 60), timezone: "UTC", days: AllDays()}
	if at := scheduledFire(rule, now); !at.IsZero() {
		t.Fatalf("event after the daily time on an allowed day should go now, got %v", at.UTC())
	}
}

func TestScheduledFireExactTimeFiresImmediately(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	rule := fireRule{minute: intPtr(10 * 60), timezone: "UTC", days: AllDays()}
	if at := scheduledFire(rule, now); !at.IsZero() {
		t.Fatalf("event exactly at the daily time should go now, got %v", at.UTC())
	}
}

func TestScheduledFireUsesTimezone(t *testing.T) {
	now := time.Date(2026, 9, 25, 8, 30, 0, 0, time.UTC) // 09:30 in Africa/Tunis (UTC+1)
	rule := fireRule{minute: intPtr(10 * 60), timezone: "Africa/Tunis", days: AllDays()}
	at := scheduledFire(rule, now)
	want := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	if !at.Equal(want) {
		t.Fatalf("at = %v, want %v", at.UTC(), want.UTC())
	}
}

func TestScheduledFireNonAllowedWeekdayWaits(t *testing.T) {
	// Sunday 2026-09-27 09:00 UTC; window is Mon-Fri (1..5). 10:00 the same day
	// must NOT fire — it waits for Monday 2026-09-28 10:00.
	now := time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC)
	rule := fireRule{minute: intPtr(10 * 60), timezone: "UTC", days: []int{1, 2, 3, 4, 5}}
	at := scheduledFire(rule, now)
	want := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	if at.IsZero() || !at.Equal(want) {
		t.Fatalf("non-allowed day should push to Monday 10:00, got %v, want %v", at.UTC(), want.UTC())
	}
}

func TestScheduledFireDelayed(t *testing.T) {
	now := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	rule := fireRule{delay: intPtr(90), timezone: "UTC", days: AllDays()} // +90 minutes
	at := scheduledFire(rule, now)
	want := time.Date(2026, 9, 25, 10, 30, 0, 0, time.UTC)
	if !at.Equal(want) {
		t.Fatalf("delayed 90m: at = %v, want %v", at.UTC(), want.UTC())
	}
}

func TestScheduledFireDelayedAcrossDayBoundary(t *testing.T) {
	now := time.Date(2026, 9, 25, 23, 30, 0, 0, time.UTC)
	rule := fireRule{delay: intPtr(60), timezone: "UTC", days: AllDays()}
	at := scheduledFire(rule, now)
	want := time.Date(2026, 9, 26, 0, 30, 0, 0, time.UTC)
	if !at.Equal(want) {
		t.Fatalf("at = %v, want %v", at.UTC(), want.UTC())
	}
}

func TestScheduledFireFallsBackToUTC(t *testing.T) {
	now := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	rule := fireRule{minute: intPtr(10 * 60), timezone: "Not/AZone", days: AllDays()}
	at := scheduledFire(rule, now)
	want := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	if !at.Equal(want) {
		t.Fatalf("invalid timezone should fall back to UTC, at = %v", at.UTC())
	}
}

func TestParseSchedule(t *testing.T) {
	if s, err := ParseSchedule("instant", "10:00", "UTC", nil, 0); err != nil || s != nil {
		t.Fatalf("instant mode should yield nil schedule, got %v err %v", s, err)
	}
	if s, err := ParseSchedule("", "10:00", "UTC", nil, 0); err != nil || s != nil {
		t.Fatalf("empty mode should yield nil schedule, got %v err %v", s, err)
	}

	s, err := ParseSchedule("fixed", "10:00", "Africa/Tunis", []string{"1", "2", "3"}, 0)
	if err != nil {
		t.Fatalf("ParseSchedule fixed: %v", err)
	}
	if s == nil || s.SendMinute == nil || *s.SendMinute != 10*60 {
		t.Fatalf("wrong fixed schedule: %+v", s)
	}
	if s.Timezone != "Africa/Tunis" || len(s.Days) != 3 || s.Days[0] != 1 {
		t.Fatalf("wrong fixed rule: %+v", s)
	}

	s, err = ParseSchedule("delayed", "", "", nil, 90)
	if err != nil {
		t.Fatalf("ParseSchedule delayed: %v", err)
	}
	if s == nil || s.DelayMinute == nil || *s.DelayMinute != 90 {
		t.Fatalf("wrong delayed schedule: %+v", s)
	}

	if _, err := ParseSchedule("fixed", "25:99", "UTC", []string{"1"}, 0); err == nil {
		t.Fatal("expected error for invalid time")
	}
	if _, err := ParseSchedule("fixed", "10:00", "Mars/Olympus", []string{"1"}, 0); err == nil {
		t.Fatal("expected error for invalid timezone")
	}
	if _, err := ParseSchedule("fixed", "10:00", "UTC", nil, 0); err == nil {
		t.Fatal("expected error for no day selected")
	}
	if _, err := ParseSchedule("fixed", "10:00", "UTC", []string{"0", "9"}, 0); err == nil {
		t.Fatal("expected error for out-of-range day")
	}
	if _, err := ParseSchedule("delayed", "", "", nil, -10); err == nil {
		t.Fatal("expected error for non-positive delay")
	}
	if _, err := ParseSchedule("weird", "", "", nil, 0); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func intPtr(v int) *int { return &v }

func mustUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewRandom()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
