-- SLA policies.

DROP TABLE IF EXISTS sla_policies CASCADE;
CREATE TABLE sla_policies (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	name TEXT NOT NULL,
	description TEXT NULL,
	first_response_time TEXT NOT NULL,
	resolution_time TEXT NOT NULL,
	next_response_time TEXT NULL,
	notifications JSONB DEFAULT '[]'::jsonb NOT NULL,
	CONSTRAINT constraint_sla_policies_on_name CHECK (length(name) <= 140),
	CONSTRAINT constraint_sla_policies_on_description CHECK (length(description) <= 300)
);
CREATE INDEX index_sla_policies_on_tenant_id ON sla_policies (tenant_id);
