package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/automations"
	"whatsappconverty/internal/config"
	"whatsappconverty/internal/database"
	"whatsappconverty/internal/delivery"
	"whatsappconverty/internal/queue"
	"whatsappconverty/internal/whatsapp"
)

// reconcileInterval is how often the delivery poller sweeps every connected
// shop for parcel status changes. Poll (rather than a live socket) is the
// chosen ingestion because Render's free tier cannot hold persistent per-shop
// websockets reliably.
const reconcileInterval = 2 * time.Minute

// Server runs the background job processor (asynq) inside the web process so
// the deploy needs only one service: Render free tier has no background-worker
// service type. On paid plans this same package can still be run standalone
// via cmd/worker.
type Server struct {
	log    *slog.Logger
	srv    *asynq.Server
	pool   *pgxpool.Pool
	stop   context.CancelFunc
	closed bool
}

// Start wires the asynq worker, begins consuming the delayed job queues, and
// launches the delivery reconcile ticker. It owns its own DB pool so it can run
// inside either process (app or worker).
func Start(cfg config.Config, logger *slog.Logger) (*Server, error) {
	redisOpt, err := queue.RedisClientOpt(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	pool, err := database.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("worker database connection failed: %w", err)
	}

	wa := whatsapp.NewService(cfg, pool, logger)
	del := delivery.NewService(cfg, pool, logger)
	automations := automations.NewProcessor(cfg, pool, logger, wa, del)

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 10,
		Logger:      asynqLogger{logger},
		Queues:      map[string]int{"default": 6, "critical": 4},
	})

	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TaskPurgeMarketingTemplate, wa.HandlePurgeMarketingTemplate)
	mux.HandleFunc(queue.TaskSendWhatsAppTemplate, wa.HandleSendWhatsAppTemplate)

	if err := srv.Start(mux); err != nil {
		pool.Close()
		return nil, fmt.Errorf("worker failed to start: %w", err)
	}

	reconcileCtx, stopReconcile := context.WithCancel(context.Background())
	go reconcileLoop(reconcileCtx, logger, automations)

	logger.Info("background worker started",
		"redis", redisOpt.Addr, "concurrency", 10, "reconcile_interval", reconcileInterval)
	return &Server{log: logger, srv: srv, pool: pool, stop: stopReconcile}, nil
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

// Shutdown stops the job processor and closes its database pool.
func (s *Server) Shutdown() {
	if s.closed {
		return
	}
	s.closed = true
	if s.stop != nil {
		s.stop()
	}
	s.srv.Shutdown()
	s.pool.Close()
	s.log.Info("background worker shut down")
}

type asynqLogger struct{ log *slog.Logger }

func (l asynqLogger) Debug(args ...any) { l.log.Debug("asynq", "msg", args) }
func (l asynqLogger) Info(args ...any)  { l.log.Info("asynq", "msg", args) }
func (l asynqLogger) Warn(args ...any)  { l.log.Warn("asynq", "msg", args) }
func (l asynqLogger) Error(args ...any) { l.log.Error("asynq", "msg", args) }
func (l asynqLogger) Fatal(args ...any) { l.log.Error("asynq fatal", "msg", args) }