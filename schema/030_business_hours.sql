-- Business hours.

DROP TABLE IF EXISTS business_hours CASCADE;
CREATE TABLE business_hours (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
	name TEXT NOT NULL,
	description TEXT NULL,
	is_always_open BOOL DEFAULT false NOT NULL,
	hours JSONB NOT NULL,
	holidays JSONB DEFAULT '{}'::jsonb NOT NULL,
	CONSTRAINT constraint_business_hours_on_name CHECK (length(name) <= 140),
	CONSTRAINT constraint_business_hours_on_description CHECK (length(description) <= 300)
);
CREATE INDEX index_business_hours_on_tenant_id ON business_hours (tenant_id);
