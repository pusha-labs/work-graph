CREATE TABLE workflow_steps (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    work_node_id uuid NOT NULL REFERENCES work_nodes(id) ON DELETE CASCADE,
    position integer NOT NULL CHECK (position > 0),
    name text NOT NULL,
    step_type text NOT NULL DEFAULT 'human' CHECK (step_type IN ('human', 'api', 'script', 'module')),
    step_status text NOT NULL DEFAULT 'pending' CHECK (step_status IN ('pending', 'ready', 'active', 'completed', 'failed')),
    claimed_by uuid REFERENCES actors(id) ON DELETE SET NULL,
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (work_node_id, position)
);

CREATE TABLE workflow_step_capabilities (
    workflow_step_id uuid NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
    capability_id uuid NOT NULL REFERENCES capabilities(id) ON DELETE CASCADE,
    PRIMARY KEY (workflow_step_id, capability_id)
);

CREATE TABLE workflow_step_knowledge (
    workflow_step_id uuid NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
    subject_id uuid NOT NULL REFERENCES knowledge_subjects(id) ON DELETE CASCADE,
    PRIMARY KEY (workflow_step_id, subject_id)
);

INSERT INTO workflow_steps(work_node_id,position,name,step_status,claimed_by,started_at,completed_at)
SELECT n.id,1,'Perform work',
       CASE n.lifecycle_status WHEN 'active' THEN 'active' WHEN 'review' THEN 'completed' WHEN 'closed' THEN 'completed' ELSE 'ready' END,
       performer.actor_id,
       CASE WHEN n.lifecycle_status IN ('active','review','closed') THEN n.updated_at END,
       CASE WHEN n.lifecycle_status IN ('review','closed') THEN n.updated_at END
FROM work_nodes n
LEFT JOIN LATERAL (
    SELECT p.actor_id FROM work_node_participants p
    WHERE p.work_node_id=n.id AND p.participant_role='performer'
    ORDER BY p.sequence_number LIMIT 1
) performer ON true;

INSERT INTO workflow_step_capabilities(workflow_step_id,capability_id)
SELECT s.id,r.capability_id FROM workflow_steps s JOIN work_node_requirements r ON r.work_node_id=s.work_node_id;

INSERT INTO workflow_step_knowledge(workflow_step_id,subject_id)
SELECT s.id,r.subject_id FROM workflow_steps s JOIN work_node_knowledge_requirements r ON r.work_node_id=s.work_node_id;
