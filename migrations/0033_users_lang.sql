-- +goose Up
ALTER TABLE users ADD COLUMN lang TEXT NOT NULL DEFAULT 'en';

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS lang;