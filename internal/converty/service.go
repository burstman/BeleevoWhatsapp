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

// Configured reports whether Converty is reachable and the OAuth authorize
// endpoint is known. Per-integration client credentials are entered by each
// merchant, so this no longer depends on global client id/secret values.
func (s *Service) Configured() bool {
	return s.cfg.ConvertyBaseURL != ""
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

// EncryptClientSecret wraps a merchant's Converty client secret; it is stored
// per integration and decrypted only when an OAuth call needs it.
func (s *Service) EncryptClientSecret(secret string) (string, error) {
	if s.cipher == nil {
		return "", errors.New("converty encryption not configured")
	}
	return s.cipher.Encrypt(secret)
}

// AccessToken returns an access token for one of the shop's integrations
// that is valid for at least the refresh buffer horizon, rotating the stored
// refresh token when the saved access token is expired or missing.
func (s *Service) AccessToken(ctx context.Context, shopID, id uuid.UUID) (string, error) {
	integ, err := s.IntegrationByID(ctx, shopID, id)
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

// WithAccessToken runs fn with an unexpired token for one of the shop's
// integrations. If the server rejects that token with 401 (revoked despite
// the local expiry clock), the refresh token is rotated once and fn is
// retried with the fresh token. The refreshed pair is always stored, since
// Converty rotates refresh tokens.
func (s *Service) WithAccessToken(ctx context.Context, shopID, id uuid.UUID, fn func(ctx context.Context, accessToken string) error) error {
	token, err := s.AccessToken(ctx, shopID, id)
	if err != nil {
		return err
	}
	if err := fn(ctx, token); err != nil {
		var apiErr APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
			return err
		}
		integ, iErr := s.IntegrationByID(ctx, shopID, id)
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

// RefreshIntegrationToken force-rotates an integration's Converty tokens via
// the OAuth refresh grant and persists the fresh pair, so a shop client can
// recover a connection whose server-side refresh token was rotated.
func (s *Service) RefreshIntegrationToken(ctx context.Context, shopID, id uuid.UUID) error {
	integ, err := s.IntegrationByID(ctx, shopID, id)
	if err != nil {
		return err
	}
	refreshToken, err := s.DecryptToken(integ.RefreshTokenEncrypted)
	if err != nil {
		return fmt.Errorf("decrypt refresh token: %w", err)
	}
	clientID, clientSecret, err := s.ClientCredentials(ctx, integ.ShopID, integ.ID)
	if err != nil {
		return fmt.Errorf("load client credentials: %w", err)
	}
	tok, err := s.RefreshToken(ctx, clientID, clientSecret, refreshToken)
	if err != nil {
		return fmt.Errorf("refresh converty token: %w", err)
	}
	if err := s.SaveTokens(ctx, shopID, id, tok, time.Now()); err != nil {
		return err
	}
	return nil
}

// TestConnection calls Converty with the integration's access token (rotating
// it first if needed), refreshes the displayed store details from the live
// response, and records the connectivity status on the row.
func (s *Service) TestConnection(ctx context.Context, shopID, id uuid.UUID) error {
	integ, err := s.IntegrationByID(ctx, shopID, id)
	if err != nil {
		return err
	}
	err = s.WithAccessToken(ctx, shopID, id, func(ctx context.Context, accessToken string) error {
		store, sErr := s.GetStore(ctx, accessToken)
		if sErr != nil {
			return sErr
		}
		if store.ID != "" && integ.ConvertyStoreID != "" && store.ID != integ.ConvertyStoreID {
			return fmt.Errorf("token belongs to store %s, not %s", store.ID, integ.ConvertyStoreID)
		}
		if store.ID != "" {
			_, uErr := s.pool.Exec(ctx, `
				UPDATE converty_integrations
				SET store_name = $3, store_slug = $4, store_domain = $5, updated_at = now()
				WHERE id = $1 AND shop_id = $2`,
				id, shopID, store.Name, store.Slug, store.Domain,
			)
			return uErr
		}
		return nil
	})
	if err != nil {
		if sErr := s.SetIntegrationStatus(ctx, shopID, id, "error"); sErr != nil {
			s.log.Warn("converty test: failed to record error status", "error", sErr)
		}
		return err
	}
	return s.SetIntegrationStatus(ctx, shopID, id, "connected")
}

func (s *Service) refreshAndSave(ctx context.Context, integ Integration, now time.Time) (string, error) {
	refreshToken, err := s.DecryptToken(integ.RefreshTokenEncrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt refresh token: %w", err)
	}
	clientID, clientSecret, err := s.ClientCredentials(ctx, integ.ShopID, integ.ID)
	if err != nil {
		return "", fmt.Errorf("load client credentials: %w", err)
	}
	tok, err := s.RefreshToken(ctx, clientID, clientSecret, refreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh converty token: %w", err)
	}
	if err := s.SaveTokens(ctx, integ.ShopID, integ.ID, tok, now); err != nil {
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
