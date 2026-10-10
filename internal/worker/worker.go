package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/automations"
	"whatsappconverty/internal/config"
	"whatsappconverty/internal/database"
	"whatsappconverty/internal/delivery"
	"whatsappconverty/internal/queue"
	"whatsappconverty/internal/whatsapp"
)

// reconcileInterval is how often the delivery poller sweeps every connected
// shop for parcel status changes. The REST sweep is a safety net on top of the
// live Mes Colis socket (near-instant delivery-webhook automations); keep the
// poll so no update is ever missed while a socket is down.
const reconcileInterval = 2 * time.Minute

// mediaRetention is how long stored voice notes are kept on the server. The
// operator asked for 30 days; after that the bytes are cleared (the message
// row and its label stay in the history).
const mediaRetention = 30 * 24 * time.Hour

// mediaPurgeInterval is how often the retention job scans for expired media.
const mediaPurgeInterval = 12 * time.Hour

// Server runs the background job processor inside the web process so the deploy
// needs only one service: Render free tier has no background-worker service
// type. On paid plans this same package can still be run standalone via
// cmd/worker.
type Server struct {
	log    *slog.Logger
	runner *queue.Runner
	pool   *pgxpool.Pool
	stop   context.CancelFunc
	closed bool
}

// Start wires the job runner, begins draining queued work, and launches the
// delivery reconcile ticker. It owns its own DB pool so it can run inside either
// process (app or worker).
func Start(cfg config.Config, logger *slog.Logger) (*Server, error) {
	pool, err := database.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("worker database connection failed: %w", err)
	}

	wa := whatsapp.NewService(cfg, pool, logger)
	del := delivery.NewService(cfg, pool, logger)
	automations := automations.NewProcessor(cfg, pool, logger, wa, del)

	runner := queue.NewRunner(pool, logger)
	runner.Handle(queue.TaskPurgeNegativeTemplate, wa.HandlePurgeNegativeTemplate)
	runner.Handle(queue.TaskSendFirstContactReply, wa.SendFirstContactReplyJob)
	runner.Handle(queue.TaskSendWhatsAppTemplate, gateAutomation(automations, wa.HandleSendWhatsAppTemplate, logger))

	deliveryCtx, stopDelivery := context.WithCancel(context.Background())
	runner.Start(deliveryCtx)
	go reconcileLoop(deliveryCtx, logger, automations)
	go del.RunSocketSupervisor(deliveryCtx, func(ctx context.Context, ch delivery.StatusChange) error {
		return automations.OnDeliveryChange(ctx, ch)
	})
	go mediaPurgeLoop(deliveryCtx, logger, wa)

	logger.Info("background worker started",
		"queue", "postgres", "job_interval", queue.DefaultInterval,
		"reconcile_interval", reconcileInterval, "mescolis_socket", delivery.MescolisSocketURL)
	return &Server{log: logger, runner: runner, pool: pool, stop: stopDelivery}, nil
}

// automationLookup is the part of the automation processor the gate needs. It is
// an interface so the gate can be tested without a database.
type automationLookup interface {
	AutomationEnabled(ctx context.Context, id uuid.UUID) (bool, error)
}

// gateAutomation re-checks an automation before its queued send is delivered.
// Pausing switches off what fires next, but the job was already parked in the
// queue — a scheduled send can sit there for hours — so without this check a
// pause would only stop future events and the pending message would still go
// out.
func gateAutomation(lookup automationLookup, next queue.Handler, logger *slog.Logger) queue.Handler {
	return func(ctx context.Context, payload []byte) error {
		var job whatsapp.SendWhatsAppTemplateJob
		if err := json.Unmarshal(payload, &job); err != nil {
			// Malformed payload: let the sender deal with it rather than guessing here.
			return next(ctx, payload)
		}
		if job.AutomationID == uuid.Nil {
			// Not an automation send, or one queued before automation ids were
			// recorded. Nothing to check.
			return next(ctx, payload)
		}

		active, err := lookup.AutomationEnabled(ctx, job.AutomationID)
		if err != nil {
			// A failed lookup must not swallow a customer message: send on and let
			// the send gate decide.
			logger.Warn("automation gate: lookup failed, sending anyway",
				"automation_id", job.AutomationID, "shop_id", job.ShopID, "error", err)
			return next(ctx, payload)
		}
		if !active {
			logger.Info("queued send dropped: automation paused or deleted",
				"automation_id", job.AutomationID, "shop_id", job.ShopID,
				"customer_id", job.CustomerID, "template_id", job.TemplateID)
			return nil
		}
		return next(ctx, payload)
	}
}

// reconcileLoop ticks the delivery poller until the context is cancelled.
func reconcileLoop(ctx context.Context, logger *slog.Logger, automations *automations.Processor) {
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()

	// Run once shortly after start so a fresh poll lands quickly; then on the
	// interval. The sweep itself is fast when nothing changed.
	allowed := func() bool {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}

	run := func() {
		start := time.Now()
		if err := automations.ReconcileDelivery(ctx); err != nil {
			logger.Warn("delivery reconcile sweep failed", "error", err)
		} else {
			logger.Debug("delivery reconcile sweep finished", "elapsed", time.Since(start))
		}
	}

	if allowed() {
		run()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

// mediaPurgeLoop clears stored chat media (voice notes) once it is older than
// the 30-day retention window. The bytes are dropped, not the message rows, so
// the conversation history keeps reading "[🎤 Voice message]" with no player.
func mediaPurgeLoop(ctx context.Context, logger *slog.Logger, wa *whatsapp.Service) {
	ticker := time.NewTicker(mediaPurgeInterval)
	defer ticker.Stop()

	run := func() {
		n, err := wa.PurgeChatMedia(ctx, time.Now().Add(-mediaRetention))
		if err != nil {
			logger.Warn("chat media purge failed", "error", err)
			return
		}
		if n > 0 {
			logger.Info("chat media purge cleared media rows", "count", n)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

// Shutdown stops the job runner and closes its database pool.
func (s *Server) Shutdown() {
	if s.closed {
		return
	}
	s.closed = true
	if s.stop != nil {
		s.stop()
	}
	s.pool.Close()
	s.log.Info("background worker shut down")
}
