package tenant

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/conectdesk/conectdesk/internal/tenant/models"
	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

func TestSubdomainSlug(t *testing.T) {
	cases := []struct {
		host, base, slug string
		ok               bool
	}{
		{"acme.yourapp.com", "yourapp.com", "acme", true},
		{"yourapp.com", "yourapp.com", "", false},
		{"a.b.yourapp.com", "yourapp.com", "", false},
		{"acmeyourapp.com", "yourapp.com", "", false},
		{"acme.other.com", "yourapp.com", "", false},
		{"acme.yourapp.com", "", "", false},
	}
	for _, c := range cases {
		slug, ok := SubdomainSlug(c.host, c.base)
		if slug != c.slug || ok != c.ok {
			t.Errorf("SubdomainSlug(%q, %q) = %q, %v; want %q, %v", c.host, c.base, slug, ok, c.slug, c.ok)
		}
	}
}

func TestNormalizeHost(t *testing.T) {
	for in, want := range map[string]string{
		"Acme.YourApp.com:9000": "acme.yourapp.com",
		"acme.yourapp.com.":     "acme.yourapp.com",
		" localhost ":           "localhost",
		"[::1]:9000":            "::1",
	} {
		if got := normalizeHost(in); got != want {
			t.Errorf("normalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestContext(t *testing.T) {
	if id := IDFromContext(context.Background()); id != 0 {
		t.Fatalf("empty context: got %d, want 0", id)
	}
	if id := IDFromContext(WithID(context.Background(), 7)); id != 7 {
		t.Fatalf("got %d, want 7", id)
	}
}

// fakeDNS holds the TXT records newManager's lookups answer from.
type fakeDNS map[string][]string

func (f fakeDNS) lookup(_ context.Context, name string) ([]string, error) {
	if r, ok := f[name]; ok {
		return r, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

// publish adds the verification record of the tenant's domain for host.
func (f fakeDNS) publish(t *testing.T, m *Manager, tenantID int, host string) models.Domain {
	t.Helper()
	domains, _ := m.Domains(tenantID)
	for _, d := range domains {
		if d.Hostname == host {
			name, value := VerificationRecord(d)
			f[name] = append(f[name], value)
			return d
		}
	}
	t.Fatalf("tenant %d has no domain %s", tenantID, host)
	return models.Domain{}
}

func newManager(t *testing.T, enabled bool) (*Manager, *sqlx.DB) {
	m, db, _ := newManagerDNS(t, enabled)
	return m, db
}

func newManagerDNS(t *testing.T, enabled bool) (*Manager, *sqlx.DB, fakeDNS) {
	t.Helper()
	db := testutil.NewDB(t, "tenant")
	lo := logf.New(logf.Opts{})
	dns := fakeDNS{}
	m, err := New(Opts{DB: db, Lo: &lo, Enabled: enabled, BaseDomain: "YourApp.com", LookupTXT: dns.lookup})
	if err != nil {
		t.Fatalf("tenant.New: %v", err)
	}
	return m, db, dns
}

func TestResolveSingleTenant(t *testing.T) {
	m, _ := newManager(t, false)
	for _, host := range []string{"localhost:9000", "anything.example.com", ""} {
		got, err := m.Resolve(host)
		if err != nil || got.ID != DefaultID {
			t.Fatalf("Resolve(%q) = %d, %v; want default tenant", host, got.ID, err)
		}
	}
}

func TestResolveMultiTenant(t *testing.T) {
	m, _, dns := newManagerDNS(t, true)

	acme, err := m.Create("acme", "Acme Inc", "growth")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	d, err := m.AddDomain(acme.ID, "support.acme.io")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	dns.publish(t, m, acme.ID, "support.acme.io")
	if _, err := m.VerifyDomain(context.Background(), acme.ID, d.ID); err != nil {
		t.Fatalf("VerifyDomain: %v", err)
	}
	if err := m.SetPrimaryDomain(acme.ID, d.ID); err != nil {
		t.Fatalf("SetPrimaryDomain: %v", err)
	}
	if _, err := m.Create("acme", "Dupe", "free"); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("duplicate slug: got %v, want ErrSlugTaken", err)
	}

	for host, wantID := range map[string]int{
		"acme.yourapp.com":     acme.ID,
		"ACME.yourapp.com:443": acme.ID,
		"support.acme.io":      acme.ID,
		"default.yourapp.com":  DefaultID,
		"yourapp.com":          0,
		"nobody.yourapp.com":   0,
		"x.acme.yourapp.com":   0,
		"unknown-domain.com":   0,
	} {
		got, err := m.Resolve(host)
		if wantID == 0 {
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("Resolve(%q): got tenant %d, err %v; want ErrNotFound", host, got.ID, err)
			}
			continue
		}
		if err != nil || got.ID != wantID {
			t.Errorf("Resolve(%q) = %d, %v; want %d", host, got.ID, err, wantID)
		}
	}

	for id, want := range map[int]string{acme.ID: "https://support.acme.io", DefaultID: "https://default.yourapp.com"} {
		if got, err := m.RootURL(id); err != nil || got != want {
			t.Errorf("RootURL(%d) = %q, %v; want %q", id, got, err, want)
		}
	}

	// Suspending invalidates the cache, so the next resolution sees it.
	if _, err := m.SetStatus(acme.ID, models.StatusSuspended); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	got, err := m.Resolve("acme.yourapp.com")
	if err != nil || got.Active() {
		t.Fatalf("after suspend: active=%v err=%v; want suspended", got.Active(), err)
	}
}

func TestResolveHelpCenterDomain(t *testing.T) {
	m, db, dns := newManagerDNS(t, true)
	acme, err := m.Create("acme", "Acme Inc", "free")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	db.MustExec(`INSERT INTO help_centers (tenant_id, name, slug, page_title, custom_domain, is_active)
		VALUES ($1, 'Acme Help', 'acme-help', 'Acme Help', 'https://Help.Acme.io/', true)`, acme.ID)

	// An unverified domain doesn't route.
	d, err := m.AddDomain(acme.ID, "https://Help.Acme.io/")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	if _, err := m.Resolve("help.acme.io"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unverified Resolve(help.acme.io): %v, want ErrNotFound", err)
	}
	dns.publish(t, m, acme.ID, "help.acme.io")
	if _, err := m.VerifyDomain(context.Background(), acme.ID, d.ID); err != nil {
		t.Fatalf("VerifyDomain: %v", err)
	}

	got, err := m.Resolve("help.acme.io")
	if err != nil || got.ID != acme.ID {
		t.Fatalf("Resolve(help.acme.io) = %d, %v; want %d", got.ID, err, acme.ID)
	}
}

// TestBaseDomainWithPort checks a base domain with a port, as for local
// development, matches hosts on any port and keeps the port in URLs.
func TestBaseDomainWithPort(t *testing.T) {
	db := testutil.NewDB(t, "tenant_port")
	lo := logf.New(logf.Opts{})
	m, err := New(Opts{DB: db, Lo: &lo, Enabled: true, BaseDomain: "localhost:9000", URLScheme: "http"})
	if err != nil {
		t.Fatal(err)
	}
	acme, _ := m.Create("acme", "Acme", "free")
	for _, host := range []string{"acme.localhost:9000", "acme.localhost"} {
		if got, err := m.Resolve(host); err != nil || got.ID != acme.ID {
			t.Errorf("Resolve(%q) = %d, %v", host, got.ID, err)
		}
	}
	if !m.IsBaseDomain("localhost:9000") || m.BaseDomain() != "localhost" {
		t.Errorf("base domain: %v %q", m.IsBaseDomain("localhost:9000"), m.BaseDomain())
	}
	if u, _ := m.RootURL(acme.ID); u != "http://acme.localhost:9000" {
		t.Errorf("RootURL = %q", u)
	}
	if u := m.BaseURL(); u != "http://localhost:9000" {
		t.Errorf("BaseURL = %q", u)
	}
}
