-- +goose Up
CREATE TABLE converty_integrations (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id                 UUID NOT NULL UNIQUE REFERENCES shops(id) ON DELETE CASCADE,
    converty_store_id       TEXT NOT NULL DEFAULT '',
    store_name              TEXT NOT NULL DEFAULT '',
    store_slug              TEXT NOT NULL DEFAULT '',
    store_domain            TEXT NOT NULL DEFAULT '',
    store_currency          TEXT NOT NULL DEFAULT '',
    store_country           TEXT NOT NULL DEFAULT '',
    scopes                  TEXT NOT NULL DEFAULT '',
    access_token_encrypted  TEXT NOT NULL DEFAULT '',
    refresh_token_encrypted TEXT NOT NULL DEFAULT '',
    access_token_expires_at TIMESTAMPTZ,
    status                  TEXT NOT NULL DEFAULT 'connected',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_converty_integrations_shop_id ON converty_integrations(shop_id);

-- +goose Down
DROP TABLE IF EXISTS converty_integrations;