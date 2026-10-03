package migrations

import (
	"slices"
	"strings"
	"testing"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/lib/pq"
)

// TestSchemaRunsTenantForeignKeys checks the schema carries the same foreign
// key block as the migration, so fresh installs and upgrades agree.
func TestSchemaRunsTenantForeignKeys(t *testing.T) {
	if !strings.Contains(testutil.Schema(t), TenantForeignKeysSQL) {
		t.Fatal("schema/ does not contain TenantForeignKeysSQL verbatim")
	}
}

// TestTenantIntegrity checks the database itself keeps tenants apart.
func TestTenantIntegrity(t *testing.T) {
	db := testutil.NewDB(t, "migration_tenant_integrity")

	// Running the migration over a current schema changes nothing.
	if err := V2_11_0(db, nil, nil); err != nil {
		t.Fatalf("V2_11_0 on current schema: %v", err)
	}

	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'acme', 'Acme'), (3, 'foo', 'Foo')`)
	db.MustExec(`INSERT INTO tags (tenant_id, name) VALUES (2, 'vip'), (3, 'vip')`)

	mustFail := func(t *testing.T, code pq.ErrorCode, query string, args ...any) {
		t.Helper()
		_, err := db.Exec(query, args...)
		pqErr, ok := err.(*pq.Error)
		if !ok || pqErr.Code != code {
			t.Fatalf("%s: got %v, want error %s", query, err, code)
		}
	}

	t.Run("inserts must name the tenant", func(t *testing.T) {
		mustFail(t, "23502", `INSERT INTO tags (name) VALUES ('orphan')`)
	})

	t.Run("names are unique per tenant", func(t *testing.T) {
		// The same name in two tenants was inserted above; a repeat within one fails.
		_, err := db.Exec(`INSERT INTO tags (tenant_id, name) VALUES (2, 'vip')`)
		if !dbutil.IsUniqueViolationError(err) {
			t.Fatalf("duplicate tag in one tenant: got %v, want unique violation", err)
		}
	})

	t.Run("rows can't reference another tenant's rows", func(t *testing.T) {
		var acmeTag, fooTag int
		db.Get(&acmeTag, `SELECT id FROM tags WHERE tenant_id = 2`)
		db.Get(&fooTag, `SELECT id FROM tags WHERE tenant_id = 3`)

		var contact, inbox, status int
		if err := db.Get(&contact, `INSERT INTO users (tenant_id, type, first_name) VALUES (2, 'contact', 'Ann') RETURNING id`); err != nil {
			t.Fatal(err)
		}
		if err := db.Get(&inbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (2, 'Mail', 'email', '{}') RETURNING id`); err != nil {
			t.Fatal(err)
		}
		if err := db.Get(&status, `INSERT INTO conversation_statuses (tenant_id, name) VALUES (2, 'Open') RETURNING id`); err != nil {
			t.Fatal(err)
		}
		var conv int
		if err := db.Get(&conv, `
			INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
			VALUES (2, $1, $2, $3, generate_reference_number(2, '')) RETURNING id`, contact, inbox, status); err != nil {
			t.Fatal(err)
		}

		db.MustExec(`INSERT INTO conversation_tags (tenant_id, conversation_id, tag_id) VALUES (2, $1, $2)`, conv, acmeTag)
		// Foo's tag on Acme's conversation, under either tenant.
		mustFail(t, "23503", `INSERT INTO conversation_tags (tenant_id, conversation_id, tag_id) VALUES (2, $1, $2)`, conv, fooTag)
		mustFail(t, "23503", `INSERT INTO conversation_tags (tenant_id, conversation_id, tag_id) VALUES (3, $1, $2)`, conv, fooTag)
		// Moving a conversation onto another tenant's inbox.
		var fooInbox int
		db.Get(&fooInbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (3, 'Mail', 'email', '{}') RETURNING id`)
		mustFail(t, "23503", `UPDATE conversations SET inbox_id = $1 WHERE id = $2`, fooInbox, conv)
	})

	t.Run("reference numbers count per tenant", func(t *testing.T) {
		var a, b, c string
		db.Get(&a, `SELECT generate_reference_number(3, '')`)
		db.Get(&b, `SELECT generate_reference_number(3, '')`)
		db.Get(&c, `SELECT generate_reference_number(1, 'T-')`)
		if a != "100" || b != "101" || c != "T-100" {
			t.Fatalf("got %s, %s, %s; want 100, 101, T-100", a, b, c)
		}
		mustFail(t, "P0001", `SELECT generate_reference_number(99, '')`)
	})

	t.Run("deleting a tenant removes all its rows", func(t *testing.T) {
		db.MustExec(`DELETE FROM tenants WHERE id = 2`)
		for _, tbl := range slices.Concat(TenantScopedTables, TenantScopedTablesV2_14_0, TenantScopedTablesV2_16_0, TenantScopedTablesV2_20_0) {
			var n int
			if err := db.Get(&n, `SELECT COUNT(*) FROM `+tbl+` WHERE tenant_id = 2`); err != nil || n != 0 {
				t.Fatalf("%s: %d rows left, %v", tbl, n, err)
			}
		}
		var n int
		db.Get(&n, `SELECT COUNT(*) FROM tags WHERE tenant_id = 3`)
		if n != 1 {
			t.Fatalf("other tenant's tags: %d, want 1", n)
		}
	})
}
