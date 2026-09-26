package delivery

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/encrypt"
)

// ProviderMescolis is the first (and currently only) delivery provider wired
// into the platform. Mes Colis Express exposes a REST status API keyed by an
// x-access-token header plus a keep-alive socket; we consume the REST side.
const ProviderMescolis = "mescolis"

// ErrNotConfigured is returned when a shop has no delivery integration (or
// its token cannot be decrypted).
var ErrNotConfigured = errors.New("delivery is not configured")

// Service stores delivery provider connections per shop (tokens encrypted at
// rest, same pattern as the WhatsApp/Converty credentials) and orchestrates
// the status polling that feeds delivery-event automations.
type Service struct {
	cfg    config.Config
	pool   *pgxpool.Pool
	log    *slog.Logger
	cipher *encrypt.Cipher
	http   *http.Client

	// socketConns tracks the live per-shop Mes Colis socket connections,
	// guarded by socketMu (see socket.go).
	socketMu    sync.Mutex
	socketConns map[string]*socketConn
}

func NewService(cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) *Service {
	cipher, err := encrypt.New(cfg.EncryptionKeyResolved())
	if err != nil {
		log.Error("delivery encryption key invalid; tokens will not persist", "error", err)
	}
	return &Service{
		cfg:    cfg,
		pool:   pool,
		log:    log,
		cipher: cipher,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

// NotConnectedReason is the user-facing text when a shop has no delivery
// provider connected but tries to use delivery-bound features.
const NotConnectedReason = "no delivery provider connected for this store — connect one in Settings first"

// Integration is one shop's delivery provider connection. The access token is
// stored encrypted at rest; account_code/allow_sub_account forward the
// provider's sub-account options (used by Mes Colis for multi-account shops).
type Integration struct {
	ID                   uuid.UUID
	ShopID               uuid.UUID
	Provider             string
	AccessTokenEncrypted string
	AccountCode          string
	AllowSubAccount      bool
	ConnectedAt          time.Time
}

// SaveIntegration creates or replaces the shop's connection for a provider.
func (s *Service) SaveIntegration(ctx context.Context, shopID uuid.UUID, provider, accessToken, accountCode string, allowSubAccount bool) error {
	enc, err := s.cipher.Encrypt(accessToken)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO delivery_integrations (
			shop_id, provider, access_token_encrypted, account_code, allow_sub_account
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (shop_id, provider) DO UPDATE SET
			access_token_encrypted = EXCLUDED.access_token_encrypted,
			account_code           = EXCLUDED.account_code,
			allow_sub_account      = EXCLUDED.allow_sub_account,
			connected_at           = now(),
			updated_at             = now()`,
		shopID, provider, enc, accountCode, allowSubAccount,
	)
	return err
}

// Integration returns the shop's connection for a provider.
func (s *Service) Integration(ctx context.Context, shopID uuid.UUID, provider string) (Integration, error) {
	var i Integration
	err := s.pool.QueryRow(ctx, `
		SELECT id, shop_id, provider, access_token_encrypted, account_code, allow_sub_account, connected_at
		FROM delivery_integrations
		WHERE shop_id = $1 AND provider = $2`,
		shopID, provider,
	).Scan(&i.ID, &i.ShopID, &i.Provider, &i.AccessTokenEncrypted, &i.AccountCode, &i.AllowSubAccount, &i.ConnectedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Integration{}, ErrNotConfigured
	}
	return i, err
}

// ListIntegrations returns every delivery integration across all shops, used
// by the reconcile loop to poll each connected shop with its own token.
func (s *Service) ListIntegrations(ctx context.Context) ([]Integration, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, shop_id, provider, access_token_encrypted, account_code, allow_sub_account, connected_at
		FROM delivery_integrations
		ORDER BY provider, connected_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Integration
	for rows.Next() {
		var i Integration
		if err := rows.Scan(&i.ID, &i.ShopID, &i.Provider, &i.AccessTokenEncrypted, &i.AccountCode, &i.AllowSubAccount, &i.ConnectedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// DeleteIntegration removes the shop's connection for a provider.
func (s *Service) DeleteIntegration(ctx context.Context, shopID uuid.UUID, provider string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM delivery_integrations
		WHERE shop_id = $1 AND provider = $2`,
		shopID, provider,
	)
	return err
}

// APIKey returns the shop's decrypted provider token, or ErrNotConfigured when
// the shop has no integration.
func (s *Service) APIKey(ctx context.Context, shopID uuid.UUID, provider string) (string, error) {
	integ, err := s.Integration(ctx, shopID, provider)
	if err != nil {
		if errors.Is(err, ErrNotConfigured) {
			return "", ErrNotConfigured
		}
		return "", err
	}
	token, err := s.cipher.Decrypt(integ.AccessTokenEncrypted)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", ErrNotConfigured
	}
	return token, nil
}

// Message is the "unsaved happens" of the delivery package: a single change of
// status observed by the poller, handed to the automation processor.
type StatusChange struct {
	ShopID      uuid.UUID
	Barcode     string
	OrderID     string
	Status      string
	Label       string
	Previous    string
	DriverName  string
	DriverPhone string
}

// Reconcile polls every connected shop's watched parcels and reports each
// observed status transition through onChanged. A transition is any divergence
// from the last stored status, including the first observation (last_status
// ""). Terminal statuses are still reported once, so a "delivered" automation
// fires, and then stop being polled later by the caller choosing not to
// refresh them.
func (s *Service) Reconcile(ctx context.Context, onChanged func(context.Context, StatusChange) error) error {
	integrations, err := s.ListIntegrations(ctx)
	if err != nil {
		// A worker that boots before the migrations land would otherwise log a
		// hard failure every tick. Pause the poller with one clear warning
		// instead; the next deploy applies the schema and it resumes.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
			s.log.Warn("delivery poller paused: delivery tables missing — redeploy or run migrations")
			return nil
		}
		return err
	}

	for _, integ := range integrations {
		if integ.Provider != ProviderMescolis {
			continue
		}
		if err := s.reconcileShop(ctx, integ, onChanged); err != nil {
			s.log.Warn("delivery reconcile failed for shop",
				"shop_id", integ.ShopID, "provider", integ.Provider, "error", err)
		}
	}
	return nil
}

