CREATE TABLE workspace_secrets (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    secret_type text NOT NULL DEFAULT 'api_token' CHECK (secret_type IN ('api_token','password','signing_key','other')),
    ciphertext bytea NOT NULL,
    nonce bytea NOT NULL,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    enabled boolean NOT NULL DEFAULT true,
    created_by uuid NOT NULL REFERENCES accounts(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);

CREATE INDEX workspace_secrets_workspace_idx ON workspace_secrets(workspace_id, enabled, name);
