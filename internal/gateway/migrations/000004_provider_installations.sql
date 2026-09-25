CREATE TABLE gateway_installations (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    provider text NOT NULL,
    installation_key text NOT NULL,
    display_name text NOT NULL,
    base_url text NOT NULL,
    auth_type text NOT NULL CHECK (auth_type IN ('oauth2','api_token','basic','none')),
    installation_status text NOT NULL DEFAULT 'draft' CHECK (installation_status IN ('draft','connected','disabled','error')),
    configuration jsonb NOT NULL DEFAULT '{}'::jsonb,
    credential_ciphertext bytea,
    credential_nonce bytea,
    credential_version integer NOT NULL DEFAULT 0,
    credential_updated_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(provider,installation_key)
);

CREATE INDEX gateway_installations_status_idx ON gateway_installations(installation_status,provider);
