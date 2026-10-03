-- Help center: sites, collections, articles, feedback and search logs.

DROP TABLE IF EXISTS help_centers CASCADE;
CREATE TABLE help_centers (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	name TEXT NOT NULL,
	slug TEXT NOT NULL,
	page_title TEXT NOT NULL DEFAULT '',
	meta_description TEXT NOT NULL DEFAULT '',
	custom_css TEXT NOT NULL DEFAULT '',
	custom_js TEXT NOT NULL DEFAULT '',
	default_locale TEXT NOT NULL DEFAULT 'en',
	allowed_locales JSONB NOT NULL DEFAULT '["en"]',
	is_active BOOLEAN NOT NULL DEFAULT true,
	theme JSONB NOT NULL DEFAULT '{}',
	custom_domain TEXT NOT NULL DEFAULT '',
	template TEXT NOT NULL DEFAULT 'classic',
	CONSTRAINT constraint_help_centers_on_template CHECK (template IN ('docs', 'classic')),
	CONSTRAINT uniq_help_centers_slug UNIQUE (tenant_id, slug)
);
CREATE INDEX index_help_centers_on_tenant_id ON help_centers (tenant_id);

DROP TABLE IF EXISTS article_collections CASCADE;
CREATE TABLE article_collections (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	help_center_id INTEGER NOT NULL REFERENCES help_centers(id) ON DELETE CASCADE,
	slug TEXT NOT NULL,
	parent_id INTEGER NULL REFERENCES article_collections(id) ON DELETE CASCADE,
	locale TEXT NOT NULL DEFAULT 'en',
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	icon TEXT NOT NULL DEFAULT '',
	sort_order INTEGER NOT NULL DEFAULT 0,
	is_published BOOLEAN NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX index_unique_article_collections_on_help_center_slug_locale ON article_collections(help_center_id, slug, locale);
CREATE INDEX index_article_collections_on_help_center_id ON article_collections(help_center_id);
CREATE INDEX index_article_collections_on_parent_id ON article_collections(parent_id);
CREATE INDEX index_article_collections_on_tenant_id ON article_collections (tenant_id);

DROP TABLE IF EXISTS help_articles CASCADE;
CREATE TABLE help_articles (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	collection_id INTEGER NOT NULL REFERENCES article_collections(id) ON DELETE CASCADE,
	author_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
	created_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
	slug TEXT NOT NULL,
	locale TEXT NOT NULL DEFAULT 'en',
	title TEXT NOT NULL,
	content TEXT NOT NULL DEFAULT '',
	excerpt TEXT NOT NULL DEFAULT '',
	meta_title TEXT NOT NULL DEFAULT '',
	meta_description TEXT NOT NULL DEFAULT '',
	meta_image_url TEXT NOT NULL DEFAULT '',
	sort_order INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'draft',
	view_count INTEGER NOT NULL DEFAULT 0,
	ai_enabled BOOLEAN NOT NULL DEFAULT false,
	embedded_fingerprint TEXT NOT NULL DEFAULT '',
	-- left() caps the indexed body below the 1MB tsvector limit so oversized articles still save.
	search_tsv TSVECTOR GENERATED ALWAYS AS (
		setweight(to_tsvector(help_article_search_config(locale), title), 'A') ||
		setweight(to_tsvector(help_article_search_config(locale), excerpt), 'B') ||
		setweight(to_tsvector(help_article_search_config(locale), left(content, 100000)), 'C')
	) STORED,
	CONSTRAINT constraint_help_articles_on_status CHECK (status IN ('draft', 'published'))
);
CREATE UNIQUE INDEX index_unique_help_articles_on_collection_slug_locale ON help_articles(collection_id, slug, locale);
CREATE INDEX index_help_articles_on_collection_id ON help_articles(collection_id);
CREATE INDEX index_help_articles_on_author_id ON help_articles(author_id);
CREATE INDEX index_help_articles_on_title_trgm ON help_articles USING gin (title gin_trgm_ops);
CREATE INDEX index_help_articles_on_content_trgm ON help_articles USING gin (content gin_trgm_ops);
CREATE INDEX index_help_articles_on_search_tsv ON help_articles USING gin (search_tsv);
CREATE INDEX index_help_articles_on_tenant_id ON help_articles (tenant_id);

DROP TABLE IF EXISTS help_article_feedback CASCADE;
CREATE TABLE help_article_feedback (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	article_id INTEGER NOT NULL REFERENCES help_articles(id) ON DELETE CASCADE,
	is_helpful BOOLEAN NOT NULL
);
CREATE INDEX index_help_article_feedback_on_article_id ON help_article_feedback(article_id);
CREATE INDEX index_help_article_feedback_on_tenant_id ON help_article_feedback (tenant_id);

DROP TABLE IF EXISTS help_search_queries CASCADE;
CREATE TABLE help_search_queries (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	help_center_id INTEGER NOT NULL REFERENCES help_centers(id) ON DELETE CASCADE,
	query TEXT NOT NULL,
	results_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX index_help_search_queries_on_help_center_id ON help_search_queries(help_center_id);
CREATE INDEX index_help_search_queries_on_tenant_id ON help_search_queries (tenant_id);
