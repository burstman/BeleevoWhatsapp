package dashboard

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/database"
)

// The Not-on-WhatsApp counter is a JSONB containment scan over meta_errors, the
// exact flavor of silent-return-nothing lookup that shipped bugs before. The
// send-time path and the webhook path record the reason in the same slot, so
// both must be counted and neither may overcount plain failures or other codes.
func statsTestDB(t *testing.T) (*pgxpool.Pool, []uuid.UUID) {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the dashboard stats database tests")
	}
	if err := database.Migrate(context.Background(), url, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	ids := seedStatsShop(t, pool)
	t.Cleanup(func() {
		ctx := context.Background()
		for _, q := range []string{
			`DELETE FROM messages WHERE shop_id = ANY($1::uuid[])`,
			`DELETE FROM shops WHERE id = ANY($1::uuid[])`,
		} {
			if _, err := pool.Exec(ctx, q, ids); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
		pool.Close()
	})
	return pool, ids
}

func seedStatsShop(t *testing.T, db database.Querier) []uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	for _, name := range []string{"stats test shop A", "stats test shop B"} {
		id := uuid.New()
		if _, err := db.Exec(context.Background(), `INSERT INTO shops (id, name) VALUES ($1, $2)`, id, name); err != nil {
			t.Fatalf("insert shop: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func seedStatsMessage(t *testing.T, db database.Querier, shopID uuid.UUID, idem, status, metaErrors string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO messages (shop_id, recipient_phone, template_variables, idempotency_key, status, meta_errors)
		VALUES ($1, '21624118849', '{}'::jsonb, $2, $3, $4::jsonb)`,
		shopID, idem, status, metaErrors); err != nil {
		t.Fatalf("insert message: %v", err)
	}
}

func TestStatsAllCountsNotOnWhatsApp(t *testing.T) {
	ctx := context.Background()
	pool, shops := statsTestDB(t)
	a, b := shops[0], shops[1]

	seedStatsMessage(t, pool, a, "a-1", "failed", `[{"code":131047,"title":"Recipient phone number not in WhatsApp."}]`)
	seedStatsMessage(t, pool, a, "a-2", "failed", `[{"code":132001,"title":"Could not deliver message to the specified phone number."}]`)
	seedStatsMessage(t, pool, a, "a-3", "failed", `[{"code":999,"title":"Some other failure."}]`)
	seedStatsMessage(t, pool, a, "a-4", "failed", `[]`)
	seedStatsMessage(t, pool, a, "a-5", "sent", `[]`)
	seedStatsMessage(t, pool, a, "a-6", "delivered", `[]`)

	seedStatsMessage(t, pool, b, "b-1", "failed", `[{"code":131047,"title":"Recipient phone number not in WhatsApp."}]`)
	seedStatsMessage(t, pool, b, "b-2", "failed", `[{"code":130429,"title":"Rate limited."}]`)

	all, err := NewRepository(pool).StatsAll(ctx)
	if err != nil {
		t.Fatalf("StatsAll: %v", err)
	}
	if all.MessagesFailed != 6 {
		t.Fatalf("MessagesFailed = %d, want 6", all.MessagesFailed)
	}
	if all.MessagesNotOnWhatsapp != 3 {
		t.Fatalf("MessagesNotOnWhatsapp = %d, want 3", all.MessagesNotOnWhatsapp)
	}

	sa, err := NewRepository(pool).Stats(ctx, a)
	if err != nil {
		t.Fatalf("Stats(A): %v", err)
	}
	if sa.MessagesFailed != 4 || sa.MessagesNotOnWhatsapp != 2 {
		t.Fatalf("Stats(A) failed=%d notOnWA=%d, want 4/2", sa.MessagesFailed, sa.MessagesNotOnWhatsapp)
	}
}