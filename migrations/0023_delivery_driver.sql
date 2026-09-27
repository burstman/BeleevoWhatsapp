-- +goose Up
-- The deliveryman arrives on the parcel only once the carrier has assigned one,
-- which is often after the status change the automation triggers on. Keeping the
-- last seen driver on the parcel lets the Delivery page answer "is a driver
-- attached yet?" and lets a later poll restore the name a socket event carried
-- but an empty REST response would otherwise drop.
ALTER TABLE delivery_orders ADD COLUMN IF NOT EXISTS driver_name TEXT NOT NULL DEFAULT '';
ALTER TABLE delivery_orders ADD COLUMN IF NOT EXISTS driver_phone TEXT NOT NULL DEFAULT '';

-- A barcode the carrier stops listing has been deleted on their side. A single
-- miss is not proof: a parcel can be briefly unknown while the carrier replicates
-- it, and marking it finished on one answer would stop tracking a live parcel
-- forever. Consecutive misses are counted instead, and any sighting resets the
-- count, so only a barcode that stays unknown across several sweeps settles.
ALTER TABLE delivery_orders ADD COLUMN IF NOT EXISTS missing_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE delivery_orders ADD COLUMN IF NOT EXISTS last_missing_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE delivery_orders DROP COLUMN IF EXISTS last_missing_at;
ALTER TABLE delivery_orders DROP COLUMN IF EXISTS missing_count;
ALTER TABLE delivery_orders DROP COLUMN IF EXISTS driver_phone;
ALTER TABLE delivery_orders DROP COLUMN IF EXISTS driver_name;
