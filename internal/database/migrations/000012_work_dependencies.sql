CREATE TABLE work_node_dependencies (
    work_node_id uuid NOT NULL REFERENCES work_nodes(id) ON DELETE CASCADE,
    depends_on_node_id uuid NOT NULL REFERENCES work_nodes(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (work_node_id, depends_on_node_id),
    CHECK (work_node_id <> depends_on_node_id)
);

CREATE INDEX work_node_dependencies_blocker_idx
    ON work_node_dependencies(depends_on_node_id, work_node_id);
