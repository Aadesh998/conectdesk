-- Automation rules.

DROP TABLE IF EXISTS automation_rules CASCADE;
CREATE TABLE automation_rules (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    "name" TEXT NOT NULL,
    description TEXT NULL,
    "type" VARCHAR NOT NULL,
    rules JSONB NULL,
    events TEXT[] DEFAULT '{}'::TEXT[] NOT NULL,
    enabled BOOL DEFAULT TRUE NOT NULL,
	weight INT DEFAULT 0 NOT NULL,
	execution_mode automation_execution_mode DEFAULT 'all' NOT NULL,
    CONSTRAINT constraint_automation_rules_on_name CHECK (length("name") <= 140),
    CONSTRAINT constraint_automation_rules_on_description CHECK (length(description) <= 300)
);
CREATE INDEX index_automation_rules_on_enabled_and_weight ON automation_rules(enabled, weight);
CREATE INDEX index_automation_rules_on_type_and_weight ON automation_rules(type, weight);
CREATE INDEX index_automation_rules_on_tenant_id ON automation_rules (tenant_id);
