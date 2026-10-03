-- Custom attribute definitions.

DROP TABLE IF EXISTS custom_attribute_definitions CASCADE;
CREATE TABLE custom_attribute_definitions (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"name" TEXT NOT NULL,
	description TEXT NOT NULL,
	applies_to TEXT NOT NULL,
	key TEXT NOT NULL,
	values TEXT[] DEFAULT '{}'::TEXT[] NOT NULL,
	data_type TEXT NOT NULL,
	regex TEXT NULL,
	regex_hint TEXT NULL,
	CONSTRAINT constraint_custom_attribute_definitions_on_name CHECK (length("name") <= 140),
	CONSTRAINT constraint_custom_attribute_definitions_on_description CHECK (length(description) <= 300),
	CONSTRAINT constraint_custom_attribute_definitions_on_key CHECK (length(key) <= 140),
	CONSTRAINT constraint_custom_attribute_definitions_on_applies_to CHECK (length(applies_to) <= 50),
	CONSTRAINT constraint_custom_attribute_definitions_on_data_type CHECK (length(data_type) <= 100),
	CONSTRAINT constraint_custom_attribute_definitions_on_regex CHECK (length(regex) <= 1000),
	CONSTRAINT constraint_custom_attribute_definitions_on_regex_hint CHECK (length(regex_hint) <= 1000),
	CONSTRAINT constraint_custom_attribute_definitions_key_applies_to_unique UNIQUE (tenant_id, key, applies_to)
);
CREATE INDEX index_custom_attribute_definitions_on_tenant_id ON custom_attribute_definitions (tenant_id);
