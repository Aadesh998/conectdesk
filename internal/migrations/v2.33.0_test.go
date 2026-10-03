package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestV2_33_0 upgrades a database without report_copilot_messages, checks the
// table comes back and takes a chat turn, that a user of another tenant is
// refused, and that the migration can run twice.
func TestV2_33_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_report_copilot")
	for _, tbl := range TenantScopedTablesV2_33_0 {
		db.MustExec(`DROP TABLE ` + tbl)
	}
	for i := range 2 {
		if err := V2_33_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	var agent int
	db.Get(&agent, `INSERT INTO users (tenant_id, type, first_name, email) VALUES (1, 'agent', 'Asha', 'a@acme.example') RETURNING id`)
	db.MustExec(`INSERT INTO report_copilot_messages (tenant_id, user_id, scope, role, content) VALUES (1, $1, 'sla', 'user', 'hi')`, agent)

	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'acme', 'Acme')`)
	if _, err := db.Exec(`INSERT INTO report_copilot_messages (tenant_id, user_id, scope, role, content) VALUES (2, $1, 'sla', 'user', 'hi')`, agent); err == nil {
		t.Fatal("a chat turn for another tenant's user was accepted")
	}
	if _, err := db.Exec(`INSERT INTO report_copilot_messages (tenant_id, user_id, scope, role, content) VALUES (1, $1, 'csat', 'user', 'hi')`, agent); err == nil {
		t.Fatal("an unknown scope was accepted")
	}
}
