-- Roles and their permissions.

DROP TABLE IF EXISTS roles CASCADE;
CREATE TABLE roles (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    permissions TEXT[] DEFAULT '{}'::TEXT[] NOT NULL,
    "name" TEXT NOT NULL,
    description TEXT NULL,
	CONSTRAINT constraint_roles_on_name CHECK (length("name") <= 50),
	CONSTRAINT constraint_roles_on_description CHECK (length(description) <= 300),
	CONSTRAINT uniq_roles_name UNIQUE (tenant_id, "name")
);
CREATE INDEX index_roles_on_tenant_id ON roles (tenant_id);
