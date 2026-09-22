package whatsapp

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Public error codes returned by the HTTP API and the send gate. They are
// stable identifiers the frontend/API can match on without leaking internals.
const (
	ErrCodeMerchantNotAuthorized     = "merchant_not_authorized"
	ErrCodeServiceDisabled           = "whatsapp_service_disabled"
	ErrCodeTermsNotAccepted          = "whatsapp_terms_not_accepted"
	ErrCodeCustomerNotOwned          = "customer_not_owned_by_merchant"
	ErrCodeOptInRequired             = "whatsapp_opt_in_required"
	ErrCodeOptInRevoked              = "whatsapp_opt_in_revoked"
	ErrCodeTemplateNotFound          = "template_not_found"
	ErrCodeTemplateNotApproved       = "template_not_approved"
	ErrCodeTemplateUnintendedPurpose = "template_unintended_purpose"
	ErrCodeTemplateVariableInvalid   = "template_variable_invalid"
	ErrCodeRateLimited               = "rate_limited"
	ErrCodeMetaAPIError              = "meta_api_error"
	ErrCodeDuplicateSend             = "duplicate_send"
)

// SendRejection records why a message was refused before Meta was contacted.
type SendRejection struct {
	Code   string
	Reason string
}

func (e *SendRejection) Error() string {
	return e.Code + ": " + e.Reason
}

// NewSendRejection builds a *SendRejection with a stable code.
func NewSendRejection(code, reason string) *SendRejection {
	return &SendRejection{Code: code, Reason: reason}
}

// MerchantState is the merchant's WhatsApp eligibility snapshot read from the
// shops row.
type MerchantState struct {
	ShopID          uuid.UUID
	Status          string
	WhatsappEnabled bool
	TermsAcceptedAt *time.Time
}

// CustomerState is the tenant-scoped customer ownership snapshot.
type CustomerState struct {
	ID          uuid.UUID
	Phone       string
	OwnedByShop bool
}

// ConsentState is the customer's latest WhatsApp consent snapshot. Status ""
// means no record exists (never opted in).
type ConsentState struct {
	Status   string // "" | opt_in | revoked
	Category string
}

// TemplateState is the merchant's template snapshot used by the send gate.
type TemplateState struct {
	ID             uuid.UUID
	Name           string
	Language       string
	Category       string
	ApprovalStatus string // pending | approved | rejected | paused | deleted
	Components     []TemplateComponent
	NumVariables   int
}

// SendRequest is a single message send attempt. Purpose is the business
// category the sender declares (e.g. "order_updates"); it must match the
// template category. IdempotencyKey makes retries safe ("" = no dedup).
type SendRequest struct {
	ShopID          uuid.UUID
	CustomerID      uuid.UUID
	TemplateID      uuid.UUID
	ConvertyOrderID string
	Purpose         string
	Variables       map[string]string
	IdempotencyKey  string
}

// SendResult reports the outcome of an accepted send.
type SendResult struct {
	MessageID     uuid.UUID
	MetaMessageID string
	Status        string
}

