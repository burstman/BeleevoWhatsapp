-- +goose Up
-- Customer inbox: conversations are the customers who messaged the operator's
-- WhatsApp number, chat_messages the thread under each. This is deliberately a
-- separate ledger from `messages` (template/automation sends): a reply is a
-- free-form customer service message, not a template send, and inbound
-- messages have no template at all. Mixing them into `messages` would pollute
-- send history and the dashboard's send counters.
CREATE TABLE conversations (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id              UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    customer_phone       TEXT NOT NULL,
    customer_name        TEXT NOT NULL DEFAULT '',
    last_message_direction TEXT NOT NULL DEFAULT 'inbound',
    last_message_body    TEXT NOT NULL DEFAULT '',
    last_message_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    unread_count         INT NOT NULL DEFAULT 0,
    -- Free-form replies are allowed only while the customer service window is
    -- open: 24h after the customer's last inbound message.
    window_expires_at    TIMESTAMPTZ NOT NULL DEFAULT to_timestamp(0),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (shop_id, customer_phone)
);

CREATE INDEX idx_conversations_shop_last ON conversations(shop_id, last_message_at DESC);

CREATE TABLE chat_messages (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id   UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    direction         TEXT NOT NULL, -- inbound | outbound
    body              TEXT NOT NULL DEFAULT '',
    meta_message_id   TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'queued', -- queued|sent|delivered|read|failed
    error_message     TEXT NOT NULL DEFAULT '',
    sent_at           TIMESTAMPTZ,
    delivered_at      TIMESTAMPTZ,
    read_at           TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_chat_messages_conversation ON chat_messages(conversation_id, created_at);
CREATE INDEX idx_chat_messages_meta ON chat_messages(meta_message_id);

-- +goose Down
DROP TABLE chat_messages;
DROP TABLE conversations;