-- +goose Up
-- Each Converty OAuth app belongs to one of the merchant's own stores; the
-- operator does not share one client id/secret across the platform. Store the
-- client's own Converty app credentials per integration (the secret encrypted
-- at rest like tokens). A pending row first records the credentials, and the
-- OAuth callback later fills in the store + tokens.
ALTER TABLE converty_integrations
    ADD COLUMN IF NOT EXISTS converty_client_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS converty_client_secret_encrypted TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE converty_integrations
    DROP COLUMN IF EXISTS converty_client_secret_encrypted;
ALTER TABLE converty_integrations
    DROP COLUMN IF EXISTS converty_client_id;