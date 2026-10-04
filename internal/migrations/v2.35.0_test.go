package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestV2_35_0 upgrades a database without agent_sip_accounts, checks the table
// comes back and takes an account, that a second account for the agent or a
// user of another tenant is refused, and that the migration can run twice.
func TestV2_35_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_agent_sip_accounts")
	for _, tbl := range TenantScopedTablesV2_35_0 {
		db.MustExec(`DROP TABLE ` + tbl)
	}
	for i := range 2 {
		if err := V2_35_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	var agent int
	db.Get(&agent, `INSERT INTO users (tenant_id, type, first_name, email) VALUES (1, 'agent', 'Asha', 'a@acme.example') RETURNING id`)
	ins := `INSERT INTO agent_sip_accounts (tenant_id, user_id, username, password, domain) VALUES ($1, $2, $3, 'enc:x', 'sip.exotel.example')`
	db.MustExec(ins, 1, agent, "asha")

	if _, err := db.Exec(ins, 1, agent, "asha2"); err == nil {
		t.Fatal("a second SIP account for the agent was accepted")
	}
	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'acme', 'Acme')`)
	if _, err := db.Exec(ins, 2, agent, "other"); err == nil {
		t.Fatal("a SIP account for another tenant's user was accepted")
	}
}
