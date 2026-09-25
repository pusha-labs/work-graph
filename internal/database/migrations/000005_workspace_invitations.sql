CREATE TABLE workspace_invitations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email text NOT NULL,
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 200),
    workspace_role text NOT NULL DEFAULT 'member' CHECK (workspace_role IN ('admin', 'member')),
    token_hash bytea NOT NULL UNIQUE,
    invited_by uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX workspace_invitations_pending_email_idx
    ON workspace_invitations(workspace_id, lower(email))
    WHERE accepted_at IS NULL;

CREATE INDEX workspace_invitations_workspace_idx ON workspace_invitations(workspace_id, created_at DESC);
