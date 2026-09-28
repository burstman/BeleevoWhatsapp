package viewsdashboard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/automations"
	"whatsappconverty/internal/whatsapp"
	"whatsappconverty/web/views/components"
)

// The history page is the first place a merchant reads a status word, so every
// status WhatsApp writes must have a label and a readable pill.
func TestStatusLabelAndBadgeCoverEveryStoredStatus(t *testing.T) {
	for _, status := range []string{"queued", "sent", "delivered", "read", "failed"} {
		label := statusLabel(status)
		if label == "" || label == status {
			t.Errorf("status %q renders as %q, which tells the merchant nothing", status, label)
		}
		badge := statusBadgeClass(status)
		// text-slate-400 on white is the washed-out look this project avoids, so
		// keep the pill text at 600 or darker.
		if badge == "" {
			t.Errorf("status %q has no badge class", status)
		}
	}
}

func TestStatusLabelFallsBackToRawStatus(t *testing.T) {
	// A status WhatsApp adds later must still render rather than blank out.
	if got := statusLabel("something_new"); got != "something_new" {
		t.Fatalf("unknown status should render verbatim, got %q", got)
	}
	if got := statusBadgeClass("something_new"); got == "" {
		t.Fatal("unknown status needs a badge class")
	}
}

// A zero count must not look like a real number, and a non-zero failure count
// is the one number that should stand out.
func TestCountClasses(t *testing.T) {
	if countClass(0) == countClass(5) {
		t.Fatal("an empty count and a real count must not look alike")
	}
	if failureClass(0) == failureClass(3) {
		t.Fatal("failures must stand out from nothing")
	}
	if failureClass(0) != countClass(0) {
		t.Fatal("no failures should look like any other empty count")
	}
}

func TestHistoryTimeRendersDashUntilTheEventHappens(t *testing.T) {
	if got := historyTime(nil); got != "\u2014" {
		t.Fatalf("a message with no delivery should show a dash, got %q", got)
	}
	at := time.Date(2026, 3, 4, 15, 30, 0, 0, time.UTC)
	if got := historyTime(&at); got == "" || got == "\u2014" {
		t.Fatalf("a delivered message should show its time, got %q", got)
	}
}

func TestHistoryWhenAlwaysRenders(t *testing.T) {
	if got := historyWhen(time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)); got == "" {
		t.Fatal("created_at is always set, so the column must never be empty")
	}
}

// Meta reports failures in two places: the error text we store ourselves and the
// error array the webhook carries. The page should prefer the first and fall
// back to the second, including the subcode, rather than showing a blank cell
// on the row the merchant most wants to read.
func TestHistoryErrorPrefersStoredTextThenWebhook(t *testing.T) {
	if got := historyError(whatsapp.AutomationMessage{}); got != "" {
		t.Fatalf("a clean message should have no error, got %q", got)
	}
	withMeta := whatsapp.AutomationMessage{}
	withMeta.MetaErrors = []whatsapp.MetaError{{Code: 131047, Title: "Re-engagement message"}}
	if got := historyError(withMeta); got != "Re-engagement message (131047)" {
		t.Fatalf("webhook error not surfaced: %q", got)
	}
	withMeta.ErrorMessage = "message failed to send"
	if got := historyError(withMeta); got != "message failed to send" {
		t.Fatalf("stored error should win over the webhook array, got %q", got)
	}
}

// The filled placeholders are shown under the customer so a merchant can see
// what the message actually carried; order matters so the column is stable.
func TestHistoryVariablesAreOrderedAndEscaped(t *testing.T) {
	m := whatsapp.AutomationMessage{}
	m.ID = uuid.New()
	m.TemplateVariables = map[string]string{"2": "BC-7", "1": "Amine", "3": ""}
	got := historyVariables(m)
	want := "1=Amine \u00b7 2=BC-7 \u00b7 3="
	if got != want {
		t.Fatalf("variables rendered as %q, want %q", got, want)
	}
	if got := historyVariables(whatsapp.AutomationMessage{}); got != "" {
		t.Fatalf("a template with no variables should render nothing, got %q", got)
	}
}

