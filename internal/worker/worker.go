package worker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/database"
	"whatsappconverty/internal/queue"
	"whatsappconverty/internal/whatsapp"
)

// Server runs the background job processor (asynq) inside the web process so
// the deploy needs only one service: Render free tier has no background-worker
// service type. On paid plans this same package can still be run standalone
// via cmd/worker.
type Server struct {
	log    *slog.Logger
	srv    *asynq.Server
	pool   *pgxpool.Pool
	closed bool
}

// Start wires the asynq worker and begins consuming the delayed job queues.
// It owns its own DB pool so it can run inside either process (app or worker).
func Start(cfg config.Config, logger *slog.Logger) (*Server, error) {
	redisOpt, err := queue.RedisClientOpt(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	pool, err := database.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("worker database connection failed: %w", err)
	}

	svc := whatsapp.NewService(cfg, pool, logger)

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 10,
		Logger:      asynqLogger{logger},
		Queues:      map[string]int{"default": 6, "critical": 4},
	})

	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TaskPurgeMarketingTemplate, svc.HandlePurgeMarketingTemplate)

	if err := srv.Start(mux); err != nil {
		pool.Close()
		return nil, fmt.Errorf("worker failed to start: %w", err)
	}

	logger.Info("background worker started", "redis", redisOpt.Addr, "concurrency", 10)
	return &Server{log: logger, srv: srv, pool: pool}, nil
}

// Shutdown stops the job processor and closes its database pool.
func (s *Server) Shutdown() {
	if s.closed {
		return
	}
	s.closed = true
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
