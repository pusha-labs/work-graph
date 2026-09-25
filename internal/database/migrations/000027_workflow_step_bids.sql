CREATE TABLE workflow_step_bids (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workflow_step_id uuid NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
    actor_id uuid NOT NULL REFERENCES actors(id) ON DELETE CASCADE,
    promised_duration_minutes integer NOT NULL CHECK (promised_duration_minutes BETWEEN 1 AND 525600),
    bid_status text NOT NULL DEFAULT 'active' CHECK (bid_status IN ('active','withdrawn','won','lost')),
    submitted_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    withdrawn_at timestamptz,
    UNIQUE (workflow_step_id, actor_id)
);

CREATE INDEX workflow_step_bids_active_step_idx
    ON workflow_step_bids(workflow_step_id, promised_duration_minutes, submitted_at, actor_id)
    WHERE bid_status='active';
