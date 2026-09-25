package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	DatabaseURL      string
	Address          string
	CoreURL          string
	PublicCoreURL    string
	WorkspaceID      string
	ServiceToken     string
	InstallationKey  string
	AdminToken       string
	MasterKey        string
	PublicURL        string
	AtlassianAuthURL string
	AtlassianAPIURL  string
}

func Run(ctx context.Context, logger *slog.Logger) error {
	config := Config{
		DatabaseURL: env("GATEWAY_DATABASE_URL", "postgres://gateway:gateway@localhost:5433/gateway?sslmode=disable"),
		Address:     env("GATEWAY_HTTP_ADDRESS", ":8090"), CoreURL: env("WORK_GRAPH_URL", "http://localhost:8088"),
		PublicCoreURL: env("WORK_GRAPH_PUBLIC_URL", "http://localhost:8088"),
		WorkspaceID:   os.Getenv("WORK_GRAPH_WORKSPACE_ID"), ServiceToken: os.Getenv("WORK_GRAPH_SERVICE_TOKEN"),
		InstallationKey:  env("GATEWAY_INSTALLATION_KEY", "mock-local"),
		AdminToken:       os.Getenv("GATEWAY_ADMIN_TOKEN"),
		MasterKey:        os.Getenv("GATEWAY_MASTER_KEY"),
		PublicURL:        env("GATEWAY_PUBLIC_URL", "http://localhost:8090"),
		AtlassianAuthURL: env("ATLASSIAN_AUTH_URL", "https://auth.atlassian.com"),
		AtlassianAPIURL:  env("ATLASSIAN_API_URL", "https://api.atlassian.com"),
	}
	db, err := pgxpool.New(ctx, config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		return err
	}
	if err := migrate(ctx, db); err != nil {
		return err
	}
	handler := NewHandler(logger, db, config)
	go handler.runRetryWorker(ctx)
	go handler.runEventConsumer(ctx)
	go handler.runSyncWorker(ctx)
	server := &http.Server{Addr: config.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("integration gateway listening", "address", config.Address)
		errCh <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
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
