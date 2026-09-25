package gateway

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var gatewayMigrations embed.FS

func migrate(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS gateway_schema_migrations(version text PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := gatewayMigrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var applied bool
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM gateway_schema_migrations WHERE version=$1)`, entry.Name()).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		body, err := gatewayMigrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(body)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO gateway_schema_migrations(version) VALUES($1)`, entry.Name())
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply gateway migration %s: %w", entry.Name(), err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
