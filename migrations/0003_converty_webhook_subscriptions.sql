-- +goose Up
ALTER TABLE converty_integrations
    ADD COLUMN webhook_subscriptions jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE converty_integrations
    DROP COLUMN IF EXISTS webhook_subscriptions;
