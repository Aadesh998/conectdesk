package importer

import (
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/i18ncat"

	"github.com/knadh/go-i18n"
	"github.com/zerodha/logf"
)

// TestJobsPerTenant checks one tenant's import neither blocks another's nor
// shows in its status.
func TestJobsPerTenant(t *testing.T) {
	tr, _ := i18n.New([]byte(`{"_.code":"en","_.name":"English"}`))
	lo := logf.New(logf.Opts{})
	imp := New(Opts{Lo: &lo, I18n: i18ncat.Single(tr)})
	defer imp.Close()

	release := make(chan struct{})
	job := func(tenantID int, log string) func() error {
		return func() error {
			imp.AddLog(tenantID, "agents", log)
			<-release
			return nil
		}
	}
	if err := imp.Submit(1, "agents", job(1, "acme: alice@acme.io")); err != nil {
		t.Fatal(err)
	}
	if err := imp.Submit(1, "agents", job(1, "again")); err == nil {
		t.Fatal("a tenant ran two imports of one namespace at once")
	}
	if err := imp.Submit(2, "agents", job(2, "globex: gina@globex.io")); err != nil {
		t.Fatalf("another tenant's import was blocked: %v", err)
	}
	if _, err := imp.GetStatus(3, "agents"); err == nil {
		t.Fatal("a tenant without an import sees one")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		a, _ := imp.GetStatus(1, "agents")
		g, _ := imp.GetStatus(2, "agents")
		imp.mu.RLock()
		done := len(a.Logs) == 1 && len(g.Logs) == 1
		ok := done && a.Logs[0] == "acme: alice@acme.io" && g.Logs[0] == "globex: gina@globex.io"
		imp.mu.RUnlock()
		if ok {
			break
		}
		if done || time.Now().After(deadline) {
			t.Fatalf("logs mixed between tenants: %v / %v", a.Logs, g.Logs)
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
}
