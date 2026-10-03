-- OpenID Connect single sign-on providers.

DROP TABLE IF EXISTS oidc CASCADE;
CREATE TABLE oidc (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"name" TEXT NULL,
	provider_url TEXT NOT NULL,
	client_id TEXT NOT NULL,
	client_secret TEXT NOT NULL,
	enabled bool DEFAULT TRUE NOT NULL,
	provider VARCHAR NULL,
	logo_url TEXT NOT NULL DEFAULT '',
	CONSTRAINT constraint_oidc_on_name CHECK (length("name") <= 140)
);
CREATE INDEX index_oidc_on_tenant_id ON oidc (tenant_id);
