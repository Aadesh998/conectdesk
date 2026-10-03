package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_13_0 adds pending_signups, which holds signups until their email is verified.
func V2_13_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pending_signups (
			id SERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			token_hash TEXT NOT NULL,
			slug TEXT NOT NULL,
			company TEXT NOT NULL,
			first_name TEXT NOT NULL,
			last_name TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			CONSTRAINT constraint_pending_signups_on_company CHECK (length(company) <= 140),
			CONSTRAINT constraint_pending_signups_on_email CHECK (length(email) <= 254)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_pending_signups_on_token_hash ON pending_signups (token_hash);
		CREATE UNIQUE INDEX IF NOT EXISTS index_pending_signups_on_slug ON pending_signups (slug);
	`)
	return err
}
