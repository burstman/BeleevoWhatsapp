-- +goose Up
-- Every Meta webhook POST is written here before it is processed, so a
-- delivery that is rejected (missing config, bad signature) or fails to map
-- to a connected number is visible instead of silently vanishing. This is the
-- inbox / status trail the settings page shows as "Recent webhook deliveries".
CREATE TABLE webhook_receipts (
    id              BIGSERIAL PRIMARY KEY,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    kind            TEXT NOT NULL DEFAULT 'lost',
    meta_message_id TEXT NOT NULL DEFAULT '',
    from_phone      TEXT NOT NULL DEFAULT '',
    phone_number_id TEXT NOT NULL DEFAULT '',
    shop_id         UUID,
    detail          TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_webhook_receipts_received ON webhook_receipts(received_at DESC);

-- +goose Down
DROP TABLE webhook_receipts;