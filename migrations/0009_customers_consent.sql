-- +goose Up
-- Customers that a merchant may message, and the per-customer WhatsApp
-- opt-in record.
--
-- IMPORTANT: a phone number in the customers table does NOT mean WhatsApp
-- opt-in. Every conversation must check whatsapp_consent.
CREATE TABLE customers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id     UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    name        TEXT NOT NULL DEFAULT '',
    phone       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (shop_id, phone)
);

CREATE INDEX idx_customers_shop_id ON customers(shop_id);

CREATE TABLE whatsapp_consent (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id     UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'opt_in', -- opt_in | revoked
    category    TEXT NOT NULL DEFAULT 'order_updates', -- purpose of the opt-in
    source      TEXT NOT NULL DEFAULT 'merchant_declared',
    evidence    TEXT NOT NULL DEFAULT '', -- external reference/consent proof if available
    obtained_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_whatsapp_consent_shop_customer ON whatsapp_consent(shop_id, customer_id);
CREATE INDEX idx_whatsapp_consent_customer ON whatsapp_consent(customer_id);

-- +goose Down
DROP TABLE IF EXISTS whatsapp_consent;
DROP TABLE IF EXISTS customers;