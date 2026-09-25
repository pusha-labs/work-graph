CREATE TABLE workspace_diagnostic_settings (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    wide_branch_children integer NOT NULL DEFAULT 8 CHECK (wide_branch_children BETWEEN 4 AND 50),
    requester_review_days integer NOT NULL DEFAULT 7 CHECK (requester_review_days BETWEEN 1 AND 90),
    blocked_work_days integer NOT NULL DEFAULT 14 CHECK (blocked_work_days BETWEEN 1 AND 365),
    updated_at timestamptz NOT NULL DEFAULT now()
);
