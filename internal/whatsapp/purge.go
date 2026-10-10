package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/queue"
)

// PurgeNegativeTemplateJob is the delayed task payload for removing a template
// Meta did not approve (marketing flag, rejected or paused).
type PurgeNegativeTemplateJob struct {
	ShopID     uuid.UUID `json:"shop_id"`
	TemplateID uuid.UUID `json:"template_id"`
	Name       string    `json:"name"`
	Language   string    `json:"language"`
}

// enqueueUnscheduledPurges queues the purge job for every template that is in a
// negative review state but has no scheduled purge yet. The job fires when the
// grace window elapses, keyed on the episode so resubmitting cannot pile up
// duplicate purges. PurgeExpiredNegative is the lazy path that cleans up on the
// next templates page load even if a job is lost.
func (s *Service) enqueueUnscheduledPurges(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		SELECT id, shop_id, meta_template_name, language, negative_at
		FROM templates
		WHERE negative_at IS NOT NULL AND purge_scheduled_at IS NULL
		  AND approval_status <> 'deleted'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type unscheduled struct {
		PurgeNegativeTemplateJob
		NegatedAt time.Time
	}
	var jobs []unscheduled
	for rows.Next() {
		var j unscheduled
		if err := rows.Scan(&j.TemplateID, &j.ShopID, &j.Name, &j.Language, &j.NegatedAt); err != nil {
			return err
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, j := range jobs {
		if err := s.enqueuePurge(ctx, j.PurgeNegativeTemplateJob, j.NegatedAt); err != nil {
			s.log.Warn("negative template purge enqueue failed", "template_id", j.TemplateID, "error", err)
			continue
		}
		if _, err := s.pool.Exec(ctx,
			`UPDATE templates SET purge_scheduled_at = now() WHERE id = $1`, j.TemplateID); err != nil {
			return err
		}
	}
	return nil
}

// enqueuePurge parks one purge until the grace window from negatedAt has passed.
// The dedupe key names the episode (template id plus when it turned negative) so
// a later re-flag of the same template can still schedule its own purge.
func (s *Service) enqueuePurge(ctx context.Context, j PurgeNegativeTemplateJob, negatedAt time.Time) error {
	payload, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return queue.Enqueue(ctx, s.pool, queue.Params{
		Kind:      queue.TaskPurgeNegativeTemplate,
		ShopID:    j.ShopID,
		Payload:   payload,
		RunAt:     negatedAt.Add(s.cfg.TemplateNegativeGrace),
		DedupeKey: fmt.Sprintf("%s:%d", j.TemplateID, negatedAt.Unix()),
	})
}

// PurgeExpiredNegative marks every template whose negative grace window has
// passed as deleted: marketing-flagged, rejected and paused alike. It is a soft
// delete (the row stays for history) and is safe to run repeatedly. Returns the
// number newly deleted.
func (s *Service) PurgeExpiredNegative(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, shop_id, meta_template_name, language
		FROM templates
		WHERE negative_at IS NOT NULL
		  AND negative_at < now() - ($1::interval)
		  AND approval_status <> 'deleted'`,
		pgInterval(s.cfg.TemplateNegativeGrace),
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var expired []PurgeNegativeTemplateJob
	for rows.Next() {
		var j PurgeNegativeTemplateJob
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
			s.log.Warn("negative template purge failed; will retry", "template_id", j.TemplateID, "error", err)
		}
	}
	return len(expired), nil
}

// HandlePurgeNegativeTemplate is the queue handler for the delayed purge job. It
// is safe to run repeatedly: missing rows, templates already deleted, or
// templates that turned non-negative (e.g. successfully edited) are skipped.
func (s *Service) HandlePurgeNegativeTemplate(ctx context.Context, payload []byte) error {
	var j PurgeNegativeTemplateJob
	if err := json.Unmarshal(payload, &j); err != nil {
		return queue.Permanent(fmt.Errorf("whatsapp: decode purge job: %w", err))
	}

	var negativeAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT negative_at FROM templates WHERE id = $1 AND shop_id = $2`,
		j.TemplateID, j.ShopID,
	).Scan(&negativeAt)
	if err != nil {
		// Row already gone — job is done.
		return nil
	}
	// A NULL negative_at means the template was edited or approved since the
	// purge was scheduled, so there is nothing left to remove.
	if negativeAt == nil || time.Now().Before(negativeAt.Add(s.cfg.TemplateNegativeGrace)) {
		return nil
	}
	return s.purgeTemplate(ctx, j)
}

// DeleteByMerchant removes a template at the operator's request: best-effort
// removal from the messaging account that owns it, then the local row moves to
// the "deleted" lifecycle state so it can never be selected or sent. Same
// lifecycle as the review-grace purge; only the trigger differs.
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
		    negative_at        = NULL,
		    updated_at         = now()
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
func (s *Service) purgeTemplate(ctx context.Context, j PurgeNegativeTemplateJob) error {
	s.log.Warn("deleting negative template after grace window",
		"shop_id", j.ShopID, "template_id", j.TemplateID, "name", j.Name, "language", j.Language)

	if creds, cErr := s.Credentials(ctx, j.ShopID); cErr == nil {
		if err := s.DeleteTemplate(ctx, creds.AccessToken, creds.MessagingAccountID, j.Name, j.Language); err != nil {
			// Best-effort: Meta may already have removed it (404). The local
			// row still transitions to deleted; the shop's account stays under
			// Meta's policy.
			s.log.Warn("meta delete of template failed", "name", j.Name, "error", err)
		}
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE templates
		SET approval_status = 'deleted',
		    purge_scheduled_at = NULL,
		    negative_at        = NULL,
		    updated_at         = now()
		WHERE id = $1 AND shop_id = $2 AND approval_status <> 'deleted'`,
		j.TemplateID, j.ShopID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		s.log.Info("negative template deleted", "shop_id", j.ShopID, "template_id", j.TemplateID)
	}
	return nil
}
