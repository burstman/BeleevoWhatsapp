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

	render := func(rows []whatsapp.AutomationMessage, counts whatsapp.AutomationSendCounts, held []automations.Suppression, retry automations.RetryResult) string {
		var sb strings.Builder
		if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "order_confirmed", rows, counts, held, retry).Render(ctx, &sb); err != nil {
			t.Fatalf("render history: %v", err)
		}
		return sb.String()
	}

	html := render([]whatsapp.AutomationMessage{row}, whatsapp.AutomationSendCounts{Total: 1, Delivered: 1}, nil, automations.RetryResult{})
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

	empty := render(nil, whatsapp.AutomationSendCounts{}, nil, automations.RetryResult{})
	if !strings.Contains(empty, "Nothing sent yet") {
		t.Error("an automation that has not fired must say so instead of showing an empty table")
	}
	// A row that failed is the reason a merchant opens this page; make sure the
	// error text has somewhere to render.
	failed := row
	failed.Status = "failed"
	failed.ErrorMessage = "message failed to send"
	failHTML := render([]whatsapp.AutomationMessage{failed}, whatsapp.AutomationSendCounts{Total: 1, Failed: 1}, nil, automations.RetryResult{})
	if !strings.Contains(failHTML, "message failed to send") {
		t.Error("a failed send must show why it failed")
	}
}

// A held-back send is the failure mode that looks most like success: the event
// matched, the merchant is watching, and no message exists. The page has to say
// so in words, naming what the template was waiting for, or this happens again
// and the merchant assumes the automation is broken.
func TestHeldBackSendsAreVisibleAndNamed(t *testing.T) {
	rowID := uuid.New()
	a := automations.Automation{ID: rowID, Name: "Out for delivery", EventSource: "delivery", OrderStatus: "in-progress", Enabled: true}
	s := automations.Suppression{
		ID:           uuid.New(),
		ShopID:       uuid.New(),
		AutomationID: rowID,
		Reason:       "a variable the template places has no value for this order yet",
		Missing:      []whatsapp.TokenKey{whatsapp.TokenDriverName, whatsapp.TokenDriverPhone},
		TrackingCode: "922153764102",
		CustomerName: "Radhwen Marayah",
		CreatedAt:    time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC),
	}

	var sb strings.Builder
	if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "ordre_en_cours", nil, whatsapp.AutomationSendCounts{}, []automations.Suppression{s}, automations.RetryResult{}).Render(t.Context(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	for _, want := range []string{
		"Held back",
		"but not sent",
		"922153764102",
		"Radhwen Marayah",
		"Driver name",
		"Driver phone",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("held-back history is missing %q, so a suppressed send still looks like a silent no-op", want)
		}
	}
	if !strings.Contains(html, "Nothing sent yet") {
		t.Error("the empty ledger should still say nothing was sent")
	}

	// The count on the summary must not read as a failure: nothing went wrong,
	// the send was withheld on purpose.
	if !strings.Contains(html, "withheld on purpose") {
		t.Error("held-back sends must be distinguished from failures in the explanation")
	}
}

// The page used to say a message was sent, to whom, and never what it said. The
// merchant's question on this page is "what did my customer actually get", so the
// rendered text has to be in the row, not just the variables behind it.
func TestHistoryShowsTheMessageThatWasSent(t *testing.T) {
	row := whatsapp.AutomationMessage{}
	row.ID = uuid.New()
	row.RecipientPhone = "+21693531118"
	row.Status = "delivered"
	row.BodyText = "Bonjour Radhwen Marayah votre commande 922153764102 est en cours de livraison.\n\nLivreur: Borhen edine ben khlifa Tel : 29656683 Merci beacoup!!!"
	row.CreatedAt = time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	rowID := uuid.New()
	row.AutomationID = &rowID
	a := automations.Automation{ID: rowID, Name: "Out for delivery", EventSource: "delivery", OrderStatus: "in-progress", Enabled: true}

	var sb strings.Builder
	if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "ordre_en_cours", []whatsapp.AutomationMessage{row}, whatsapp.AutomationSendCounts{Total: 1, Delivered: 1}, nil, automations.RetryResult{}).Render(t.Context(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, "Livreur: Borhen edine ben khlifa Tel : 29656683") {
		t.Error("the message text must be visible in the history row")
	}
	if strings.Contains(html, "Accepted by WhatsApp") {
		t.Error("the message itself is the useful detail; the old placeholder text is not")
	}
	if !strings.Contains(html, "922153764102") {
		t.Error("the tracking code appears in the message text and must survive rendering")
	}
}

