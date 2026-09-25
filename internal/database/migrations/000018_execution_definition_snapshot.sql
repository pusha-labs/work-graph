ALTER TABLE workflow_step_executions ADD COLUMN definition_snapshot jsonb;

UPDATE workflow_step_executions e SET definition_snapshot=jsonb_build_object(
    'name',s.name,
    'stepType',s.step_type,
    'moduleId',s.module_id,
    'moduleVersion',s.module_version,
    'configuration',s.configuration,
    'capabilityIds',COALESCE((SELECT jsonb_agg(capability_id) FROM workflow_step_capabilities WHERE workflow_step_id=s.id),'[]'::jsonb),
    'subjectIds',COALESCE((SELECT jsonb_agg(subject_id) FROM workflow_step_knowledge WHERE workflow_step_id=s.id),'[]'::jsonb)
) FROM workflow_steps s WHERE s.id=e.workflow_step_id;

ALTER TABLE workflow_step_executions ALTER COLUMN definition_snapshot SET NOT NULL;
