package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_28_0 adds the telegram channel, a Telegram bot connected with its token.
func V2_28_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`ALTER TYPE "channels" ADD VALUE IF NOT EXISTS 'telegram'`)
	return err
}
