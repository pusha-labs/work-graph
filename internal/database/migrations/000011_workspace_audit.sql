CREATE TABLE workspace_audit_events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    event_type text NOT NULL,
    summary text NOT NULL,
    detail text NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX workspace_audit_events_timeline_idx
    ON workspace_audit_events(workspace_id, occurred_at DESC);
