package viewsdashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/automations"
	"whatsappconverty/web/views/components"
)

// The dropdown must be a list the merchant can actually read, led by the zone
// the platform serves, and every entry must be a real IANA name the server will
// accept on save.
func TestTimezoneOptionsLeadWithTheDefault(t *testing.T) {
	opts := timezoneOptions(nil)
	if len(opts) == 0 {
		t.Fatal("the timezone dropdown must not be empty")
	}
	if opts[0].Name != automations.DefaultTimezone {
		t.Fatalf("dropdown starts at %q, want the default %q", opts[0].Name, automations.DefaultTimezone)
	}
	for _, o := range opts {
		if o.Name == "" || o.Label == "" {
			t.Errorf("option %+v has no name or label", o)
		}
		if !strings.Contains(o.Label, o.Name) {
			t.Errorf("label %q should name its zone %q", o.Label, o.Name)
		}
	}
}

// A zone saved before the list existed must survive an unrelated edit. If the
// stored value is dropped from the dropdown, the browser posts the first option
// instead and the merchant's automation silently changes timezone on save.
func TestTimezoneOptionsKeepsAnUnknownStoredZone(t *testing.T) {
	saved := &automations.Automation{SendTimezone: "Pacific/Auckland"}
	opts := timezoneOptions(saved)
	if opts[0].Name != "Pacific/Auckland" {
		t.Fatalf("stored zone must come first so it stays selected, got %q", opts[0].Name)
	}
	found := false
	for _, o := range opts {
		if o.Name == "Pacific/Auckland" {
			found = true
		}
	}
	if !found {
		t.Fatal("stored zone is missing from the dropdown")
	}
	if got := sendTimezoneValue(saved); got != "Pacific/Auckland" {
		t.Fatalf("stored timezone not preserved, got %q", got)
	}
}

func TestTimezoneValueDefaultsToTunis(t *testing.T) {
	if got := sendTimezoneValue(nil); got != "Africa/Tunis" {
		t.Fatalf("a new automation should default to %q, got %q", automations.DefaultTimezone, got)
	}
	if got := sendTimezoneValue(&automations.Automation{}); got != "Africa/Tunis" {
		t.Fatalf("an automation with no timezone should default to %q, got %q", automations.DefaultTimezone, got)
	}
}

// The whole point of the label is to show the offset, because "10:00" is
// ambiguous without it. Africa/Tunis is UTC+1 year-round, so a rule typed as
// 10:00 there must not read as 10:00 UTC.
func TestTimezoneLabelShowsTheOffset(t *testing.T) {
	tun := timezoneLabel("Africa/Tunis")
	if !strings.Contains(tun, "UTC+1") {
		t.Errorf("Africa/Tunis should read UTC+1, got %q", tun)
	}
	utc := timezoneLabel("UTC")
	if !strings.Contains(utc, "UTC+0") {
		t.Errorf("UTC should read UTC+0, got %q", utc)
	}
	// An unknown zone must render its raw name rather than an empty option.
	if got := timezoneLabel("Not/AZone"); got != "Not/AZone" {
		t.Errorf("unknown zone should render verbatim, got %q", got)
	}
}

// The form posts send_timezone; make sure the value that reaches the server is
// the one the select shows, and that the new default is what the server falls
// back to when the field is empty.
func TestScheduledSendDefaultsToTunisOnTheServer(t *testing.T) {
	s, err := automations.ParseSchedule("fixed", "10:00", "", []string{"1", "2", "3"}, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Timezone != automations.DefaultTimezone {
		t.Fatalf("an omitted timezone became %q, want %q", s.Timezone, automations.DefaultTimezone)
	}
	s, err = automations.ParseSchedule("fixed", "10:00", "Africa/Tunis", []string{"1"}, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Timezone != "Africa/Tunis" {
		t.Fatalf("chosen timezone not kept, got %q", s.Timezone)
	}
	// A name the server cannot load is still refused.
	if _, err := automations.ParseSchedule("fixed", "10:00", "Not/AZone", []string{"1"}, 0); err == nil {
		t.Fatal("an invalid timezone must be rejected")
	}
}

// The form renders a select, not a free-text box: a typo'd zone was the failure
// mode this replaces.
func TestAutomationFormRendersTimezoneSelect(t *testing.T) {
	page := components.Page{Title: "Automations", Active: "automations"}
	var sb strings.Builder
	err := AutomationFormPage(page, nil, nil, nil, nil, false, AutomationFlash{}, "converty", map[string]int{"converty": 0, "delivery": 0}).
		Render(t.Context(), &sb)
	if err != nil {
		t.Fatalf("render form: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, `<select id="sendTimezone"`) {
		t.Error("the timezone control should be a select the merchant picks from")
	}
	if !strings.Contains(html, `name="send_timezone"`) {
		t.Error("the select must still post send_timezone")
	}
	if strings.Contains(html, `placeholder="e.g. Africa/Tunis, UTC"`) {
		t.Error("the free-text timezone input is gone; its placeholder should not come back")
	}
	if !strings.Contains(html, "Africa/Tunis (UTC+1)") {
		t.Error("the default zone should be offered with its offset")
	}
	// Editing an automation that sits on UTC must offer that value too.
	sendAt := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	utc := &automations.Automation{ID: uuid.New(), SendTimezone: "UTC", SendTime: &sendAt}
	var edit strings.Builder
	if err := AutomationFormPage(page, utc, nil, nil, nil, true, AutomationFlash{}, "converty", map[string]int{"converty": 0, "delivery": 0}).
		Render(t.Context(), &edit); err != nil {
		t.Fatalf("render edit form: %v", err)
	}
	if !strings.Contains(edit.String(), `<option value="UTC" selected`) {
		t.Error("the stored zone must be the selected option when editing")
	}
}
