ALTER TABLE workspaces
    ADD COLUMN work_distribution_mode text NOT NULL DEFAULT 'simple'
    CHECK (work_distribution_mode IN ('simple','exchange'));

ALTER TABLE workflow_steps
    ADD COLUMN distribution_mode text NOT NULL DEFAULT 'inherit'
    CHECK (distribution_mode IN ('inherit','simple','exchange'));
