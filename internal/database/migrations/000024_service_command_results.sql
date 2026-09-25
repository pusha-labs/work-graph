CREATE TABLE service_command_results (
    service_account_id uuid NOT NULL REFERENCES service_accounts(id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 200),
    request_hash bytea NOT NULL,
    response_status integer NOT NULL CHECK (response_status BETWEEN 200 AND 299),
    response_body jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (service_account_id, idempotency_key)
);

CREATE INDEX service_command_results_workspace_created_idx
    ON service_command_results(workspace_id, created_at);
