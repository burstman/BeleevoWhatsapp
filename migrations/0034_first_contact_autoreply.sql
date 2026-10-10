-- +goose Up
-- Per-shop first-contact auto-reply. When a customer's very first inbound
-- message creates a conversation, the platform can greet them with a free-form
-- text, image or audio message. The inbound opens the 24h customer service
-- window, so no approved template is required. One row per shop, configured on
-- the shop that owns the WhatsApp number.
CREATE TABLE shop_autoreplies (
    shop_id           UUID PRIMARY KEY REFERENCES shops(id) ON DELETE CASCADE,
    enabled           BOOLEAN NOT NULL DEFAULT false,
    kind              TEXT NOT NULL DEFAULT 'text', -- text | image | audio
    text_body         TEXT NOT NULL DEFAULT '',     -- kind=text
    caption           TEXT NOT NULL DEFAULT '',     -- kind=image
    media_mime        TEXT NOT NULL DEFAULT '',
    media_bytes       BYTEA,
    media_duration_ms INT NOT NULL DEFAULT 0,
    media_filename    TEXT NOT NULL DEFAULT '',
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Once-only guard: set atomically when the greeting is claimed, so a replayed
-- webhook or a queue retry can never greet the same customer twice.
ALTER TABLE conversations
    ADD COLUMN first_contact_autoreplied_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE conversations DROP COLUMN IF EXISTS first_contact_autoreplied_at;
DROP TABLE shop_autoreplies;
