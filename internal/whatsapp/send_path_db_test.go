package whatsapp

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/database"
)

// The message row is reserved before Meta is called, and that INSERT is where
// the body snapshot is written. It once named $10 while only using $1-$8, so
// Postgres refused the whole statement with "could not determine data type of
// parameter $9" - meaning no inline send could record anything at all.
//
// The history test for the snapshot wrote its own row, so it proved the read
// path and was happy while the write path did not exist. This one goes through
// the statement itself.
//
//	go test ./internal/whatsapp/ -run TestQueuedMessage -v
func TestQueuedMessageStoresTheBodyItWillSend(t *testing.T) {
	ctx := context.Background()
	pool := sendPathDB(t)
	svc := NewService(config.Config{}, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	shop := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO shops (id, name) VALUES ($1, 'send path test')`, shop); err != nil {
		t.Fatalf("insert shop: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM shops WHERE id = $1`, shop) })

	customer := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO customers (id, shop_id, name, phone) VALUES ($1, $2, 'Radhwen Marayah', '+21693531118')`,
		customer, shop); err != nil {
		t.Fatalf("insert customer: %v", err)
	}

	templateID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO templates (id, shop_id, meta_template_name, language, category, status, approval_status)
		VALUES ($1, $2, 'ordre_en_cours', 'fr', 'UTILITY', 'approved', 'approved')`,
		templateID, shop); err != nil {
		t.Fatalf("insert template: %v", err)
	}

	template := TemplateState{
		ID:         templateID,
		Language:   "fr",
		Components: []TemplateComponent{{Type: "body"}},
		RawComponents: []byte(`[{"type":"BODY","text":"Bonjour {{1}}, votre commande {{2}} est en route. ` +
			`Livreur: {{3}} Tel: {{4}}"}]`),
		NumVariables: 4,
	}
	req := SendRequest{
		ShopID:          shop,
		CustomerID:      customer,
		TemplateID:      template.ID,
		ConvertyOrderID: "6ab7cab7",
		Purpose:         "UTILITY",
		IdempotencyKey:  "msc:shop:in-progress:922153764102",
		Variables: map[string]string{
			"1": "Radhwen Marayah", "2": "922153764102",
			"3": "Borhen edine ben khlifa", "4": "29656683",
		},
	}

	id, err := svc.createQueuedMessage(ctx, req, template)
	if err != nil {
		t.Fatalf("createQueuedMessage: %v", err)
	}

	var status, body string
	if err := pool.QueryRow(ctx, `SELECT status, body_text FROM messages WHERE id = $1`, id).
		Scan(&status, &body); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if status != "queued" {
		t.Errorf("status = %q, want queued: the row is reserved before Meta is called", status)
	}
	if body == "" {
		t.Fatal("body_text is empty: the history would fall back to 'accepted by WhatsApp' and show no message")
	}
	if want := "Livreur: Borhen edine ben khlifa Tel: 29656683"; !strings.Contains(body, want) {
		t.Errorf("body = %q, want the rendered driver in it", body)
	}
	// The snapshot has to be the text, not the placeholders: the history claims
	// to show what the customer received.
	if strings.Contains(body, "{{") {
		t.Errorf("body = %q, want every placeholder substituted", body)
	}
}

// A template with no placeholders is not a reason to store nothing: the body is
// the whole message, and the history should still show it.
func TestQueuedMessageWithoutPlaceholdersStillShowsItsText(t *testing.T) {
	ctx := context.Background()
	pool := sendPathDB(t)
	svc := NewService(config.Config{}, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	shop := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO shops (id, name) VALUES ($1, 'no placeholder test')`, shop); err != nil {
		t.Fatalf("insert shop: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM shops WHERE id = $1`, shop) })

	customer := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO customers (id, shop_id, name, phone) VALUES ($1, $2, 'Badri Mondher', '+21698162832')`,
		customer, shop); err != nil {
		t.Fatalf("insert customer: %v", err)
	}

	templateID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO templates (id, shop_id, meta_template_name, language, category, status, approval_status)
		VALUES ($1, $2, 'sans_variables', 'fr', 'UTILITY', 'approved', 'approved')`,
		templateID, shop); err != nil {
		t.Fatalf("insert template: %v", err)
	}

	template := TemplateState{
		ID:            templateID,
		Language:      "fr",
		Components:    []TemplateComponent{{Type: "body"}},
		RawComponents: []byte(`[{"type":"BODY","text":"Votre commande est en route."}]`),
	}
	id, err := svc.createQueuedMessage(ctx, SendRequest{
		ShopID:         shop,
		CustomerID:     customer,
		TemplateID:     template.ID,
		Purpose:        "UTILITY",
		IdempotencyKey: "msc:shop:in-progress:948758517362",
	}, template)
	if err != nil {
		t.Fatalf("createQueuedMessage: %v", err)
	}

	var body *string
	if err := pool.QueryRow(ctx, `SELECT body_text FROM messages WHERE id = $1`, id).Scan(&body); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if body == nil || *body != "Votre commande est en route." {
		t.Errorf("body = %v, want the literal text: a template with no placeholders still says something", body)
	}
}

// With no body component at all there is no text to show, and the column lands
// NULL rather than an empty string, so the history can say in words that it has
// nothing to display instead of showing a blank message.
func TestQueuedMessageWithNoBodyStoresNull(t *testing.T) {
	ctx := context.Background()
	pool := sendPathDB(t)
	svc := NewService(config.Config{}, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	shop := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO shops (id, name) VALUES ($1, 'no body test')`, shop); err != nil {
		t.Fatalf("insert shop: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM shops WHERE id = $1`, shop) })

	customer := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO customers (id, shop_id, name, phone) VALUES ($1, $2, 'Borhen client', '+21629656683')`,
		customer, shop); err != nil {
		t.Fatalf("insert customer: %v", err)
	}

	templateID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO templates (id, shop_id, meta_template_name, language, category, status, approval_status)
		VALUES ($1, $2, 'header_seulement', 'fr', 'UTILITY', 'approved', 'approved')`,
		templateID, shop); err != nil {
		t.Fatalf("insert template: %v", err)
	}

	template := TemplateState{ID: templateID, Language: "fr", Components: []TemplateComponent{{Type: "header"}}}
	id, err := svc.createQueuedMessage(ctx, SendRequest{
		ShopID:         shop,
		CustomerID:     customer,
		TemplateID:     template.ID,
		Purpose:        "UTILITY",
		IdempotencyKey: "msc:shop:in-progress:100000000000",
	}, template)
	if err != nil {
		t.Fatalf("createQueuedMessage: %v", err)
	}

	var body *string
	if err := pool.QueryRow(ctx, `SELECT body_text FROM messages WHERE id = $1`, id).Scan(&body); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if body != nil && *body != "" {
		t.Errorf("body = %q, want NULL so the history can say it has no text to show", *body)
	}
}

func sendPathDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the send path database tests")
	}
	ctx := context.Background()
	if err := database.Migrate(ctx, url, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
