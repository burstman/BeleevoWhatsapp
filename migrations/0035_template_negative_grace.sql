-- +goose Up
-- The grace timestamp now covers every negative Meta review outcome (marketing
-- flag, rejected, paused), not just marketing. Rename for accuracy; the value
-- semantics are unchanged. Kept idempotent so a concurrent test migration run
-- cannot fail on an already-renamed column.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'templates' AND column_name = 'marketing_flagged_at'
    ) THEN
        ALTER TABLE templates RENAME COLUMN marketing_flagged_at TO negative_at;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS templates_negative_at_idx
    ON templates (negative_at)
    WHERE approval_status <> 'deleted';

-- +goose Down
DROP INDEX IF EXISTS templates_negative_at_idx;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'templates' AND column_name = 'negative_at'
    ) THEN
        ALTER TABLE templates RENAME COLUMN negative_at TO marketing_flagged_at;
    END IF;
END $$;
