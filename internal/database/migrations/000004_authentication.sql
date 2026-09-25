CREATE TABLE accounts (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    email text NOT NULL,
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 200),
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX accounts_email_idx ON accounts(lower(email));

CREATE TABLE account_actors (
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    actor_id uuid NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    workspace_role text NOT NULL DEFAULT 'owner' CHECK (workspace_role IN ('owner', 'admin', 'member')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, actor_id),
    UNIQUE (actor_id)
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sessions_expiry_idx ON sessions(expires_at);
