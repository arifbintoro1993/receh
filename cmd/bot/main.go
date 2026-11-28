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
	"github.com/arifbintoro1993/receh/internal/expense"
	"github.com/arifbintoro1993/receh/internal/i18n"
	"github.com/arifbintoro1993/receh/internal/store"
	"github.com/arifbintoro1993/receh/internal/telegram"

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

	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := store.Migrate(ctx, pool); err != nil {
		return err
	}
	slog.Info("database ready")

	st := store.New(pool)
	svc, err := expense.NewService(st, expense.User{
		TZ:            cfg.TZ.String(),
		CycleStartDay: cfg.CycleStartDay,
		ReportTypes:   cfg.ReportTypes,
		ReportHour:    cfg.ReportHour,
	}, expense.DefaultSeed())
	if err != nil {
		return fmt.Errorf("expense service: %w", err)
	}
	messages, err := i18n.New(cfg.Locale)
	if err != nil {
		return err
	}

	client, err := telegram.NewClient(cfg.TelegramToken, slog.Default())
	if err != nil {
		return err
	}
	handler := telegram.NewHandler(telegram.Deps{
		AllowedChatIDs:  cfg.AllowedChatIDs,
		Log:             svc.Log,
		MarkProcessed:   st.MarkUpdateProcessed,
		UnmarkProcessed: st.UnmarkUpdate,
		Summarize:       svc.Summarize,
		Last:            svc.Last,
		Notifier:        client,
		Messages:        messages,
		Logger:          slog.Default(),
	})

	slog.Info("polling telegram")
	client.Run(ctx, handler) // returns once ctx is cancelled and in-flight updates finish
	slog.Info("shutting down")
	return nil
}
