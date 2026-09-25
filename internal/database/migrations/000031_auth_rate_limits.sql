CREATE TABLE auth_rate_limits (
    scope text NOT NULL,
    identifier_hash bytea NOT NULL,
    window_started_at timestamptz NOT NULL,
    attempt_count integer NOT NULL CHECK (attempt_count >= 0),
    blocked_until timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (scope, identifier_hash)
);

CREATE INDEX auth_rate_limits_cleanup_idx ON auth_rate_limits(updated_at);
