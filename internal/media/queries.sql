-- name: insert-media
INSERT INTO media (tenant_id, store, filename, content_type, size, meta, model_id, model_type, disposition, content_id, uuid, private)
VALUES(
  $1,
  $2,
  $3,
  $4,
  $5,
  $6,
  NULLIF($7, 0),
  NULLIF($8, ''),
  $9,
  $10,
  $11,
  $12
)
RETURNING id;

-- name: get-media
SELECT id, tenant_id, created_at, updated_at, "uuid", store, filename, content_type, content_id, model_id, model_type, disposition, "size", meta, private
FROM media
WHERE tenant_id = $1 AND (
   ($2 > 0 AND id = $2)
   OR
   ($3 != '' AND uuid = NULLIF($3, '')::uuid)
)

-- name: get-media-by-uuid
SELECT id, tenant_id, created_at, updated_at, "uuid", store, filename, content_type, content_id, model_id, model_type, disposition, "size", meta, private
FROM media
WHERE tenant_id = $1 AND uuid = $2;

-- name: delete-media
DELETE FROM media
WHERE tenant_id = $1 AND uuid = $2;

-- name: link-message-media
UPDATE media
SET model_type = 'messages',
    model_id = $2,
    content_id = CASE
        WHEN uuid = ANY($4::uuid[]) THEN COALESCE(NULLIF(content_id, ''), 'ldsk-' || uuid::TEXT)
        ELSE content_id
    END
WHERE tenant_id = $1
  AND (id = ANY($3::INT[]) OR uuid = ANY($4::uuid[]))
  AND COALESCE(model_type, 'messages') = 'messages'
  AND COALESCE(model_id, 0) = 0;

-- name: get-model-media
SELECT id, tenant_id, created_at, updated_at, "uuid", store, filename, content_type, content_id, model_id, model_type, disposition, "size", meta, private
FROM media
WHERE tenant_id = $1
    AND model_type = $2
    AND model_id = $3;

-- name: get-unlinked-message-media
-- tenant: all, the cleaner sweeps every tenant and deletes each row under its own tenant_id.
SELECT id, tenant_id, created_at, updated_at, "uuid", store, filename, content_type, content_id, model_id, model_type, disposition, "size", meta, private
FROM media
WHERE model_type = 'messages'
  AND (
    ((model_id IS NULL OR model_id = 0) AND created_at < NOW() - INTERVAL '7 days')
    OR (model_id > 0 AND created_at < NOW() - INTERVAL '24 hours' AND NOT EXISTS (SELECT 1 FROM conversation_messages cm WHERE cm.id = media.model_id))
  );

-- name: get-unlinked-help-article-media
-- tenant: all, the cleaner sweeps every tenant and deletes each row under its own tenant_id.
SELECT id, tenant_id, created_at, updated_at, "uuid", store, filename, content_type, content_id, model_id, model_type, disposition, "size", meta, private
FROM media
WHERE model_type = 'help_articles'
  AND (model_id IS NULL OR model_id = 0 OR NOT EXISTS (SELECT 1 FROM help_articles ha WHERE ha.id = media.model_id))
  AND NOT EXISTS (SELECT 1 FROM help_articles ha WHERE ha.tenant_id = media.tenant_id AND POSITION(media.uuid::TEXT IN ha.content) > 0)
  AND updated_at < NOW() - INTERVAL '7 days';

-- name: link-help-article-media
UPDATE media
SET model_id = $2, updated_at = NOW()
WHERE tenant_id = $1
  AND model_type = 'help_articles'
  AND uuid = ANY($3::uuid[])
  AND (model_id IS NULL OR model_id = 0 OR model_id = $2);

-- name: unlink-help-article-media
UPDATE media
SET model_id = NULL, updated_at = NOW()
WHERE tenant_id = $1
  AND model_type = 'help_articles'
  AND model_id = $2
  AND NOT (uuid = ANY($3::uuid[]));

-- name: content-id-exists
SELECT m.uuid
FROM media m
INNER JOIN conversation_messages cm ON cm.id = m.model_id
WHERE m.tenant_id = $1
  AND m.model_type = 'messages'
  AND m.content_id = $2
  AND cm.conversation_id = (SELECT id FROM conversations WHERE tenant_id = $1 AND uuid = $3::uuid LIMIT 1);

-- name: get-media-by-content-ids
SELECT m.id, m.tenant_id, m.created_at, m.updated_at, m."uuid", m.store, m.filename, m.content_type, m.content_id, m.model_id, m.model_type, m.disposition, m."size", m.meta, m.private
FROM media m
INNER JOIN conversation_messages cm ON cm.id = m.model_id
WHERE m.tenant_id = $1
  AND m.model_type = 'messages'
  AND m.content_id = ANY($2)
  AND cm.conversation_id = (SELECT id FROM conversations WHERE tenant_id = $1 AND uuid = $3::uuid LIMIT 1);

-- name: get-draft-inline-media
SELECT m.id, m.tenant_id, m.created_at, m.updated_at, m."uuid", m.store, m.filename, m.content_type, m.content_id, m.model_id, m.model_type, m.disposition, m."size", m.meta
FROM media m
LEFT JOIN conversation_messages cm ON cm.id = m.model_id AND m.model_type = 'messages'
WHERE m.tenant_id = $1
  AND m.uuid = $2
  AND (COALESCE(m.model_id, 0) = 0 OR cm.conversation_id = $3);

-- name: get-storage-used
-- Bytes of media the tenant stores, as plan storage limits count them; its
-- own data exports don't count.
SELECT COALESCE(SUM(size), 0)::BIGINT FROM media WHERE tenant_id = $1 AND model_type IS DISTINCT FROM 'data_exports';
