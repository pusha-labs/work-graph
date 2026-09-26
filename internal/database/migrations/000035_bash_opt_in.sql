UPDATE workspace_module_installations
SET enabled=false,updated_at=now()
WHERE module_id='builtin.bash';
