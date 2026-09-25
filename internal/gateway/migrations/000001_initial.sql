CREATE TABLE gateway_candidates (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    provider text NOT NULL,
    installation_key text NOT NULL,
    external_id text NOT NULL,
    external_version text NOT NULL,
    title text NOT NULL,
    description text NOT NULL DEFAULT '',
    raw_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL CHECK (status IN ('unplaced','suggested','placed','failed')),
    recommendations jsonb NOT NULL DEFAULT '[]'::jsonb,
    work_graph_node_id uuid,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, installation_key, external_id)
);

CREATE TABLE gateway_mappings (
    provider text NOT NULL,
    installation_key text NOT NULL,
    external_id text NOT NULL,
    external_version text NOT NULL,
    workspace_id uuid NOT NULL,
    work_graph_node_id uuid NOT NULL,
    last_native_revision bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, installation_key, external_id)
);

CREATE INDEX gateway_candidates_inbox_idx ON gateway_candidates(status, updated_at DESC);
