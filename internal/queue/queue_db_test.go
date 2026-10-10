package queue

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/database"
)

// These tests exercise the queue's real SQL — the jsonb payload, the leased
// claim, the dedupe index — against a Postgres that is rolled back afterwards.
// Set TEST_DATABASE_URL (any database you are happy to write to, since the
// migration also runs) to enable them; without it they skip.
//
//	go test ./internal/queue/ -run TestDB
func testDB(t *testing.T) database.Querier {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the queue database tests")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Migrate(ctx, url, logger); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	return tx
}

// A shop id that satisfies the foreign key, without touching real data: the
// transaction rolls back, so a synthetic row is safe.
func testShop(t *testing.T, db database.Querier) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.Exec(context.Background(),
		`INSERT INTO shops (id, name) VALUES ($1, $2)`, id, "queue test shop"); err != nil {
		t.Skipf("shops table unavailable: %v", err)
	}
	return id
}

func countJobs(t *testing.T, db database.Querier) int {
	t.Helper()
	var n int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM job_queue`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestDBEnqueueStoresPayloadAndDeduplicates(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	shop := testShop(t, db)

	params := Params{
		Kind:      TaskSendWhatsAppTemplate,
		ShopID:    shop,
		Payload:   []byte(`{"shop_id":"abc","customer_id":"def","order_id":"CVY-1"}`),
		DedupeKey: "conv:shop:paid:order-1",
	}
	if err := Enqueue(ctx, db, params); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// The same event arriving twice must not queue a second send.
	if err := Enqueue(ctx, db, params); err != nil {
		t.Fatalf("re-enqueue: %v", err)
	}
	if got := countJobs(t, db); got != 1 {
		t.Fatalf("queue holds %d rows after a duplicate event, want 1", got)
	}

	// A different event is a different row even for the same shop.
	params.DedupeKey = "conv:shop:shipped:order-1"
	if err := Enqueue(ctx, db, params); err != nil {
		t.Fatalf("enqueue second event: %v", err)
	}
	if got := countJobs(t, db); got != 2 {
		t.Fatalf("queue holds %d rows, want 2", got)
	}

	var payload []byte
	if err := db.QueryRow(ctx,
		`SELECT payload FROM job_queue WHERE dedupe_key = $1`, "conv:shop:paid:order-1").Scan(&payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	if !jsonEqual(payload, params.Payload) {
		t.Fatalf("stored payload %s, want %s", payload, params.Payload)
	}
}

func TestDBEnqueueHonoursRunAt(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	shop := testShop(t, db)

	due := time.Now().Add(-time.Minute)
	if err := Enqueue(ctx, db, Params{
		Kind: TaskPurgeNegativeTemplate, ShopID: shop,
		Payload: []byte(`{}`), RunAt: due,
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	var runAt time.Time
	if err := db.QueryRow(ctx, `SELECT run_at FROM job_queue`).Scan(&runAt); err != nil {
		t.Fatalf("read run_at: %v", err)
	}
	if !runAt.Before(time.Now()) {
		t.Fatalf("a job due in the past must stay claimable, got %s", runAt)
	}
}

// A job that is due is leased and run once, then gone. A job that is not due yet
// is invisible, and a job whose handler failed waits out its backoff instead of
// spinning.
func TestDBRunnerClaimsOnceReschedulesAndDeletes(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	shop := testShop(t, db)

	if err := Enqueue(ctx, db, Params{
		Kind: TaskSendWhatsAppTemplate, ShopID: shop, Payload: []byte(`{"n":1}`),
	}); err != nil {
		t.Fatalf("enqueue due job: %v", err)
	}
	if err := Enqueue(ctx, db, Params{
		Kind: TaskSendWhatsAppTemplate, ShopID: shop, Payload: []byte(`{"n":2}`),
		RunAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("enqueue future job: %v", err)
	}

	runner := NewRunner(db, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// A job whose kind nobody registered is dropped rather than retried forever.
	runner.Handle("send:unregistered", func(context.Context, []byte) error { return nil })
	ran := 0
	runner.Handle(TaskSendWhatsAppTemplate, func(context.Context, []byte) error {
		ran++
		return nil
	})
	if err := runner.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if ran != 1 {
		t.Fatalf("ran %d due jobs, want 1 (the future job must wait)", ran)
	}
	if got := countJobs(t, db); got != 1 {
		t.Fatalf("queue holds %d rows after a successful run, want only the future job", got)
	}
}

func TestDBRunnerRetriesAndGivesUp(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	shop := testShop(t, db)

	if err := Enqueue(ctx, db, Params{
		Kind: TaskSendWhatsAppTemplate, ShopID: shop, Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	runner := NewRunner(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	attempts := 0
	runner.Handle(TaskSendWhatsAppTemplate, func(context.Context, []byte) error {
		attempts++
		return errors.New("meta is down")
	})

	if err := runner.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("handler ran %d times, want 1", attempts)
	}

	// The row survived with its lease released and a future run_at, so the next
	// drain leaves it alone until the backoff is up.
	var leaseNull bool
	var retryAt time.Time
	var lastError string
	if err := db.QueryRow(ctx,
		`SELECT lease_until IS NULL, run_at, last_error FROM job_queue`).Scan(&leaseNull, &retryAt, &lastError); err != nil {
		t.Fatalf("read retried job: %v", err)
	}
	if !leaseNull {
		t.Fatal("a rescheduled job must not keep its lease, or nothing could ever claim it")
	}
	if !retryAt.After(time.Now()) {
		t.Fatalf("retry scheduled for %s, want a moment in the future", retryAt)
	}
	if lastError == "" {
		t.Fatal("the failure reason must be kept on the row for the operator")
	}

	// A second drain must not pick it up, and a permanent failure must not be
	// left behind at all.
	if err := runner.Drain(ctx); err != nil {
		t.Fatalf("drain again: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("handler ran %d times, want 1: the backoff was not respected", attempts)
	}
	runner.Handle(TaskSendWhatsAppTemplate, func(context.Context, []byte) error {
		return Permanent(errors.New("the gate refused this message"))
	})
	if _, err := db.Exec(ctx, `UPDATE job_queue SET run_at = now()`); err != nil {
		t.Fatalf("make job due: %v", err)
	}
	if err := runner.Drain(ctx); err != nil {
		t.Fatalf("drain permanent: %v", err)
	}
	if got := countJobs(t, db); got != 0 {
		t.Fatalf("a permanent failure left %d rows behind, want 0", got)
	}
}

// jsonEqual compares two JSON documents by value. Postgres jsonb does not keep
// the key order or the spacing it was given, so the stored bytes are decoded
// before they are compared.
func jsonEqual(a, b []byte) bool {
	var want, got any
	if err := json.Unmarshal(a, &want); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &got); err != nil {
		return false
	}
	return reflect.DeepEqual(want, got)
}
