// Package testutil provides a Postgres test harness for integration tests: start one with `make test-db` or set CONECTDESK_TEST_DB_DSN, else tests skip.
package testutil

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"

	_ "github.com/lib/pq"
)

const (
	dbDSNEnv     = "CONECTDESK_TEST_DB_DSN"
	dbDefaultDSN = "postgres://conectdesk:conectdesk@127.0.0.1:5433/conectdesk?sslmode=disable&connect_timeout=3"
)

// NewDB provisions a fresh database named conectdesk_test_<name> with the schema applied, skipping the test if Postgres is unreachable.
func NewDB(t *testing.T, name string) *sqlx.DB {
	t.Helper()

	dsn := os.Getenv(dbDSNEnv)
	if dsn == "" {
		dsn = dbDefaultDSN
	}
	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Skipf("test database unreachable, set %s or start the dev postgres: %v", dbDSNEnv, err)
	}
	defer admin.Close()

	dbName := "conectdesk_test_" + name
	admin.MustExec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, dbName)
	admin.MustExec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, dbName))
	admin.MustExec(fmt.Sprintf(`CREATE DATABASE %s`, dbName))

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parsing test DSN: %v", err)
	}
	u.Path = "/" + dbName
	db, err := sqlx.Connect("postgres", u.String())
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(Schema(t)); err != nil {
		t.Fatalf("applying schema: %v", err)
	}
	return db
}

// Schema returns the schema files in schema/ joined in file name order, as the
// installer runs them.
func Schema(t *testing.T) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(repoRoot(t), "schema", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("finding schema files: %v", err)
	}
	sort.Strings(files)
	var sb strings.Builder
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		sb.Write(b)
		sb.WriteString("\n")
	}
	return sb.String()
}

// NewI18n loads the en-US translations.
func NewI18n(t *testing.T) *i18ncat.Catalog {
	t.Helper()
	mgr, err := i18n.NewFromFile(filepath.Join(repoRoot(t), "i18n", "en-US.json"))
	if err != nil {
		t.Fatalf("loading i18n: %v", err)
	}
	return i18ncat.Single(mgr)
}

// repoRoot walks up from the working directory to the go.mod directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found in any parent directory")
		}
		dir = parent
	}
}
