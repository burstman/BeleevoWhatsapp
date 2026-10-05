package whatsapp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/encrypt"
)

// ErrNotConfigured is returned when no WhatsApp integration exists for the
// shop (or its token cannot be decrypted).
var ErrNotConfigured = errors.New("whatsapp is not configured")

// Service talks to the Meta Graph API and enforces the platform's
// single-WABA sending rules. Meta credentials are held centrally in config;
// per-shop rows in whatsapp_integrations are legacy (Phase-1 token-paste).
type Service struct {
	cfg    config.Config
	pool   *pgxpool.Pool
	log    *slog.Logger
	cipher *encrypt.Cipher
	http   *http.Client
	rate   *RateLimiter
}

func NewService(cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) *Service {
	cipher, err := encrypt.New(cfg.EncryptionKeyResolved())
	if err != nil {
		log.Error("whatsapp encryption key invalid; tokens will not persist", "error", err)
	}
	return &Service{
		cfg:    cfg,
		pool:   pool,
		log:    log,
		cipher: cipher,
		http:   &http.Client{Timeout: 20 * time.Second},
		rate:   NewRateLimiter(pool, log),
	}
}

// DecryptToken unwraps an encrypted Meta token stored for a shop.
func (s *Service) DecryptToken(encrypted string) (string, error) {
	if s.cipher == nil {
		return "", errors.New("whatsapp encryption not configured")
	}
	return s.cipher.Decrypt(encrypted)
}

// Token returns the shop's decrypted Meta access token from the stored
// integration. A system-user token does not expire or rotate, so no refresh
// logic is needed (unlike the Converty OAuth flow).
func (s *Service) Token(ctx context.Context, shopID uuid.UUID) (string, error) {
	creds, err := s.Credentials(ctx, shopID)
	if err != nil {
		return "", err
	}
	return creds.AccessToken, nil
}

// Credentials are the shop's owned Meta credentials (its own WhatsApp
// Business number + messaging account). Every Meta call in the pipeline is
// scoped to these, never to a platform-wide token.
type Credentials struct {
	AccessToken        string
	PhoneNumberID      string
	MessagingAccountID string
	PhoneNumber        string
}

// Credentials returns the shop's decrypted Meta credentials from its stored
// integration. ErrNotConfigured is returned when the shop never connected a
// WhatsApp number in Settings.
func (s *Service) Credentials(ctx context.Context, shopID uuid.UUID) (Credentials, error) {
	integ, err := s.Integration(ctx, shopID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Credentials{}, ErrNotConfigured
		}
		return Credentials{}, err
	}
	if integ.AccessTokenEncrypted == "" {
		// A stored row with no token is a half-finished connect; treat it as
		// not configured rather than failing decryption.
		return Credentials{}, ErrNotConfigured
	}
	token, err := s.DecryptToken(integ.AccessTokenEncrypted)
	if err != nil {
		return Credentials{}, err
	}
	if token == "" {
		return Credentials{}, ErrNotConfigured
	}
	return Credentials{
		AccessToken:        token,
		PhoneNumberID:      integ.PhoneNumberID,
		MessagingAccountID: integ.MessagingAccountID,
		PhoneNumber:        integ.PhoneNumber,
	}, nil
}

// NotConnectedReason is the user-facing text when a shop has no WhatsApp
// number connected yet and tries to use number-bound features.
const NotConnectedReason = "no WhatsApp number connected for this store — connect one in Settings first"
