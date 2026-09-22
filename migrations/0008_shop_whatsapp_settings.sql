-- +goose Up
-- The platform bills WhatsApp through ONE centrally owned WABA + number. A
-- merchant does not connect its own WABA; instead it opts into the platform
-- service and explicitly accepts the WhatsApp service terms (which cover the
-- customer opt-in/consent obligations). These two flags gate every send.
ALTER TABLE shops
    ADD COLUMN phone                   TEXT NOT NULL DEFAULT '',
    ADD COLUMN whatsapp_terms_accepted_at TIMESTAMPTZ,
    ADD COLUMN whatsapp_enabled        BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE shops
    DROP COLUMN IF EXISTS whatsapp_enabled,
    DROP COLUMN IF EXISTS whatsapp_terms_accepted_at,
    DROP COLUMN IF EXISTS phone;