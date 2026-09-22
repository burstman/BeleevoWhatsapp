-- +goose Up
-- Link sent messages to the customer they concerned and snapshot the template
-- variables used, so delivery status can be attributed back to a customer and
-- audit/history can render without re-reading live template state.
ALTER TABLE messages
    ADD COLUMN customer_id      UUID REFERENCES customers(id) ON DELETE SET NULL,
    ADD COLUMN template_variables JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN idempotency_key  TEXT NOT NULL DEFAULT '',
    ADD COLUMN meta_errors      JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX idx_messages_customer_id ON messages(customer_id);
CREATE UNIQUE INDEX idx_messages_idempotency_key ON messages(shop_id, idempotency_key)
    WHERE idempotency_key <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_messages_idempotency_key;
DROP INDEX IF EXISTS idx_messages_customer_id;

ALTER TABLE messages
    DROP COLUMN IF EXISTS meta_errors,
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS template_variables,
    DROP COLUMN IF EXISTS customer_id;