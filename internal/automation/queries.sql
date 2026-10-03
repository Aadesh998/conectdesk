-- name: get-enabled-rules
-- tenant: all, the engine caches the enabled rules of every tenant, each tagged with its tenant_id.
select
    tenant_id,
    type,
    events,
    rules,
    execution_mode
from automation_rules where enabled is TRUE ORDER BY weight ASC;

-- name: get-all
SELECT id, created_at, updated_at, "name", description, "type", rules, events, enabled, weight, execution_mode from automation_rules where tenant_id = $1 AND type = $2 ORDER BY weight ASC;

-- name: get-rule
SELECT id, created_at, updated_at, "name", description, "type", rules, events, enabled, weight, execution_mode from automation_rules where tenant_id = $1 AND id = $2;

-- name: update-rule
UPDATE automation_rules SET
    name = $3,
    description = $4,
    type = $5,
    events = $6,
    rules = $7,
    enabled = $8,
    updated_at = now()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: insert-rule
INSERT into automation_rules (tenant_id, name, description, type, events, rules) 
values ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: delete-rule
delete from automation_rules where tenant_id = $1 AND id = $2;

-- name: toggle-rule
UPDATE automation_rules 
SET enabled = NOT enabled, updated_at = NOW() 
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: update-rule-weight
UPDATE automation_rules
SET weight = $3, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2;

-- name: update-rule-execution-mode
UPDATE automation_rules
SET execution_mode = $3, updated_at = NOW()
WHERE tenant_id = $1 AND type = $2;
