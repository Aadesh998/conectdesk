-- Exports of all of a tenant's data, as a ZIP an admin downloads. The file is
-- a private media row (model_type 'data_exports'), removed when it expires.
DROP TABLE IF EXISTS data_exports CASCADE;
CREATE TABLE data_exports (
	tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	"uuid" uuid DEFAULT gen_random_uuid() NOT NULL UNIQUE,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	finished_at TIMESTAMPTZ NULL,
	expires_at TIMESTAMPTZ NULL,
	status TEXT NOT NULL DEFAULT 'running',
	-- The user who asked; not a reference, so the record outlives them.
	requested_by INT NULL,
	requested_by_email TEXT NOT NULL DEFAULT '',
	media_uuid uuid NULL,
	"size" BIGINT NOT NULL DEFAULT 0,
	manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
	error TEXT NOT NULL DEFAULT '',
	CONSTRAINT constraint_data_exports_on_status CHECK (status IN ('running', 'ready', 'failed', 'expired')),
	CONSTRAINT uniq_data_exports_tenant_id_id UNIQUE (tenant_id, id)
);
-- One export runs at a time per tenant.
CREATE UNIQUE INDEX index_data_exports_running ON data_exports (tenant_id) WHERE status = 'running';
