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
	DriverName    string // last driver the carrier reported, may legitimately be ""
	DriverPhone   string
	MissingCount  int // consecutive sweeps the carrier did not know this barcode
	LastMissingAt *time.Time
	LastSeenAt    time.Time
}

// MissingFor reports how long the carrier has not listed the parcel, or 0 when it
// answered the last sweep.
func (t TrackedOrder) MissingFor(now time.Time) time.Duration {
	if t.LastMissingAt == nil {
		return 0
	}
	return now.Sub(*t.LastMissingAt)
}

// HasDriver reports whether a deliveryman is attached to the parcel. A delivery
// template that places {{driver_name}} or {{driver_phone}} cannot send while
// this is false, which is worth seeing on the Delivery page.
func (t TrackedOrder) HasDriver() bool {
	return t.DriverName != "" || t.DriverPhone != ""
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
		       last_status, status_label, driver_name, driver_phone,
		       missing_count, last_missing_at, last_seen_at
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
			&t.CustomerPhone, &t.LastStatus, &t.StatusLabel, &t.DriverName, &t.DriverPhone,
			&t.MissingCount, &t.LastMissingAt, &t.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TrackedByBarcode returns one watched parcel scoped to a shop. The scan list has
// to name every column selected: pgx refuses a mismatched count, and a caller
// that reads a failed lookup as "no such parcel" turns that into a silently
// dropped send rather than a visible error.
func (s *Service) TrackedByBarcode(ctx context.Context, shopID uuid.UUID, barcode string) (TrackedOrder, error) {
	var t TrackedOrder
	err := s.pool.QueryRow(ctx, `
		SELECT id, shop_id, barcode, order_id, customer_id, customer_name, customer_phone,
		       last_status, status_label, driver_name, driver_phone,
		       missing_count, last_missing_at, last_seen_at
		FROM delivery_orders
		WHERE shop_id = $1 AND barcode = $2`,
		shopID, barcode,
	).Scan(&t.ID, &t.ShopID, &t.Barcode, &t.OrderID, &t.CustomerID, &t.CustomerName,
		&t.CustomerPhone, &t.LastStatus, &t.StatusLabel, &t.DriverName, &t.DriverPhone,
		&t.MissingCount, &t.LastMissingAt, &t.LastSeenAt)
	if err == pgx.ErrNoRows {
		return TrackedOrder{}, nil
	}
	return t, err
}

// TrackedAnyShopByBarcode finds a watched parcel by barcode alone. The carrier
// account is the operator's, so a socket connection opened for one shop can
// carry events for parcels another shop registered; the row says which shop
// owns it, and the transition has to be recorded there.
func (s *Service) TrackedAnyShopByBarcode(ctx context.Context, barcode string) (TrackedOrder, error) {
	var t TrackedOrder
	err := s.pool.QueryRow(ctx, `
		SELECT id, shop_id, barcode, order_id, customer_id, customer_name, customer_phone,
		       last_status, status_label, driver_name, driver_phone,
		       missing_count, last_missing_at, last_seen_at
		FROM delivery_orders
		WHERE barcode = $1
		ORDER BY last_seen_at DESC
		LIMIT 1`,
		barcode,
	).Scan(&t.ID, &t.ShopID, &t.Barcode, &t.OrderID, &t.CustomerID, &t.CustomerName,
		&t.CustomerPhone, &t.LastStatus, &t.StatusLabel, &t.DriverName, &t.DriverPhone,
		&t.MissingCount, &t.LastMissingAt, &t.LastSeenAt)
	if err == pgx.ErrNoRows {
		return TrackedOrder{}, nil
	}
	return t, err
}

// UpdateTrackedStatus persists an observed status transition along with the
// driver the carrier reported, if any. An empty observation keeps whatever was
// stored before: the REST poll and the socket both feed this funnel and a
// partial answer (a status without a deliveryman) must not erase a name another
// call already supplied.
func (s *Service) UpdateTrackedStatus(ctx context.Context, shopID uuid.UUID, barcode, status, label, driverName, driverPhone string) error {
	if label == "" {
		label = LabelFor(status)
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE delivery_orders
		SET last_status = $3, status_label = $4,
		    driver_name = CASE WHEN $5 = '' THEN delivery_orders.driver_name ELSE $5 END,
		    driver_phone = CASE WHEN $6 = '' THEN delivery_orders.driver_phone ELSE $6 END,
		    missing_count = 0, last_missing_at = NULL,
		    last_seen_at = now(), updated_at = now()
		WHERE shop_id = $1 AND barcode = $2`,
		shopID, barcode, status, label, driverName, driverPhone,
	)
	return err
}

// AttachDriver stores a courier the carrier reported without touching the status
// or its label. Mes Colis pushes a courier attached after the parcel is already
// moving as the *same* status with a driver attached, and the label it showed us
// earlier is the merchant's wording, so overwriting it here would throw away a
// better label than the one we would put back.
func (s *Service) AttachDriver(ctx context.Context, shopID uuid.UUID, barcode, driverName, driverPhone string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE delivery_orders
		SET driver_name = CASE WHEN $3 = '' THEN delivery_orders.driver_name ELSE $3 END,
		    driver_phone = CASE WHEN $4 = '' THEN delivery_orders.driver_phone ELSE $4 END,
		    last_seen_at = now(), updated_at = now()
		WHERE shop_id = $1 AND barcode = $2`,
		shopID, barcode, driverName, driverPhone,
	)
	return err
}

// MarkMissingUpstream records that the carrier did not know the barcode. Only
// after missesBeforeMissedInARow consecutive misses is the parcel settled as
// StatusRemovedUpstream, so one replication hiccup cannot silently end tracking.
func (s *Service) MarkMissingUpstream(ctx context.Context, shopID uuid.UUID, barcode string, missesBefore int) (settled bool, err error) {
	if missesBefore < 1 {
		missesBefore = 1
	}
	var label string
	// last_seen_at is left alone on purpose: it answers "when did the carrier last
	// know this parcel", and a miss is not a sighting. last_missing_at carries the
	// other half of the picture.
	err = s.pool.QueryRow(ctx, `
		UPDATE delivery_orders
		SET missing_count = missing_count + 1,
		    last_missing_at = now(),
		    last_status = CASE WHEN missing_count + 1 >= $3 THEN $4 ELSE delivery_orders.last_status END,
		    status_label = CASE WHEN missing_count + 1 >= $3 THEN $5 ELSE delivery_orders.status_label END,
		    updated_at = now()
		WHERE shop_id = $1 AND barcode = $2
		RETURNING last_status`,
		shopID, barcode, missesBefore, StatusRemovedUpstream, LabelFor(StatusRemovedUpstream),
	).Scan(&label)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return label == StatusRemovedUpstream, nil
}

// TouchTracked bumps last_seen_at without changing the status (an unchanged
// observation), so the UI's "last seen" stays fresh.
func (s *Service) TouchTracked(ctx context.Context, shopID uuid.UUID, barcode string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE delivery_orders
		SET last_seen_at = now(), missing_count = 0, last_missing_at = NULL
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
