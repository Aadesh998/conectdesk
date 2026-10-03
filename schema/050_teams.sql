-- Teams.

DROP TABLE IF EXISTS teams CASCADE;
CREATE TABLE teams (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"name" TEXT NOT NULL,
	emoji TEXT NULL,
	conversation_assignment_type conversation_assignment_type NOT NULL,
	max_auto_assigned_conversations INT DEFAULT 0 NOT NULL,

	-- Set to NULL when business hours or SLA policy is deleted.
	business_hours_id INT REFERENCES business_hours(id) ON DELETE SET NULL ON UPDATE CASCADE NULL,
	sla_policy_id INT REFERENCES sla_policies(id) ON DELETE SET NULL ON UPDATE CASCADE NULL,

	timezone TEXT NULL,
	CONSTRAINT constraint_teams_on_emoji CHECK (length(emoji) <= 50),
	CONSTRAINT constraint_teams_on_name CHECK (length("name") <= 140),
	CONSTRAINT constraint_teams_on_timezone CHECK (length(timezone) <= 140),
	CONSTRAINT constraint_teams_on_name_unique UNIQUE (tenant_id, "name")
);
CREATE INDEX index_teams_on_tenant_id ON teams (tenant_id);
