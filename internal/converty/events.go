package converty

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// WebhookEvent is a raw Converty webhook delivery with best-effort parsed
// fields. The payload is kept verbatim because the delivery shape is not
// fully documented yet.
type WebhookEvent struct {
	ShopID      uuid.UUID
	EventType   string
	OrderID     string
	OrderStatus string
	Payload     []byte
	PayloadHash string
	ReceivedAt  time.Time
	// Duplicate reports whether the body was already ingested.
	Duplicate bool
}

// webhookDecoded holds the fields we defensively extract from a Converty
// payload. Extraction is best-effort and never blocks ingestion.
type webhookDecoded struct {
	EventType   string
	OrderID     string
	OrderStatus string
	StoreID     string
	StoreSlug   string
}

// CaptureWebhook ingests one Converty webhook delivery into order_events,
// deduplicating identical bodies by their sha256. Shop attribution is
// best-effort: when no store reference can be matched to an integration the
// event is still stored with a NULL shop_id.
func (s *Service) CaptureWebhook(ctx context.Context, body []byte) (WebhookEvent, error) {
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])

	decoded := decodeWebhook(body)

	var event WebhookEvent
	event.EventType = decoded.EventType
	event.OrderID = decoded.OrderID
	event.OrderStatus = decoded.OrderStatus
	event.Payload = body
	event.PayloadHash = hash
	event.ReceivedAt = time.Now().UTC()

	shopID, _ := s.lookupShopID(ctx, decoded)
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO order_events (shop_id, event_id, event_type, order_id, order_status, payload, payload_hash, processed_at)
		VALUES ($1, '', $2, $3, $4, $5, $6, $7)
		ON CONFLICT (payload_hash) DO NOTHING`,
		nullableUUID(shopID), decoded.EventType, decoded.OrderID, decoded.OrderStatus, body, hash, event.ReceivedAt,
	)
	if err != nil {
		return WebhookEvent{}, err
	}
	event.ShopID = shopID
	event.Duplicate = tag.RowsAffected() == 0

	return event, nil
}

// lookupShopID attributes a payload to a shop by matching the store id or
// slug it references against a connected integration.
func (s *Service) lookupShopID(ctx context.Context, d webhookDecoded) (uuid.UUID, error) {
	if d.StoreID == "" && d.StoreSlug == "" {
		return uuid.Nil, errors.New("payload carries no store reference")
	}
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT shop_id
		FROM converty_integrations
		WHERE ($1 <> '' AND converty_store_id = $1)
		   OR ($2 <> '' AND store_slug = $2)
		LIMIT 1`,
		d.StoreID, d.StoreSlug,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, errors.New("no integration matches payload store")
	}
	return id, err
}

// decodeWebhook walks a payload looking for the known field names under any
// of the common envelope keys (data, order, payload, store, result).
func decodeWebhook(body []byte) webhookDecoded {
	var d webhookDecoded
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return d
	}
	d.EventType = pick(root, "event", "eventType", "event_type", "webhookType", "type")
	d.OrderID = pick(root, "orderId", "order_id", "_id", "id")
	d.OrderStatus = pick(root, "status", "orderStatus", "order_status")
	d.StoreID = pick(root, "store", "storeId", "store_id")
	d.StoreSlug = pick(root, "storeSlug", "store_slug", "slug")
	return d
}

// pick returns the first non-empty string found for any of the candidate
// keys, recursing into common envelope maps so the field can be nested.
func pick(root any, keys ...string) string {
	var walk func(any, int) string
	walk = func(node any, depth int) string {
		if depth > 4 {
			return ""
		}
		m, ok := node.(map[string]any)
		if !ok {
			return ""
		}
		for _, k := range keys {
			if v, exists := m[k]; exists {
				if str, ok := v.(string); ok && str != "" {
					return str
				}
			}
		}
		for _, envelope := range []string{"data", "order", "payload", "store", "result"} {
			if nested, exists := m[envelope]; exists {
				if res := walk(nested, depth+1); res != "" {
					return res
				}
			}
		}
		return ""
	}
	return walk(root, 0)
}

func nullableUUID(u uuid.UUID) any {
	if u == uuid.Nil {
		return nil
	}
	return u
}
