-- +goose Up
-- Templates flagged as marketing before migration 0013 added
-- marketing_flagged_at have NULL timestamps, so the 15-min purge deadline was
-- never computed. Backfill from the last sync time so those rows are purged
-- on the next sweep (they are already past the delay).
UPDATE templates
SET marketing_flagged_at = COALESCE(marketing_flagged_at, updated_at)
WHERE marketing_flagged
  AND marketing_flagged_at IS NULL
  AND approval_status <> 'deleted';

-- +goose Down
-- No safe down migration: the backfill is data, column drops happen in 0013.