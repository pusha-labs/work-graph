CREATE TABLE password_reset_tokens (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX password_reset_tokens_account_idx ON password_reset_tokens(account_id, created_at DESC);
CREATE INDEX password_reset_tokens_expiry_idx ON password_reset_tokens(expires_at) WHERE used_at IS NULL;
