package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/pusha-labs/work-graph/internal/bashrunner"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	if err := bashrunner.Run(ctx, logger, apiURL, os.Getenv("RUNNER_TOKEN")); err != nil {
		logger.Error("bash runner stopped", "error", err)
		os.Exit(1)
	}
}
