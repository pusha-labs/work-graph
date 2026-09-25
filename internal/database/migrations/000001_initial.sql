CREATE TABLE workspaces (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    home_region text NOT NULL DEFAULT 'local',
    revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE work_nodes (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    root_id uuid,
    parent_id uuid,
    partition_key uuid NOT NULL,
    node_type text NOT NULL DEFAULT 'task',
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 500),
    desired_outcome text NOT NULL DEFAULT '',
    lifecycle_status text NOT NULL DEFAULT 'planned',
    created_revision bigint NOT NULL,
    updated_revision bigint NOT NULL,
    removed_revision bigint,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT work_nodes_root_fk FOREIGN KEY (root_id) REFERENCES work_nodes(id) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT work_nodes_parent_fk FOREIGN KEY (parent_id) REFERENCES work_nodes(id) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT work_nodes_root_shape CHECK (
        (parent_id IS NULL AND root_id = id) OR
        (parent_id IS NOT NULL AND root_id IS NOT NULL)
    )
);

CREATE INDEX work_nodes_workspace_parent_idx ON work_nodes(workspace_id, parent_id) WHERE removed_revision IS NULL;
CREATE INDEX work_nodes_workspace_root_idx ON work_nodes(workspace_id, root_id) WHERE removed_revision IS NULL;

CREATE TABLE change_events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    root_id uuid,
    workspace_revision bigint NOT NULL,
    correlation_id uuid NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    event_type text NOT NULL,
    actor_id uuid,
    source text NOT NULL DEFAULT 'native',
    reason text,
    before_state jsonb,
    after_state jsonb,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, workspace_revision, id)
);

CREATE INDEX change_events_workspace_revision_idx ON change_events(workspace_id, workspace_revision, occurred_at);

CREATE TABLE outbox_events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    event_type text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    payload jsonb NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);

CREATE INDEX outbox_events_pending_idx ON outbox_events(occurred_at) WHERE published_at IS NULL;
