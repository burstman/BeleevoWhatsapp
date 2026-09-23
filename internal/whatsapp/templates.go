package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MerchantTemplate is a merchant's template on the shared messaging account,
// tracked with Meta's review lifecycle.
type MerchantTemplate struct {
	ID              uuid.UUID
	ShopID          uuid.UUID
	Name            string
	Language        string
	Category        string
	Status          string // our lifecycle bookkeeping (submitted while awaiting review)
	ApprovalStatus  string // pending | approved | rejected | paused | deleted (Meta authority)
	RejectionReason string
	MetaTemplateID  string
	Components      []byte // raw Meta components JSONB (also drives variable counting on send)
	NumVariables    int    // placeholder count derived from Components
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TemplateDraft is what a merchant submits. Raw components are validated by
// Meta itself on creation; the platform stores them verbatim so the send gate
// can count variables and the frontend can never claim approval.
type TemplateDraft struct {
	Name       string
	Language   string
	Category   string
	Components json.RawMessage
}

// CreateTemplate submits a template to Meta through the platform's central
// account and records the merchant-scoped row. Approval is Meta's to give;
// the platform merely reflects pending/rejected/approved as it comes back.
// Re-submitting the same name+language refreshes the existing row.
func (s *Service) CreateTemplate(ctx context.Context, shopID uuid.UUID, draft TemplateDraft) (MerchantTemplate, error) {
	if s.cfg.MetaSystemUserToken == "" || s.cfg.MetaMessagingAccountID == "" {
		return MerchantTemplate{}, &SendRejection{Code: ErrCodeMetaAPIError, Reason: "platform meta credentials not configured"}
	}

	body := map[string]any{
		"name":       draft.Name,
		"language":   draft.Language,
		"category":   draft.Category,
		"components": json.RawMessage(draft.Components),
	}

	var resp struct {
		ID string `json:"id"`
	}
	err := s.postJSON(ctx, s.cfg.MetaSystemUserToken,
		fmt.Sprintf("%s/%s/%s/message_templates", s.cfg.MetaGraphURL, metaAPIVersion, s.cfg.MetaMessagingAccountID),
		body, &resp)

	approval := "pending"
	rejection := ""
	if err != nil {
		// Meta refused the submission (e.g. name collision, bad variables).
		approval = "rejected"
		rejection = err.Error()
	}

	var t MerchantTemplate
	t.ShopID = shopID
	t.Name = draft.Name
	t.Language = draft.Language
	t.Category = draft.Category
	t.Status = "submitted"
	t.ApprovalStatus = approval
	t.RejectionReason = rejection
	t.MetaTemplateID = resp.ID
	t.Components = draft.Components

	rowErr := s.pool.QueryRow(ctx, `
		INSERT INTO templates (
			shop_id, meta_template_name, meta_template_id, language, category,
			status, approval_status, rejection_reason, components
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)
		ON CONFLICT (shop_id, meta_template_name, language) DO UPDATE SET
			meta_template_id  = EXCLUDED.meta_template_id,
			category          = EXCLUDED.category,
			status            = EXCLUDED.status,
			approval_status   = EXCLUDED.approval_status,
			rejection_reason  = EXCLUDED.rejection_reason,
			components        = EXCLUDED.components,
			updated_at        = now()
		RETURNING id, created_at, updated_at`,
		shopID, draft.Name, resp.ID, draft.Language, draft.Category,
		t.Status, approval, rejection, draft.Components,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if rowErr != nil {
		return MerchantTemplate{}, fmt.Errorf("store template: %w", rowErr)
	}
	if err != nil {
		return t, &SendRejection{Code: ErrCodeMetaAPIError, Reason: rejection}
	}
	return t, nil
}

// SyncTemplates refreshes the approval state of every merchant template whose
// name+language now exists on the messaging account. Only status/rejection
// are updated — ownership is never touched, and approval never comes from the
// frontend.
func (s *Service) SyncTemplates(ctx context.Context) (int, error) {
	if s.cfg.MetaSystemUserToken == "" || s.cfg.MetaMessagingAccountID == "" {
		return 0, nil
	}
	templates, err := s.ListTemplates(ctx, s.cfg.MetaSystemUserToken, s.cfg.MetaMessagingAccountID)
	if err != nil {
		return 0, err
	}
	updated := 0
	for _, t := range templates {
		tag, uErr := s.pool.Exec(ctx, `
			UPDATE templates
			SET approval_status = $2, updated_at = now()
			WHERE meta_template_name = $1 AND language = $3
			  AND approval_status <> $2`,
			t.Name, normalizeApproval(t.Status), t.Language,
		)
		if uErr != nil {
			return updated, uErr
		}
		updated += int(tag.RowsAffected())
	}
	return updated, nil
}

// normalizeApproval maps Meta's uppercase statuses to the stored vocabulary.
func normalizeApproval(status string) string {
	switch status {
	case "APPROVED":
		return "approved"
	case "PENDING", "IN_APPEAL":
		return "pending"
	case "REJECTED":
		return "rejected"
	case "PAUSED":
		return "paused"
	case "DELETED":
		return "deleted"
	default:
		return "pending"
	}
}

// Templates lists a merchant's templates, newest first.
func (s *Service) Templates(ctx context.Context, shopID uuid.UUID) ([]MerchantTemplate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, shop_id, meta_template_name, language, category, status,
		       approval_status, rejection_reason, meta_template_id, components, created_at, updated_at
		FROM templates
		WHERE shop_id = $1
		ORDER BY created_at DESC`,
		shopID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MerchantTemplate
	for rows.Next() {
		var t MerchantTemplate
		if err := rows.Scan(&t.ID, &t.ShopID, &t.Name, &t.Language, &t.Category,
			&t.Status, &t.ApprovalStatus, &t.RejectionReason, &t.MetaTemplateID,
			&t.Components, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.NumVariables = countVariablesFromJSON(t.Components)
		out = append(out, t)
	}
	return out, rows.Err()
}

// Template returns one merchant template scoped to its shop.
func (s *Service) Template(ctx context.Context, shopID, templateID uuid.UUID) (MerchantTemplate, error) {
	var t MerchantTemplate
	err := s.pool.QueryRow(ctx, `
		SELECT id, shop_id, meta_template_name, language, category, status,
		       approval_status, rejection_reason, meta_template_id, components, created_at, updated_at
		FROM templates
		WHERE id = $1 AND shop_id = $2`,
		templateID, shopID,
	).Scan(&t.ID, &t.ShopID, &t.Name, &t.Language, &t.Category,
		&t.Status, &t.ApprovalStatus, &t.RejectionReason, &t.MetaTemplateID,
		&t.Components, &t.CreatedAt, &t.UpdatedAt)
	if err == pgx.ErrNoRows {
		return MerchantTemplate{}, nil
	}
	t.NumVariables = countVariablesFromJSON(t.Components)
	return t, err
}

// countVariablesFromJSON derives the placeholder count from stored Meta
// components so senders can build the correct variable set without re-parsing.
func countVariablesFromJSON(raw []byte) int {
	var comps []TemplateComponent
	if err := json.Unmarshal(raw, &comps); err != nil {
		return 0
	}
	return countTemplateVariables(comps)
}
