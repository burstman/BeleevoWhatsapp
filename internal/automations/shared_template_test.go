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
