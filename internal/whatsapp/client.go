package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// metaAPIVersion is the Graph API version the platform targets. The WhatsApp
// Cloud API is currently on v21.0.
const metaAPIVersion = "v21.0"

// TemplateComponent is one variable-bearing part of a template send
// (e.g. a BODY with positional parameters).
type TemplateComponent struct {
	Type       string              `json:"type"`
	SubType    string              `json:"sub_type,omitempty"`
	Index      string              `json:"index,omitempty"`
	Parameters []TemplateParameter `json:"parameters,omitempty"`
}

// TemplateParameter supplies a value for a {{N}} placeholder in a component.
type TemplateParameter struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Template is one approved/available template on a messaging account,
// listed via GET /<messaging_account_id>/message_templates. Components stay
// opaque (json.RawMessage) since their shape is version-specific.
type Template struct {
	Name           string          `json:"name"`
	Status         string          `json:"status"`
	Category       string          `json:"category"`
	Language       string          `json:"language"`
	RejectedReason string          `json:"rejected_reason"`
	Components     json.RawMessage `json:"components"`
}

// PhoneNumber is a WhatsApp number owned by the messaging account, used for
// the connect-time sanity check.
type PhoneNumber struct {
	ID                     string `json:"id"`
	DisplayPhoneNumber     string `json:"display_phone_number"`
	VerifiedName           string `json:"verified_name"`
	QualityRating          string `json:"quality_rating"`
	NameStatus             string `json:"name_status"`
	CodeVerificationStatus string `json:"code_verification_status"`
}

// MessageNumber is the minimal shape of a number when listing the messaging
// account's phone_numbers connection. PhoneNumber is kept for the
// connect/seed sanity checks (it carries quality/verification fields);
// MessageNumber is what the BSP number-pool importer needs.
type MessageNumber struct {
	ID                 string `json:"id"`
	DisplayPhoneNumber string `json:"display_phone_number"`
	VerifiedName       string `json:"verified_name"`
}

// ListMessageNumbers returns the phone numbers registered under a messaging
// account — the platform's rentable number pool for the BSP onboarding.
func (s *Service) ListMessageNumbers(ctx context.Context, token, messagingAccountID string) ([]MessageNumber, error) {
	q := url.Values{}
	q.Set("fields", "id,display_phone_number,verified_name")
	q.Set("limit", "100")

	var resp struct {
		Data []MessageNumber `json:"data"`
	}
	if err := s.getJSON(ctx, token, s.cfg.MetaGraphURL+"/"+metaAPIVersion+"/"+messagingAccountID+"/phone_numbers?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// Option is a functional option for the Meta build of a message send.
type Option func(map[string]any)

// MessagingAccountParam attaches the messaging_account_id parameter, which is
// optional in Phase 1 but required for multi-messaging-account setups from
// Phase 2 of the new account model.
func MessagingAccountParam(messagingAccountID string) Option {
	return func(m map[string]any) {
		if messagingAccountID != "" {
			m["messaging_account_id"] = messagingAccountID
		}
	}
}

// SendTemplate delivers a template message to a recipient. components must
// cover every {{N}} placeholder defined by the template. It returns the Meta
// message id assigned to the delivery.
func (s *Service) SendTemplate(ctx context.Context, token, phoneNumberID, to, name, language string, components []TemplateComponent, opts ...Option) (string, error) {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to,
		"type":              "template",
		"template": map[string]any{
			"name": name,
			"language": map[string]string{
				"code": language,
			},
		},
	}
	if components != nil {
		payload["template"].(map[string]any)["components"] = components
	}
	for _, opt := range opts {
		opt(payload)
	}

	var resp struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := s.postJSON(ctx, token, fmt.Sprintf("%s/%s/%s/messages", s.cfg.MetaGraphURL, metaAPIVersion, phoneNumberID), payload, &resp); err != nil {
		return "", err
	}
	if len(resp.Messages) == 0 {
		return "", fmt.Errorf("whatsapp send accepted without a message id")
	}
	return resp.Messages[0].ID, nil
}

// ListTemplates returns all templates on the messaging account. Categories
// (utility/marketing/authentication) and status are included for UI use.
func (s *Service) ListTemplates(ctx context.Context, token, messagingAccountID string) ([]Template, error) {
	q := url.Values{}
	q.Set("fields", "name,status,category,language,rejected_reason,components")
	q.Set("limit", "100")

	var resp struct {
		Data []Template `json:"data"`
	}
	if err := s.getJSON(ctx, token, s.cfg.MetaGraphURL+"/"+metaAPIVersion+"/"+messagingAccountID+"/message_templates?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// DeleteTemplate removes a template from the messaging account by
// name+language.
func (s *Service) DeleteTemplate(ctx context.Context, token, messagingAccountID, name, language string) error {
	q := url.Values{}
	q.Set("name", name)
	q.Set("language", language)
	return s.deleteJSON(ctx, token,
		s.cfg.MetaGraphURL+"/"+metaAPIVersion+"/"+messagingAccountID+"/message_templates?"+q.Encode())
}

// GetPhoneNumber verifies a phone-number id resolves and returns its identity
// details, for connect/seed-time sanity checks.
func (s *Service) GetPhoneNumber(ctx context.Context, token, phoneNumberID string) (PhoneNumber, error) {
	q := url.Values{}
	q.Set("fields", "id,display_phone_number,verified_name,quality_rating,name_status,code_verification_status")

	var p PhoneNumber
	if err := s.getJSON(ctx, token, s.cfg.MetaGraphURL+"/"+metaAPIVersion+"/"+phoneNumberID+"?"+q.Encode(), &p); err != nil {
		return PhoneNumber{}, err
	}
	return p, nil
}

// APIError carries a Meta Graph error payload.
type APIError struct {
	StatusCode int
	Code       int
	SubCode    int
	Message    string
	Type       string
}

func (e APIError) Error() string {
	return fmt.Sprintf("meta api error (status %d): %s", e.StatusCode, e.Message)
}

// graphError holds the Meta Graph error envelope {error:{message,...}}.
type graphError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
		SubCode int    `json:"error_subcode"`
	} `json:"error"`
}

func (s *Service) getJSON(ctx context.Context, token, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return s.doJSON(req, out)
}

func (s *Service) deleteJSON(ctx context.Context, token, endpoint string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	var out any
	return s.doJSON(req, &out)
}

func (s *Service) postJSON(ctx context.Context, token, endpoint string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(raw)))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return s.doJSON(req, out)
}

func (s *Service) doJSON(req *http.Request, out any) error {
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("meta request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		var g graphError
		_ = json.Unmarshal(body, &g)
		if g.Error.Message == "" {
			snippet := strings.TrimSpace(string(body))
			if len(snippet) > 300 {
				snippet = snippet[:300]
			}
			g.Error.Message = snippet
		}
		return APIError{StatusCode: resp.StatusCode, Code: g.Error.Code, SubCode: g.Error.SubCode, Message: g.Error.Message, Type: g.Error.Type}
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
