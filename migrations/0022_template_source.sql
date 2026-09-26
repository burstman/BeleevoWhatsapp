-- +goose Up
-- Link each template to the event source it is written for. A Converty
-- template has no deliveryman, a Mes Colis template does; 'any' keeps a
-- template usable from both automations and is the default for rows that
-- predate this column.
ALTER TABLE templates
    ADD COLUMN source TEXT NOT NULL DEFAULT 'any';

-- +goose Down
ALTER TABLE templates
    DROP COLUMN IF EXISTS source;
