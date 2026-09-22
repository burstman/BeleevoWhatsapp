-- +goose Up
-- The platform owns a pool of rentable WhatsApp numbers (BSP model). Each
-- is a real number registered on the platform's Meta messaging account; a
-- workshop picks an available number during onboarding and the platform
-- provisions the connection (no client-side Meta access needed).
CREATE TABLE whatsapp_numbers (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_phone_number TEXT NOT NULL,
    phone_number_id      TEXT NOT NULL,
    messaging_account_id TEXT NOT NULL,
    waac_id              TEXT NOT NULL DEFAULT '',
    verified_name        TEXT NOT NULL DEFAULT '',
    status               TEXT NOT NULL DEFAULT 'available', -- available | assigned
    shop_id              UUID REFERENCES shops(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_whatsapp_numbers_phone_number_id ON whatsapp_numbers(phone_number_id);
CREATE INDEX idx_whatsapp_numbers_status ON whatsapp_numbers(status);

-- +goose Down
DROP TABLE whatsapp_numbers;