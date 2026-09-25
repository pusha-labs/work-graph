CREATE TABLE service_accounts (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    actor_id uuid NOT NULL UNIQUE REFERENCES actors(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    token_hash bytea NOT NULL UNIQUE,
    token_prefix text NOT NULL,
    scopes text[] NOT NULL DEFAULT ARRAY['work:read']::text[],
    created_by_account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at timestamptz,
    CONSTRAINT service_accounts_scopes_check CHECK (
        cardinality(scopes) > 0
        AND scopes <@ ARRAY['work:read','work:write']::text[]
    ),
    UNIQUE (workspace_id, name)
);

CREATE INDEX service_accounts_workspace_idx ON service_accounts(workspace_id, created_at);
CREATE INDEX service_accounts_active_token_idx ON service_accounts(token_hash) WHERE revoked_at IS NULL;