// The page itself: the header must name the automation and its template, and
// every column the merchant reads has to be present in the markup.
func TestAutomationHistoryPageRendersRowsAndEmptyState(t *testing.T) {
	ctx := context.Background()
	a := automations.Automation{
		ID:          uuid.New(),
		Name:        "Order confirmed",
		EventSource: "converty",
		OrderStatus: "en_cours",
		Enabled:     true,
	}

	row := whatsapp.AutomationMessage{}
	row.ID = uuid.New()
	row.RecipientPhone = "+21624118849"
	row.Status = "delivered"
	row.CreatedAt = time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	row.AutomationID = &a.ID
	row.TemplateName = "order_confirmed"
	row.CustomerName = "Amine Trabelsi"
	row.TemplateVariables = map[string]string{"1": "Amine"}
	delivered := row.CreatedAt.Add(time.Minute)
	row.DeliveredAt = &delivered

	render := func(rows []whatsapp.AutomationMessage, counts whatsapp.AutomationSendCounts) string {
		var sb strings.Builder
		if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "order_confirmed", rows, counts).Render(ctx, &sb); err != nil {
			t.Fatalf("render history: %v", err)
		}
		return sb.String()
	}

	html := render([]whatsapp.AutomationMessage{row}, whatsapp.AutomationSendCounts{Total: 1, Delivered: 1})
	for _, want := range []string{
		"Order confirmed",
		"send history",
		"/automations",
		"converty",
		"en_cours",
		"order_confirmed",
		"+21624118849",
		"Amine Trabelsi",
		"1=Amine",
		"Delivered",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("history page missing %q", want)
		}
	}

	empty := render(nil, whatsapp.AutomationSendCounts{})
	if !strings.Contains(empty, "Nothing sent yet") {
		t.Error("an automation that has not fired must say so instead of showing an empty table")
	}
	// A row that failed is the reason a merchant opens this page; make sure the
	// error text has somewhere to render.
	failed := row
	failed.Status = "failed"
	failed.ErrorMessage = "message failed to send"
	failHTML := render([]whatsapp.AutomationMessage{failed}, whatsapp.AutomationSendCounts{Total: 1, Failed: 1})
	if !strings.Contains(failHTML, "message failed to send") {
		t.Error("a failed send must show why it failed")
	}
}

// "Waiting" read like a schedule, so a merchant thought a row meant "will send
// later". A row only exists once the job ran, so a stuck one is a send that did
// not complete and must not be dressed as pending.
func TestQueuedRowReadsAsAFailureNotAPendingSchedule(t *testing.T) {
	if got := statusLabel("queued"); got != "Not sent" {
		t.Fatalf("queued label is %q, which still reads as a schedule", got)
	}
	if got := statusLabel("queued"); got == "Waiting" {
		t.Fatal(`"Waiting" implies the send is still to come`)
	}

	row := whatsapp.AutomationMessage{}
	row.ID = uuid.New()
	row.RecipientPhone = "+21624118849"
	row.Status = "queued"
	row.CreatedAt = time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	rowID := uuid.New()
	row.AutomationID = &rowID
	a := automations.Automation{ID: rowID, Name: "Order confirmed", EventSource: "delivery", OrderStatus: "in-progress", Enabled: true}

	var sb strings.Builder
	if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "tpl", []whatsapp.AutomationMessage{row}, whatsapp.AutomationSendCounts{Total: 1, Queued: 1}).Render(t.Context(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "Not sent") {
		t.Error("the row should be labelled as not sent")
	}
	if !strings.Contains(html, "WhatsApp never confirmed it") {
		t.Error("the row should say the send did not complete")
	}
	if strings.Contains(html, "Waiting for its send time") {
		t.Error("the old pending-schedule wording is back; it is what confused the merchant")
	}
}

// The note tells the merchant when a row appears, so an empty list before the
// first send reads as normal rather than broken.
func TestHistoryNoteExplainsWhenRowsAppear(t *testing.T) {
	day := 10
	delay := 30
	sendAt := time.Date(2026, 3, 4, day, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		auto automations.Automation
		want string
	}{
		{"scheduled", automations.Automation{SendTime: &sendAt, SendTimezone: "Africa/Tunis"}, "10:00 Africa/Tunis"},
		{"delayed", automations.Automation{DelayMinutes: &delay}, "30 minutes after the event"},
		{"instant", automations.Automation{}, "as soon as the event happens"},
	}
	for _, c := range cases {
		got := historyNote(c.auto)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s note says %q, want it to mention %q", c.name, got, c.want)
		}
		if !strings.Contains(got, "only once they have been sent") {
			t.Errorf("%s note must say a row appears only after the send: %q", c.name, got)
		}
	}

	// A scheduled automation with a blank stored zone falls back to the default
	// rather than printing an empty one.
	blank := automations.Automation{SendTime: &sendAt}
	if got := historyNote(blank); strings.Contains(got, "10:00 \u00b7") || !strings.Contains(got, "Africa/Tunis") {
		t.Errorf("blank timezone should render the default, got %q", got)
	}

	a := automations.Automation{ID: uuid.New(), Name: "Order confirmed", EventSource: "delivery", OrderStatus: "in-progress", Enabled: true}
	var sb strings.Builder
	if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "tpl", nil, whatsapp.AutomationSendCounts{}).Render(t.Context(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "not a broken automation") {
		t.Error("the empty state must say an empty list is not a broken automation")
	}
	if !strings.Contains(html, "Messages appear here only once they have been sent") {
		t.Error("the page must explain when messages appear")
	}
}
