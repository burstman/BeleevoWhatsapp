package automations

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/database"
	"whatsappconverty/internal/delivery"
	"whatsappconverty/internal/whatsapp"
)

// Templates are shared across the operator's shops, and the automation editor
// deliberately offers them all: a merchant on one shop may use a template that
// another shop's WhatsApp number owns. The send path resolves the template by id
// for the same reason.
//
// The regression this pins is a whole class of silent failure. Resolving the
// template by (id, shop) made every send from a shared template fail as "no
// longer exists" - no error, no message, and a page that blamed the order for
// data the template never even got to look at.
//
//	go test ./internal/automations/ -run 'TestSharedTemplate|TestGoneTemplate' -v
type scopedTemplateDB struct {
	*pgxpool.Pool
	shops []uuid.UUID
}

func (d *scopedTemplateDB) shop(t *testing.T) uuid.UUID {
	t.Helper()
	id := seedShop(t, d.Pool)
	d.shops = append(d.shops, id)
	return id
}

// The rows are committed rather than rolled back, because the send path resolves
// its template through a real pool instead of the transaction. The cleanup order
// is forced by the schema: an automation holds its template, so the template
// cannot go before the automation that points at it.
func sharedTemplateDB(t *testing.T) *scopedTemplateDB {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the shared template database tests")
	}
	if err := database.Migrate(context.Background(), url, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	d := &scopedTemplateDB{Pool: pool}
	t.Cleanup(func() {
		ctx := context.Background()
		ids := d.shops
		for _, q := range []string{
			`DELETE FROM automations WHERE shop_id = ANY($1::uuid[])`,
			`DELETE FROM templates WHERE shop_id = ANY($1::uuid[])`,
			`DELETE FROM shops WHERE id = ANY($1::uuid[])`,
		} {
			if _, err := pool.Exec(ctx, q, ids); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
		pool.Close()
	})
	return d
}

func TestSharedTemplateResolvesAcrossShops(t *testing.T) {
	ctx := context.Background()
	db := sharedTemplateDB(t)

	templateOwner := db.shop(t)
	automationShop := db.shop(t)

	templateID := uuid.New()
	if _, err := db.Exec(ctx, `
		INSERT INTO templates
			(id, shop_id, meta_template_name, language, category, status,
			 approval_status, meta_template_id, components, variables_map, source)
		VALUES ($1, $2, 'order_en_cours', 'fr', 'UTILITY', 'approved',
			'approved', '9988776655',
			'[{"type":"BODY","text":"Votre commande {{1}} est en route. Livreur: {{2}} Tel: {{3}}"}]'::jsonb,
			'["customer_name","driver_name","driver_phone"]'::jsonb, 'delivery')`,
		templateID, templateOwner); err != nil {
		t.Fatalf("insert template: %v", err)
	}

	customerID := seedCustomer(t, db, automationShop, "Radhwen Marayah", "+21693531118")
	automationID := uuid.New()
	if _, err := db.Exec(ctx, `
		INSERT INTO automations (id, shop_id, order_status, name, event_source, template_id)
		VALUES ($1, $2, 'in-progress', 'Out for delivery', 'delivery', $3)`,
		automationID, automationShop, templateID); err != nil {
		t.Fatalf("insert automation: %v", err)
	}

	p := newTestProcessor(t, db)
	outcome, err := p.send(ctx, SendInput{
		ShopID:         automationShop,
		AutomationID:   automationID,
		CustomerID:     customerID,
		TemplateID:     templateID,
		CustomerName:   "Radhwen Marayah",
		CustomerPhone:  "+21693531118",
		OrderID:        "6ab7cab7",
		StatusLabel:    "in-progress",
		TrackingCode:   "922153764102",
		IdempotencyKey: "msc:" + automationShop.String() + ":in-progress:922153764102",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	// A driverless parcel: the send stops at the missing variable, before any Meta
	// call. That makes the stop itself the proof - reaching it means the template
	// was found, so the shop that owns it did not get in the way.
	if outcome == outcomeTemplateMissing {
		t.Fatal("a template owned by another shop must still resolve: the editor offers it, so the send has to find it")
	}
	if outcome != outcomeHeldBack {
		t.Errorf("outcome = %q, want the send held back on the missing driver", outcome)
	}
}

// The template existing is not the same as it being usable, and the two failures
// must not read the same way to a merchant: a gone template is fixed by picking
// another one, a missing driver is the carrier's to supply.
func TestGoneTemplateIsReportedApartFromMissingData(t *testing.T) {
	ctx := context.Background()
	db := sharedTemplateDB(t)

	shop := db.shop(t)
	customer := seedCustomer(t, db, shop, "Badri Mondher", "+21698162832")
	automationID := seedAutomation(t, db, shop)

	p := newTestProcessor(t, db)
	outcome, err := p.send(ctx, SendInput{
		ShopID:         shop,
		AutomationID:   automationID,
		CustomerID:     customer,
		TemplateID:     uuid.New(), // an automation pointing at a deleted template
		CustomerName:   "Badri Mondher",
		CustomerPhone:  "+21698162832",
		TrackingCode:   "948758517362",
		IdempotencyKey: "msc:" + shop.String() + ":in-progress:948758517362",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if outcome != outcomeTemplateMissing {
		t.Errorf("outcome = %q, want the missing template named on its own", outcome)
	}
}

func newTestProcessor(t *testing.T, db *scopedTemplateDB) *Processor {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Processor{
		pool:     db.Pool,
		whatsapp: whatsapp.NewService(config.Config{}, db.Pool, log),
		log:      log,
	}
}

func TestDriverWithDefaults(t *testing.T) {
	cases := []struct {
		name, phone         string
		wantName, wantPhone string
	}{
		{"", "", DefaultDriverName, DefaultDriverPhone},
		{"Borhen edine ben khlifa", "", "Borhen edine ben khlifa", DefaultDriverPhone},
		{"", "29656683", DefaultDriverName, "29656683"},
		{"Borhen edine ben khlifa", "29656683", "Borhen edine ben khlifa", "29656683"},
	}
	for _, c := range cases {
		gotName, gotPhone := driverWithDefaults(c.name, c.phone)
		if gotName != c.wantName || gotPhone != c.wantPhone {
			t.Errorf("driverWithDefaults(%q, %q) = %q, %q; want %q, %q",
				c.name, c.phone, gotName, gotPhone, c.wantName, c.wantPhone)
		}
	}
}

// A courier the carrier never reports must not cost the customer the message.
// The template is marketing-flagged so the send gate refuses it: the point is
// that the send gets past variable resolution without a Meta call.
func TestDriverlessParcelIsNotHeldBack(t *testing.T) {
	ctx := context.Background()
	db := sharedTemplateDB(t)

	shop := db.shop(t)
	customer := seedCustomer(t, db, shop, "Badri Mondher", "+21698162832")
	barcode := "948758517362"

	templateID := uuid.New()
	if _, err := db.Exec(ctx, `
		INSERT INTO templates
			(id, shop_id, meta_template_name, language, category, status,
			 approval_status, marketing_flagged, components, variables_map, source)
		VALUES ($1, $2, 'ordre_en_cours', 'fr', 'UTILITY', 'approved', 'approved', true,
			'[{"type":"BODY","text":"Bonjour {{1}}, votre commande {{2}} est en route. Livreur: {{3}} Tel: {{4}}"}]'::jsonb,
			'["customer_name","tracking_code","driver_name","driver_phone"]'::jsonb, 'delivery')`,
		templateID, shop); err != nil {
		t.Fatalf("insert template: %v", err)
	}

	automationID := uuid.New()
	if _, err := db.Exec(ctx, `
		INSERT INTO automations (id, shop_id, order_status, name, event_source, template_id)
		VALUES ($1, $2, 'in-progress', 'Out for delivery', 'delivery', $3)`,
		automationID, shop, templateID); err != nil {
		t.Fatalf("insert automation: %v", err)
	}

	// A parcel the carrier has moved to in-progress without naming anyone.
	if _, err := db.Exec(ctx, `
		INSERT INTO delivery_orders
			(shop_id, barcode, order_id, customer_id, customer_name, customer_phone,
			 last_status, status_label, driver_name, driver_phone)
		VALUES ($1, $2, '6ab7cab7', $3, 'Badri Mondher', '+21698162832',
			'in-progress', 'En cours', '', '')`,
		shop, barcode, customer); err != nil {
		t.Fatalf("insert parcel: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := &Processor{
		pool:     db.Pool,
		whatsapp: whatsapp.NewService(config.Config{}, db.Pool, log),
		delivery: delivery.NewService(config.Config{}, db.Pool, log),
		log:      log,
	}

	res, err := p.RetrySuppressions(ctx, automationID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if res.Attempted != 1 {
		t.Fatalf("attempted = %d, want the driverless parcel to be tried", res.Attempted)
	}
	if res.StillHeld != 0 {
		t.Errorf("still held = %d, want 0: a missing courier must not cost the customer the message", res.StillHeld)
	}
	// The marketing flag is why the send stops here, and it must be reported as
	// a refusal rather than folded into "this order is missing data".
	if res.Rejected != 1 {
		t.Errorf("rejected = %d, want 1: a send the rules refuse is not the same as a held-back order", res.Rejected)
	}
	if res.TemplateMissing != 0 {
		t.Errorf("template missing = %d, want 0", res.TemplateMissing)
	}

	// Nothing recorded as held back, and no Meta call: the gate refused.
	rows, err := SuppressionsForAutomation(ctx, db.Pool, automationID, 50)
	if err != nil {
		t.Fatalf("read suppressions: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("suppressions = %d, want 0: the driver defaults should have satisfied the template", len(rows))
	}
}

// A held-back message is a promise that it goes out when the missing piece
// arrives. Once the courier is known the send happens by itself, and the row
// that said "waiting on this" has to go with it: left behind, the merchant is
// told to send a message the customer already received.
//
// The send here is scheduled rather than instant so it never reaches Meta - the
// point under test is what happens to the held-back row, not the carrier's reply.
//
//	go test ./internal/automations/ -run TestSentMessageClearsHeldBackRow -v
func TestSentMessageClearsHeldBackRow(t *testing.T) {
	ctx := context.Background()
	db := sharedTemplateDB(t)

	shop := db.shop(t)
	customer := seedCustomer(t, db, shop, "Radhwen Marayah", "+21693531118")

	templateID := uuid.New()
	if _, err := db.Exec(ctx, `
		INSERT INTO templates
			(id, shop_id, meta_template_name, language, category, status,
			 approval_status, marketing_flagged, components, variables_map, source)
		VALUES ($1, $2, 'ordre_en_cours', 'fr', 'UTILITY', 'approved', 'approved', false,
			'[{"type":"BODY","text":"Bonjour {{1}}, votre commande {{2}} est en route. Livreur: {{3}} Tel: {{4}}"}]'::jsonb,
			'["customer_name","tracking_code","driver_name","driver_phone"]'::jsonb, 'delivery')`,
		templateID, shop); err != nil {
		t.Fatalf("insert template: %v", err)
	}

	automationID := uuid.New()
	if _, err := db.Exec(ctx, `
		INSERT INTO automations (id, shop_id, order_status, name, event_source, template_id, delay_minutes)
		VALUES ($1, $2, 'in-progress', 'Out for delivery', 'delivery', $3, 30)`,
		automationID, shop, templateID); err != nil {
		t.Fatalf("insert automation: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := &Processor{
		pool:     db.Pool,
		whatsapp: whatsapp.NewService(config.Config{}, db.Pool, log),
		delivery: delivery.NewService(config.Config{}, db.Pool, log),
		log:      log,
	}
	automation, err := p.match(ctx, shop, SourceDelivery, "in-progress")
	if err != nil {
		t.Fatalf("match: %v", err)
	}

	in := SendInput{
		ShopID:         shop,
		AutomationID:   automationID,
		CustomerID:     customer,
		TemplateID:     templateID,
		CustomerName:   "Radhwen Marayah",
		CustomerPhone:  "+21693531118",
		OrderID:        "922153764102",
		StatusLabel:    "En cours",
		TrackingCode:   "922153764102",
		DriverName:     "Borhen edine ben khlifa",
		DriverPhone:    "29656683",
		IdempotencyKey: "msc:" + shop.String() + ":in-progress:922153764102",
		fire:           ruleFromSchedule(automation.Schedule()),
	}

	// The courier is unknown, so the message is held back.
	held := in
	held.DriverName, held.DriverPhone = "", ""
	p.RecordSuppression(ctx, held, []whatsapp.TokenKey{whatsapp.TokenDriverName, whatsapp.TokenDriverPhone})
	rows, err := SuppressionsForAutomation(ctx, db.Pool, automationID, 50)
	if err != nil || len(rows) != 1 {
		t.Fatalf("held-back rows = %d (err %v), want 1", len(rows), err)
	}

	outcome, err := p.send(ctx, in)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if outcome != outcomeScheduled {
		t.Fatalf("outcome = %q, want %q", outcome, outcomeScheduled)
	}

	rows, err = SuppressionsForAutomation(ctx, db.Pool, automationID, 50)
	if err != nil {
		t.Fatalf("read held-back rows: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("held-back rows = %d after the send, want 0: the customer has the message now", len(rows))
	}
}
