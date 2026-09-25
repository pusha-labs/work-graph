package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/pusha-labs/work-graph/internal/gateway"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := gateway.Run(ctx, logger); err != nil {
		logger.Error("gateway stopped", "error", err)
		os.Exit(1)
	}
}
