package whatsapp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/database"
)

// These tests run against a Postgres in a transaction that is rolled back
// afterwards. Set TEST_DATABASE_URL to enable them.
//
//	go test ./internal/whatsapp/ -run TestDBAutomationHistory
//
// seedShop creates a throwaway shop; messages.shop_id is a foreign key, so a
// random uuid would be rejected by the constraint.
func seedShop(t *testing.T, db database.Querier) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.Exec(context.Background(), `INSERT INTO shops (id, name) VALUES ($1, $2)`, id, "history test shop"); err != nil {
		t.Fatalf("insert shop: %v", err)
	}
	return id
}

// seedAutomation creates a real automation row: migration 0025 makes
// messages.automation_id a foreign key, so a random uuid would be rejected.
func seedAutomation(t *testing.T, db database.Querier, shopID uuid.UUID, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO automations (id, shop_id, order_status) VALUES ($1, $2, $3)`,
		id, shopID, status+"-"+id.String()[:8]); err != nil {
		t.Fatalf("insert automation: %v", err)
	}
	return id
}

func seedMessage(t *testing.T, db database.Querier, shopID uuid.UUID, automationID *uuid.UUID, idem string, status string, vars map[string]string) uuid.UUID {
	t.Helper()
	varsJSON, err := json.Marshal(vars)
	if err != nil {
		t.Fatalf("marshal vars: %v", err)
	}
	var id uuid.UUID
	if err := db.QueryRow(context.Background(), `
		INSERT INTO messages (shop_id, automation_id, recipient_phone, template_variables, idempotency_key, status)
		VALUES ($1, $2, '+21624118849', $3::jsonb, $4, $5)
		RETURNING id`,
		shopID, automationID, varsJSON, idem, status,
	).Scan(&id); err != nil {
		t.Fatalf("insert message: %v", err)
	}
	return id
}

// A message sent by an automation is attributed to it and appears in that
// automation's history, and a message from another automation does not leak in.
func TestDBAutomationHistoryScopesToOneAutomation(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	shop := seedShop(t, db)
	mine := seedAutomation(t, db, shop, "en_cours")
	other := seedAutomation(t, db, shop, "livree")
	shared := map[string]string{"1": "x"}

	first := seedMessage(t, db, shop, &mine, "conv:1", "delivered", shared)
	seedMessage(t, db, shop, &mine, "conv:2", "failed", shared)
	seedMessage(t, db, shop, &other, "conv:3", "read", shared)
	seedMessage(t, db, shop, nil, "", "sent", shared)

	rows, err := messagesForAutomation(ctx, db, mine, 200)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want only this automation's 2 messages, got %d", len(rows))
	}
	// Newest first: the seeded rows share a timestamp, so order by created_at
	// then id is not stable; assert the set instead.
	seen := map[uuid.UUID]bool{}
	for _, r := range rows {
		if r.AutomationID == nil || *r.AutomationID != mine {
			t.Fatalf("row %s carries automation %v, want %v", r.ID, r.AutomationID, mine)
		}
		if r.RecipientPhone == "" || r.Status == "" {
			t.Fatalf("row %s is missing the fields the page shows: %+v", r.ID, r)
		}
		seen[r.ID] = true
	}
	if !seen[first] {
		t.Fatal("the delivered message is missing from its automation history")
	}

	counts, err := automationSendCounts(ctx, db, mine)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts.Total != 2 || counts.Delivered != 1 || counts.Failed != 1 || counts.Sent != 0 {
		t.Fatalf("counts wrong: %+v", counts)
	}
}

// The history must not depend on template or customer rows surviving: a message
// whose template was deleted still shows, with a blank label rather than a
// missing row.
func TestDBAutomationHistoryKeepsRowsWithDeletedTemplate(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	shop := seedShop(t, db)
	automation := seedAutomation(t, db, shop, "en_cours")
	seedMessage(t, db, shop, &automation, "conv:x", "sent", nil)

	rows, err := messagesForAutomation(ctx, db, automation, 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].TemplateName != "" || rows[0].CustomerName != "" {
		t.Fatalf("unjoined labels should be blank, got %q / %q", rows[0].TemplateName, rows[0].CustomerName)
	}
	if rows[0].TemplateVariables == nil {
		t.Fatal("template variables must never be nil; the view renders them")
	}
}

// The page shows real event times, so a delivered timestamp is rendered rather
// than a zero time. Guards the column order of the joined query.
func TestDBAutomationHistoryReturnsEventTimes(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	shop := seedShop(t, db)
	automation := seedAutomation(t, db, shop, "en_cours")
	sentAt := time.Now().UTC().Truncate(time.Second)
	id := seedMessage(t, db, shop, &automation, "conv:t", "delivered", map[string]string{"1": "y"})
	if _, err := db.Exec(ctx, `UPDATE messages SET sent_at = $2, delivered_at = $2 WHERE id = $1`, id, sentAt); err != nil {
		t.Fatalf("stamp message: %v", err)
	}

	rows, err := messagesForAutomation(ctx, db, automation, 200)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(rows) != 1 || rows[0].SentAt == nil || rows[0].DeliveredAt == nil {
		t.Fatalf("event times missing: %+v", rows)
	}
	if !rows[0].SentAt.UTC().Truncate(time.Second).Equal(sentAt) {
		t.Fatalf("sent_at scanned into the wrong column: got %v want %v", rows[0].SentAt.UTC(), sentAt)
	}
	if rows[0].TemplateVariables["1"] != "y" {
		t.Fatalf("template variables did not survive the join: %+v", rows[0].TemplateVariables)
	}
}
