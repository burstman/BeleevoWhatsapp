package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/database"
	"whatsappconverty/internal/logutil"
	"whatsappconverty/internal/queue"
	"whatsappconverty/internal/whatsapp"
)

func main() {
	cfg := config.Load()

	logger := logutil.New(cfg)
	slog.SetDefault(logger)

	redisOpt, err := queue.RedisClientOpt(cfg.RedisURL)
	if err != nil {
		logger.Error("invalid redis url", "error", err)
		os.Exit(1)
	}

	pool, err := database.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	svc := whatsapp.NewService(cfg, pool, logger)

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 10,
		Logger:      asynqLogger{logger},
		Queues:      map[string]int{"default": 6, "critical": 4},
	})

	mux := asynq.NewServeMux()
	registerHandlers(mux, svc)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Start(mux); err != nil {
		logger.Error("worker failed to start", "error", err)
		os.Exit(1)
	}
	logger.Info("worker started", "redis", redisOpt.Addr, "concurrency", 10)

	<-ctx.Done()
	logger.Info("worker shutting down")
	srv.Shutdown()
}

func registerHandlers(mux *asynq.ServeMux, svc *whatsapp.Service) {
	mux.HandleFunc(queue.TaskPurgeMarketingTemplate, svc.HandlePurgeMarketingTemplate)
}

type asynqLogger struct{ log *slog.Logger }

func (l asynqLogger) Debug(args ...any) { l.log.Debug("asynq", "msg", args) }
func (l asynqLogger) Info(args ...any)  { l.log.Info("asynq", "msg", args) }
func (l asynqLogger) Warn(args ...any)  { l.log.Warn("asynq", "msg", args) }
func (l asynqLogger) Error(args ...any) { l.log.Error("asynq", "msg", args) }
func (l asynqLogger) Fatal(args ...any) { l.log.Error("asynq fatal", "msg", args) }
