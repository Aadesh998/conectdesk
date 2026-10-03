-- Activity logs.

DROP TABLE IF EXISTS activity_logs CASCADE;
CREATE TABLE activity_logs (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	activity_type activity_log_type NOT NULL,
	activity_description TEXT NOT NULL,
	actor_id INT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	target_model_type TEXT NOT NULL,
	target_model_id BIGINT NOT NULL,
	ip INET
);
CREATE INDEX IF NOT EXISTS index_activity_logs_on_actor_id ON activity_logs (actor_id);
CREATE INDEX IF NOT EXISTS index_activity_logs_on_activity_type ON activity_logs (activity_type);
CREATE INDEX IF NOT EXISTS index_activity_logs_on_created_at ON activity_logs (created_at);
CREATE INDEX index_activity_logs_on_tenant_id ON activity_logs (tenant_id);
