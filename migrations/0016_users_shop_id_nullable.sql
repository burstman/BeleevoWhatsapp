-- +goose Up
-- Single-operator deployment: the one admin account (the client) manages
-- multiple shops, so a user is no longer bound to exactly one shop. The admin
-- user has shop_id NULL; shops are selected per request from the operator's
-- list instead.
ALTER TABLE users ALTER COLUMN shop_id DROP NOT NULL;

-- +goose Down
DELETE FROM users WHERE shop_id IS NULL;
ALTER TABLE users ALTER COLUMN shop_id SET NOT NULL;