func (s *Service) reconcileShop(ctx context.Context, integ Integration, onChanged func(context.Context, StatusChange) error) error {
	token, err := s.cipher.Decrypt(integ.AccessTokenEncrypted)
	if err != nil || token == "" {
		return ErrNotConfigured
	}

	tracked, err := s.Tracked(ctx, integ.ShopID)
	if err != nil {
		return err
	}
	if len(tracked) == 0 {
		s.log.Debug("delivery reconcile: no tracked parcels", "shop_id", integ.ShopID)
		return nil
	}

	// Skip parcels that already reached a terminal state (no more events).
	active := make([]TrackedOrder, 0, len(tracked))
	barcodes := make([]string, 0, len(tracked))
	for _, t := range tracked {
		if StatusTerminal(t.LastStatus) {
			continue
		}
		active = append(active, t)
		barcodes = append(barcodes, t.Barcode)
	}
	if len(active) == 0 {
		return nil
	}

	client := NewMescolisClient(token, integ.AllowSubAccount, integ.AccountCode, s.cfg, s.http)
	resp, err := client.GetOrders(ctx, barcodes)
	if err != nil {
		return err
	}

	for _, remote := range resp.Orders {
		var prev, orderID string
		for _, t := range active {
			if t.Barcode != remote.Barcode {
				continue
			}
			prev = t.LastStatus
			orderID = t.OrderID
			break
		}
		if err := s.recordTransition(ctx, integ.ShopID, remote.Barcode, orderID, prev, remote.Status, remote.StatusLabel,
			remote.DeliverymanName, remote.DeliverymanPhoneNumber, onChanged); err != nil {
			s.log.Warn("delivery reconcile: transition failed", "barcode", remote.Barcode, "error", err)
		}
	}

	for _, missing := range resp.NotFound {
		s.log.Info("delivery reconcile: barcode not found upstream (parcel removed?)",
			"shop_id", integ.ShopID, "barcode", missing)
	}
	return nil
}
