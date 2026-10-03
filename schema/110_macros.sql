-- Macros.

DROP TABLE IF EXISTS macros CASCADE;
CREATE TABLE macros (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
   id SERIAL PRIMARY KEY,
   created_at TIMESTAMPTZ DEFAULT NOW(),
   updated_at TIMESTAMPTZ DEFAULT NOW(),
   name TEXT NOT NULL,
   actions JSONB DEFAULT '{}'::jsonb NOT NULL,
   visibility macro_visibility NOT NULL,
   visible_when macro_visible_when[] NOT NULL DEFAULT ARRAY['replying', 'starting_conversation', 'adding_private_note']::macro_visible_when[],
   message_content TEXT NOT NULL,
   -- Cascade deletes when user is deleted.
   user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE,
   team_id BIGINT REFERENCES teams(id) ON DELETE CASCADE ON UPDATE CASCADE,
   usage_count INT DEFAULT 0 NOT NULL,
   CONSTRAINT name_length CHECK (length(name) <= 140),
   CONSTRAINT message_content_length CHECK (length(message_content) <= 5000)
);
CREATE INDEX index_macros_on_tenant_id ON macros (tenant_id);
