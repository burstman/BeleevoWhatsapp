package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/queue"
)

// The first-contact greeting is only ever sent once per conversation, and its
// media must survive a switch back to text without lingering. These are exactly
// the silent-failure shapes the rest of the package guards against.
//
//	go test ./internal/whatsapp/ -run TestFirstContactAutoreply -v
func TestFirstContactAutoreply(t *testing.T) {
	ctx := context.Background()
	pool := chatTestDB(t)
	svc := NewService(config.Config{}, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if _, err := pool.Exec(ctx, `TRUNCATE shop_autoreplies, conversations, chat_messages`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	shop := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO shops (id, name) VALUES ($1, 'autoreply test')`, shop); err != nil {
		t.Fatalf("insert shop: %v", err)
	}

	// A missing row is an unconfigured greeting, not an error.
	cfg, err := svc.GetFirstContactAutoreply(ctx, shop)
	if err != nil {
		t.Fatalf("GetFirstContactAutoreply (empty): %v", err)
	}
	if cfg.Enabled || cfg.Kind != AutoreplyText {
		t.Errorf("empty config = %+v, want disabled text", cfg)
	}

	// Text round-trips.
	if err := svc.SaveFirstContactAutoreply(ctx, FirstContactAutoreply{
		ShopID: shop, Enabled: true, Kind: AutoreplyText, TextBody: "Bienvenue !",
	}); err != nil {
		t.Fatalf("save text: %v", err)
	}
	cfg, err = svc.GetFirstContactAutoreply(ctx, shop)
	if err != nil {
		t.Fatalf("get text: %v", err)
	}
	if !cfg.Enabled || cfg.Kind != AutoreplyText || cfg.TextBody != "Bienvenue !" {
		t.Errorf("text config = %+v", cfg)
	}

	// Image stores bytes and mime; switching to text clears the bytes.
	if err := svc.SaveFirstContactAutoreply(ctx, FirstContactAutoreply{
		ShopID: shop, Enabled: true, Kind: AutoreplyImage,
		MediaMime: "image/png", MediaBytes: []byte("PNGDATA"), MediaFilename: "photo.png",
	}); err != nil {
		t.Fatalf("save image: %v", err)
	}
	media, mime, err := svc.FirstContactAutoreplyMedia(ctx, shop)
	if err != nil {
		t.Fatalf("image media: %v", err)
	}
	if string(media) != "PNGDATA" || mime != "image/png" {
		t.Errorf("image media = %q/%q, want PNGDATA/image/png", media, mime)
	}
	if err := svc.SaveFirstContactAutoreply(ctx, FirstContactAutoreply{
		ShopID: shop, Enabled: true, Kind: AutoreplyText, TextBody: "salut",
	}); err != nil {
		t.Fatalf("switch to text: %v", err)
	}
	if media, _, err = svc.FirstContactAutoreplyMedia(ctx, shop); err != nil {
		t.Fatalf("media after text: %v", err)
	} else if len(media) != 0 {
		t.Errorf("media after switching to text = %q, want empty", media)
	}

	// Scheduling a disabled greeting is a no-op (nothing queued).
	if _, err := pool.Exec(ctx, `UPDATE shop_autoreplies SET enabled = false WHERE shop_id = $1`, shop); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := svc.ScheduleFirstContactReply(ctx, shop, uuid.New()); err != nil {
		t.Fatalf("schedule disabled: %v", err)
	}
	var queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM job_queue WHERE kind = $1`, queue.TaskSendFirstContactReply).Scan(&queued); err != nil {
		t.Fatalf("count queued: %v", err)
	}
	if queued != 0 {
		t.Errorf("queued = %d, want 0 when disabled", queued)
	}
}

// A first-contact greeting is claimed exactly once: a queue retry for the same
// conversation must not send a second time.
func TestFirstContactAutoreplyClaimsOnce(t *testing.T) {
	ctx := context.Background()
	pool := chatTestDB(t)
	svc := NewService(config.Config{}, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if _, err := pool.Exec(ctx, `TRUNCATE shop_autoreplies, conversations, chat_messages`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	shop := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO shops (id, name) VALUES ($1, 'autoreply claim test')`, shop); err != nil {
		t.Fatalf("insert shop: %v", err)
	}
	if err := svc.SaveFirstContactAutoreply(ctx, FirstContactAutoreply{
		ShopID: shop, Enabled: true, Kind: AutoreplyText, TextBody: "Bienvenue !",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Create a conversation via the real inbound path so the window is open.
	res, err := svc.UpsertInbound(ctx, shop, InboundMessage{
		MetaMessageID: "wamid.FC1",
		From:          "21655000000",
		Timestamp:     time.Now().Unix(),
		Type:          "text",
		TextBody:      "Bonjour",
	})
	if err != nil {
		t.Fatalf("UpsertInbound: %v", err)
	}
	if !res.FirstContact {
		t.Fatal("expected FirstContact on a new conversation")
	}

	job := FirstContactReplyJob{ShopID: shop, ConversationID: res.ConversationID}
	payload, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal job: %v", err)
	}

	// No Meta credentials are configured, so the send fails — but the claim must
	// still be taken, because a greeting is at-most-once.
	err = svc.SendFirstContactReplyJob(ctx, payload)
	if err == nil {
		t.Log("send unexpectedly succeeded (a fake integration exists)")
	} else if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("job error = %v, want ErrNotConfigured", err)
	}

	var greeted *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT first_contact_autoreplied_at FROM conversations WHERE id = $1`, res.ConversationID,
	).Scan(&greeted); err != nil {
		t.Fatalf("read claim: %v", err)
	}
	if greeted == nil {
		t.Fatal("first_contact_autoreplied_at must be set after the job claims it")
	}

	// Second run: already claimed, so it is a silent no-op.
	if err := svc.SendFirstContactReplyJob(ctx, payload); err != nil {
		t.Fatalf("second job run = %v, want nil (already greeted)", err)
	}
}
