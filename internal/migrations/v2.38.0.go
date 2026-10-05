package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// PlatformTablesV2_38_0 are the platform tables added by V2_38_0.
var PlatformTablesV2_38_0 = []string{"platform_docs_viewers", "platform_docs_sessions"}

// V2_38_0 adds the docs viewers, who log in to the internal documentation on
// docs.<base_domain> and can open nothing else, and their sessions.
func V2_38_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS platform_docs_viewers (
			id SERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			email TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			last_login_at TIMESTAMPTZ NULL,
			CONSTRAINT constraint_platform_docs_viewers_on_email CHECK (email = LOWER(email) AND length(email) <= 254)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_platform_docs_viewers_on_email ON platform_docs_viewers (email);

		CREATE TABLE IF NOT EXISTS platform_docs_sessions (
			id SERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			viewer_id INT NOT NULL REFERENCES platform_docs_viewers(id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL,
			ip TEXT NOT NULL DEFAULT ''
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_platform_docs_sessions_on_token_hash ON platform_docs_sessions (token_hash);
	`)
	return err
}
