-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE shops (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id        UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    email          TEXT NOT NULL UNIQUE,
    password_hash  TEXT NOT NULL,
    name           TEXT NOT NULL DEFAULT '',
    role           TEXT NOT NULL DEFAULT 'owner',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_shop_id ON users(shop_id);

CREATE TABLE auth_sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_auth_sessions_user_id ON auth_sessions(user_id);

CREATE TABLE whatsapp_integrations (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id                UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    waac_id                TEXT NOT NULL DEFAULT '',
    phone_number_id        TEXT NOT NULL DEFAULT '',
    messaging_account_id   TEXT NOT NULL DEFAULT '',
    business_portfolio_id  TEXT NOT NULL DEFAULT '',
    phone_number           TEXT NOT NULL DEFAULT '',
    access_token_encrypted TEXT NOT NULL DEFAULT '',
    status                 TEXT NOT NULL DEFAULT 'disconnected',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_whatsapp_integrations_shop_id ON whatsapp_integrations(shop_id);

CREATE TABLE templates (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    whatsapp_integration_id  UUID NOT NULL REFERENCES whatsapp_integrations(id) ON DELETE CASCADE,
    meta_template_name       TEXT NOT NULL,
    language                 TEXT NOT NULL DEFAULT 'en_US',
    category                 TEXT NOT NULL DEFAULT '',
    status                   TEXT NOT NULL DEFAULT 'pending',
    components               JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_templates_integration_id ON templates(whatsapp_integration_id);

CREATE TABLE automations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id       UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    order_status  TEXT NOT NULL,
    template_id   UUID REFERENCES templates(id) ON DELETE SET NULL,
    enabled       BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (shop_id, order_status)
);

CREATE INDEX idx_automations_shop_id ON automations(shop_id);

CREATE TABLE messages (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id                  UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    whatsapp_integration_id  UUID REFERENCES whatsapp_integrations(id) ON DELETE SET NULL,
    converty_order_id        TEXT NOT NULL DEFAULT '',
    recipient_phone          TEXT NOT NULL,
    template_id              UUID REFERENCES templates(id) ON DELETE SET NULL,
    meta_message_id          TEXT NOT NULL DEFAULT '',
    status                   TEXT NOT NULL DEFAULT 'queued',
    error_code               TEXT NOT NULL DEFAULT '',
    error_message            TEXT NOT NULL DEFAULT '',
    sent_at                  TIMESTAMPTZ,
    delivered_at             TIMESTAMPTZ,
    read_at                  TIMESTAMPTZ,
    failed_at                TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_messages_shop_id ON messages(shop_id);
CREATE INDEX idx_messages_status ON messages(status);
CREATE INDEX idx_messages_meta_message_id ON messages(meta_message_id);
CREATE INDEX idx_messages_created_at ON messages(created_at DESC);

CREATE TABLE order_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id       UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    event_id      TEXT NOT NULL,
    event_type    TEXT NOT NULL,
    order_id      TEXT NOT NULL,
    payload       JSONB NOT NULL DEFAULT '{}'::jsonb,
    processed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (shop_id, event_id)
);

CREATE INDEX idx_order_events_shop_id ON order_events(shop_id);

-- +goose Down
DROP TABLE IF EXISTS order_events;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS automations;
DROP TABLE IF EXISTS templates;
DROP TABLE IF EXISTS whatsapp_integrations;
DROP TABLE IF EXISTS auth_sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS shops;