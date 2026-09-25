CREATE TABLE gateway_sync_schedules (
    installation_id uuid PRIMARY KEY REFERENCES gateway_installations(id) ON DELETE CASCADE,
    enabled boolean NOT NULL DEFAULT true,
    interval_seconds integer NOT NULL DEFAULT 300 CHECK (interval_seconds BETWEEN 60 AND 86400),
    next_run_at timestamptz NOT NULL DEFAULT now(),
    lease_until timestamptz,
    last_started_at timestamptz,
    last_completed_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    consecutive_failures integer NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO gateway_sync_schedules(installation_id)
SELECT id FROM gateway_installations WHERE provider='jira_cloud' AND installation_status='connected'
ON CONFLICT DO NOTHING;

CREATE INDEX gateway_sync_schedules_due_idx
ON gateway_sync_schedules(next_run_at)
WHERE enabled;
