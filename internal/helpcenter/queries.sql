-- name: get-all-help-centers
SELECT id, created_at, updated_at, name, slug, page_title, meta_description, custom_css, custom_js, default_locale, allowed_locales, is_active, theme, custom_domain, template
FROM help_centers
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: get-active-help-centers
SELECT id, created_at, updated_at, name, slug, page_title, meta_description, custom_css, custom_js, default_locale, allowed_locales, is_active, theme, custom_domain, template
FROM help_centers
WHERE tenant_id = $1 AND is_active = true
ORDER BY created_at DESC;

-- name: get-help-center-by-id
SELECT id, created_at, updated_at, name, slug, page_title, meta_description, custom_css, custom_js, default_locale, allowed_locales, is_active, theme, custom_domain, template
FROM help_centers
WHERE tenant_id = $1 AND id = $2;

-- name: get-help-center-by-slug
SELECT id, created_at, updated_at, name, slug, page_title, meta_description, custom_css, custom_js, default_locale, allowed_locales, is_active, theme, custom_domain, template
FROM help_centers
WHERE tenant_id = $1 AND slug = $2 AND is_active = true;

-- name: insert-help-center
INSERT INTO help_centers (tenant_id, name, slug, page_title, meta_description, custom_css, custom_js, default_locale, allowed_locales, theme, custom_domain, template)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: update-help-center
UPDATE help_centers
SET name = $3, slug = $4, page_title = $5, meta_description = $6, custom_css = $7, custom_js = $8, default_locale = $9, allowed_locales = $10, theme = $11, custom_domain = $12, template = $13, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: toggle-help-center-active
UPDATE help_centers
SET is_active = NOT is_active, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: delete-help-center
DELETE FROM help_centers
WHERE tenant_id = $1 AND id = $2;

-- name: get-help-center-locales-in-use
SELECT DISTINCT locale FROM article_collections WHERE tenant_id = $1 AND help_center_id = $2
UNION
SELECT DISTINCT a.locale
FROM help_articles a
JOIN article_collections c ON c.id = a.collection_id
WHERE c.tenant_id = $1 AND c.help_center_id = $2;

-- name: get-collections-by-help-center
SELECT id, created_at, updated_at, help_center_id, slug, parent_id, locale, name, description, icon, sort_order, is_published
FROM article_collections
WHERE tenant_id = $1 AND help_center_id = $2
ORDER BY sort_order ASC, created_at DESC;

-- name: get-collection-by-id
SELECT id, created_at, updated_at, help_center_id, slug, parent_id, locale, name, description, icon, sort_order, is_published
FROM article_collections
WHERE tenant_id = $1 AND id = $2;

-- name: get-collection-subtree-depth
WITH RECURSIVE subtree AS (
    SELECT id, 1 AS depth FROM article_collections WHERE tenant_id = $1 AND id = $2
    UNION ALL
    SELECT c.id, s.depth + 1 FROM article_collections c JOIN subtree s ON c.parent_id = s.id
)
SELECT COALESCE(MAX(depth), 1) FROM subtree;

-- name: lock-help-center
SELECT id FROM help_centers WHERE tenant_id = $1 AND id = $2 FOR UPDATE;

-- name: lock-help-center-by-collection
SELECT hc.id FROM help_centers hc
JOIN article_collections c ON c.help_center_id = hc.id
WHERE c.tenant_id = $1 AND c.id = $2
FOR UPDATE OF hc;

-- name: collection-has-content
SELECT EXISTS(SELECT 1 FROM article_collections WHERE tenant_id = $1 AND parent_id = $2)
    OR EXISTS(SELECT 1 FROM help_articles WHERE tenant_id = $1 AND collection_id = $2);

-- name: get-article-ids-in-collection-subtree
WITH RECURSIVE subtree AS (
    SELECT id FROM article_collections WHERE tenant_id = $1 AND id = $2
    UNION ALL
    SELECT c.id FROM article_collections c JOIN subtree s ON c.parent_id = s.id
)
SELECT a.id FROM help_articles a WHERE a.tenant_id = $1 AND a.collection_id IN (SELECT id FROM subtree);

