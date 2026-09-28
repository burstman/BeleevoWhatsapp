-- +goose Up
-- The three fixes around held-back sends landed together, but the events they
-- recover from were dropped earlier and left no trace: a delivery parcel sitting
-- in an automation's status, with no message for it, is exactly the shape of an
-- event that was withheld. Those are recoverable from the tracked parcels
-- alone, so they are backfilled here rather than left stranded.
--
-- The idempotency key is rebuilt in the same shape the processor uses, so a
-- message that does exist for the event is left alone and a retry reuses the key
-- rather than double-sending.
--
-- Scoped to delivery automations on purpose. A Converty order event is not
-- recoverable this way: order_events keeps the raw payload and the customer's
-- name and phone live inside it in a shape this cannot read reliably, and a
-- guess would put the wrong name in somebody's message.
INSERT INTO automation_suppressions (
    shop_id, automation_id, customer_id, template_id, reason, missing_variables,
    trigger_label, tracking_code, order_id, idempotency_key, created_at
)
SELECT
    a.shop_id,
    a.id,
    d.customer_id,
    a.template_id,
    'a variable the template places has no value for this order yet',
    missing.missing_variables,
    COALESCE(d.status_label, ''),
    d.barcode,
    COALESCE(d.order_id, ''),
    'msc:' || a.shop_id::text || ':' || d.last_status || ':' || d.barcode,
    COALESCE(d.last_seen_at, d.created_at, now())
FROM automations a
JOIN delivery_orders d
  ON d.shop_id = a.shop_id
 AND d.last_status = a.order_status
LEFT JOIN templates t ON t.id = a.template_id
JOIN LATERAL (
    SELECT COALESCE(jsonb_agg(elem #>> '{}'), '[]'::jsonb) AS missing_variables
    FROM jsonb_array_elements(COALESCE(t.variables_map, '[]'::jsonb)) AS elem
    WHERE CASE elem #>> '{}'
        WHEN 'customer_name'  THEN COALESCE(d.customer_name, '') = ''
        WHEN 'customer_phone' THEN COALESCE(d.customer_phone, '') = ''
        WHEN 'order_id'       THEN COALESCE(d.order_id, '') = ''
        WHEN 'order_status'   THEN COALESCE(d.status_label, '') = ''
        WHEN 'tracking_code'  THEN COALESCE(d.barcode, '') = ''
        WHEN 'driver_name'    THEN COALESCE(d.driver_name, '') = ''
        WHEN 'driver_phone'   THEN COALESCE(d.driver_phone, '') = ''
        ELSE false
    END
) AS missing ON true
WHERE a.event_source = 'delivery'
  AND jsonb_array_length(missing.missing_variables) > 0
  AND NOT EXISTS (
      SELECT 1 FROM messages m
      WHERE m.automation_id = a.id
        AND m.idempotency_key = 'msc:' || a.shop_id::text || ':' || d.last_status || ':' || d.barcode
  )
ON CONFLICT DO NOTHING;

-- +goose Down
-- The backfilled rows are indistinguishable from live ones, so there is nothing
-- safe to roll back: dropping the table above removes them all.
