-- +goose Up
-- Meta flags some submitted templates at creation (warnings array on the
-- create response) or later during review; when the warning indicates the
-- content is/will be treated as marketing, the template is permanently
-- unsendable on this platform. Record the flag and Meta's own warning text so
-- the merchant can be notified and the send gate can refuse it.
ALTER TABLE templates
    ADD COLUMN marketing_flagged BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN meta_warnings     TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE templates
    DROP COLUMN IF EXISTS marketing_flagged,
    DROP COLUMN IF EXISTS meta_warnings;