// EvaluateSendRule is the pure CAN_SEND rule. It must verify every condition
// before any Meta call. Returning nil means the send is permitted.
func EvaluateSendRule(merchant MerchantState, customer CustomerState, consent ConsentState, template TemplateState, req SendRequest) error {
	if merchant.Status != "active" {
		return NewSendRejection(ErrCodeMerchantNotAuthorized, "merchant account is not active")
	}
	if !merchant.WhatsappEnabled {
		return NewSendRejection(ErrCodeServiceDisabled, "whatsapp service is not enabled for this shop")
	}
	if merchant.TermsAcceptedAt == nil {
		return NewSendRejection(ErrCodeTermsNotAccepted, "whatsapp service terms not accepted by the merchant")
	}
	if !customer.OwnedByShop {
		return NewSendRejection(ErrCodeCustomerNotOwned, "customer does not belong to this merchant")
	}
	switch consent.Status {
	case "":
		return NewSendRejection(ErrCodeOptInRequired, "customer has no recorded whatsapp opt-in")
	case "revoked":
		return NewSendRejection(ErrCodeOptInRevoked, "customer whatsapp consent was revoked")
	case "opt_in":
		if consent.Category != "" && req.Purpose != "" && consent.Category != req.Purpose {
			return NewSendRejection(ErrCodeOptInRevoked, "consent category does not cover the requested purpose")
		}
	default:
		return NewSendRejection(ErrCodeOptInRequired, "customer whatsapp consent state is invalid")
	}
	if template.ID == uuid.Nil || template.Name == "" {
		return NewSendRejection(ErrCodeTemplateNotFound, "template not found for this merchant")
	}
	if template.ApprovalStatus != "approved" {
		return NewSendRejection(ErrCodeTemplateNotApproved, "template is not approved ("+template.ApprovalStatus+")")
	}
	if req.Purpose != "" && template.Category != req.Purpose {
		return NewSendRejection(ErrCodeTemplateUnintendedPurpose, "template category does not match the requested purpose")
	}
	if err := validateVariables(template, req.Variables); err != nil {
		return err
	}
	return nil
}

// validateVariables ensures the variables map covers every {{N}} placeholder
// defined by the template's approved components, and nothing more.
func validateVariables(t TemplateState, vars map[string]string) error {
	if len(vars) != t.NumVariables {
		return NewSendRejection(ErrCodeTemplateVariableInvalid,
			"template requires "+strconv.Itoa(t.NumVariables)+" variables, got "+strconv.Itoa(len(vars)))
	}
	return nil
}

// buildComponents turns positional variables into the Meta component body,
// matching the template's stored components. Variables are ordered by the
// {{N}} index in the first variable-bearing component (body, or header when
// the body has none).
func buildComponents(t TemplateState, vars map[string]string) ([]TemplateComponent, error) {
	var components []TemplateComponent
	for _, c := range t.Components {
		switch c.Type {
		case "header", "body":
			text := ""
			for _, p := range c.Parameters {
				if p.Type == "text" && strings.Contains(p.Text, "{{") {
					text = p.Text
					break
				}
			}
			if text == "" {
				components = append(components, c)
				continue
			}
			numbers := placeholderNumbers(text)
			params := make([]TemplateParameter, 0, len(numbers))
			for _, n := range numbers {
				v, ok := vars[strconv.Itoa(n)]
				if !ok {
					return nil, NewSendRejection(ErrCodeTemplateVariableInvalid, "missing variable {{"+strconv.Itoa(n)+"}}")
				}
				params = append(params, TemplateParameter{Type: "text", Text: v})
			}
			components = append(components, TemplateComponent{Type: c.Type, Parameters: params})
		case "button":
			// URL buttons may carry the trailing {{1}} substitution as the last
			// parameter set. Variables were already validated above.
			components = append(components, c)
		default:
			components = append(components, c)
		}
	}

	// Append the URL-button parameter set that Meta requires when any button
	// element references a placeholder (verified on order_confirmed_v2).
	if hasURLPlaceholder(t.Components) {
		components = append(components, TemplateComponent{
			Type: "button",
			Parameters: []TemplateParameter{
				{Type: "text", Text: vars["1"]},
			},
		})
	}
	return components, nil
}

// countTemplateVariables counts the distinct {{N}} placeholders across the
// template's header/body text.
func countTemplateVariables(components []TemplateComponent) int {
	seen := map[int]bool{}
	for _, c := range components {
		if c.Type != "header" && c.Type != "body" {
			continue
		}
		for _, n := range placeholderNumbers(componentText(c)) {
			seen[n] = true
		}
	}
	return len(seen)
}

func componentText(c TemplateComponent) string {
	if len(c.Parameters) == 0 || c.Parameters[0].Type != "text" {
		return ""
	}
	return c.Parameters[0].Text
}

func hasURLPlaceholder(components []TemplateComponent) bool {
	for _, c := range components {
		if c.Type == "button" && strings.Contains(componentText(c), "{{") {
			return true
		}
	}
	return false
}

