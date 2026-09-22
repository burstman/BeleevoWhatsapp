-- +goose Up
-- Templates are owned by a merchant (tenant) but live on the platform's single
-- messaging account, so approval state comes from Meta's template review.
-- Re-key templates from the legacy per-shop integration to shop_id and add the
-- Meta lifecycle fields the send gate needs.
ALTER TABLE templates
    ADD COLUMN shop_id        UUID REFERENCES shops(id) ON DELETE CASCADE,
    ADD COLUMN meta_template_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN approval_status   TEXT NOT NULL DEFAULT 'pending', -- pending|approved|rejected|paused|deleted
    ADD COLUMN rejection_reason  TEXT NOT NULL DEFAULT '';

-- Backfill shop_id from the integration each row hangs off (templates were
-- historically created per whatsapp_integration).
UPDATE templates t
SET shop_id = wi.shop_id
FROM whatsapp_integrations wi
WHERE t.whatsapp_integration_id = wi.id;

ALTER TABLE templates ALTER COLUMN shop_id SET NOT NULL;
ALTER TABLE templates ALTER COLUMN whatsapp_integration_id DROP NOT NULL;

CREATE UNIQUE INDEX idx_templates_shop_name_language ON templates(shop_id, meta_template_name, language);
CREATE INDEX idx_templates_shop_approval ON templates(shop_id, approval_status);

-- +goose Down
ALTER TABLE templates
    DROP COLUMN IF EXISTS rejection_reason,
    DROP COLUMN IF EXISTS approval_status,
    DROP COLUMN IF EXISTS meta_template_id,
    DROP COLUMN IF EXISTS shop_id;
ALTER TABLE templates ALTER COLUMN whatsapp_integration_id SET NOT NULL;
DROP INDEX IF EXISTS idx_templates_shop_approval;
DROP INDEX IF EXISTS idx_templates_shop_name_language;