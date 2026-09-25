CREATE TABLE gateway_oauth_states (
    state_hash bytea PRIMARY KEY,
    installation_id uuid NOT NULL REFERENCES gateway_installations(id) ON DELETE CASCADE,
    redirect_uri text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX gateway_oauth_states_expiry_idx ON gateway_oauth_states(expires_at);
