CREATE TABLE workspace_module_installations (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    module_id text NOT NULL,
    module_version text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    allowed_hosts text[] NOT NULL DEFAULT '{}',
    allow_secrets boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id,module_id,module_version),
    FOREIGN KEY (module_id,module_version) REFERENCES workflow_modules(module_id,module_version)
);

INSERT INTO workspace_module_installations(workspace_id,module_id,module_version)
SELECT w.id,m.module_id,m.module_version FROM workspaces w CROSS JOIN workflow_modules m;
