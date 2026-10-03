-- Contact notes.

DROP TABLE IF EXISTS contact_notes CASCADE;
CREATE TABLE contact_notes (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	contact_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE,
	note TEXT NOT NULL,
	user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX index_contact_notes_on_contact_id_created_at ON contact_notes (contact_id, created_at);
CREATE INDEX index_contact_notes_on_tenant_id ON contact_notes (tenant_id);
