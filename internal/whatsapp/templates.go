package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MaxTemplateBodyChars is Meta's hard limit for a template BODY component,
// including the {{N}} placeholders.
const MaxTemplateBodyChars = 1024

// MerchantTemplate is a merchant's template on the shared messaging account,
// tracked with Meta's review lifecycle.
type MerchantTemplate struct {
	ID               uuid.UUID
	ShopID           uuid.UUID
	Name             string
	Language         string
	Category         string
	Status           string // our lifecycle bookkeeping (submitted while awaiting review)
	ApprovalStatus   string // pending | approved | rejected | paused | deleted (Meta authority)
	RejectionReason  string
	MarketingFlagged bool   // Meta warned this template is/will be treated as marketing
	MetaWarnings     string // Meta's own warning text, verbatim, for client display
	MetaTemplateID   string
	NegativeAt       *time.Time // when the template last entered a negative state (marketing/rejected/paused); nil when non-negative
	Components       []byte     // raw Meta components JSONB (also drives variable counting on send)
	NumVariables     int        // placeholder count derived from Components
	Variables        []TokenKey // semantic map in placeholder order (nil == legacy positional)
	Source           string     // converty | delivery | any: the event source this message is written for
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// TemplateBody extracts the BODY text of a template's components, used by the
// automations UI to preview what a message looks like. Returns "" when the
// components cannot be parsed or carry no body.
func TemplateBody(t MerchantTemplate) string {
	var comps []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(t.Components, &comps); err != nil {
		return ""
	}
	for _, c := range comps {
		if c.Type == "BODY" {
			return c.Text
		}
	}
	return ""
}

// SemanticBodyForEdit renders a template's stored BODY as editable semantic
// text: positional {{1..N}} placeholders are rewritten back to their chip
// tokens so the drag-drop editor can round-trip a template for editing.
// Placeholders with no stored token fall back to the legacy positional mapping;
// unknown ones are left untouched.
func SemanticBodyForEdit(t MerchantTemplate) string {
	body := TemplateBody(t)
	if body == "" {
		return ""
	}
	return positionalPattern.ReplaceAllStringFunc(body, func(m string) string {
		n, err := strconv.Atoi(strings.Trim(m, "{}"))
		if err != nil || n < 1 {
			return m
		}
		key := tokenAtPosition(t, n)
		if key == "" {
			return m
		}
		return "{{" + string(key) + "}}"
	})
}

// tokenAtPosition resolves the chip token stored for a given 1-based positional
// placeholder, falling back to the legacy positional mapping.
func tokenAtPosition(t MerchantTemplate, pos int) TokenKey {
	if pos >= 1 && pos <= len(t.Variables) && t.Variables[pos-1] != "" {
		return t.Variables[pos-1]
	}
	return DefaultTokenForPosition(pos)
}

// TemplateExamples maps each chip token of a template to the example value Meta
// stored in its components, so the editor can prefill them on edit.
func TemplateExamples(t MerchantTemplate) map[string]string {
	var comps []struct {
		Type    string `json:"type"`
		Example struct {
			BodyText [][]string `json:"body_text"`
		} `json:"example"`
	}
	if err := json.Unmarshal(t.Components, &comps); err != nil {
		return nil
	}
	for _, c := range comps {
		if c.Type != "BODY" || len(c.Example.BodyText) == 0 {
			continue
		}
		out := make(map[string]string)
		for i, val := range c.Example.BodyText[0] {
			if key := tokenAtPosition(t, i+1); key != "" {
				out[string(key)] = val
			}
		}
		return out
	}
	return nil
}

// TemplateGraceRemaining reports how long the merchant still has to edit a
// template before it is auto-deleted. ok is false when the template is not in an
// editable negative state or the window has already closed.
func TemplateGraceRemaining(t MerchantTemplate, grace time.Duration, now time.Time) (remaining time.Duration, ok bool) {
	if !NegativeReview(t.ApprovalStatus, t.MarketingFlagged) {
		return 0, false
	}
	if t.NegativeAt == nil {
		// Legacy row with no recorded episode: treat the full window as open.
		return grace, true
	}
	deadline := t.NegativeAt.Add(grace)
	if !now.Before(deadline) {
		return 0, false
	}
	return deadline.Sub(now), true
}

// GraceStatus is TemplateGraceRemaining using the service's configured window.
func (s *Service) GraceStatus(t MerchantTemplate) (time.Duration, bool) {
	return TemplateGraceRemaining(t, s.cfg.TemplateNegativeGrace, time.Now())
}

// ErrTemplateNotEditable is returned when a template cannot be edited: it is
// missing, has no Meta id, is not in a negative review state, or its grace
// window has closed.
var ErrTemplateNotEditable = errors.New("template is not editable")

// UpdateTemplate edits a template in place at Meta and, on success, clears its
// negative state so the pending auto-delete is cancelled. Only templates in a
// negative review state (marketing, rejected, paused) with a known Meta id and
// an open grace window are editable. Meta's edit endpoint locks name and
// language, so those are read from the stored row and never sent.
func (s *Service) UpdateTemplate(ctx context.Context, templateID uuid.UUID, draft TemplateDraft) (MerchantTemplate, error) {
	t, err := s.TemplateByID(ctx, templateID)
	if err != nil {
		return MerchantTemplate{}, err
	}
	if t.ID == uuid.Nil || t.MetaTemplateID == "" {
		return MerchantTemplate{}, ErrTemplateNotEditable
	}
	if _, ok := s.GraceStatus(t); !ok {
		return MerchantTemplate{}, ErrTemplateNotEditable
	}

	creds, err := s.Credentials(ctx, t.ShopID)
	if err != nil {
		return MerchantTemplate{}, &SendRejection{Code: ErrCodeMetaAPIError, Reason: NotConnectedReason}
	}

	if err := s.metaUpdateTemplate(ctx, creds.AccessToken, t.MetaTemplateID, draft.Components); err != nil {
		// Meta refused the edit: keep the template negative and restart the
		// grace window so the merchant can correct it and try again.
		if _, dbErr := s.pool.Exec(ctx, `
			UPDATE templates
			SET approval_status = 'rejected',
			    rejection_reason = $2,
			    negative_at = now(),
			    purge_scheduled_at = NULL,
			    updated_at = now()
			WHERE id = $1`, t.ID, err.Error()); dbErr != nil {
			return MerchantTemplate{}, dbErr
		}
		return MerchantTemplate{}, &SendRejection{Code: ErrCodeMetaAPIError, Reason: err.Error()}
	}

	// Meta accepted the edit and will re-review, so clear the negative state and
	// cancel the pending purge.
	if _, err := s.pool.Exec(ctx, `
		UPDATE templates
		SET approval_status    = 'pending',
		    status             = 'submitted',
		    rejection_reason   = '',
		    marketing_flagged  = false,
		    meta_warnings      = '',
		    negative_at        = NULL,
		    purge_scheduled_at = NULL,
		    components         = $2::jsonb,
		    variables_map      = $3::jsonb,
		    source             = $4,
		    updated_at         = now()
		WHERE id = $1`,
		t.ID, draft.Components, draft.Variables, NormalizeSource(draft.Source)); err != nil {
		return MerchantTemplate{}, err
	}

	return s.TemplateByID(ctx, t.ID)
}

// TemplateDraft is what a merchant submits. Raw components are validated by
// Meta itself on creation; the platform stores them verbatim so the send gate
// can count variables and the frontend can never claim approval.
type TemplateDraft struct {
	Name       string
	Language   string
	Category   string
	Components json.RawMessage
	Variables  []TokenKey // semantic variable map (nil == legacy positional)
	Source     string     // converty | delivery | any (see SourceConverty etc.)
}

// CreateTemplate submits a template to Meta through the platform's central
// account and records the merchant-scoped row. Approval is Meta's to give;
// the platform merely reflects pending/rejected/approved as it comes back.
// Meta's creation-time warnings are captured verbatim; a warning that the
// content is treated as marketing sets MarketingFlagged (and such templates
// are never sendable). Re-submitting the same name+language refreshes the
// existing row.
func (s *Service) CreateTemplate(ctx context.Context, shopID uuid.UUID, draft TemplateDraft) (MerchantTemplate, error) {
	creds, err := s.Credentials(ctx, shopID)
	if err != nil {
		return MerchantTemplate{}, &SendRejection{Code: ErrCodeMetaAPIError, Reason: NotConnectedReason}
	}

	body := map[string]any{
		"name":       draft.Name,
		"language":   draft.Language,
		"category":   draft.Category,
		"components": json.RawMessage(draft.Components),
	}

	var resp struct {
		ID       string `json:"id"`
		Warnings []struct {
			Message string `json:"message"`
		} `json:"warnings"`
	}
	err = s.postJSON(ctx, creds.AccessToken,
		fmt.Sprintf("%s/%s/%s/message_templates", s.cfg.MetaGraphURL, metaAPIVersion, creds.MessagingAccountID),
		body, &resp)

	approval := "pending"
	rejection := ""
	warnings := warningStrings(resp.Warnings)
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
	t.MarketingFlagged = warningsIndicateMarketing(warnings)
	t.MetaWarnings = warnings
	t.MetaTemplateID = resp.ID
	t.Components = draft.Components
	t.Variables = draft.Variables
	t.Source = NormalizeSource(draft.Source)

	negative := NegativeReview(approval, t.MarketingFlagged)
	rowErr := s.pool.QueryRow(ctx, `
		INSERT INTO templates (
			shop_id, meta_template_name, meta_template_id, language, category,
			status, approval_status, rejection_reason, marketing_flagged, meta_warnings,
			negative_at, components, variables_map, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13::jsonb, $14)
		ON CONFLICT (shop_id, meta_template_name, language) DO UPDATE SET
			meta_template_id   = EXCLUDED.meta_template_id,
			category           = EXCLUDED.category,
			status             = EXCLUDED.status,
			approval_status    = EXCLUDED.approval_status,
			rejection_reason   = EXCLUDED.rejection_reason,
			marketing_flagged  = EXCLUDED.marketing_flagged,
			meta_warnings      = EXCLUDED.meta_warnings,
			-- A merchant resubmission restarts the grace window, so a rejection
			-- after an edit earns its own full window.
			negative_at        = EXCLUDED.negative_at,
			purge_scheduled_at = NULL,
			components         = EXCLUDED.components,
			variables_map      = EXCLUDED.variables_map,
			source             = EXCLUDED.source,
			updated_at         = now()
		RETURNING id, created_at, updated_at`,
		shopID, draft.Name, resp.ID, draft.Language, draft.Category,
		t.Status, approval, rejection, t.MarketingFlagged, t.MetaWarnings,
		negativeAtExpr(negative), draft.Components, draft.Variables, t.Source,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if rowErr != nil {
		return MerchantTemplate{}, fmt.Errorf("store template: %w", rowErr)
	}
	if negative {
		if err := s.enqueueUnscheduledPurges(ctx); err != nil {
			s.log.Warn("negative template purge scheduling failed", "error", err)
		}
	}
	if err != nil {
		return t, &SendRejection{Code: ErrCodeMetaAPIError, Reason: rejection}
	}
	return t, nil
}

// negativeAtExpr yields the column value assigned on insert: now() when the
// template is in a negative state right now, NULL otherwise.
func negativeAtExpr(negative bool) any {
	if negative {
		return time.Now()
	}
	return nil
}

// NegativeReview reports whether Meta left a template in a state that earns the
// grace window — a marketing flag, a rejection, or a pause — as opposed to a
// clean approved or pending review.
func NegativeReview(approvalStatus string, marketingFlagged bool) bool {
	if marketingFlagged {
		return true
	}
	return approvalStatus == "rejected" || approvalStatus == "paused"
}

// warningStrings joins Meta's warning messages into a single display string.
func warningStrings(w warnings) string {
	var b strings.Builder
	for i, w := range w {
		if w.Message == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		b.WriteString(w.Message)
		if i >= 5 {
			break
		}
	}
	return b.String()
}

// warningsIndicateMarketing reports whether Meta's creation-time warnings say
// the template is (or will be) treated as marketing content. The authoritative
// signal is Meta itself — the platform never guesses from the body text.
func warningsIndicateMarketing(warnings string) bool {
	low := strings.ToLower(warnings)
	if low == "" {
		return false
	}
	if strings.Contains(low, "not a marketing") || strings.Contains(low, "not marketing") ||
		strings.Contains(low, "isn't marketing") || strings.Contains(low, "not considered marketing") {
		return false
	}
	return strings.Contains(low, "marketing") || strings.Contains(low, "promotional")
}

// warnings is the payload subset of Meta's create-response warnings array.
type warnings []struct {
	Message string `json:"message"`
}

// SyncTemplates refreshes the approval state of one shop's templates from its
// own messaging account. Only status/rejection are updated — ownership is never
// touched, and approval never comes from the frontend. Templates Meta reports
// as MARKETING category (or rejected for marketing) are also flagged
// unsendable.
func (s *Service) SyncTemplates(ctx context.Context, shopID uuid.UUID) (int, error) {
	creds, err := s.Credentials(ctx, shopID)
	if err != nil {
		// No number connected for this shop — nothing to sync.
		return 0, nil
	}
	templates, err := s.ListTemplates(ctx, creds.AccessToken, creds.MessagingAccountID)
	if err != nil {
		return 0, err
	}
	updated := 0
	for _, t := range templates {
		approval := NormalizeApproval(t.Status)
		marketing := marketingSignal(t.Category, t.RejectedReason)
		negative := NegativeReview(approval, marketing)
		tag, uErr := s.pool.Exec(ctx, `
			UPDATE templates
			SET approval_status = $4,
			    rejection_reason = $5,
			    marketing_flagged = $6,
			    meta_template_id = CASE WHEN $8 <> '' THEN $8 ELSE meta_template_id END,
			    negative_at = CASE WHEN $7 THEN COALESCE(negative_at, now()) ELSE NULL END,
			    purge_scheduled_at = CASE WHEN $7 THEN purge_scheduled_at ELSE NULL END,
			    updated_at = now()
			WHERE shop_id = $1 AND meta_template_name = $2 AND language = $3
			  AND (approval_status <> $4
			       OR rejection_reason <> $5
			       OR marketing_flagged <> $6
			       OR (negative_at IS NOT NULL) <> $7
			       OR meta_template_id IS DISTINCT FROM $8)`,
			shopID, t.Name, t.Language, approval, t.RejectedReason, marketing, negative, t.ID,
		)
		if uErr != nil {
			return updated, uErr
		}
		updated += int(tag.RowsAffected())
	}
	if err := s.enqueueUnscheduledPurges(ctx); err != nil {
		s.log.Warn("negative template purge scheduling failed after sync", "error", err)
	}
	return updated, nil
}

// marketingSignal is Meta's post-review signal that a template is marketing:
// explicit category, or a rejection that cites the marketing policies.
func marketingSignal(category, rejectedReason string) bool {
	if strings.EqualFold(strings.TrimSpace(category), "MARKETING") {
		return true
	}
	return warningsIndicateMarketing(rejectedReason)
}

// NormalizeApproval maps Meta's uppercase statuses to the stored vocabulary.
func NormalizeApproval(status string) string {
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
		       approval_status, rejection_reason, marketing_flagged, meta_warnings,
		       meta_template_id, negative_at, components, variables_map, source, created_at, updated_at
		FROM templates
		WHERE shop_id = $1 AND approval_status <> 'deleted'
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
			&t.Status, &t.ApprovalStatus, &t.RejectionReason, &t.MarketingFlagged, &t.MetaWarnings,
			&t.MetaTemplateID, &t.NegativeAt, &t.Components, &t.Variables, &t.Source, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.NumVariables = countVariablesFromJSON(t.Components)
		out = append(out, t)
	}
	return out, rows.Err()
}

// TemplatesAll lists every template regardless of which shop it was synced
// into. On this single-client deployment templates belong to the operator, so
// an automation of any shop may reference a template created for another shop
// (they all send from the same client WhatsApp sender).
func (s *Service) TemplatesAll(ctx context.Context) ([]MerchantTemplate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, shop_id, meta_template_name, language, category, status,
		       approval_status, rejection_reason, marketing_flagged, meta_warnings,
		       meta_template_id, negative_at, components, variables_map, source, created_at, updated_at
		FROM templates
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MerchantTemplate
	for rows.Next() {
		var t MerchantTemplate
		if err := rows.Scan(&t.ID, &t.ShopID, &t.Name, &t.Language, &t.Category,
			&t.Status, &t.ApprovalStatus, &t.RejectionReason, &t.MarketingFlagged, &t.MetaWarnings,
			&t.MetaTemplateID, &t.NegativeAt, &t.Components, &t.Variables, &t.Source, &t.CreatedAt, &t.UpdatedAt); err != nil {
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
		       approval_status, rejection_reason, marketing_flagged, meta_warnings,
		       meta_template_id, negative_at, components, variables_map, source, created_at, updated_at
		FROM templates
		WHERE id = $1 AND shop_id = $2`,
		templateID, shopID,
	).Scan(&t.ID, &t.ShopID, &t.Name, &t.Language, &t.Category,
		&t.Status, &t.ApprovalStatus, &t.RejectionReason, &t.MarketingFlagged, &t.MetaWarnings,
		&t.MetaTemplateID, &t.NegativeAt, &t.Components, &t.Variables, &t.Source, &t.CreatedAt, &t.UpdatedAt)
	if err == pgx.ErrNoRows {
		return MerchantTemplate{}, nil
	}
	t.NumVariables = countVariablesFromJSON(t.Components)
	return t, err
}

// TemplateByID loads one template regardless of which shop owns it. Templates
// are shared across the platform's shops, so a request may name a row that does
// not belong to the shop the request came from.
func (s *Service) TemplateByID(ctx context.Context, templateID uuid.UUID) (MerchantTemplate, error) {
	var t MerchantTemplate
	err := s.pool.QueryRow(ctx, `
		SELECT id, shop_id, meta_template_name, language, category, status,
		       approval_status, rejection_reason, marketing_flagged, meta_warnings,
		       meta_template_id, negative_at, components, variables_map, source, created_at, updated_at
		FROM templates
		WHERE id = $1`,
		templateID,
	).Scan(&t.ID, &t.ShopID, &t.Name, &t.Language, &t.Category,
		&t.Status, &t.ApprovalStatus, &t.RejectionReason, &t.MarketingFlagged, &t.MetaWarnings,
		&t.MetaTemplateID, &t.NegativeAt, &t.Components, &t.Variables, &t.Source, &t.CreatedAt, &t.UpdatedAt)
	if err == pgx.ErrNoRows {
		return MerchantTemplate{}, nil
	}
	t.NumVariables = countVariablesFromJSON(t.Components)
	return t, err
}

// countVariablesFromJSON derives the placeholder count from stored Meta
// components so senders can build the correct variable set without re-parsing.
func countVariablesFromJSON(raw []byte) int {
	return countTemplateVariablesRaw(raw)
}
