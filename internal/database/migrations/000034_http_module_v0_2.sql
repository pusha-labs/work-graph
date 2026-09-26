INSERT INTO workflow_modules(module_id,module_version,name,description,step_type,search_terms,configuration_schema,publisher)
SELECT module_id,'0.2.0',name,'Call an endpoint with controlled credentials, timeout, and response capture.',step_type,search_terms,configuration_schema,publisher
FROM workflow_modules
WHERE module_id='builtin.http' AND module_version='0.1.0';