-- name: get-article-ids-in-help-center
SELECT a.id FROM help_articles a
JOIN article_collections c ON c.id = a.collection_id
WHERE c.tenant_id = $1 AND c.help_center_id = $2;

-- name: update-collection-sort-order
UPDATE article_collections
SET sort_order = $4, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2 AND help_center_id = $3;

-- name: get-collection-by-id-for-update
SELECT id, created_at, updated_at, help_center_id, slug, parent_id, locale, name, description, icon, sort_order, is_published
FROM article_collections
WHERE tenant_id = $1 AND id = $2
FOR UPDATE;

-- name: collection-slug-exists-in-help-center
SELECT EXISTS(
    SELECT 1
    FROM article_collections
    WHERE tenant_id = $1 AND help_center_id = $2 AND slug = $3 AND locale = $4
);

-- name: insert-collection
INSERT INTO article_collections (tenant_id, help_center_id, slug, parent_id, locale, name, description, icon, sort_order, is_published)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: update-collection
UPDATE article_collections
SET slug = $3, parent_id = $4, locale = $5, name = $6, description = $7, icon = $8, sort_order = $9, is_published = $10, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: toggle-collection-published
UPDATE article_collections
SET is_published = NOT is_published, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: delete-collection
DELETE FROM article_collections
WHERE tenant_id = $1 AND id = $2;

-- name: get-article-by-id
SELECT a.id, a.created_at, a.updated_at, a.collection_id, a.author_id, a.created_by, a.slug, a.locale, a.title, a.content,
    a.excerpt, a.meta_title, a.meta_description, a.meta_image_url, a.sort_order, a.status, a.view_count, a.ai_enabled,
    TRIM(u.first_name || ' ' || COALESCE(u.last_name, '')) AS author_name,
    TRIM(cu.first_name || ' ' || COALESCE(cu.last_name, '')) AS created_by_name,
    (SELECT COUNT(*) FROM help_article_feedback f WHERE f.article_id = a.id AND f.is_helpful) AS helpful_count,
    (SELECT COUNT(*) FROM help_article_feedback f WHERE f.article_id = a.id AND NOT f.is_helpful) AS not_helpful_count
FROM help_articles a
LEFT JOIN users u ON u.id = a.author_id
LEFT JOIN users cu ON cu.id = a.created_by
WHERE a.tenant_id = $1 AND a.id = $2;

