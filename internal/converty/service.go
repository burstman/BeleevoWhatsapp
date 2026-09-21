package converty

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/encrypt"
)

var ErrNotConfigured = errors.New("converty is not configured")

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

// WebhookURL returns the absolute webhook target for a UI-origin AppURL.
func (s *Service) WebhookURL(appURL string) string {
	for len(appURL) > 0 && appURL[len(appURL)-1] == '/' {
		appURL = appURL[:len(appURL)-1]
	}
	return appURL + webhookPath
}