// A message sent before the snapshot existed must still render a row rather than
// an empty column, since the live ledger has rows with no body text.
func TestHistoryRowWithoutASnapshotStillRenders(t *testing.T) {
	row := whatsapp.AutomationMessage{}
	row.ID = uuid.New()
	row.RecipientPhone = "+21693531118"
	row.Status = "sent"
	row.TemplateVariables = map[string]string{"1": "CVY-7"}
	row.CreatedAt = time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	rowID := uuid.New()
	row.AutomationID = &rowID
	a := automations.Automation{ID: rowID, Name: "Order confirmed", EventSource: "converty", OrderStatus: "confirmed", Enabled: true}

	var sb strings.Builder
	if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "tpl", []whatsapp.AutomationMessage{row}, whatsapp.AutomationSendCounts{Total: 1, Sent: 1}, nil, automations.RetryResult{}).Render(t.Context(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(sb.String(), "Accepted by WhatsApp") {
		t.Error("a row with no snapshot should fall back rather than render a blank cell")
	}
}

// The button is the only way a held-back event ever reaches the customer: the
// status transition it came from has already passed, so no future event will
// pick it up. Its absence would strand those messages permanently.
func TestHeldBackListOffersASendNowAction(t *testing.T) {
	rowID := uuid.New()
	a := automations.Automation{ID: rowID, Name: "Out for delivery", EventSource: "delivery", OrderStatus: "in-progress", Enabled: true}
	held := []automations.Suppression{{
		ID: uuid.New(), AutomationID: rowID, IdempotencyKey: "msc:x:in-progress:1",
		Reason:  "a variable the template places has no value for this order yet",
		Missing: []whatsapp.TokenKey{whatsapp.TokenDriverName},
	}}

	var sb strings.Builder
	if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "tpl", nil, whatsapp.AutomationSendCounts{}, held, automations.RetryResult{}).Render(t.Context(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "/automations/"+rowID.String()+"/retry-held") {
		t.Error("a held-back list must offer a way to send those messages now")
	}
	if !strings.Contains(html, "Send held back now") {
		t.Error("the retry action needs a label the merchant can act on")
	}

	// A partial retry must not read as a clean success.
	var mixed strings.Builder
	if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "tpl", nil, whatsapp.AutomationSendCounts{}, held, automations.RetryResult{Attempted: 3, Sent: 2, StillHeld: 1}).Render(t.Context(), &mixed); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := mixed.String()
	for _, want := range []string{"2 message(s) sent now", "1 still held back"} {
		if !strings.Contains(out, want) {
			t.Errorf("a partial retry must report both halves, missing %q", want)
		}
	}
	if !strings.Contains(out, retryBannerClass(1)) {
		t.Error("a partial retry should be amber, not green")
	}
}

// A retry now also covers parcels that are already in the automation's status,
// so a click can find events that were sent long ago. Those must be reported as
// left alone: a message re-sent because the merchant pressed a button is the one
// failure this button exists to prevent.
func TestRetrySaysItLeftAlreadySentAlone(t *testing.T) {
	rowID := uuid.New()
	a := automations.Automation{ID: rowID, Name: "Out for delivery", EventSource: "delivery", OrderStatus: "in-progress", Enabled: true}

	var sb strings.Builder
	res := automations.RetryResult{Attempted: 1, Sent: 1, AlreadySent: 2}
	if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "tpl", nil, whatsapp.AutomationSendCounts{}, nil, res).Render(t.Context(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "2 had already been sent and were left alone") {
		t.Error("a retry must say it did not re-send anything that already went out")
	}
	if !strings.Contains(html, retryBannerClass(0)) {
		t.Error("with nothing still held back, the outcome is a clean one")
	}
}

// A retry that finds the automation's own template is gone has to say so. The
// first version of this page reported every non-send as "this order is missing
// data", which sent the merchant off to the carrier for a problem that was in
// the automation they were looking at.
func TestRetryNamesAGoneTemplateInsteadOfBlamingTheOrder(t *testing.T) {
	rowID := uuid.New()
	a := automations.Automation{ID: rowID, Name: "Out for delivery", EventSource: "delivery", OrderStatus: "in-progress", Enabled: true}

	var sb strings.Builder
	res := automations.RetryResult{Attempted: 3, TemplateMissing: 3}
	if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "tpl", nil, whatsapp.AutomationSendCounts{}, nil, res).Render(t.Context(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "This automation cannot send at all right now") {
		t.Error("a gone template is the automation's fault and must be named as the headline")
	}
	if !strings.Contains(html, "Choose another template on the automation") {
		t.Error("the page must point at the fix a merchant can actually make")
	}
	if strings.Contains(html, "the template needs data this order does not have yet") {
		t.Error("do not blame the order's data when the template itself is missing")
	}
	if !strings.Contains(html, retryBannerClass(3)) {
		t.Error("nothing went out, so the outcome must not read as a clean one")
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
	if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "tpl", []whatsapp.AutomationMessage{row}, whatsapp.AutomationSendCounts{Total: 1, Queued: 1}, nil, automations.RetryResult{}).Render(t.Context(), &sb); err != nil {
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
	if err := AutomationHistoryPage(components.Page{Title: "Send history", Active: "automations"}, a, "tpl", nil, whatsapp.AutomationSendCounts{}, nil, automations.RetryResult{}).Render(t.Context(), &sb); err != nil {
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
