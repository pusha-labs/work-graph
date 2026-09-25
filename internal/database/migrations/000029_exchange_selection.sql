ALTER TABLE workflow_steps DROP CONSTRAINT workflow_steps_step_status_check;
ALTER TABLE workflow_steps ADD CONSTRAINT workflow_steps_step_status_check
    CHECK (step_status IN ('pending','ready','assigned','active','completed','failed'));

ALTER TABLE workflow_steps
    ADD COLUMN selected_bid_id uuid REFERENCES workflow_step_bids(id) ON DELETE SET NULL,
    ADD COLUMN selected_at timestamptz;

ALTER TABLE workflow_step_executions
    ADD COLUMN selected_bid_id uuid REFERENCES workflow_step_bids(id) ON DELETE SET NULL,
    ADD COLUMN promised_duration_minutes integer CHECK (promised_duration_minutes BETWEEN 1 AND 525600);
