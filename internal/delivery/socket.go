package delivery

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kingecg/gosocketio/socketio"
)

// MescolisSocketURL is the keep-alive endpoint documented in
// "Documentation Socket": a socket.io v4 server (EIO=4) that broadcasts one
// "mescolis-events" push per parcel status change, per connected account.
const MescolisSocketURL = "https://api.mescolis.tn:4001/socket.io/"

// socketWatchInterval is how often the supervisor re-scans integrations so a
// new connection is dialed (or a changed token re-dialed) promptly.
const socketWatchInterval = 30 * time.Second

// socketBaseBackoff / socketMaxBackoff bound the reconnect delay.
const (
	socketBaseBackoff = 5 * time.Second
	socketMaxBackoff  = 60 * time.Second
)

// mescolisEvent mirrors the JSON payload pushed on the "mescolis-events"
// channel. Only barcode/status are required; the extra fields narrow down the
// context of some statuses (see "Documentation Socket", Événements).
type mescolisEvent struct {
	Barcode                string `json:"barcode"`
	Status                 string `json:"status"`
	UpdatedAt              string `json:"updated_at"`
	DeliverymanName        string `json:"deliveryman_name"`
	DeliverymanPhoneNumber string `json:"deliveryman_phone_number"`
	Qualification          string `json:"qualification"`
	DeliveredAt            string `json:"delivered_at"`
	ReceiverName           string `json:"receiver_name"`
	DestinationAgencyName  string `json:"destination_agency_name"`
	DestinationAgencyCode  string `json:"destination_agency_code"`
	Motif                  string `json:"motif"`
}

type socketConn struct {
	cancel context.CancelFunc
	token  string
}

// RunSocketSupervisor keeps one live Mes Colis socket per connected shop and
// runs until ctx is cancelled. Every pushed status goes through the same
// recordTransition pipeline as the REST poller, so automations trigger
// immediately instead of only on the 2-minute sweep. Callers run it in their
// own goroutine (the worker process does).
func (s *Service) RunSocketSupervisor(ctx context.Context, onChanged func(context.Context, StatusChange) error) {
	s.socketMu.Lock()
	s.socketConns = make(map[string]*socketConn)
	s.socketMu.Unlock()

	s.socketSweep(ctx, onChanged)
	ticker := time.NewTicker(socketWatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.socketShutdown()
			return
		case <-ticker.C:
			s.socketSweep(ctx, onChanged)
		}
	}
}

// socketShutdown cancels every per-shop connection.
func (s *Service) socketShutdown() {
	s.socketMu.Lock()
	defer s.socketMu.Unlock()
	for key, conn := range s.socketConns {
		conn.cancel()
		delete(s.socketConns, key)
	}
}

// socketSweep reconciles the live connections with the integration table:
// drop connections whose token changed or whose integration was removed, and
// dial fresh connections for newly configured shops.
func (s *Service) socketSweep(ctx context.Context, onChanged func(context.Context, StatusChange) error) {
	integrations, err := s.ListIntegrations(ctx)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
			s.log.Warn("mescolis socket paused: delivery tables missing — redeploy or run migrations")
			return
		}
		s.log.Warn("mescolis socket sweep: integration lookup failed", "error", err)
		return
	}

	desired := make(map[string]string) // shopID.String() -> token
	for _, integ := range integrations {
		if integ.Provider != ProviderMescolis {
			continue
		}
		if s.cipher == nil {
			s.log.Debug("mescolis socket sweep: encryption unavailable, skipping", "shop_id", integ.ShopID)
			continue
		}
		token, err := s.cipher.Decrypt(integ.AccessTokenEncrypted)
		if err != nil || token == "" {
			s.log.Debug("mescolis socket sweep: token unavailable", "shop_id", integ.ShopID)
			continue
		}
		desired[integ.ShopID.String()] = token
	}

	// Stop connections that are gone or re-tokened.
	s.socketMu.Lock()
	for key, conn := range s.socketConns {
		token, ok := desired[key]
		if !ok || token != conn.token {
			conn.cancel()
			delete(s.socketConns, key)
		}
	}
	s.socketMu.Unlock()

	// Dial connections that are missing.
	for key, token := range desired {
		s.socketMu.Lock()
		if _, ok := s.socketConns[key]; ok {
			s.socketMu.Unlock()
			continue
		}
		connCtx, cancel := context.WithCancel(ctx)
		s.socketConns[key] = &socketConn{cancel: cancel, token: token}
		s.socketMu.Unlock()

		go func(shopKey, shopToken string) {
			defer func() {
				s.socketMu.Lock()
				delete(s.socketConns, shopKey)
				s.socketMu.Unlock()
			}()
			s.socketLoop(connCtx, shopKey, shopToken, onChanged)
		}(key, token)
	}
}

// socketLoop dials and re-dials one shop's connection until ctx is cancelled.
func (s *Service) socketLoop(ctx context.Context, shopID string, token string, onChanged func(context.Context, StatusChange) error) {
	backoff := socketBaseBackoff
	attempt := 0
	for {
		if !s.socketRunOnce(ctx, shopID, token, onChanged, attempt) {
			return
		}
		attempt++
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > socketMaxBackoff {
			backoff = socketMaxBackoff
		}
		backoff += time.Duration(rand.Intn(5)) * time.Second
	}
}

