-- name: get-provider-by-type
SELECT id, created_at, updated_at, name, provider, type, config, is_default FROM ai_providers WHERE tenant_id = $1 AND type = $2;

-- name: update-provider-config
UPDATE ai_providers SET config = $3, updated_at = now() WHERE tenant_id = $1 AND type = $2;

-- name: get-prompt
SELECT id, created_at, updated_at, key, title, content FROM ai_prompts WHERE tenant_id = $1 AND key = $2;

-- name: get-prompts
SELECT id, created_at, updated_at, key, title FROM ai_prompts WHERE tenant_id = $1 ORDER BY title;

-- name: get-knowledge-base-items
SELECT id, created_at, updated_at, type, title, content, enabled, source, source_url, embedded_fingerprint FROM ai_knowledge_base WHERE tenant_id = $1 ORDER BY updated_at DESC;

-- name: get-knowledge-base-item
SELECT id, created_at, updated_at, type, title, content, enabled, source, source_url, embedded_fingerprint FROM ai_knowledge_base WHERE tenant_id = $1 AND id = $2;

-- name: knowledge-base-item-exists
SELECT EXISTS(SELECT 1 FROM ai_knowledge_base WHERE tenant_id = $1 AND id = $2);

-- name: insert-knowledge-base-item
INSERT INTO ai_knowledge_base (tenant_id, type, title, content, enabled, source, source_url) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: update-knowledge-base-item
UPDATE ai_knowledge_base SET title = $3, content = $4, enabled = $5, updated_at = now() WHERE tenant_id = $1 AND id = $2 RETURNING *;

-- name: delete-knowledge-base-item
DELETE FROM ai_knowledge_base WHERE tenant_id = $1 AND id = $2;

-- name: set-knowledge-base-embedded-fingerprint
UPDATE ai_knowledge_base SET embedded_fingerprint = $3 WHERE tenant_id = $1 AND id = $2;

-- name: get-embeddable-help-articles
WITH RECURSIVE published_collections AS (
    SELECT c.id FROM article_collections c
    JOIN help_centers hc ON hc.id = c.help_center_id AND hc.is_active
    WHERE c.tenant_id = $1 AND c.parent_id IS NULL AND c.is_published = true
    UNION
    SELECT c.id FROM article_collections c JOIN published_collections p ON c.parent_id = p.id
    WHERE c.is_published = true
)
SELECT a.id, a.title, a.content, a.status, a.ai_enabled, a.embedded_fingerprint,
    a.collection_id IN (SELECT id FROM published_collections) AS is_reachable
FROM help_articles a
WHERE a.tenant_id = $1 AND ((a.status = 'published' AND a.ai_enabled) OR a.embedded_fingerprint <> '');

-- name: get-embeddable-help-article
WITH RECURSIVE published_collections AS (
    SELECT c.id FROM article_collections c
    JOIN help_centers hc ON hc.id = c.help_center_id AND hc.is_active
    WHERE c.tenant_id = $1 AND c.parent_id IS NULL AND c.is_published = true
    UNION
    SELECT c.id FROM article_collections c JOIN published_collections p ON c.parent_id = p.id
    WHERE c.is_published = true
)
SELECT a.id, a.title, a.content, a.status, a.ai_enabled, a.embedded_fingerprint,
    a.collection_id IN (SELECT id FROM published_collections) AS is_reachable
FROM help_articles a
WHERE a.tenant_id = $1 AND a.id = $2;

-- name: help-article-exists
SELECT EXISTS(SELECT 1 FROM help_articles WHERE tenant_id = $1 AND id = $2);

-- name: set-help-article-embedded-fingerprint
UPDATE help_articles SET embedded_fingerprint = $3 WHERE tenant_id = $1 AND id = $2;

-- name: delete-orphan-help-article-embeddings
DELETE FROM embeddings e
WHERE e.tenant_id = $1 AND e.source_type = 'help_article' AND NOT EXISTS (SELECT 1 FROM help_articles a WHERE a.tenant_id = $1 AND a.id = e.source_id)
RETURNING e.source_id;

-- name: insert-embedding
INSERT INTO embeddings (tenant_id, source_type, source_id, chunk_text, embedding, dimensions) VALUES ($1, $2, $3, $4, $5::vector, $6);

