CREATE TABLE knowledge_subjects (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    subject_type text NOT NULL DEFAULT 'service' CHECK (subject_type IN ('service', 'project', 'system', 'domain', 'other')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name, subject_type)
);

CREATE TABLE actor_knowledge (
    actor_id uuid NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    subject_id uuid NOT NULL REFERENCES knowledge_subjects(id) ON DELETE CASCADE,
    claim_source text NOT NULL DEFAULT 'admin' CHECK (claim_source IN ('self', 'admin', 'inferred', 'imported')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, subject_id, claim_source)
);

CREATE INDEX actor_knowledge_subject_idx ON actor_knowledge(subject_id, actor_id);
