DROP TYPE IF EXISTS "tenant_status" CASCADE; CREATE TYPE "tenant_status" AS ENUM ('active', 'suspended');

-- Tenants are the businesses (workspaces) sharing this installation. Every other table
-- carries tenant_id. Tenant 1 is the default tenant used in single-tenant mode.
DROP TABLE IF EXISTS tenants CASCADE;
CREATE TABLE tenants (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	slug TEXT NOT NULL,
	"name" TEXT NOT NULL,
	custom_domain TEXT NULL,
	status tenant_status NOT NULL DEFAULT 'active',
	"plan" TEXT NOT NULL DEFAULT 'free',
	meta JSONB NOT NULL DEFAULT '{}'::jsonb,
	next_reference_number BIGINT NOT NULL DEFAULT 100,
	-- When the tenant and all its data are deleted. Until then it is suspended
	-- and can be restored.
	deletion_scheduled_at TIMESTAMPTZ NULL,
	CONSTRAINT constraint_tenants_on_slug CHECK (slug ~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'),
	CONSTRAINT constraint_tenants_on_name CHECK (length("name") <= 140)
);
CREATE UNIQUE INDEX index_tenants_on_slug ON tenants (slug);
CREATE UNIQUE INDEX index_tenants_on_custom_domain ON tenants (LOWER(custom_domain)) WHERE custom_domain IS NOT NULL;
INSERT INTO tenants (id, slug, "name") VALUES (1, 'default', 'Default');
SELECT setval('tenants_id_seq', 1);
