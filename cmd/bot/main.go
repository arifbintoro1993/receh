package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"

	"github.com/arifbintoro1993/receh/internal/config"

	// Distroless has no zoneinfo; embed it so users.tz always resolves.
	_ "time/tzdata"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(env.ToMap(os.Environ()))
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return fmt.Errorf("parse LOG_LEVEL: %w", err)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("starting", "tz", cfg.TZ.String(), "locale", cfg.Locale, "allowlist_size", len(cfg.AllowedChatIDs))
	<-ctx.Done()
	slog.Info("shutting down")
	return nil
}
