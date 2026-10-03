package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestV2_22_0 checks domains recorded as verified without a DNS check are
// marked trusted, and DNS-verified ones aren't.
func TestV2_22_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_recheck")
	db.MustExec(`ALTER TABLE custom_domains DROP COLUMN failed_checks, DROP COLUMN trusted`)
	db.MustExec(`INSERT INTO tenants (id, slug, "name") VALUES (2, 'acme', 'Acme')`)
	db.MustExec(`INSERT INTO custom_domains (tenant_id, hostname, token, verified_at, created_at) VALUES
		(2, 'legacy.acme.io', 'a', '2026-01-01 10:00', '2026-01-01 10:00'),
		(2, 'dns.acme.io', 'b', '2026-01-02 10:05', '2026-01-02 10:00'),
		(2, 'pending.acme.io', 'c', NULL, '2026-01-03 10:00')`)
	for i := range 2 {
		if err := V2_22_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	for host, want := range map[string]bool{"legacy.acme.io": true, "dns.acme.io": false, "pending.acme.io": false} {
		var got bool
		db.Get(&got, `SELECT trusted FROM custom_domains WHERE hostname = $1`, host)
		if got != want {
			t.Errorf("%s trusted = %v, want %v", host, got, want)
		}
	}
}
