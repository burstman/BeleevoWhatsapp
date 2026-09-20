package logutil

import (
	"log/slog"
	"os"

	"whatsappconverty/internal/config"
)

// New returns a JSON logger in production and a text logger otherwise.
func New(cfg config.Config) *slog.Logger {
	level := slog.LevelInfo
	if cfg.IsDevelopment() {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
