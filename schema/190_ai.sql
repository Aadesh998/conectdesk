-- AI: providers, prompts, knowledge base, embeddings.

DROP TABLE IF EXISTS ai_providers CASCADE;
CREATE TABLE ai_providers (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	name TEXT NOT NULL,
	provider ai_provider NOT NULL,
	type TEXT NOT NULL DEFAULT 'completion',
	config JSONB NOT NULL DEFAULT '{}',
	is_default BOOLEAN NOT NULL DEFAULT FALSE,
	CONSTRAINT constraint_ai_providers_on_name CHECK (length(name) <= 140),
	CONSTRAINT constraint_ai_providers_on_type CHECK (type IN ('completion', 'embedding')),
	CONSTRAINT uniq_ai_providers_name UNIQUE (tenant_id, "name")
);
CREATE UNIQUE INDEX index_unique_ai_providers_on_is_default_when_is_default_is_true ON ai_providers USING btree (tenant_id, is_default)
WHERE (is_default = true);
CREATE UNIQUE INDEX index_unique_ai_providers_on_type ON ai_providers(tenant_id, type);
CREATE INDEX index_ai_providers_on_tenant_id ON ai_providers (tenant_id);

DROP TABLE IF EXISTS ai_prompts CASCADE;
CREATE TABLE ai_prompts (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	title TEXT NOT NULL,
    key TEXT NOT NULL,
    content TEXT NOT NULL,
	CONSTRAINT constraint_prompts_on_title CHECK (length(title) <= 140),
    CONSTRAINT constraint_prompts_on_key CHECK (length(key) <= 140),
	CONSTRAINT uniq_ai_prompts_key UNIQUE (tenant_id, "key")
);
CREATE INDEX index_ai_prompts_on_key ON ai_prompts USING btree (key);
CREATE INDEX index_ai_prompts_on_tenant_id ON ai_prompts (tenant_id);

DROP TABLE IF EXISTS ai_knowledge_base CASCADE;
CREATE TABLE ai_knowledge_base (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	type ai_knowledge_type NOT NULL DEFAULT 'snippet',
	title TEXT NOT NULL DEFAULT '',
	content TEXT NOT NULL,
	enabled BOOLEAN NOT NULL DEFAULT true,
	source TEXT NOT NULL DEFAULT 'manual',
	source_url TEXT NOT NULL DEFAULT '',
	embedded_fingerprint TEXT NOT NULL DEFAULT ''
);
CREATE INDEX index_ai_knowledge_base_on_type_enabled ON ai_knowledge_base(type, enabled);
CREATE INDEX index_ai_knowledge_base_on_tenant_id ON ai_knowledge_base (tenant_id);

DROP TABLE IF EXISTS embeddings CASCADE;
CREATE TABLE embeddings (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	source_type TEXT NOT NULL,
	source_id BIGINT NOT NULL,
	chunk_text TEXT NOT NULL,
	-- Untyped so each tenant can pick its own embedding model; dimensions records the length,
	-- and search only compares vectors of the query's length.
	embedding vector NOT NULL,
	dimensions INTEGER NOT NULL DEFAULT 0,
	meta JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX index_embeddings_on_source_type_source_id ON embeddings(source_type, source_id);
CREATE INDEX index_embeddings_on_tenant_id ON embeddings (tenant_id);
-- Search scans one tenant's vectors of one length; this narrows it to exactly those rows.
CREATE INDEX index_embeddings_on_tenant_source_type_dimensions ON embeddings (tenant_id, source_type, dimensions);
