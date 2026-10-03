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

-- Signups waiting for their email to be verified. A workspace is only created
-- once its owner opens the emailed link, and its slug is held until then.
DROP TABLE IF EXISTS pending_signups CASCADE;
CREATE TABLE pending_signups (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	-- SHA-256 of the emailed token; the token itself is never stored.
	token_hash TEXT NOT NULL,
	slug TEXT NOT NULL,
	company TEXT NOT NULL,
	first_name TEXT NOT NULL,
	last_name TEXT NOT NULL DEFAULT '',
	email TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	CONSTRAINT constraint_pending_signups_on_company CHECK (length(company) <= 140),
	CONSTRAINT constraint_pending_signups_on_email CHECK (length(email) <= 254)
);
CREATE UNIQUE INDEX index_pending_signups_on_token_hash ON pending_signups (token_hash);
CREATE UNIQUE INDEX index_pending_signups_on_slug ON pending_signups (slug);
