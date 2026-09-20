package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/logutil"
	"whatsappconverty/internal/queue"
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

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 10,
		Logger:      asynqLogger{logger},
		Queues:      map[string]int{"default": 6, "critical": 4},
	})

	mux := asynq.NewServeMux()
	// Handlers for send:whatsapp_template, sync:whatsapp_templates and
	// process:meta_webhook are registered in later phases.
	registerHandlers(mux)

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

func registerHandlers(mux *asynq.ServeMux) {
	// Placeholder: task handlers land with the WhatsApp/automation phases.
}

type asynqLogger struct{ log *slog.Logger }

func (l asynqLogger) Debug(args ...any) { l.log.Debug("asynq", "msg", args) }
func (l asynqLogger) Info(args ...any)  { l.log.Info("asynq", "msg", args) }
func (l asynqLogger) Warn(args ...any)  { l.log.Warn("asynq", "msg", args) }
func (l asynqLogger) Error(args ...any) { l.log.Error("asynq", "msg", args) }
func (l asynqLogger) Fatal(args ...any) { l.log.Error("asynq fatal", "msg", args) }