-- name: insert-article
INSERT INTO help_articles (tenant_id, collection_id, author_id, created_by, slug, locale, title, content, excerpt, meta_title, meta_description, meta_image_url, sort_order, status, ai_enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: update-article
UPDATE help_articles
SET collection_id = COALESCE($10, collection_id), slug = $3, locale = $4, title = $5, content = $6, sort_order = $7, status = $8, ai_enabled = $9,
    excerpt = $11, meta_title = $12, meta_description = $13, meta_image_url = $14, author_id = $15, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: user-is-author-assignable
SELECT EXISTS(
    SELECT 1 FROM users
    WHERE tenant_id = $1 AND id = $2 AND type IN ('agent', 'ai_assistant') AND deleted_at IS NULL
);

-- name: article-slug-exists-in-help-center
SELECT EXISTS(
    SELECT 1
    FROM help_articles a
    JOIN article_collections c ON c.id = a.collection_id
    WHERE c.tenant_id = $1 AND c.help_center_id = (SELECT help_center_id FROM article_collections WHERE tenant_id = $1 AND id = $2)
        AND a.slug = $3 AND a.locale = $4
);

-- name: other-article-slug-exists-in-help-center
SELECT EXISTS(
    SELECT 1
    FROM help_articles a
    JOIN article_collections c ON c.id = a.collection_id
    WHERE c.tenant_id = $1 AND c.help_center_id = (SELECT help_center_id FROM article_collections WHERE tenant_id = $1 AND id = $2)
        AND a.slug = $3 AND a.locale = $4 AND a.id <> $5
);

-- name: move-article-to-collection
UPDATE help_articles
SET collection_id = $3, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: update-article-sort-order
UPDATE help_articles
SET sort_order = $4, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2 AND collection_id = $3;

-- name: update-article-status
UPDATE help_articles
SET status = $3, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: delete-article
DELETE FROM help_articles
WHERE tenant_id = $1 AND id = $2;

-- name: get-help-center-tree-data
SELECT
    'collection' AS type,
    c.id,
    c.created_at,
    c.updated_at,
    c.help_center_id,
    c.slug,
    c.parent_id,
    c.locale,
    c.name,
    c.description,
    c.icon,
    c.sort_order,
    c.is_published,
    NULL::INTEGER AS collection_id,
    NULL::TEXT AS title,
    NULL::TEXT AS content,
    NULL::TEXT AS status,
    NULL::INTEGER AS view_count,
    NULL::BOOLEAN AS ai_enabled,
    NULL::TEXT AS author_name,
    NULL::TEXT AS author_avatar
FROM article_collections c
WHERE c.tenant_id = $1 AND c.help_center_id = $2 AND ($3 = '' OR c.locale = $3)

UNION ALL

SELECT
    'article' AS type,
    a.id,
    a.created_at,
    a.updated_at,
    c.help_center_id,
    a.slug,
    NULL::INTEGER AS parent_id,
    a.locale,
    a.title AS name,
    NULL::TEXT AS description,
    NULL::TEXT AS icon,
    a.sort_order,
    NULL::BOOLEAN AS is_published,
    a.collection_id,
    a.title,
    '' AS content,
    a.status,
    a.view_count,
    a.ai_enabled,
    NULL::TEXT AS author_name,
    NULL::TEXT AS author_avatar
FROM help_articles a
JOIN article_collections c ON a.collection_id = c.id
WHERE c.tenant_id = $1 AND c.help_center_id = $2 AND ($3 = '' OR a.locale = $3)

ORDER BY type DESC, parent_id NULLS FIRST, sort_order, name;

-- name: get-public-tree-data
WITH RECURSIVE published_collections AS (
    SELECT id FROM article_collections
    WHERE tenant_id = $1 AND help_center_id = $2 AND parent_id IS NULL AND is_published = true AND ($3 = '' OR locale = $3)
    UNION
    SELECT c.id FROM article_collections c
    JOIN published_collections p ON c.parent_id = p.id
    WHERE c.is_published = true AND ($3 = '' OR c.locale = $3)
)
SELECT
    'collection' AS type,
    c.id,
    c.created_at,
    c.updated_at,
    c.help_center_id,
    c.slug,
    c.parent_id,
    c.locale,
    c.name,
    c.description,
    c.icon,
    c.sort_order,
    c.is_published,
    NULL::INTEGER AS collection_id,
    NULL::TEXT AS title,
    NULL::TEXT AS content,
    NULL::TEXT AS status,
    NULL::INTEGER AS view_count,
    NULL::BOOLEAN AS ai_enabled,
    NULL::TEXT AS author_name,
    NULL::TEXT AS author_avatar
FROM article_collections c
WHERE c.id IN (SELECT id FROM published_collections)

UNION ALL

SELECT
    'article' AS type,
    a.id,
    a.created_at,
    a.updated_at,
    c.help_center_id,
    a.slug,
    NULL::INTEGER AS parent_id,
    a.locale,
    a.title AS name,
    NULL::TEXT AS description,
    NULL::TEXT AS icon,
    a.sort_order,
    NULL::BOOLEAN AS is_published,
    a.collection_id,
    a.title,
    '' AS content,
    a.status,
    a.view_count,
    a.ai_enabled,
    TRIM(u.first_name || ' ' || COALESCE(u.last_name, '')) AS author_name,
    u.avatar_url AS author_avatar
FROM help_articles a
JOIN article_collections c ON a.collection_id = c.id AND c.locale = a.locale
LEFT JOIN users u ON u.id = a.author_id
WHERE c.id IN (SELECT id FROM published_collections) AND a.status = 'published'
    AND ($3 = '' OR a.locale = $3)

ORDER BY type DESC, parent_id NULLS FIRST, sort_order, name;

-- name: get-published-article-by-slug
WITH RECURSIVE published_collections AS (
    SELECT c.id FROM article_collections c
    JOIN help_centers h ON h.id = c.help_center_id
    WHERE h.tenant_id = $1 AND h.slug = $2 AND c.parent_id IS NULL AND c.is_published = true
    UNION
    SELECT c.id FROM article_collections c
    JOIN published_collections p ON c.parent_id = p.id
    WHERE c.is_published = true
)
SELECT a.id, a.created_at, a.updated_at, a.collection_id, a.author_id, a.slug, a.locale, a.title, a.content,
    a.excerpt, a.meta_title, a.meta_description, a.meta_image_url, a.sort_order, a.status, a.view_count, a.ai_enabled,
    TRIM(u.first_name || ' ' || COALESCE(u.last_name, '')) AS author_name,
    u.avatar_url AS author_avatar
FROM help_articles a
JOIN article_collections c ON c.id = a.collection_id AND c.locale = a.locale AND c.id IN (SELECT id FROM published_collections)
LEFT JOIN users u ON u.id = a.author_id
WHERE a.slug = $3 AND a.status = 'published' AND ($4 = '' OR a.locale = $4)
ORDER BY c.sort_order, a.sort_order
LIMIT 1;

-- name: get-published-article-locales
WITH RECURSIVE published_collections AS (
    SELECT c.id FROM article_collections c
    JOIN help_centers h ON h.id = c.help_center_id
    WHERE h.tenant_id = $1 AND h.slug = $2 AND c.parent_id IS NULL AND c.is_published = true
    UNION
    SELECT c.id FROM article_collections c
    JOIN published_collections p ON c.parent_id = p.id
    WHERE c.is_published = true
)
SELECT DISTINCT a.locale
FROM help_articles a
JOIN article_collections c ON c.id = a.collection_id AND c.locale = a.locale AND c.id IN (SELECT id FROM published_collections)
WHERE a.slug = $3 AND a.status = 'published';

-- name: get-published-collection-locales
WITH RECURSIVE published_collections AS (
    SELECT c.id FROM article_collections c
    JOIN help_centers h ON h.id = c.help_center_id
    WHERE h.tenant_id = $1 AND h.slug = $2 AND c.parent_id IS NULL AND c.is_published = true
    UNION
    SELECT c.id FROM article_collections c
    JOIN published_collections p ON c.parent_id = p.id
    WHERE c.is_published = true
)
SELECT DISTINCT c.locale
FROM article_collections c
WHERE c.slug = $3 AND c.id IN (SELECT id FROM published_collections);

-- name: get-published-articles
WITH RECURSIVE published_collections AS (
    SELECT c.id FROM article_collections c
    JOIN help_centers h ON h.id = c.help_center_id
    WHERE h.tenant_id = $1 AND h.slug = $2 AND c.parent_id IS NULL AND c.is_published = true
    UNION
    SELECT c.id FROM article_collections c
    JOIN published_collections p ON c.parent_id = p.id
    WHERE c.is_published = true
)
SELECT a.id, a.created_at, a.updated_at, a.collection_id, a.slug, a.locale, a.title, a.excerpt, '' AS content, a.sort_order, a.status, a.view_count, a.ai_enabled
FROM help_articles a
JOIN article_collections c ON c.id = a.collection_id AND c.locale = a.locale AND c.id IN (SELECT id FROM published_collections)
WHERE a.status = 'published' AND ($3 = '' OR a.locale = $3)
ORDER BY a.view_count DESC, a.created_at DESC
LIMIT $4;

-- name: get-published-articles-by-collection
SELECT a.id, a.created_at, a.updated_at, a.collection_id, a.slug, a.locale, a.title, a.excerpt, '' AS content, a.sort_order, a.status, a.view_count, a.ai_enabled
FROM help_articles a
JOIN article_collections c ON c.id = a.collection_id AND c.locale = a.locale
WHERE a.tenant_id = $1 AND a.collection_id = $2 AND a.id != $3 AND a.status = 'published' AND ($4 = '' OR a.locale = $4)
ORDER BY a.sort_order ASC, a.created_at DESC
LIMIT $5;

-- name: search-published-articles
WITH RECURSIVE published_collections AS (
    SELECT c.id FROM article_collections c
    JOIN help_centers h ON h.id = c.help_center_id
    WHERE h.tenant_id = $1 AND h.slug = $2 AND c.parent_id IS NULL AND c.is_published = true
    UNION
    SELECT c.id FROM article_collections c
    JOIN published_collections p ON c.parent_id = p.id
    WHERE c.is_published = true
)
SELECT a.id, a.created_at, a.updated_at, a.collection_id, a.slug, a.locale, a.title,
    a.excerpt AS content,
    a.sort_order, a.status, a.view_count, a.ai_enabled
FROM help_articles a
JOIN article_collections c ON c.id = a.collection_id AND c.locale = a.locale AND c.id IN (SELECT id FROM published_collections)
WHERE a.status = 'published' AND a.locale = $5
    AND (a.search_tsv @@ to_tsquery(help_article_search_config($5), NULLIF($6, ''))
        OR a.title ILIKE '%' || $3 || '%' OR a.content ILIKE '%' || $3 || '%')
ORDER BY ts_rank(a.search_tsv, to_tsquery(help_article_search_config($5), NULLIF($6, '')), 2) DESC, a.view_count DESC, a.created_at DESC
LIMIT $4;

-- name: increment-article-view-count
UPDATE help_articles
SET view_count = view_count + 1
WHERE tenant_id = $1 AND id = $2;

-- name: insert-article-feedback
INSERT INTO help_article_feedback (tenant_id, article_id, is_helpful)
SELECT $1, $2, $3
WHERE EXISTS (SELECT 1 FROM help_articles WHERE tenant_id = $1 AND id = $2 AND status = 'published');

-- name: insert-search-query
INSERT INTO help_search_queries (tenant_id, help_center_id, query, results_count)
VALUES ($1, $2, $3, $4);

-- name: get-top-search-terms
SELECT LOWER(query) AS query, COUNT(*) AS count,
    COUNT(*) FILTER (WHERE results_count = 0) AS no_results,
    MAX(created_at)::text AS last_search
FROM help_search_queries
WHERE tenant_id = $1 AND help_center_id = $2 AND created_at >= NOW() - ($4 * INTERVAL '1 day')
GROUP BY LOWER(query)
ORDER BY count DESC, last_search DESC
LIMIT $3;

-- name: get-no-result-search-terms
SELECT LOWER(query) AS query, COUNT(*) AS count,
    COUNT(*) AS no_results,
    MAX(created_at)::text AS last_search
FROM help_search_queries
WHERE tenant_id = $1 AND help_center_id = $2 AND results_count = 0 AND created_at >= NOW() - ($4 * INTERVAL '1 day')
GROUP BY LOWER(query)
ORDER BY count DESC, last_search DESC
LIMIT $3;

-- name: delete-stale-search-queries
-- tenant: all, the search log cleaner applies to every tenant.
DELETE FROM help_search_queries
WHERE created_at < NOW() - ($1 * INTERVAL '1 day');

-- name: custom-domain-used-by-other-tenant
-- tenant: all, a custom domain routes requests to its tenant, so it must be unique across tenants.
SELECT EXISTS(
    SELECT 1 FROM help_centers
    WHERE tenant_id <> $1 AND custom_domain <> ''
      AND LOWER(SUBSTRING(custom_domain FROM '^(?:[a-zA-Z][a-zA-Z0-9+.-]*://)?([^/:?#]+)')) = LOWER($2)
) OR EXISTS(
    SELECT 1 FROM tenants WHERE id <> $1 AND LOWER(custom_domain) = LOWER($2)
);
