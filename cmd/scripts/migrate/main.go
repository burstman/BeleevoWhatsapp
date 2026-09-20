package main

import (
	"context"
	"os"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/database"
	"whatsappconverty/internal/logutil"
)

func main() {
	cfg := config.Load()
	logger := logutil.New(cfg)

	if err := database.Migrate(context.Background(), cfg.DatabaseURL, logger); err != nil {
		logger.Error("migrate failed", "error", err)
		os.Exit(1)
	}
}
