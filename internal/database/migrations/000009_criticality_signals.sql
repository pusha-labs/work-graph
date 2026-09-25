CREATE TABLE criticality_signals (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    work_node_id uuid NOT NULL REFERENCES work_nodes(id) ON DELETE CASCADE,
    reason text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 1000),
    critical_until timestamptz NOT NULL,
    created_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    CHECK (critical_until > created_at)
);

CREATE INDEX criticality_signals_active_idx
    ON criticality_signals(workspace_id, critical_until)
    WHERE revoked_at IS NULL;