-- name: search-embeddings
-- Exact cosine search over one tenant's vectors of the query's length; vectors from an older model are skipped, never compared.
SELECT source_type, source_id, chunk_text, 1 - (embedding <=> $3::vector) AS score
FROM embeddings
WHERE tenant_id = $1 AND source_type = ANY($2) AND dimensions = $4
ORDER BY embedding <=> $3::vector
LIMIT $5;

-- name: count-mismatched-embeddings
SELECT COUNT(*) FROM embeddings WHERE tenant_id = $1 AND source_type = ANY($2) AND dimensions <> $3;

-- name: get-tag-embedding-texts
SELECT source_id, chunk_text FROM embeddings WHERE tenant_id = $1 AND source_type = 'tag';

-- name: delete-embeddings-by-source
DELETE FROM embeddings WHERE tenant_id = $1 AND source_type = $2 AND source_id = $3;

-- name: delete-embeddings-by-source-ids
DELETE FROM embeddings WHERE tenant_id = $1 AND source_type = $2 AND source_id = ANY($3);

-- name: delete-embeddings-by-source-type
DELETE FROM embeddings WHERE tenant_id = $1 AND source_type = $2;

-- name: get-tags
SELECT id, name FROM tags WHERE tenant_id = $1 ORDER BY id;

-- name: get-tools
SELECT id, created_at, updated_at, name, description, url, method, auth, parameters, enabled, requires_verification FROM ai_tools WHERE tenant_id = $1 ORDER BY updated_at DESC;

-- name: get-enabled-tools-by-ids
SELECT id, created_at, updated_at, name, description, url, method, auth, parameters, enabled, requires_verification FROM ai_tools WHERE tenant_id = $1 AND enabled = true AND id = ANY($2);

-- name: get-tool
SELECT id, created_at, updated_at, name, description, url, method, auth, parameters, enabled, requires_verification FROM ai_tools WHERE tenant_id = $1 AND id = $2;

-- name: get-tool-auth
SELECT auth FROM ai_tools WHERE tenant_id = $1 AND id = $2;

-- name: insert-tool
INSERT INTO ai_tools (tenant_id, name, description, url, method, auth, parameters, enabled, requires_verification)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: update-tool
UPDATE ai_tools SET name = $3, description = $4, url = $5, method = $6, auth = $7, parameters = $8, enabled = $9, requires_verification = $10, updated_at = now()
WHERE tenant_id = $1 AND id = $2 RETURNING *;

-- name: delete-tool
DELETE FROM ai_tools WHERE tenant_id = $1 AND id = $2;

-- name: get-copilot-messages
SELECT role, content FROM (
    SELECT id, role, content FROM copilot_messages WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3 ORDER BY id DESC LIMIT $4
) latest ORDER BY id;

-- name: insert-copilot-message
INSERT INTO copilot_messages (tenant_id, conversation_id, user_id, role, content) VALUES ($1, $2, $3, $4, $5);

-- name: delete-copilot-messages
DELETE FROM copilot_messages WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3;

-- name: consume-ai-request
-- Counts one AI request this month, unless the tenant has used its limit $2
-- (<= 0 means unlimited); then no row comes back.
INSERT INTO ai_usage (tenant_id, "month", requests) VALUES ($1, date_trunc('month', NOW())::date, 1)
ON CONFLICT (tenant_id, "month") DO UPDATE SET requests = ai_usage.requests + 1
WHERE $2 <= 0 OR ai_usage.requests < $2
RETURNING requests;

-- name: get-ai-usage
SELECT COALESCE((SELECT requests FROM ai_usage WHERE tenant_id = $1 AND "month" = date_trunc('month', NOW())::date), 0);

-- name: get-report-copilot-messages
SELECT role, content FROM (
    SELECT id, role, content FROM report_copilot_messages WHERE tenant_id = $1 AND user_id = $2 AND scope = $3 ORDER BY id DESC LIMIT $4
) latest ORDER BY id;

-- name: insert-report-copilot-message
INSERT INTO report_copilot_messages (tenant_id, user_id, scope, role, content) VALUES ($1, $2, $3, $4, $5);

-- name: delete-report-copilot-messages
DELETE FROM report_copilot_messages WHERE tenant_id = $1 AND user_id = $2 AND scope = $3;
