-- Context links.

DROP TABLE IF EXISTS context_links CASCADE;
CREATE TABLE context_links (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	name TEXT NOT NULL,
	url_template TEXT NOT NULL,
	signing_secret TEXT NOT NULL DEFAULT '',
	token_expiry_seconds INT NOT NULL DEFAULT 1200,
	is_active BOOLEAN DEFAULT true,
	CONSTRAINT constraint_context_links_on_name CHECK (length(name) <= 255),
	CONSTRAINT constraint_context_links_on_url_template CHECK (length(url_template) <= 2048),
	CONSTRAINT constraint_context_links_on_signing_secret CHECK (length(signing_secret) <= 500)
);
CREATE INDEX index_context_links_on_tenant_id ON context_links (tenant_id);
