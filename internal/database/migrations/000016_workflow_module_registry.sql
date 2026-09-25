CREATE TABLE workflow_modules (
    module_id text NOT NULL,
    module_version text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    step_type text NOT NULL CHECK (step_type IN ('api', 'script', 'module')),
    search_terms text[] NOT NULL DEFAULT '{}',
    configuration_schema jsonb NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    publisher text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (module_id, module_version)
);

INSERT INTO workflow_modules(module_id,module_version,name,description,step_type,search_terms,configuration_schema,publisher) VALUES
('builtin.http','0.1.0','HTTP request','Call an endpoint and capture its result.','api',ARRAY['api','http','url','request','webhook'],
 $json${"fields":[{"key":"method","label":"Method","type":"select","required":true,"default":"POST","options":["GET","POST","PUT","PATCH","DELETE"]},{"key":"url","label":"URL","type":"url","required":true,"placeholder":"https://api.example.com/action"},{"key":"body","label":"JSON body","type":"textarea","required":false,"placeholder":"{ \"key\": \"{{variable}}\" }"},{"key":"timeoutSeconds","label":"Timeout (seconds)","type":"number","required":true,"default":60,"min":1,"max":3600}]}$json$::jsonb,
 'Work Graph'),
('builtin.bash','0.1.0','Bash','Run a script in an isolated runner.','script',ARRAY['bash','shell','script','command'],
 $json${"fields":[{"key":"script","label":"Script","type":"code","required":true,"placeholder":"#!/bin/sh\n\necho \"Hello from Work Graph\""},{"key":"timeoutSeconds","label":"Timeout (seconds)","type":"number","required":true,"default":60,"min":1,"max":3600}]}$json$::jsonb,
 'Work Graph');
