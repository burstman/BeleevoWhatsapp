package whatsapp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/database"
)

// These tests run the limiter's real upsert against a Postgres, in a transaction
// that is rolled back afterwards. Set TEST_DATABASE_URL to enable them.
//
//	go test ./internal/whatsapp/ -run TestDBRateLimit
func testDB(t *testing.T) database.Querier {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the rate limiter database tests")
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

// A send is counted against its own shop and the platform window, and once a
// window is full the next send is refused with the right scope.
func TestDBRateLimitRefusesTheSpentWindow(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	l := NewRateLimiter(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	l.perShop = 2
	l.global = 600
	shop := uuid.New()

	for i := 1; i <= 2; i++ {
		if err := l.Allow(ctx, shop); err != nil {
			t.Fatalf("send %d was refused inside the budget: %v", i, err)
		}
	}
	err := l.Allow(ctx, shop)
	var exceeded *RateLimitExceeded
	if !errors.As(err, &exceeded) {
		t.Fatalf("third send returned %v, want a rate limit error", err)
	}
	if !exceeded.PerShop {
		t.Fatal("a shop over its own budget must be reported as a per-shop limit")
	}

	// A send that never happened must not have spent the platform's capacity:
	// one merchant over their budget would otherwise be able to close the door
	// for everyone else.
	var globalCount int
	if err := db.QueryRow(ctx,
		`SELECT count FROM send_rate_windows WHERE scope = 'global'`).Scan(&globalCount); err != nil {
		t.Fatalf("read global window: %v", err)
	}
	if globalCount != 2 {
		t.Fatalf("platform window holds %d sends, want 2: a refused send was counted", globalCount)
	}

	// A different shop has its own budget, and the platform window still counts
	// what the first shop spent.
	if err := l.Allow(ctx, uuid.New()); err != nil {
		t.Fatalf("a second shop was refused by the first shop's budget: %v", err)
	}

	// The platform window: the sends above already spent three of it, so raise
	// the ceiling to exactly what is left and let one more shop through before
	// the platform budget runs out.
	l.perShop = 600
	l.global = 4
	if err := l.Allow(ctx, uuid.New()); err != nil {
		t.Fatalf("the last send inside the platform budget was refused: %v", err)
	}
	err = l.Allow(ctx, uuid.New())
	if !errors.As(err, &exceeded) || exceeded.PerShop {
		t.Fatalf("a shop with headroom must be told the platform budget is spent, got %v", err)
	}
}

func TestDBRateLimitWindowsAreTimeBoxed(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	l := NewRateLimiter(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	l.perShop = 1
	l.global = 600
	shop := uuid.New()

	if err := l.Allow(ctx, shop); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if err := l.Allow(ctx, shop); err == nil {
		t.Fatal("a second send in the same window must be refused")
	}

	// Move the limiter's clock into the next window: the budget is per window,
	// so the send must pass again.
	l.window = 500 * time.Millisecond
	if err := l.Allow(ctx, shop); err != nil {
		t.Fatalf("a send in a new window must pass, got %v", err)
	}

	var windows int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM send_rate_windows`).Scan(&windows); err != nil {
		t.Fatalf("count windows: %v", err)
	}
	if windows > 4 {
		t.Fatalf("%d window rows for one shop, want one or two per window", windows)
	}
}