// placeholderNumbers returns the {{N}} indexes in text order.
func placeholderNumbers(text string) []int {
	var nums []int
	for i := 0; i < len(text); i++ {
		if text[i] != '{' || i+1 >= len(text) || text[i+1] != '{' {
			continue
		}
		j := i + 2
		for j < len(text) && text[j] >= '0' && text[j] <= '9' {
			j++
		}
		if j > i+2 && j < len(text) && text[j] == '}' {
			if n, err := strconv.Atoi(text[i+2 : j]); err == nil {
				nums = append(nums, n)
			}
		}
		i = j
	}
	return nums
}

// SendTemplateMessage is the full tenant-scoped send flow: it loads the
// merchant/customer/consent/template state, applies CAN_SEND, enforces rate
// and idempotency controls, then calls Meta and records the outcome. Any
// refusal surfaces as *SendRejection and no Meta call is made.
func (s *Service) SendTemplateMessage(ctx context.Context, req SendRequest) (*SendResult, error) {
	if req.IdempotencyKey != "" {
		existing, err := s.findMessageByIdempotency(ctx, req.ShopID, req.IdempotencyKey)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}

	merchant, err := s.merchantState(ctx, req.ShopID)
	if err != nil {
		return nil, err
	}
	customer, err := s.customerState(ctx, req.ShopID, req.CustomerID)
	if err != nil {
		return nil, err
	}
	consent, err := s.consentState(ctx, req.ShopID, req.CustomerID)
	if err != nil {
		return nil, err
	}
	template, err := s.templateState(ctx, req.ShopID, req.TemplateID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, NewSendRejection(ErrCodeTemplateNotFound, "template not found for this merchant")
		}
		return nil, err
	}

	if err := EvaluateSendRule(merchant, customer, consent, template, req); err != nil {
		var rej *SendRejection
		if errors.As(err, &rej) {
			s.log.Warn("whatsapp send rejected",
				"shop_id", req.ShopID, "customer_id", req.CustomerID,
				"template_id", req.TemplateID, "code", rej.Code, "reason", rej.Reason)
		}
		return nil, err
	}

	if s.rate != nil {
		if err := s.rate.Allow(ctx, req.ShopID); err != nil {
			return nil, err
		}
	}

	components, err := buildComponents(template, req.Variables)
	if err != nil {
		return nil, err
	}

	msgID, err := s.createQueuedMessage(ctx, req, template)
	if err != nil {
		return nil, err
	}

	if s.cfg.MetaSystemUserToken == "" || s.cfg.MetaPhoneNumberID == "" {
		_ = s.markMessageFailed(ctx, msgID, ErrCodeMetaAPIError, "platform meta credentials not configured")
		return nil, NewSendRejection(ErrCodeMetaAPIError, "platform meta credentials not configured")
	}

	metaID, sendErr := s.SendTemplate(ctx, s.cfg.MetaSystemUserToken, s.cfg.MetaPhoneNumberID,
		customer.Phone, template.Name, template.Language, components,
		MessagingAccountParam(s.cfg.MetaMessagingAccountID))
	if sendErr != nil {
		_ = s.markMessageFailed(ctx, msgID, ErrCodeMetaAPIError, sendErr.Error())
		return nil, wrapMetaError(sendErr)
	}

	if err := s.markMessageSent(ctx, msgID, metaID); err != nil {
		s.log.Error("whatsapp: could not mark message sent", "error", err)
	}
	s.log.Info("whatsapp message sent",
		"shop_id", req.ShopID, "customer_id", req.CustomerID,
		"template", template.Name, "meta_message_id", metaID)

	return &SendResult{MessageID: msgID, MetaMessageID: metaID, Status: "sent"}, nil
}

func wrapMetaError(err error) error {
	var apiErr APIError
	if errors.As(err, &apiErr) {
		return NewSendRejection(ErrCodeMetaAPIError, apiErr.Message)
	}
	return NewSendRejection(ErrCodeMetaAPIError, err.Error())
}
