CREATE TABLE workflow_step_executions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workflow_step_id uuid NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
    attempt_number integer NOT NULL CHECK (attempt_number > 0),
    execution_status text NOT NULL CHECK (execution_status IN ('running','succeeded','returned','superseded','failed','cancelled','timed_out')),
    started_by uuid REFERENCES actors(id) ON DELETE SET NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    result jsonb NOT NULL DEFAULT '{}'::jsonb,
    error_message text,
    return_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workflow_step_id, attempt_number)
);

CREATE UNIQUE INDEX workflow_step_one_running_execution
    ON workflow_step_executions(workflow_step_id)
    WHERE execution_status='running';

INSERT INTO workflow_step_executions(workflow_step_id,attempt_number,execution_status,started_by,started_at,finished_at)
SELECT id,1,CASE step_status WHEN 'active' THEN 'running' ELSE 'succeeded' END,claimed_by,
       COALESCE(started_at,created_at),CASE WHEN step_status='completed' THEN COALESCE(completed_at,created_at) END
FROM workflow_steps WHERE step_status IN ('active','completed');
