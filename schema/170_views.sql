-- Saved conversation views.

DROP TABLE IF EXISTS views CASCADE;
CREATE TABLE views (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name TEXT NOT NULL,
    filters JSONB NOT NULL,
    visibility view_visibility NOT NULL DEFAULT 'user',
    -- Delete user views when user / team is deleted.
    user_id BIGINT REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE,
    team_id BIGINT REFERENCES teams(id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT constraint_views_on_name CHECK (length(name) <= 140),
    CONSTRAINT constraint_views_visibility_user CHECK (visibility != 'user' OR user_id IS NOT NULL),
    CONSTRAINT constraint_views_visibility_team CHECK (visibility != 'team' OR team_id IS NOT NULL)
);
CREATE INDEX index_views_on_user_id ON views(user_id);
CREATE INDEX index_views_on_visibility ON views(visibility);
CREATE INDEX index_views_on_team_id ON views(team_id);
CREATE INDEX index_views_on_tenant_id ON views (tenant_id);
