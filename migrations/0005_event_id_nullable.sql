-- +goose Up
-- The Converty delivery event_id is not exposed yet, so event_id was stored
-- as ''. With shop attribution that made UNIQUE(shop_id, event_id) collide
-- across different payloads of the same order. NULLs are distinct in a
-- Postgres unique index, so store NULL instead; body-hash dedup remains the
-- primary idempotency key.
ALTER TABLE order_events ALTER COLUMN event_id DROP NOT NULL;

-- +goose Down
ALTER TABLE order_events ALTER COLUMN event_id SET NOT NULL;