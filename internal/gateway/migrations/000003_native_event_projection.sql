CREATE TABLE gateway_event_cursors (
    consumer_key text PRIMARY KEY,
    workspace_id uuid NOT NULL,
    cursor uuid,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE gateway_outbound_events (
    source_event_id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    provider text NOT NULL,
    installation_key text NOT NULL,
    external_id text NOT NULL,
    work_graph_node_id uuid NOT NULL,
    event_type text NOT NULL,
    workspace_revision bigint,
    correlation_id uuid,
    actor_name text,
    payload jsonb,
    projection_status text NOT NULL CHECK (projection_status IN ('pending','suppressed','delivered','failed')),
    suppression_reason text NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX gateway_outbound_events_status_idx ON gateway_outbound_events(projection_status, occurred_at);