// socketRunOnce dials the socket, installs the event handlers, and blocks while
// the connection is alive. It returns false when ctx was cancelled (stop), true
// when the connection dropped and the caller should back off and retry.
func (s *Service) socketRunOnce(ctx context.Context, shopID, token string, onChanged func(context.Context, StatusChange) error, attempt int) bool {
	client, err := socketio.Dial(ctx, MescolisSocketURL, &socketio.Options{
		Transports: []string{"websocket"},
		Auth:       map[string]any{"token": token},
		Timeout:    20 * time.Second,
	})
	if err != nil {
		if ctx.Err() != nil {
			return false
		}
		if attempt == 0 {
			s.log.Warn("mescolis socket connect failed", "shop_id", shopID, "error", err)
		} else {
			s.log.Debug("mescolis socket connect failed", "shop_id", shopID, "attempt", attempt, "error", err)
		}
		return true
	}
	defer func() { _ = client.Close() }()

	disconnected := make(chan string, 1)
	client.OnConnect("/", func() {
		s.log.Info("mescolis socket connected", "shop_id", shopID)
	})
	client.OnDisconnect("/", func(reason string) {
		select {
		case disconnected <- reason:
		default:
		}
	})
	client.OnError("/", func(err error) {
		s.log.Debug("mescolis socket error", "shop_id", shopID, "error", err)
	})
	client.OnEvent("/", "mescolis-events", func(e mescolisEvent) {
		if err := s.onSocketEvent(ctx, shopID, e, onChanged); err != nil {
			s.log.Warn("mescolis socket event failed",
				"shop_id", shopID, "barcode", e.Barcode, "error", err)
		}
	})

	select {
	case <-ctx.Done():
		return false
	case reason := <-disconnected:
		s.log.Warn("mescolis socket disconnected", "shop_id", shopID, "reason", reason)
		return true
	}
}

// onSocketEvent turns one pushed status into the same tracked-parcel pipeline
// the REST poller uses, so delivery automations fire in near real time.
func (s *Service) onSocketEvent(ctx context.Context, shopID string, e mescolisEvent, onChanged func(context.Context, StatusChange) error) error {
	if _, err := uuid.Parse(shopID); err != nil {
		return err
	}
	if e.Barcode == "" || e.Status == "" {
		s.log.Debug("mescolis socket event missing barcode/status", "shop_id", shopID)
		return nil
	}
	// The connection was opened with the operator's account token, not a single
	// shop's, so the event is resolved by barcode and the transition is recorded
	// against the shop that actually registered the parcel.
	tracked, err := s.TrackedAnyShopByBarcode(ctx, e.Barcode)
	if err != nil {
		return err
	}
	if tracked.ID == uuid.Nil {
		s.log.Debug("mescolis socket event for untracked parcel, ignoring",
			"connection_shop_id", shopID, "barcode", e.Barcode)
		return nil
	}
	return s.recordTransition(ctx, tracked.ShopID, e.Barcode, tracked.OrderID, tracked.LastStatus, e.Status, "",
		e.DeliverymanName, e.DeliverymanPhoneNumber, onChanged)
}

// recordTransition persists one observed status and reports the change to the
// automation pipeline. An unchanged status only bumps last_seen_at (the caller
// models "seen" semantics as a touch). This is the single funnel for both the
// REST poller and the live socket.
func (s *Service) recordTransition(ctx context.Context, shopID uuid.UUID, barcode, orderID, prev, status, labelHint, driverName, driverPhone string, onChanged func(context.Context, StatusChange) error) error {
	if prev == status {
		if driverName == "" && driverPhone == "" {
			return s.TouchTracked(ctx, shopID, barcode)
		}
		// The status did not change but a courier came with it, and that is the
		// one observation a message held back on that courier is waiting for.
		// Dropping it because "nothing changed" is how a name that has already
		// arrived left a customer without their message all day.
		if err := s.AttachDriver(ctx, shopID, barcode, driverName, driverPhone); err != nil {
			return err
		}
		s.log.Info("delivery driver attached",
			"shop_id", shopID, "barcode", barcode, "status", status,
			"driver_name", driverName, "driver_phone", driverPhone)
		if onChanged == nil {
			return nil
		}
		// Re-run the pipeline for the same status. Every send this reaches is
		// keyed on the parcel, so an order already messaged is skipped rather
		// than messaged twice.
		return onChanged(ctx, StatusChange{
			ShopID:      shopID,
			Barcode:     barcode,
			OrderID:     orderID,
			Status:      status,
			Label:       LabelFor(status),
			Previous:    prev,
			DriverName:  driverName,
			DriverPhone: driverPhone,
		})
	}

	label := labelHint
	if label == "" {
		label = LabelFor(status)
	}
	if err := s.UpdateTrackedStatus(ctx, shopID, barcode, status, label, driverName, driverPhone); err != nil {
		return err
	}

	// The driver is worth logging separately: it is what a delivery template
	// places in the message, and a status change often arrives before the carrier
	// assigns anyone, which is why a send can be skipped for a missing variable.
	s.log.Info("delivery status changed",
		"shop_id", shopID, "barcode", barcode, "from", prev, "to", status,
		"label", label, "driver_attached", driverName != "" || driverPhone != "",
		"driver_name", driverName, "terminal", StatusTerminal(status))
	if onChanged != nil {
		return onChanged(ctx, StatusChange{
			ShopID:      shopID,
			Barcode:     barcode,
			OrderID:     orderID,
			Status:      status,
			Label:       label,
			Previous:    prev,
			DriverName:  driverName,
			DriverPhone: driverPhone,
		})
	}
	return nil
}
