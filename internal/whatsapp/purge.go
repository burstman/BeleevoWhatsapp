package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"whatsappconverty/internal/queue"
)

// PurgeMarketingTemplateJob is the delayed task payload for removing a
// template Meta flagged as marketing.
type PurgeMarketingTemplateJob struct {
	ShopID     uuid.UUID `json:"shop_id"`
	TemplateID uuid.UUID `json:"template_id"`
	Name       string    `json:"name"`
	Language   string    `json:"language"`
}

// enqueueUnscheduledPurges queues the purge job for every template that is
// flagged as marketing but has no scheduled purge yet. The task fires after
// the configured delay; asynq's Unique option dedupes re-submissions of the
// same job. When no asynq client exists (no Redis), the lazy purge path in
// PurgeExpiredMarketing still cleans up on the next templates page load.
func (s *Service) enqueueUnscheduledPurges(ctx context.Context) error {
	if s.queue == nil {
		return nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, shop_id, meta_template_name, language
		FROM templates
		WHERE marketing_flagged AND purge_scheduled_at IS NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var jobs []PurgeMarketingTemplateJob
	for rows.Next() {
		var j PurgeMarketingTemplateJob
		if err := rows.Scan(&j.TemplateID, &j.ShopID, &j.Name, &j.Language); err != nil {
			return err
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, j := range jobs {
		if err := s.enqueuePurge(ctx, j); err != nil {
			s.log.Warn("marketing purge enqueue failed", "template_id", j.TemplateID, "error", err)
			continue
		}
		if _, err := s.pool.Exec(ctx,
			`UPDATE templates SET purge_scheduled_at = now() WHERE id = $1`, j.TemplateID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) enqueuePurge(ctx context.Context, j PurgeMarketingTemplateJob) error {
	payload, err := json.Marshal(j)
	if err != nil {
		return err
	}
	task := asynq.NewTask(queue.TaskPurgeMarketingTemplate, payload)
	_, err = s.queue.Enqueue(task,
		asynq.ProcessIn(s.cfg.MarketingPurgeDelay),
		asynq.Unique(s.cfg.MarketingPurgeDelay*4), // collapse re-submits of the same row
		asynq.Retention(24*time.Hour),
	)
	return err
}

// PurgeExpiredMarketing marks every flagged template whose purge deadline has
// passed as deleted. It is the lazy fallback that keeps the "gone after N
// minutes" promise even when no worker is running, and the row stays visible
// to the merchant as "deleted". Returns the number newly deleted.
func (s *Service) PurgeExpiredMarketing(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, shop_id, meta_template_name, language
		FROM templates
		WHERE marketing_flagged
		  AND marketing_flagged_at IS NOT NULL
		  AND marketing_flagged_at < now() - ($1::interval)
		  AND approval_status <> 'deleted'`,
		pgInterval(s.cfg.MarketingPurgeDelay),
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var expired []PurgeMarketingTemplateJob
	for rows.Next() {
		var j PurgeMarketingTemplateJob
		if err := rows.Scan(&j.TemplateID, &j.ShopID, &j.Name, &j.Language); err != nil {
			return 0, err
		}
		expired = append(expired, j)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, j := range expired {
		if err := s.purgeTemplate(ctx, j); err != nil {
			s.log.Warn("marketing purge failed; will retry", "template_id", j.TemplateID, "error", err)
		}
	}
	return len(expired), nil
}

// HandlePurgeMarketingTemplate is the asynq worker handler for the delayed
// purge job. It is safe to run repeatedly: missing rows or templates that are
// no longer flagged are skipped.
func (s *Service) HandlePurgeMarketingTemplate(ctx context.Context, task *asynq.Task) error {
	var j PurgeMarketingTemplateJob
	if err := json.Unmarshal(task.Payload(), &j); err != nil {
		return err
	}

	var stillFlagged bool
	err := s.pool.QueryRow(ctx, `
		SELECT marketing_flagged FROM templates WHERE id = $1 AND shop_id = $2`,
		j.TemplateID, j.ShopID,
	).Scan(&stillFlagged)
	if err != nil {
		// Row already gone — job is done.
		return nil
	}
	if !stillFlagged {
		return nil
	}
	return s.purgeTemplate(ctx, j)
}

// DeleteByMerchant removes a template at the operator's request: best-effort
// removal from the messaging account that owns it, then the local row moves to
// the "deleted" lifecycle state so it can never be selected or sent. Same
// lifecycle as the marketing purge; only the trigger differs.
func (s *Service) DeleteByMerchant(ctx context.Context, t MerchantTemplate) error {
	s.log.Info("deleting template at operator request",
		"shop_id", t.ShopID, "template_id", t.ID, "name", t.Name, "language", t.Language)

	if creds, cErr := s.Credentials(ctx, t.ShopID); cErr == nil {
		if err := s.DeleteTemplate(ctx, creds.AccessToken, creds.MessagingAccountID, t.Name, t.Language); err != nil {
			// Best-effort: Meta may already have removed it (404). The local row
			// still transitions to deleted, which is what actually stops sends.
			s.log.Warn("meta delete of template failed", "name", t.Name, "error", err)
		}
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE templates
		SET approval_status = 'deleted',
		    purge_scheduled_at = NULL,
		    updated_at        = now()
		WHERE id = $1 AND shop_id = $2 AND approval_status <> 'deleted'`,
		t.ID, t.ShopID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("template is already deleted")
	}
	return nil
}

// pgInterval converts a Go duration to text Postgres accepts as an interval
// literal (time.Duration.String() like "15m0s" is not valid Postgres syntax).
func pgInterval(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int64(d.Seconds()))
}

// purgeTemplate moves the template to the "deleted" lifecycle state so the
// merchant sees the deletion notice, after best-effort removal from the
// shop's own messaging account. Deleted templates can never be sent.
func (s *Service) purgeTemplate(ctx context.Context, j PurgeMarketingTemplateJob) error {
	s.log.Warn("deleting marketing-flagged template",
		"shop_id", j.ShopID, "template_id", j.TemplateID, "name", j.Name, "language", j.Language)

	if creds, cErr := s.Credentials(ctx, j.ShopID); cErr == nil {
		if err := s.DeleteTemplate(ctx, creds.AccessToken, creds.MessagingAccountID, j.Name, j.Language); err != nil {
			// Best-effort: Meta may already have removed it (404). The local
			// row still transitions to deleted; the shop's account stays under
			// Meta's policy.
			s.log.Warn("meta delete of marketing template failed", "name", j.Name, "error", err)
		}
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE templates
		SET approval_status = 'deleted',
		    purge_scheduled_at = NULL,
		    updated_at        = now()
		WHERE id = $1 AND shop_id = $2 AND approval_status <> 'deleted'`,
		j.TemplateID, j.ShopID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		s.log.Info("marketing template deleted", "shop_id", j.ShopID, "template_id", j.TemplateID)
	}
	return nil
}
