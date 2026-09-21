-- +goose Up
-- Deduplicate deliveries by content hash (the Converty event_id shape is not
-- documented yet), keep the order status handy for automations, and allow
-- events that cannot yet be attributed to a shop to still be stored.
ALTER TABLE order_events
    ADD COLUMN order_status TEXT NOT NULL DEFAULT '',
    ADD COLUMN payload_hash TEXT NOT NULL DEFAULT '';

ALTER TABLE order_events ALTER COLUMN shop_id DROP NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS order_events_payload_hash_key ON order_events(payload_hash);

-- +goose Down
DROP INDEX IF EXISTS order_events_payload_hash_key;

ALTER TABLE order_events ALTER COLUMN shop_id SET NOT NULL;

ALTER TABLE order_events
    DROP COLUMN IF EXISTS payload_hash,
    DROP COLUMN IF EXISTS order_status;