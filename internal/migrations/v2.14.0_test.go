package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestV2_14_0 checks custom domains in use before verification existed are
// recorded as verified, the first holder of a hostname keeping it, and that the
// migration can run twice.
func TestV2_14_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_domains")
	db.MustExec(`DROP TABLE custom_domains`)
	db.MustExec(`INSERT INTO tenants (id, slug, "name", custom_domain) VALUES (2, 'acme', 'Acme', 'Support.Acme.io')`)
	db.MustExec(`INSERT INTO help_centers (tenant_id, name, slug, page_title, custom_domain) VALUES
		(1, 'Docs', 'docs', 'Docs', 'https://docs.example.com'),
		(2, 'Acme Help', 'acme-help', 'Help', 'https://help.acme.io/'),
		(1, 'Clash', 'clash', 'Clash', 'https://support.acme.io')`)

	for i := range 2 {
		if err := V2_14_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	var got []struct {
		TenantID int    `db:"tenant_id"`
		Hostname string `db:"hostname"`
		Verified bool   `db:"verified"`
	}
	if err := db.Select(&got, `SELECT tenant_id, hostname, verified_at IS NOT NULL AS verified FROM custom_domains ORDER BY hostname`); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		tenant int
		host   string
	}{{1, "docs.example.com"}, {2, "help.acme.io"}, {2, "support.acme.io"}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %v", got, want)
	}
	for i, w := range want {
		if got[i].TenantID != w.tenant || got[i].Hostname != w.host || !got[i].Verified {
			t.Errorf("row %d = %+v, want tenant %d %s verified", i, got[i], w.tenant, w.host)
		}
	}
}
