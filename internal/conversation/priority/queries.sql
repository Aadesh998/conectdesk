-- name: get-all
SELECT created_at, id, name from conversation_priorities WHERE tenant_id = $1;

-- name: get
SELECT created_at, id, name from conversation_priorities WHERE tenant_id = $1 AND id = $2;
