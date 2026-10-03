-- Team membership.

DROP TABLE IF EXISTS team_members CASCADE;
CREATE TABLE team_members (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	-- Cascade deletes when team or user is deleted.
	team_id BIGINT REFERENCES teams(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	emoji TEXT NULL,
	CONSTRAINT constraint_team_members_on_emoji CHECK (length(emoji) <= 1)
);
CREATE UNIQUE INDEX index_unique_team_members_on_team_id_and_user_id ON team_members (team_id, user_id);
CREATE INDEX index_team_members_on_user_id ON team_members (user_id);
CREATE INDEX index_team_members_on_tenant_id ON team_members (tenant_id);
