package whatsapp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/database"
)

// The customer inbox lives on the same Postgres ledger as everything else, so
// it is covered by the same class of scan-bug checks the other packages use.
//
//	go test ./internal/whatsapp/ -run TestChatInbox -v
func TestChatInbox(t *testing.T) {
	ctx := context.Background()
	pool := chatTestDB(t)
	svc := NewService(config.Config{}, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// The test database is shared across runs and other packages; the chat
	// tables are private to this test, so clear them.
	if _, err := pool.Exec(ctx, `TRUNCATE conversations, chat_messages`); err != nil {
		t.Fatalf("truncate chat tables: %v", err)
	}

	shop := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO shops (id, name) VALUES ($1, 'chat inbox test')`, shop); err != nil {
		t.Fatalf("insert shop: %v", err)
	}
	otherShop := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO shops (id, name) VALUES ($1, 'chat second shop')`, otherShop); err != nil {
		t.Fatalf("insert shop: %v", err)
	}

	// Operator number resolves back to the shop that owns it. The id is
	// unique per run so a reused test database cannot shadow the lookup.
	phoneNumberID := "pn-" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO whatsapp_integrations (shop_id, phone_number_id, phone_number)
		VALUES ($1, $2, '+21624118849')`, shop, phoneNumberID); err != nil {
		t.Fatalf("insert integration: %v", err)
	}
	resolved, err := svc.ShopByPhoneNumberID(ctx, phoneNumberID)
	if err != nil {
		t.Fatalf("ShopByPhoneNumberID: %v", err)
	}
	if resolved != shop {
		t.Errorf("resolved shop = %v, want %v", resolved, shop)
	}
	if _, err := svc.ShopByPhoneNumberID(ctx, "99999"); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("unknown number: err = %v, want ErrNoRows", err)
	}

	// A customer message creates the conversation and the first bubble.
	inbound := InboundMessage{
		MetaMessageID: "wamid.IN1",
		From:          "21655123456",
		ProfileName:   "Ahmed Ben Ali",
		Timestamp:     time.Now().Add(-2 * time.Second).Unix(),
		Type:          "text",
		TextBody:      "Is my order ready?",
		PhoneNumberID: phoneNumberID,
	}
	res, err := svc.UpsertInbound(ctx, shop, inbound)
	if err != nil {
		t.Fatalf("UpsertInbound: %v", err)
	}
	if !res.FirstContact {
		t.Error("first inbound must report FirstContact")
	}
	if res.ConversationID == uuid.Nil {
		t.Error("first inbound must return a conversation id")
	}
	// Meta re-delivers on retries; the idempotency key must hold, and a
	// duplicate must never be treated as a first contact.
	if res2, err := svc.UpsertInbound(ctx, shop, inbound); err != nil {
		t.Fatalf("UpsertInbound (duplicate): %v", err)
	} else if res2.FirstContact {
		t.Error("a duplicate message must not report FirstContact")
	}

	threads, err := svc.Threads(ctx)
	if err != nil {
		t.Fatalf("Threads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("len(threads) = %d, want 1", len(threads))
	}
	c := threads[0]
	if c.CustomerName != "Ahmed Ben Ali" || c.CustomerPhone != "21655123456" {
		t.Errorf("conversation identity = %q/%q, want the customer", c.CustomerName, c.CustomerPhone)
	}
	if c.UnreadCount != 1 {
		t.Errorf("unread = %d, want 1 (duplicate inbound must not double-count)", c.UnreadCount)
	}
	if c.LastBody != "Is my order ready?" || c.LastDirection != "inbound" {
		t.Errorf("last = %q/%q, want the customer's message", c.LastDirection, c.LastBody)
	}
	if !c.WindowOpen {
		t.Error("window must be open right after an inbound message")
	}

	// A second message from another customer on the other shop: newest first,
	// and the old conversation re-reads with both messages in order.
	second := InboundMessage{
		MetaMessageID: "wamid.IN2",
		From:          "21655333333",
		Timestamp:     time.Now().Unix(),
		Type:          "text",
		TextBody:      "Hello?",
		PhoneNumberID: phoneNumberID,
	}
	if resSecond, err := svc.UpsertInbound(ctx, otherShop, second); err != nil {
		t.Fatalf("second UpsertInbound: %v", err)
	} else if !resSecond.FirstContact {
		t.Error("the first inbound on a second shop must report FirstContact")
	}
	threads, err = svc.Threads(ctx)
	if err != nil {
		t.Fatalf("Threads: %v", err)
	}
	if len(threads) != 2 || threads[0].CustomerPhone != "21655333333" || threads[1].CustomerPhone != "21655123456" {
		t.Errorf("threads order wrong: %+v", threads)
	}

	conv, msgs, err := svc.Thread(ctx, c.ID)
	if err != nil {
		t.Fatalf("Thread: %v", err)
	}
	if conv.ID != c.ID {
		t.Errorf("thread id = %v, want %v", conv.ID, c.ID)
	}
	if len(msgs) != 1 || msgs[0].Direction != "inbound" || msgs[0].Body != "Is my order ready?" {
		t.Fatalf("msgs = %+v, want the single inbound bubble", msgs)
	}
	if _, _, err := svc.Thread(ctx, uuid.New()); !errors.Is(err, ErrConversationNotFound) {
		t.Errorf("unknown thread: err = %v, want ErrConversationNotFound", err)
	}

	if err := svc.MarkThreadRead(ctx, c.ID); err != nil {
		t.Fatalf("MarkThreadRead: %v", err)
	}
	conv, _, _ = svc.Thread(ctx, c.ID)
	if conv.UnreadCount != 0 {
		t.Errorf("unread after read = %d, want 0", conv.UnreadCount)
	}

	// Replies outside the window are refused before any network call.
	pool.Exec(ctx, `
		UPDATE conversations SET window_expires_at = '2001-01-01' WHERE id = $1`, c.ID)
	if _, err := svc.SendReply(ctx, c.ID, "Oui"); !errors.Is(err, ErrReplyWindowClosed) {
		t.Errorf("closed-window reply: err = %v, want ErrReplyWindowClosed", err)
	}

	// Inside the window but with no Meta creds: refuse with ErrNotConfigured
	// rather than touching the network.
	pool.Exec(ctx, `
		UPDATE conversations SET window_expires_at = now() + interval '1 hour' WHERE id = $1`, c.ID)
	if _, err := svc.SendReply(ctx, c.ID, "Oui"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("open-window reply without creds: err = %v, want ErrNotConfigured", err)
	}

	// An empty reply is refused too.
	if _, err := svc.SendReply(ctx, c.ID, "   "); err == nil {
		t.Error("blank reply must be refused")
	}
}

// The delivery trail (settings page "Recent webhook deliveries") must round-trip
// a receipt exactly, including a shop-scoped inbound, newest first.
func TestWebhookReceipts(t *testing.T) {
	ctx := context.Background()
	pool := chatTestDB(t)
	svc := NewService(config.Config{}, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if _, err := pool.Exec(ctx, `TRUNCATE webhook_receipts`); err != nil {
		t.Fatalf("truncate receipts: %v", err)
	}
	shop := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO shops (id, name) VALUES ($1, 'receipts test')`, shop); err != nil {
		t.Fatalf("insert shop: %v", err)
	}

	svc.RecordWebhookReceipt(ctx, WebhookReceipt{Kind: "config_missing", Detail: "no secret"})
	svc.RecordWebhookReceipt(ctx, WebhookReceipt{
		Kind:          "inbound",
		MetaMessageID: "wamid.R1",
		FromPhone:     "21654116584",
		PhoneNumberID: "pn-1",
		ShopID:        &shop,
	})

	rs, err := svc.RecentWebhookReceipts(ctx, 10)
	if err != nil {
		t.Fatalf("RecentWebhookReceipts: %v", err)
	}
	if len(rs) != 2 {
		t.Fatalf("len(rs) = %d, want 2", len(rs))
	}
	if rs[0].Kind != "inbound" || rs[0].MetaMessageID != "wamid.R1" || rs[0].FromPhone != "21654116584" {
		t.Errorf("newest receipt wrong: %+v", rs[0])
	}
	if rs[0].ShopID == nil || *rs[0].ShopID != shop {
		t.Errorf("inbound shop_id = %v, want %v", rs[0].ShopID, shop)
	}
	fresh := rs[0].ReceivedAt.Sub(time.Now())
	if fresh > 2*time.Minute || fresh < -2*time.Minute {
		t.Errorf("received_at %v is not now", rs[0].ReceivedAt)
	}
	if rs[1].Kind != "config_missing" {
		t.Errorf("oldest receipt wrong: %+v", rs[1])
	}
}

// The chat status advance must skip messages that are not chat replies; the
// template ledger owns those ids.
func TestChatStatusSkipsTemplateLedger(t *testing.T) {
	ctx := context.Background()
	pool := chatTestDB(t)
	svc := NewService(config.Config{}, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := svc.ApplyChatStatusUpdate(ctx, StatusUpdate{MetaMessageID: "wamid.NOPE", Status: "delivered"}); err != nil {
		t.Fatalf("ApplyChatStatusUpdate for a template-ledger id must be a no-op, got %v", err)
	}
}

func chatTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the chat database tests")
	}
	if err := database.Migrate(context.Background(), url, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}