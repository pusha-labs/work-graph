package httpapi

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
)

type auditExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func recordAudit(ctx context.Context, executor auditExecutor, workspaceID, accountID, eventType, summary, detail string) error {
	_, err := executor.Exec(ctx, `INSERT INTO workspace_audit_events(workspace_id,account_id,event_type,summary,detail) VALUES ($1,$2,$3,$4,$5)`, workspaceID, accountID, eventType, summary, detail)
	return err
}
