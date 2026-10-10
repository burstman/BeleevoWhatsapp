package whatsapp

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/config"
)

// insertNegativeTemplate stores a template in a negative review state with a
// caller-chosen negative_at, so the grace-window logic can be driven without
// waiting on Meta.
func insertNegativeTemplate(t *testing.T, pool *pgxpool.Pool, shop uuid.UUID, approval string, negated time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO templates
			(id, shop_id, meta_template_name, language, category, status,
			 approval_status, components, variables_map, source, negative_at)
		VALUES ($1, $2, $3, 'ar', 'UTILITY', 'submitted',
			$4, '[]'::jsonb, '[]'::jsonb, 'any', $5)`,
		id, shop, "neg_"+id.String()[:8], approval, negated); err != nil {
		t.Fatalf("insert negative template: %v", err)
	}
	return id
}

func TestPurgeExpiredNegativeOnlyRemovesElapsedWindow(t *testing.T) {
	ctx := context.Background()
	pool := chatTestDB(t)
	svc := NewService(config.Config{TemplateNegativeGrace: 15 * time.Minute}, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	shop := seedShop(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM job_queue WHERE shop_id = $1`, shop)
		_, _ = pool.Exec(ctx, `DELETE FROM templates WHERE shop_id = $1`, shop)
		_, _ = pool.Exec(ctx, `DELETE FROM shops WHERE id = $1`, shop)
	})

	expired := insertNegativeTemplate(t, pool, shop, "rejected", time.Now().Add(-20*time.Minute))
	fresh := insertNegativeTemplate(t, pool, shop, "paused", time.Now().Add(-1*time.Minute))

	n, err := svc.PurgeExpiredNegative(ctx)
	if err != nil {
		t.Fatalf("PurgeExpiredNegative: %v", err)
	}
	if n < 1 {
		t.Fatalf("purged = %d, want at least the one expired template", n)
	}

	if got := approvalOf(t, pool, expired); got != "deleted" {
		t.Fatalf("expired template approval = %q, want deleted", got)
	}
	if neg := negativeAtOf(t, pool, expired); neg != nil {
		t.Fatalf("expired template negative_at should be cleared, got %v", neg)
	}

	if got := approvalOf(t, pool, fresh); got != "paused" {
		t.Fatalf("fresh template approval = %q, want paused", got)
	}
	if neg := negativeAtOf(t, pool, fresh); neg == nil {
		t.Fatal("fresh template must keep its negative_at inside the window")
	}
}

func TestEnqueueUnscheduledPurgesSchedulesEachEpisodeOnce(t *testing.T) {
	ctx := context.Background()
	pool := chatTestDB(t)
	svc := NewService(config.Config{TemplateNegativeGrace: 15 * time.Minute}, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	shop := seedShop(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM job_queue WHERE shop_id = $1`, shop)
		_, _ = pool.Exec(ctx, `DELETE FROM templates WHERE shop_id = $1`, shop)
		_, _ = pool.Exec(ctx, `DELETE FROM shops WHERE id = $1`, shop)
	})

	id := insertNegativeTemplate(t, pool, shop, "rejected", time.Now())

	if err := svc.enqueueUnscheduledPurges(ctx); err != nil {
		t.Fatalf("enqueueUnscheduledPurges: %v", err)
	}

	var scheduled *time.Time
	if err := pool.QueryRow(ctx, `SELECT purge_scheduled_at FROM templates WHERE id = $1`, id).Scan(&scheduled); err != nil {
		t.Fatalf("read purge_scheduled_at: %v", err)
	}
	if scheduled == nil {
		t.Fatal("an unscheduled negative template must have a purge scheduled")
	}

	// Running again must not schedule a second job for the same episode.
	if err := svc.enqueueUnscheduledPurges(ctx); err != nil {
		t.Fatalf("second enqueueUnscheduledPurges: %v", err)
	}
	var jobs int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM job_queue
		WHERE shop_id = $1 AND kind = 'purge:marketing_template'`, shop).Scan(&jobs); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if jobs != 1 {
		t.Fatalf("scheduled %d purge jobs, want exactly 1", jobs)
	}
}

func approvalOf(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) string {
	t.Helper()
	var got string
	if err := pool.QueryRow(context.Background(), `SELECT approval_status FROM templates WHERE id = $1`, id).Scan(&got); err != nil {
		t.Fatalf("read approval_status: %v", err)
	}
	return got
}

func negativeAtOf(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) *time.Time {
	t.Helper()
	var neg *time.Time
	if err := pool.QueryRow(context.Background(), `SELECT negative_at FROM templates WHERE id = $1`, id).Scan(&neg); err != nil {
		t.Fatalf("read negative_at: %v", err)
	}
	return neg
}
