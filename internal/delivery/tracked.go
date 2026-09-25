package delivery

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TrackedOrder is one parcel the platform watches for a shop, so delivery
// status changes can fire order-event automations. Rows are created either
// automatically when a Converty order event carries a tracking barcode or
// manually from the Delivery settings page.
type TrackedOrder struct {
	ID            uuid.UUID
	ShopID        uuid.UUID
	Barcode       string
	OrderID       string
	CustomerID    uuid.UUID
	CustomerName  string
	CustomerPhone string
	LastStatus    string
	StatusLabel   string
	LastSeenAt    time.Time
}

// UpsertTracked records a parcel to watch. Already-known barcodes are
// refreshed (later order events can fill in the order id/customer that were
// missing on the first sighting).
func (s *Service) UpsertTracked(ctx context.Context, shopID uuid.UUID, barcode, orderID string, customerID uuid.UUID, customerName, customerPhone string) error {
	if customerName == "" {
		customerName = existingValue(ctx, s, shopID, barcode, "customer_name")
	}
	if customerPhone == "" {
		customerPhone = existingValue(ctx, s, shopID, barcode, "customer_phone")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO delivery_orders (shop_id, barcode, order_id, customer_id, customer_name, customer_phone)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (shop_id, barcode) DO UPDATE SET
			order_id       = COALESCE(NULLIF(EXCLUDED.order_id, ''), delivery_orders.order_id),
			customer_id    = COALESCE(delivery_orders.customer_id, EXCLUDED.customer_id),
			customer_name  = CASE WHEN EXCLUDED.customer_name = '' THEN delivery_orders.customer_name ELSE EXCLUDED.customer_name END,
			customer_phone = CASE WHEN EXCLUDED.customer_phone = '' THEN delivery_orders.customer_phone ELSE EXCLUDED.customer_phone END,
			updated_at     = now()`,
		shopID, barcode, orderID, nullableUUID(customerID), customerName, customerPhone,
	)
	return err
}

// existingValue reads a single column of an existing tracked row, used to
// preserve customer info on an upsert when the new sighting carries none.
func existingValue(ctx context.Context, s *Service, shopID uuid.UUID, barcode, column string) string {
	var v string
	_ = s.pool.QueryRow(ctx,
		`SELECT `+column+` FROM delivery_orders WHERE shop_id = $1 AND barcode = $2`,
		shopID, barcode,
	).Scan(&v)
	return v
}

// Tracked returns every watched parcel for a shop, most recently seen first.
func (s *Service) Tracked(ctx context.Context, shopID uuid.UUID) ([]TrackedOrder, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, shop_id, barcode, order_id, customer_id, customer_name, customer_phone,
		       last_status, status_label, last_seen_at
		FROM delivery_orders
		WHERE shop_id = $1
		ORDER BY last_seen_at DESC`,
		shopID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TrackedOrder
	for rows.Next() {
		var t TrackedOrder
		if err := rows.Scan(&t.ID, &t.ShopID, &t.Barcode, &t.OrderID, &t.CustomerID, &t.CustomerName,
			&t.CustomerPhone, &t.LastStatus, &t.StatusLabel, &t.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TrackedByBarcode returns one watched parcel scoped to a shop.
func (s *Service) TrackedByBarcode(ctx context.Context, shopID uuid.UUID, barcode string) (TrackedOrder, error) {
	var t TrackedOrder
	err := s.pool.QueryRow(ctx, `
		SELECT id, shop_id, barcode, order_id, customer_id, customer_name, customer_phone,
		       last_status, status_label, last_seen_at
		FROM delivery_orders
		WHERE shop_id = $1 AND barcode = $2`,
		shopID, barcode,
	).Scan(&t.ID, &t.ShopID, &t.Barcode, &t.OrderID, &t.CustomerID, &t.CustomerName,
		&t.CustomerPhone, &t.LastStatus, &t.StatusLabel, &t.LastSeenAt)
	if err == pgx.ErrNoRows {
		return TrackedOrder{}, nil
	}
	return t, err
}

// UpdateTrackedStatus persists an observed status transition.
func (s *Service) UpdateTrackedStatus(ctx context.Context, shopID uuid.UUID, barcode, status, label string) error {
	if label == "" {
		label = LabelFor(status)
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE delivery_orders
		SET last_status = $3, status_label = $4, last_seen_at = now(), updated_at = now()
		WHERE shop_id = $1 AND barcode = $2`,
		shopID, barcode, status, label,
	)
	return err
}

// TouchTracked bumps last_seen_at without changing the status (an unchanged
// observation), so the UI's "last seen" stays fresh.
func (s *Service) TouchTracked(ctx context.Context, shopID uuid.UUID, barcode string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE delivery_orders
		SET last_seen_at = now()
		WHERE shop_id = $1 AND barcode = $2`,
		shopID, barcode,
	)
	return err
}

// RemoveTracked stops watching a parcel.
func (s *Service) RemoveTracked(ctx context.Context, shopID uuid.UUID, barcode string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM delivery_orders
		WHERE shop_id = $1 AND barcode = $2`,
		shopID, barcode,
	)
	return err
}

func nullableUUID(u uuid.UUID) any {
	if u == uuid.Nil {
		return nil
	}
	return u
}