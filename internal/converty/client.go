package converty

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultScopes = "read-stores read-orders read-hooks create-hooks delete-hooks"
	webhookPath   = "/webhooks/converty"
)

var supportedEvents = []string{"order.create", "order.update", "product.create", "product.update"}

// Token is the Converty OAuth token response.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (t Token) ExpiresAt(now time.Time) time.Time {
	return now.Add(time.Duration(t.ExpiresIn) * time.Second)
}

// Store is the authenticated seller's store (GET /api/v1/stores/me).
type Store struct {
	ID       string `json:"_id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Domain   string `json:"domain"`
	Currency string `json:"currency"`
	Country  string `json:"country"`
}

// APIError carries the Converty error payload ({success, message}).
type APIError struct {
	StatusCode int
	Message    string
}

func (e APIError) Error() string {
	return fmt.Sprintf("converty api error (status %d): %s", e.StatusCode, e.Message)
}

// AuthorizeURL builds the OAuth authorization redirect URL for a state value.
func (s *Service) AuthorizeURL(state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", s.cfg.ConvertyClientID)
	q.Set("redirect_uri", s.cfg.ConvertyRedirectURI)
	q.Set("scope", DefaultScopes)
	q.Set("state", state)
	return s.cfg.ConvertyBaseURL + "/oauth2/authorize?" + q.Encode()
}

// ExchangeCode trades an authorization code for tokens.
func (s *Service) ExchangeCode(ctx context.Context, code string) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", s.cfg.ConvertyClientID)
	form.Set("client_secret", s.cfg.ConvertyClientSecret)
	return s.requestToken(ctx, form)
}

// RefreshToken exchanges a refresh token for fresh tokens.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", s.cfg.ConvertyClientID)
	form.Set("client_secret", s.cfg.ConvertyClientSecret)
	return s.requestToken(ctx, form)
}

// GetStore returns the authenticated seller's store.
func (s *Service) GetStore(ctx context.Context, accessToken string) (Store, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.ConvertyBaseURL+"/api/v1/stores/me", nil)
	if err != nil {
		return Store{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	var resp struct {
		Success bool  `json:"success"`
		Data    Store `json:"data"`
	}
	if err := s.doJSON(req, &resp); err != nil {
		return Store{}, err
	}
	return resp.Data, nil
}

// SubscribeHook registers a webhook subscription with Converty.
func (s *Service) SubscribeHook(ctx context.Context, accessToken, targetURL, event string) error {
	body, err := json.Marshal(struct {
		TargetURL string `json:"targetUrl"`
		Event     string `json:"event"`
	}{TargetURL: targetURL, Event: event})
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.ConvertyBaseURL+"/api/v1/hooks/subscribe", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	return s.doJSON(req, &struct{}{})
}

func (s *Service) requestToken(ctx context.Context, form url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.ConvertyBaseURL+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var t Token
	if err := s.doJSON(req, &t); err != nil {
		return Token{}, err
	}
	if t.AccessToken == "" {
		return Token{}, fmt.Errorf("converty token response missing access_token")
	}
	return t, nil
}

func (s *Service) doJSON(req *http.Request, out any) error {
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("converty request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		var e struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &e)
		return APIError{StatusCode: resp.StatusCode, Message: e.Message}
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
