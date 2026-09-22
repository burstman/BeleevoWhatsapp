-- +goose Up
-- The merchant dashboard and seed flow upsert one WhatsApp integration per
-- shop. whatsapp_integrations.shop_id needs a UNIQUE constraint for
-- ON CONFLICT (shop_id) to work.
DROP INDEX IF EXISTS idx_whatsapp_integrations_shop_id;
CREATE UNIQUE INDEX idx_whatsapp_integrations_shop_id ON whatsapp_integrations(shop_id);

-- +goose Down
DROP INDEX IF EXISTS idx_whatsapp_integrations_shop_id;
CREATE INDEX idx_whatsapp_integrations_shop_id ON whatsapp_integrations(shop_id);