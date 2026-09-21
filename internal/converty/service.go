package converty

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/encrypt"
)

var ErrNotConfigured = errors.New("converty is not configured")

// refreshBuffer refreshes the access token five minutes before it actually
// expires, so in-flight requests never race an about-to-expire credential.
const refreshBuffer = 5 * time.Minute

// Service talks to the Converty OAuth server and persists shop integrations.
type Service struct {
	cfg    config.Config
	pool   *pgxpool.Pool
	log    *slog.Logger
	cipher *encrypt.Cipher
	http   *http.Client
}

func NewService(cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) *Service {
	cipher, err := encrypt.New(cfg.EncryptionKeyResolved())
	if err != nil {
		log.Error("converty encryption key invalid; tokens will not persist", "error", err)
	}
	return &Service{
		cfg:    cfg,
		pool:   pool,
		log:    log,
		cipher: cipher,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

// Configured reports whether OAuth credentials are present.
func (s *Service) Configured() bool {
	return s.cfg.ConvertyClientID != "" && s.cfg.ConvertyClientSecret != ""
}

// SupportedEvents are the webhook events subscribed after connecting.
func SupportedEvents() []string {
	return append([]string(nil), supportedEvents...)
}

// DecryptToken unwraps an encrypted Converty token stored for an integration.
func (s *Service) DecryptToken(encrypted string) (string, error) {
	if s.cipher == nil {
		return "", errors.New("converty encryption not configured")
	}
	return s.cipher.Decrypt(encrypted)
}

// AccessToken returns an access token for the shop that is valid for at least
// the refresh buffer horizon, rotating the stored refresh token when the
// saved access token is expired or missing.
func (s *Service) AccessToken(ctx context.Context, shopID uuid.UUID) (string, error) {
	integ, err := s.Integration(ctx, shopID)
	if err != nil {
		return "", err
	}
	if integ.AccessTokenExpiresAt != nil &&
		time.Now().Add(refreshBuffer).Before(*integ.AccessTokenExpiresAt) {
		if token, dErr := s.DecryptToken(integ.AccessTokenEncrypted); dErr == nil && token != "" {
			return token, nil
		}
	}
	return s.refreshAndSave(ctx, integ, time.Now())
}

// WithAccessToken runs fn with an unexpired token for the shop. If the server
// rejects that token with 401 (revoked despite the local expiry clock), the
// refresh token is rotated once and fn is retried with the fresh token. The
// refreshed pair is always stored, since Converty rotates refresh tokens.
func (s *Service) WithAccessToken(ctx context.Context, shopID uuid.UUID, fn func(ctx context.Context, accessToken string) error) error {
	token, err := s.AccessToken(ctx, shopID)
	if err != nil {
		return err
	}
	if err := fn(ctx, token); err != nil {
		var apiErr APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
			return err
		}
		integ, iErr := s.Integration(ctx, shopID)
		if iErr != nil {
			return err
		}
		fresh, rErr := s.refreshAndSave(ctx, integ, time.Now())
		if rErr != nil {
			return err
		}
		return fn(ctx, fresh)
	}
	return nil
}

func (s *Service) refreshAndSave(ctx context.Context, integ Integration, now time.Time) (string, error) {
	refreshToken, err := s.DecryptToken(integ.RefreshTokenEncrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt refresh token: %w", err)
	}
	tok, err := s.RefreshToken(ctx, refreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh converty token: %w", err)
	}
	if err := s.SaveTokens(ctx, integ.ShopID, tok, now); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// WebhookURL returns the absolute webhook target for a UI-origin AppURL.
func (s *Service) WebhookURL(appURL string) string {
	for len(appURL) > 0 && appURL[len(appURL)-1] == '/' {
		appURL = appURL[:len(appURL)-1]
	}
	return appURL + webhookPath
}
