package automations

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/database"
	"whatsappconverty/internal/whatsapp"
)

// This runs against a Postgres in a transaction that is rolled back afterwards.
// Set TEST_DATABASE_URL to enable it.
//
//	go test ./internal/automations/ -run TestDBSuppressions
//
// The rollback matters more here than elsewhere: the retry path deletes rows and
// the insert path is expected to be deduped by a real unique index, which is
// only a real constraint if a real Postgres enforces it.
func testDB(t *testing.T) database.Querier {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the suppression database tests")
	}
	if err := database.Migrate(context.Background(), url, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	return tx
}

func seedShop(t *testing.T, db database.Querier) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.Exec(context.Background(), `INSERT INTO shops (id, name) VALUES ($1, $2)`, id, "suppression test shop"); err != nil {
		t.Fatalf("insert shop: %v", err)
	}
	return id
}

func seedCustomer(t *testing.T, db database.Querier, shopID uuid.UUID, name, phone string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO customers (id, shop_id, name, phone) VALUES ($1, $2, $3, $4)`,
		id, shopID, name, phone); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	return id
}

func seedAutomation(t *testing.T, db database.Querier, shopID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO automations (id, shop_id, order_status, name, event_source)
		VALUES ($1, $2, 'in-progress', 'Out for delivery', 'delivery')`,
		id, shopID); err != nil {
		t.Fatalf("insert automation: %v", err)
	}
	return id
}

// The recorded row has to carry enough to be retried without the webhook that
// produced it, so the test asserts the fields the retry reads back rather than
// just that a row exists.
func TestDBSuppressionRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	shop := seedShop(t, db)
	customer := seedCustomer(t, db, shop, "Radhwen Marayah", "+21693531118")
	automation := seedAutomation(t, db, shop)

	insert := func(key string) {
		t.Helper()
		if _, err := db.Exec(ctx, `
			INSERT INTO automation_suppressions
				(shop_id, automation_id, customer_id, reason, missing_variables,
				 trigger_label, tracking_code, order_id, idempotency_key)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9)
			ON CONFLICT (automation_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`,
			shop, automation, customer, "a variable the template places has no value for this order yet",
			`["driver_name","driver_phone"]`, "En cours", "922153764102", "6ab7cab7", key,
		); err != nil {
			t.Fatalf("insert suppression: %v", err)
		}
	}
	insert("msc:shop:in-progress:922153764102")
	// The poller can re-report an event; the count must not inflate.
	insert("msc:shop:in-progress:922153764102")

	rows, err := SuppressionsForAutomation(ctx, db, automation, 200)
	if err != nil {
		t.Fatalf("read suppressions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("a re-reported event must be recorded once, got %d rows", len(rows))
	}

	got := rows[0]
	if got.CustomerName != "Radhwen Marayah" {
		t.Errorf("customer name = %q, want the name the send was for", got.CustomerName)
	}
	if got.TrackingCode != "922153764102" || got.OrderID != "6ab7cab7" {
		t.Errorf("retry needs the tracking code and order id, got %q / %q", got.TrackingCode, got.OrderID)
	}
	if len(got.Missing) != 2 || got.Missing[0] != whatsapp.TokenDriverName {
		t.Errorf("missing variables = %v, want the driver pair", got.Missing)
	}
	if got.IdempotencyKey == "" {
		t.Error("a retry reuses this key to avoid double-sending; it must be stored")
	}
	if got.TriggerLabel != "En cours" {
		t.Errorf("trigger label = %q, want the status label shown in the message", got.TriggerLabel)
	}
	if got.CreatedAt.IsZero() || time.Since(got.CreatedAt) > time.Hour {
		t.Errorf("created_at = %v, want roughly now", got.CreatedAt)
	}
}

// One automation's held-back list must not leak into another's, the same way the
// message history is scoped.
func TestDBSuppressionsAreScopedToOneAutomation(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	shop := seedShop(t, db)
	mine := seedAutomation(t, db, shop)
	other := uuid.New()
	if _, err := db.Exec(ctx, `
		INSERT INTO automations (id, shop_id, order_status, name, event_source)
		VALUES ($1, $2, 'delivered', 'Delivered', 'delivery')`, other, shop); err != nil {
		t.Fatalf("insert automation: %v", err)
	}

	seed := func(automationID uuid.UUID, key string) {
		t.Helper()
		if _, err := db.Exec(ctx, `
			INSERT INTO automation_suppressions (shop_id, automation_id, reason, idempotency_key)
			VALUES ($1, $2, 'a variable the template places has no value for this order yet', $3)`,
			shop, automationID, key); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	seed(mine, "msc:a")
	seed(other, "msc:b")

	rows, err := SuppressionsForAutomation(ctx, db, mine, 200)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want only this automation's 1", len(rows))
	}
	if rows[0].IdempotencyKey != "msc:a" {
		t.Errorf("key = %q, want the seeded one", rows[0].IdempotencyKey)
	}
}
