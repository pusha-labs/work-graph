ALTER TABLE workspace_module_installations
ADD COLUMN publisher_trusted boolean NOT NULL DEFAULT false;

UPDATE workspace_module_installations i
SET publisher_trusted=true
FROM workflow_modules m
WHERE m.module_id=i.module_id AND m.module_version=i.module_version AND m.publisher='Work Graph';

ALTER TABLE workspace_module_installations
ADD CONSTRAINT enabled_module_requires_trust CHECK (NOT enabled OR publisher_trusted);
