-- +goose Up
-- A template Meta flags as marketing is purged MARKETING_PURGE_DELAY after the
-- flag: marketing_flagged_at = when Meta flagged it (the purge deadline base),
-- purge_scheduled_at = when a background purge job was queued (dedup marker).
ALTER TABLE templates
    ADD COLUMN marketing_flagged_at TIMESTAMPTZ,
    ADD COLUMN purge_scheduled_at   TIMESTAMPTZ;

-- +goose Down
ALTER TABLE templates
    DROP COLUMN IF EXISTS marketing_flagged_at,
    DROP COLUMN IF EXISTS purge_scheduled_at;