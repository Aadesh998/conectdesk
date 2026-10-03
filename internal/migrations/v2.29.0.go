package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_29_0 adds the voice channel, phone calls through Exotel.
func V2_29_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`ALTER TYPE "channels" ADD VALUE IF NOT EXISTS 'voice'`)
	return err
}
