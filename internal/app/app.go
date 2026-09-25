package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/pusha-labs/work-graph/internal/database"
	"github.com/pusha-labs/work-graph/internal/httpapi"
)

func Run(ctx context.Context, logger *slog.Logger) error {
	databaseURL := env("DATABASE_URL", "postgres://workgraph:workgraph@localhost:5432/workgraph?sslmode=disable")
	address := env("HTTP_ADDRESS", ":8080")

	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := database.Migrate(ctx, db); err != nil {
		return err
	}

	server := &http.Server{
		Addr:              address,
		Handler:           httpapi.New(logger, db),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api listening", "address", address)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
