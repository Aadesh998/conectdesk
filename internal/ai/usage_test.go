package ai

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestRequestLimit checks AI requests count per tenant per month and stop at
// the tenant's limit, also when requests race.
func TestRequestLimit(t *testing.T) {
	db := testutil.NewDB(t, "ai_usage")
	db.MustExec(`INSERT INTO tenants (id, slug, "name") VALUES (2, 'acme', 'Acme'), (3, 'globex', 'Globex')`)
	m := newTestManager(t)
	if err := dbutil.ScanSQLFile("queries.sql", &m.q, db, efs); err != nil {
		t.Fatal(err)
	}
	m.SetRequestLimit(func(tenantID int) int { return map[int]int{2: 3}[tenantID] })

	// 20 racing requests from acme, limited to 3.
	var ok atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if m.consumeRequest(2) == nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 3 {
		t.Fatalf("%d of acme's racing requests allowed, want 3", ok.Load())
	}
	if n, _ := m.Usage(2); n != 3 {
		t.Fatalf("acme usage = %d, want 3", n)
	}

	// Globex, unlimited, is counted but never refused, and doesn't touch acme's count.
	for range 5 {
		if err := m.consumeRequest(3); err != nil {
			t.Fatalf("unlimited tenant refused: %v", err)
		}
	}
	if n, _ := m.Usage(3); n != 5 {
		t.Fatalf("globex usage = %d, want 5", n)
	}

	// Last month's use doesn't count against this month.
	db.MustExec(`UPDATE ai_usage SET "month" = "month" - INTERVAL '1 month' WHERE tenant_id = 2`)
	if err := m.consumeRequest(2); err != nil {
		t.Fatalf("new month still refused: %v", err)
	}
	if n, _ := m.Usage(2); n != 1 {
		t.Fatalf("acme usage in the new month = %d, want 1", n)
	}
}
