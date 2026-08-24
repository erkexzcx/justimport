package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/erkexzcx/justimport/internal/arrclient"
	"github.com/erkexzcx/justimport/internal/config"
	"github.com/erkexzcx/justimport/internal/importer"
)

var version = "dev"

func main() {
	slog.SetDefault(slog.New(newLogHandler(os.Stdout)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration: " + err.Error())
		os.Exit(1)
	}

	slog.Info(fmt.Sprintf("Starting justimport v%s (vibe-coded with ❤️)", version))

	if cfg.DryRun {
		slog.Info("Mode: DRY RUN (set DRY_RUN=false to enable imports)")
	} else {
		slog.Info("Mode: LIVE (imports will be performed)")
	}

	slog.Info(fmt.Sprintf("Poll interval: %s", cfg.PollInterval))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var clients []importer.ArrClient

	for _, instance := range cfg.Instances {
		c := arrclient.NewClient(instance.URL, instance.APIKey, instance.Type)
		appName, appVersion, connErr := c.CheckConnectivity(ctx)

		nameDisplay := strings.ToTitle(instance.Type[:1]) + instance.Type[1:]

		if connErr != nil {
			slog.Warn(fmt.Sprintf("%s: %s ✗ (failed to connect: %v) — will retry on each poll", nameDisplay, instance.URL, connErr))
		} else {
			slog.Info(fmt.Sprintf("%s: %s ✓ (connected, %s v%s)", nameDisplay, instance.URL, appName, appVersion))
		}
		clients = append(clients, c)
	}

	imp := importer.New(clients, cfg.DryRun)
	imp.Run(ctx, cfg.PollInterval)

	slog.Info("Shutting down...")
}

// logHandler is a custom slog.Handler that writes human-readable log lines.
type logHandler struct {
	w io.Writer
}

func newLogHandler(w io.Writer) *logHandler {
	return &logHandler{w: w}
}

func (h *logHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

func (h *logHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // slog.Handler interface requires slog.Record by value
	level := levelLabel(r.Level)
	timestamp := r.Time.Format(time.DateTime)
	_, err := fmt.Fprintf(h.w, "%s %s %s\n", timestamp, level, r.Message)
	return err
}

func (h *logHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *logHandler) WithGroup(_ string) slog.Handler {
	return h
}

func levelLabel(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERR"
	case l >= slog.LevelWarn:
		return "WRN"
	case l >= slog.LevelInfo:
		return "INF"
	default:
		return "DBG"
	}
}
