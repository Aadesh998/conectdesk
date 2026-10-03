package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/colorlog"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/conectdesk/conectdesk/internal/user"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/stuffbin"
)

// Install checks if the schema is already installed, prompts for confirmation, and installs the schema if needed.
// idempotent install skips the installation if the database schema is already installed.
func install(ctx context.Context, db *sqlx.DB, fs stuffbin.FileSystem, idempotentInstall, prompt bool) error {
	schemaInstalled, err := checkSchema(db)
	if err != nil {
		log.Fatalf("error checking existing db schema: %v", err)
	}

	// Make sure the system user password is strong enough.
	password := getenv("SYSTEM_USER_PASSWORD")
	if password != "" && !user.IsStrongPassword(password) && !schemaInstalled {
		log.Fatalf("system user password is not strong, %s", user.PasswordHint)
	}

	if !idempotentInstall {
		log.Println("running first time setup...")
		colorlog.Red(fmt.Sprintf("WARNING: This will wipe your entire database - '%s'", ko.String("db.database")))
	}

	if prompt {
		log.Print("Continue (y/n)? ")
		var ok string
		fmt.Scanf("%s", &ok)
		if !strings.EqualFold(ok, "y") {
			log.Fatalf("installation cancelled")
		}
	}

	if idempotentInstall {
		if schemaInstalled {
			log.Println("skipping installation as schema is already installed")
			os.Exit(0)
		}
	} else {
		log.Println("installing database schema...")
		time.Sleep(5 * time.Second)
	}

	// Install schema.
	if err := installSchema(db, fs); err != nil {
		log.Fatalf("error installing schema: %v", err)
	}

	log.Println("database schema installed successfully")

	// Create system user.
	if err := user.CreateSystemUser(ctx, tenant.DefaultID, password, db); err != nil {
		log.Fatalf("error creating system user: %v", err)
	}

	// The schema files are the current schema, so mark every migration as applied.
	// Replaying old migrations over it would recreate columns later ones renamed.
	for _, m := range migList {
		if err := recordMigrationVersion(m.version, db); err != nil {
			log.Fatalf("error recording migration version %s: %v", m.version, err)
		}
	}
	return nil
}

// setSystemUserPass prompts for pass and sets system user password.
func setSystemUserPass(ctx context.Context, db *sqlx.DB) {
	user.ChangeSystemUserPassword(ctx, tenant.DefaultID, db)
}

// checkSchema verifies if the DB schema is already installed by querying a table.
func checkSchema(db *sqlx.DB) (bool, error) {
	if _, err := db.Exec(`SELECT * FROM settings LIMIT 1`); err != nil {
		if dbutil.IsTableNotExistError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// installSchema installs the schema files in schema/, in file name order, as
// one statement batch so a failure leaves the database untouched.
func installSchema(db *sqlx.DB, fs stuffbin.FileSystem) error {
	files, err := fs.Glob("/schema/*.sql")
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no schema files found")
	}
	sort.Strings(files)

	var q strings.Builder
	for _, f := range files {
		b, err := fs.Read(f)
		if err != nil {
			return err
		}
		q.Write(b)
		q.WriteString("\n")
	}
	_, err = db.Exec(q.String())
	return err
}
