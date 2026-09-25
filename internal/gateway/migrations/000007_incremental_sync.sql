ALTER TABLE gateway_sync_schedules
    ADD COLUMN cursor_at timestamptz,
    ADD COLUMN continuation_token text NOT NULL DEFAULT '',
    ADD COLUMN cycle_query text NOT NULL DEFAULT '',
    ADD COLUMN cycle_upper_bound timestamptz;

CREATE TABLE gateway_sync_runs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    installation_id uuid NOT NULL REFERENCES gateway_installations(id) ON DELETE CASCADE,
    trigger_type text NOT NULL CHECK (trigger_type IN ('scheduled','manual')),
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    run_status text NOT NULL DEFAULT 'running' CHECK (run_status IN ('running','succeeded','failed')),
    issues_read integer NOT NULL DEFAULT 0,
    issues_imported integer NOT NULL DEFAULT 0,
    error_message text NOT NULL DEFAULT ''
);

CREATE INDEX gateway_sync_runs_installation_idx ON gateway_sync_runs(installation_id,started_at DESC);
