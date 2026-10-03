-- Tags.

DROP TABLE IF EXISTS tags CASCADE;
CREATE TABLE tags (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"name" TEXT NOT NULL,
	CONSTRAINT constraint_tags_on_name CHECK (length("name") <= 140),
	CONSTRAINT uniq_tags_name UNIQUE (tenant_id, "name")
);
CREATE INDEX index_tags_on_tenant_id ON tags (tenant_id);
