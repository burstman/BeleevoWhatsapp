package delivery

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
)

// Reading a watched parcel is the step a delivery automation depends on: the
// driver a carrier reports is stored there, and the send is built from it.
//
// TrackedByBarcode once selected the whole row but scanned ten of the fourteen
// columns, so pgx rejected the call and every parcel read failed. Nothing logged
// an error - the callers treat a failed read as "no such parcel" - which left
// every delivery automation unable to send, with a message history that looked
// empty rather than broken.
//
//	go test ./internal/delivery/ -run TestTrackedByBarcode -v
func TestTrackedByBarcode(t *testing.T) {
	ctx := context.Background()
	pool := deliveryTestDB(t)
	shop := uuid.New()
	barcode := "922153764102"

	if _, err := pool.Exec(ctx, `INSERT INTO shops (id, name) VALUES ($1, 'tracked read test')`, shop); err != nil {
		t.Fatalf("insert shop: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO delivery_orders
			(shop_id, barcode, order_id, customer_name, customer_phone, last_status,
			 status_label, driver_name, driver_phone, missing_count)
		VALUES ($1, $2, '6ab7cab7', 'Radhwen Marayah', '+21693531118', 'in-progress',
			'En cours', 'Borhen edine ben khlifa', '29656683', 2)`,
		shop, barcode); err != nil {
		t.Fatalf("insert parcel: %v", err)
	}

	tracked, err := pool2Service(t, pool).TrackedByBarcode(ctx, shop, barcode)
	if err != nil {
		t.Fatalf("TrackedByBarcode: %v", err)
	}
	if tracked.ID == uuid.Nil {
		t.Fatal("a stored parcel must be readable; this is the read a send depends on")
	}
	if tracked.DriverName != "Borhen edine ben khlifa" || tracked.DriverPhone != "29656683" {
		t.Errorf("driver = %q / %q, want the driver the carrier reported - a send is built from these",
			tracked.DriverName, tracked.DriverPhone)
	}
	if tracked.LastStatus != "in-progress" || tracked.StatusLabel != "En cours" {
		t.Errorf("status = %q / %q, want the transition the automation matched", tracked.LastStatus, tracked.StatusLabel)
	}
	if tracked.CustomerName != "Radhwen Marayah" || tracked.CustomerPhone != "+21693531118" {
		t.Errorf("customer = %q / %q, want the recipient the message goes to", tracked.CustomerName, tracked.CustomerPhone)
	}
	if tracked.MissingCount != 2 {
		t.Errorf("missing_count = %d, want 2: the trailing columns are as load-bearing as the driver", tracked.MissingCount)
	}

	// The other direction, used when a socket opened for one shop carries an
	// event for a parcel another shop registered.
	any, err := pool2Service(t, pool).TrackedAnyShopByBarcode(ctx, barcode)
	if err != nil {
		t.Fatalf("TrackedAnyShopByBarcode: %v", err)
	}
	if any.ID == uuid.Nil || any.ShopID != shop {
		t.Errorf("cross-shop read = %v / shop %v, want the parcel and the shop that owns it", any.ID, any.ShopID)
	}
	if any.DriverName != "Borhen edine ben khlifa" {
		t.Errorf("cross-shop driver = %q, want the driver", any.DriverName)
	}
}

// A parcel that does not exist is an ordinary answer, not an error: the poller
// sees parcels it has never stored and the callers branch on an empty row.
func TestTrackedByBarcodeMissingParcelIsNotAnError(t *testing.T) {
	ctx := context.Background()
	pool := deliveryTestDB(t)
	shop := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO shops (id, name) VALUES ($1, 'empty read test')`, shop); err != nil {
		t.Fatalf("insert shop: %v", err)
	}

	tracked, err := pool2Service(t, pool).TrackedByBarcode(ctx, shop, "000000000000")
	if err != nil {
		t.Fatalf("an unknown parcel must read as empty, got %v", err)
	}
	if tracked.ID != uuid.Nil {
		t.Errorf("id = %v, want the zero row", tracked.ID)
	}
}

func deliveryTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the delivery database tests")
	}
	if err := database.Migrate(context.Background(), url, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func pool2Service(t *testing.T, pool *pgxpool.Pool) *Service {
	t.Helper()
	return NewService(config.Config{}, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
}
