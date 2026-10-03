-- Settings.

DROP TABLE IF EXISTS settings CASCADE;
CREATE TABLE settings (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"key" TEXT NOT NULL,
	value jsonb DEFAULT '{}'::jsonb NOT NULL,
	CONSTRAINT uniq_settings_key UNIQUE (tenant_id, "key")
);
CREATE INDEX index_settings_on_key ON settings USING btree ("key");
CREATE INDEX index_settings_on_tenant_id ON settings (tenant_id);
