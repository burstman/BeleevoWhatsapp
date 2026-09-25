-- +goose Up
-- Delivery integrations (Mescolis first) are per-shop connections holding the
-- provider access token, encrypted at rest exactly like the WhatsApp and
-- Converty credentials. The polling reconcile loop uses these to query parcel
-- statuses so order-event automations can also trigger on delivery events.
CREATE TABLE delivery_integrations (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id                UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    provider               TEXT NOT NULL,
    access_token_encrypted TEXT NOT NULL,
    account_code           TEXT NOT NULL DEFAULT '',
    allow_sub_account      BOOLEAN NOT NULL DEFAULT false,
    connected_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (shop_id, provider)
);

CREATE INDEX idx_delivery_integrations_shop_id ON delivery_integrations(shop_id);

-- Parcels the poller watches per shop. Rows are created automatically when a
-- Converty order event carries a tracking barcode, or manually from the
-- Delivery settings page. The customer snapshot lets delivery status changes
-- trigger automated WhatsApp sends to the order's recipient.
CREATE TABLE delivery_orders (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id        UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    barcode        TEXT NOT NULL,
    order_id       TEXT NOT NULL DEFAULT '',
    customer_id    UUID REFERENCES customers(id) ON DELETE SET NULL,
    customer_name  TEXT NOT NULL DEFAULT '',
    customer_phone TEXT NOT NULL DEFAULT '',
    last_status    TEXT NOT NULL DEFAULT '',
    status_label   TEXT NOT NULL DEFAULT '',
    last_seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (shop_id, barcode)
);

CREATE INDEX idx_delivery_orders_shop_id ON delivery_orders(shop_id);

-- Automations now trigger on events from two sources: Converty order events
-- (the original order_status semantics) and delivery-provider events. The
-- trigger key stays in order_status; event_source disambiguates which origin
-- it refers to, so the same status string can be automated for both.
ALTER TABLE automations ADD COLUMN IF NOT EXISTS event_source TEXT NOT NULL DEFAULT 'converty';
ALTER TABLE automations DROP CONSTRAINT IF EXISTS automations_shop_id_order_status_key;
ALTER TABLE automations
    ADD CONSTRAINT automations_shop_source_status_key UNIQUE (shop_id, event_source, order_status);

-- +goose Down
ALTER TABLE automations DROP CONSTRAINT IF EXISTS automations_shop_source_status_key;
ALTER TABLE automations ADD CONSTRAINT automations_shop_id_order_status_key UNIQUE (shop_id, order_status);
ALTER TABLE automations DROP COLUMN IF EXISTS event_source;

DROP TABLE IF EXISTS delivery_orders;
DROP TABLE IF EXISTS delivery_integrations;