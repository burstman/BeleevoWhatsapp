package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/logutil"
	"whatsappconverty/internal/worker"
)

func main() {
	cfg := config.Load()

	logger := logutil.New(cfg)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv, err := worker.Start(cfg, logger)
	if err != nil {
		logger.Error("worker failed to start", "error", err)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("worker shutting down")
	srv.Shutdown()
}
