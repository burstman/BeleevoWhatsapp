-- +goose Up
-- A shop account can connect several Converty stores. Drop the one-integration
-- UNIQUE on shop_id and add an active flag so integrations can be paused
-- without deleting them. The partial unique index keeps one row per store per
-- shop while still allowing rows whose store id is unknown (empty).
ALTER TABLE converty_integrations
    DROP CONSTRAINT IF EXISTS converty_integrations_shop_id_key;

ALTER TABLE converty_integrations
    ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;

CREATE UNIQUE INDEX IF NOT EXISTS idx_converty_integrations_shop_store
    ON converty_integrations(shop_id, converty_store_id)
    WHERE converty_store_id <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_converty_integrations_shop_store;
ALTER TABLE converty_integrations DROP COLUMN IF EXISTS active;
ALTER TABLE converty_integrations ADD CONSTRAINT converty_integrations_shop_id_key UNIQUE (shop_id);