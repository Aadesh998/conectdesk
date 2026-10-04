package migrations

import (
	"slices"
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/lib/pq"
)

// tablesWithTenantID returns the public tables that have a tenant_id column.
func tablesWithTenantID(t *testing.T, db interface {
	Select(dest any, query string, args ...any) error
}) []string {
	t.Helper()
	var tables []string
	if err := db.Select(&tables, `
		SELECT table_name FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name = 'tenant_id'
		ORDER BY table_name`); err != nil {
		t.Fatalf("listing tenant tables: %v", err)
	}
	return tables
}

// TestSchemaTenantTables checks every table in the schema is tenant scoped and
// matches the tenant table lists, so a table added later can't skip the tenant column.
func TestSchemaTenantTables(t *testing.T) {
	db := testutil.NewDB(t, "migration_tenants_schema")

	var all []string
	if err := db.Select(&all, `
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE' AND NOT (table_name = ANY($1))
		ORDER BY table_name`, pq.Array(slices.Concat(PlatformTables, PlatformTablesV2_15_0, PlatformTablesV2_23_0))); err != nil {
		t.Fatalf("listing tables: %v", err)
	}

	want := slices.Concat(TenantScopedTables, TenantScopedTablesV2_14_0, TenantScopedTablesV2_16_0, TenantScopedTablesV2_20_0, TenantScopedTablesV2_27_0, TenantScopedTablesV2_31_0, TenantScopedTablesV2_32_0, TenantScopedTablesV2_33_0, TenantScopedTablesV2_35_0)
	slices.Sort(want)
	if !slices.Equal(all, want) {
		t.Fatalf("schema tables differ from the tenant table lists:\nschema: %v\nlist:   %v", all, want)
	}
	if got := tablesWithTenantID(t, db); !slices.Equal(got, want) {
		t.Fatalf("tables with tenant_id differ:\ngot:  %v\nwant: %v", got, want)
	}
}

// TestV2_10_0 upgrades a database without tenants and checks existing rows land
// in the default tenant, and that the migration can run twice.
func TestV2_10_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_tenants")

	// Roll the fresh schema back to its pre-tenant shape. CASCADE also drops the
	// tenant-scoped keys built on the column. Tables added later go entirely.
	for _, tbl := range slices.Concat(TenantScopedTablesV2_14_0, TenantScopedTablesV2_16_0, TenantScopedTablesV2_20_0, TenantScopedTablesV2_27_0, TenantScopedTablesV2_31_0, TenantScopedTablesV2_32_0, TenantScopedTablesV2_33_0, TenantScopedTablesV2_35_0) {
		db.MustExec(`DROP TABLE ` + tbl)
	}
	for _, tbl := range TenantScopedTables {
		db.MustExec(`ALTER TABLE ` + tbl + ` DROP COLUMN tenant_id CASCADE`)
	}
	db.MustExec(`DROP TABLE tenants CASCADE; DROP TYPE tenant_status`)
	db.MustExec(`INSERT INTO tags (name) VALUES ('vip')`)

	for i := range 2 {
		if err := V2_10_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	want := slices.Clone(TenantScopedTables)
	slices.Sort(want)
	if got := tablesWithTenantID(t, db); !slices.Equal(got, want) {
		t.Fatalf("tables with tenant_id differ:\ngot:  %v\nwant: %v", got, want)
	}

	var tenantID int
	if err := db.Get(&tenantID, `SELECT tenant_id FROM tags WHERE name = 'vip'`); err != nil || tenantID != 1 {
		t.Fatalf("existing row tenant_id = %d, %v; want 1", tenantID, err)
	}

	// The sequence must continue past the default tenant.
	var next int
	if err := db.Get(&next, `INSERT INTO tenants (slug, name) VALUES ('acme', 'Acme') RETURNING id`); err != nil || next != 2 {
		t.Fatalf("new tenant id = %d, %v; want 2", next, err)
	}

	// Deleting a tenant removes its rows.
	db.MustExec(`INSERT INTO tags (tenant_id, name) VALUES (2, 'acme-tag')`)
	db.MustExec(`DELETE FROM tenants WHERE id = 2`)
	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM tags WHERE tenant_id = 2`); err != nil || n != 0 {
		t.Fatalf("tags left after tenant delete = %d, %v; want 0", n, err)
	}
}
