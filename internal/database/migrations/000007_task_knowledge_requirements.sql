CREATE TABLE work_node_knowledge_requirements (
    work_node_id uuid NOT NULL REFERENCES work_nodes(id) ON DELETE CASCADE,
    subject_id uuid NOT NULL REFERENCES knowledge_subjects(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (work_node_id, subject_id)
);
