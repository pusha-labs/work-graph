ALTER TABLE workflow_steps
    ADD COLUMN module_id text,
    ADD COLUMN module_version text,
    ADD COLUMN configuration jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE workflow_steps
    ADD CONSTRAINT workflow_step_module_binding CHECK (
        (step_type = 'human' AND module_id IS NULL AND module_version IS NULL)
        OR
        (step_type <> 'human' AND module_id IS NOT NULL AND module_version IS NOT NULL)
    );
