// Copyright Kailash Nadh (https://github.com/knadh/listmonk)
// SPDX-License-Identifier: AGPL-3.0
// Adapted from listmonk for ConectDesk.

package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/migrations"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
	"golang.org/x/mod/semver"
)

type migFunc struct {
	version string
	fn      func(*sqlx.DB, stuffbin.FileSystem, *koanf.Koanf) error
}

var migList = []migFunc{
	{"v0.3.0", migrations.V0_3_0},
	{"v0.4.0", migrations.V0_4_0},
	{"v0.5.0", migrations.V0_5_0},
	{"v0.6.0", migrations.V0_6_0},
	{"v0.7.0", migrations.V0_7_0},
	{"v0.7.4", migrations.V0_7_4},
	{"v0.8.5", migrations.V0_8_5},
	{"v0.9.1", migrations.V0_9_1},
	{"v0.10.0", migrations.V0_10_0},
	{"v1.0.1", migrations.V1_0_1},
	{"v2.0.0", migrations.V2_0_0},
	{"v2.2.0", migrations.V2_2_0},
	{"v2.3.0", migrations.V2_3_0},
	{"v2.4.0", migrations.V2_4_0},
	{"v2.5.0", migrations.V2_5_0},
	{"v2.6.0", migrations.V2_6_0},
	{"v2.8.0", migrations.V2_8_0},
	{"v2.9.0", migrations.V2_9_0},
	{"v2.10.0", migrations.V2_10_0},
	{"v2.11.0", migrations.V2_11_0},
	{"v2.12.0", migrations.V2_12_0},
	{"v2.13.0", migrations.V2_13_0},
	{"v2.14.0", migrations.V2_14_0},
	{"v2.15.0", migrations.V2_15_0},
	{"v2.16.0", migrations.V2_16_0},
	{"v2.17.0", migrations.V2_17_0},
	{"v2.18.0", migrations.V2_18_0},
	{"v2.19.0", migrations.V2_19_0},
	{"v2.20.0", migrations.V2_20_0},
	{"v2.21.0", migrations.V2_21_0},
	{"v2.22.0", migrations.V2_22_0},
	{"v2.23.0", migrations.V2_23_0},
	{"v2.24.0", migrations.V2_24_0},
	{"v2.25.0", migrations.V2_25_0},
	{"v2.26.0", migrations.V2_26_0},
	{"v2.27.0", migrations.V2_27_0},
	{"v2.28.0", migrations.V2_28_0},
	{"v2.29.0", migrations.V2_29_0},
	{"v2.30.0", migrations.V2_30_0},
	{"v2.31.0", migrations.V2_31_0},
	{"v2.32.0", migrations.V2_32_0},
	{"v2.33.0", migrations.V2_33_0},
	{"v2.34.0", migrations.V2_34_0},
	{"v2.35.0", migrations.V2_35_0},
}

// upgrade upgrades the database to the current version by running SQL migration files
// for all version from the last known version to the current one.
func upgrade(db *sqlx.DB, fs stuffbin.FileSystem, prompt bool) {
	if prompt {
		var ok string
		fmt.Printf("** IMPORTANT: Take a backup of the database before upgrading.\n")
		fmt.Print("continue (y/n)?  ")
		if _, err := fmt.Scanf("%s", &ok); err != nil {
			log.Fatalf("error reading value from terminal: %v", err)
		}
		if !strings.EqualFold(ok, "y") {
			fmt.Println("upgrade cancelled")
			return
		}
	}

	_, toRun, err := getPendingMigrations(db)
	if err != nil {
		log.Fatalf("error checking migrations: %v", err)
	}

	// No migrations to run.
	if len(toRun) == 0 {
		log.Printf("no upgrades to run. Database is up to date.")
		return
	}

	// Execute migrations in succession.
	for _, m := range toRun {
		log.Printf("running migration %s", m.version)
		if err := m.fn(db, fs, ko); err != nil {
			log.Fatalf("error running migration %s: %v", m.version, err)
		}

		// Record the migration version in the settings table. There was no
		// settings table until v0.7.0, so ignore the no-table errors.
		if err := recordMigrationVersion(m.version, db); err != nil {
			if dbutil.IsTableNotExistError(err) {
				continue
			}
			log.Fatalf("error recording migration version %s: %v", m.version, err)
		}
	}

	log.Printf("upgrade complete")
}

// getPendingMigrations gets the pending migrations by comparing the last
// recorded migration in the DB against all migrations listed in `migrations`.
func getPendingMigrations(db *sqlx.DB) (string, []migFunc, error) {
	lastVer, err := getLastMigrationVersion(db)
	if err != nil {
		return "", nil, err
	}

	// Iterate through the migration versions and get everything above the last
	// upgraded semver.
	var toRun []migFunc
	for i, m := range migList {
		if semver.Compare(m.version, lastVer) > 0 {
			toRun = migList[i:]
			break
		}
	}

	return lastVer, toRun, nil
}

// getLastMigrationVersion returns the last migration semver recorded in the DB.
// If there isn't any, `v0.0.0` is returned.
func getLastMigrationVersion(db *sqlx.DB) (string, error) {
	var v string
	if err := db.Get(&v, `
		SELECT COALESCE(
			(SELECT value->>-1 FROM settings WHERE key='migrations'),
		'v0.0.0')`); err != nil {
		if dbutil.IsTableNotExistError(err) {
			return "v0.0.0", nil
		}
		return v, err
	}
	return v, nil
}

// recordMigrationVersion inserts the given version (of DB migration) into the
// `migrations` array in the settings table.
// It avoids ON CONFLICT and tenant_id, since the uniqueness of `key` changed
// when tenants were introduced and this runs against schemas from before and
// after. The key belongs to the default tenant through the column default.
func recordMigrationVersion(ver string, db *sqlx.DB) error {
	val := fmt.Sprintf(`["%s"]`, ver)
	res, err := db.Exec(`UPDATE settings SET value = value || $1::JSONB WHERE key = 'migrations'`, val)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES ('migrations', $1::JSONB)`, val)
	return err
}

// checkPendingUpgrade checks if the current database schema matches the expected binary version.
func checkPendingUpgrade(db *sqlx.DB) {
	lastVer, toRun, err := getPendingMigrations(db)
	if err != nil {
		log.Fatalf("error checking migrations: %v", err)
	}

	// No migrations to run.
	if len(toRun) == 0 {
		return
	}

	var vers []string
	for _, m := range toRun {
		vers = append(vers, m.version)
	}

	log.Fatalf(`there are %d pending database upgrade(s): %v. The last upgrade was %s. Backup the database and run conectdesk --upgrade`,
		len(toRun), vers, lastVer)
}
