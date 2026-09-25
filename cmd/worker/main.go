package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/database"
	"whatsappconverty/internal/logutil"
	"whatsappconverty/internal/worker"
)

func main() {
	cfg := config.Load()

	logger := logutil.New(cfg)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Keep the standalone worker self-migrating like the app bootstraps itself:
	// the async processing touches delivery/automation tables that must exist.
	if err := database.Migrate(ctx, cfg.DatabaseURL, logger); err != nil {
		logger.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	srv, err := worker.Start(cfg, logger)
	if err != nil {
		logger.Error("worker failed to start", "error", err)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("worker shutting down")
	srv.Shutdown()
}
