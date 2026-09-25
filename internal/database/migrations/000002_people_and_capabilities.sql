CREATE TABLE actors (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 200),
    actor_type text NOT NULL DEFAULT 'person' CHECK (actor_type IN ('person', 'automation')),
    has_account boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, display_name)
);

CREATE INDEX actors_workspace_idx ON actors(workspace_id, display_name);

CREATE TABLE capabilities (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    capability_type text NOT NULL DEFAULT 'role' CHECK (capability_type IN ('role', 'skill')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name, capability_type)
);

CREATE TABLE actor_capabilities (
    actor_id uuid NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    capability_id uuid NOT NULL REFERENCES capabilities(id) ON DELETE CASCADE,
    claim_source text NOT NULL DEFAULT 'admin' CHECK (claim_source IN ('self', 'admin', 'inferred', 'imported')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, capability_id, claim_source)
);

CREATE TABLE work_node_requirements (
    work_node_id uuid NOT NULL REFERENCES work_nodes(id) ON DELETE CASCADE,
    capability_id uuid NOT NULL REFERENCES capabilities(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (work_node_id, capability_id)
);

CREATE TABLE work_node_participants (
    work_node_id uuid NOT NULL REFERENCES work_nodes(id) ON DELETE CASCADE,
    actor_id uuid NOT NULL REFERENCES actors(id) ON DELETE RESTRICT,
    participant_role text NOT NULL CHECK (participant_role IN ('requester', 'performer')),
    sequence_number integer NOT NULL DEFAULT 0 CHECK (sequence_number >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (work_node_id, actor_id, participant_role)
);

INSERT INTO actors(workspace_id, display_name, has_account)
SELECT id, 'Workspace Owner', true FROM workspaces;

INSERT INTO work_node_participants(work_node_id, actor_id, participant_role, sequence_number)
SELECT n.id, a.id, 'requester', 0
FROM work_nodes n
JOIN actors a ON a.workspace_id = n.workspace_id AND a.has_account;
