package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeDomain(t *testing.T) {
	m, _ := newManager(t, true)
	for in, want := range map[string]string{
		"Support.Acme.io":             "support.acme.io",
		"https://help.acme.io/docs?x": "help.acme.io",
		"help.acme.io:8443":           "help.acme.io",
		"xn--bcher-kva.example":       "xn--bcher-kva.example",
		"localhost":                   "",
		"10.0.0.1":                    "",
		"yourapp.com":                 "", // the installation's own domains
		"acme.yourapp.com":            "",
		"a.b.yourapp.com":             "",
		"under_score.acme.io":         "",
		"-dash.acme.io":               "",
		"*.acme.io":                   "",
		"":                            "",
	} {
		got, err := m.NormalizeDomain(in)
		if want == "" {
			if !errors.Is(err, ErrInvalidDomain) {
				t.Errorf("NormalizeDomain(%q) = %q, %v; want ErrInvalidDomain", in, got, err)
			}
		} else if got != want || err != nil {
			t.Errorf("NormalizeDomain(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestDomains(t *testing.T) {
	m, db, dns := newManagerDNS(t, true)
	ctx := context.Background()
	acme, _ := m.Create("acme", "Acme", "free")
	evil, _ := m.Create("evil", "Evil", "free")

	d, err := m.AddDomain(acme.ID, "support.acme.io")
	if err != nil || d.Verified() || d.Token == "" {
		t.Fatalf("AddDomain: %+v, %v", d, err)
	}
	if _, err := m.AddDomain(acme.ID, "SUPPORT.acme.io"); !errors.Is(err, ErrDomainExists) {
		t.Fatalf("adding twice: %v, want ErrDomainExists", err)
	}

	// Without a record, and with a wrong one, the check fails and says why.
	d, err = m.VerifyDomain(ctx, acme.ID, d.ID)
	if err != nil || d.Verified() || !strings.Contains(d.LastError, "No TXT record found at _conectdesk.support.acme.io") {
		t.Fatalf("no record: verified=%v err=%v last_error=%q", d.Verified(), err, d.LastError)
	}
	dns["_conectdesk.support.acme.io"] = []string{"v=spf1 -all", "conectdesk-verification=wrong"}
	d, _ = m.VerifyDomain(ctx, acme.ID, d.ID)
	if d.Verified() || !strings.Contains(d.LastError, "doesn't have the value") {
		t.Fatalf("wrong record: verified=%v last_error=%q", d.Verified(), d.LastError)
	}
	if err := m.SetPrimaryDomain(acme.ID, d.ID); !errors.Is(err, ErrDomainNotVerified) {
		t.Fatalf("primary before verifying: %v, want ErrDomainNotVerified", err)
	}

	// Another workspace may add the hostname, but only the one whose token is
	// published verifies it; afterwards nobody else can claim it.
	e, err := m.AddDomain(evil.ID, "support.acme.io")
	if err != nil {
		t.Fatalf("evil AddDomain before acme verifies: %v", err)
	}
	dns.publish(t, m, acme.ID, "support.acme.io")
	if e, _ = m.VerifyDomain(ctx, evil.ID, e.ID); e.Verified() {
		t.Fatal("evil verified with acme's record")
	}
	if d, err = m.VerifyDomain(ctx, acme.ID, d.ID); err != nil || !d.Verified() || d.LastError != "" {
		t.Fatalf("acme verify: verified=%v err=%v last_error=%q", d.Verified(), err, d.LastError)
	}
	if _, err := m.VerifyDomain(ctx, evil.ID, e.ID); !errors.Is(err, ErrDomainTaken) {
		t.Fatalf("evil verify after acme: %v, want ErrDomainTaken", err)
	}
	if _, err := m.AddDomain(evil.ID, "support.acme.io"); !errors.Is(err, ErrDomainTaken) {
		t.Fatalf("evil add after acme verified: %v, want ErrDomainTaken", err)
	}
	if _, err := m.Domain(evil.ID, d.ID); !errors.Is(err, ErrDomainNotFound) {
		t.Fatalf("evil reads acme's domain: %v, want ErrDomainNotFound", err)
	}
	if err := m.SetPrimaryDomain(evil.ID, d.ID); !errors.Is(err, ErrDomainNotFound) {
		t.Fatalf("evil sets acme's domain as primary: %v, want ErrDomainNotFound", err)
	}

	// A verified domain routes once it is used, as the workspace address here.
	if _, err := m.Resolve("support.acme.io"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("verified but unused domain resolves: %v", err)
	}
	if err := m.SetPrimaryDomain(acme.ID, d.ID); err != nil {
		t.Fatalf("SetPrimaryDomain: %v", err)
	}
	if got, err := m.Resolve("support.acme.io"); err != nil || got.ID != acme.ID {
		t.Fatalf("Resolve after primary: %d, %v", got.ID, err)
	}
	if u, _ := m.RootURL(acme.ID); u != "https://support.acme.io" {
		t.Fatalf("RootURL = %q", u)
	}

	for host, want := range map[string]bool{
		"yourapp.com": true, "acme.yourapp.com": true, "support.acme.io": true,
		"nobody.yourapp.com": false, "other.io": false,
	} {
		if got, err := m.ServesHost(host); got != want || err != nil {
			t.Errorf("ServesHost(%q) = %v, %v; want %v", host, got, err, want)
		}
	}

	// A domain a help center is served on can't be removed.
	db.MustExec(`INSERT INTO help_centers (tenant_id, name, slug, page_title, custom_domain, is_active)
		VALUES ($1, 'Acme Help', 'acme-help', 'Acme Help', 'https://support.acme.io', true)`, acme.ID)
	if err := m.RemoveDomain(acme.ID, d.ID); !errors.Is(err, ErrDomainInUse) {
		t.Fatalf("removing a help center's domain: %v, want ErrDomainInUse", err)
	}
	db.MustExec(`DELETE FROM help_centers WHERE tenant_id = $1`, acme.ID)

	// Removing the primary domain puts the workspace back on its subdomain.
	if err := m.RemoveDomain(acme.ID, d.ID); err != nil {
		t.Fatalf("RemoveDomain: %v", err)
	}
	if u, _ := m.RootURL(acme.ID); u != "https://acme.yourapp.com" {
		t.Fatalf("RootURL after removal = %q", u)
	}
	if _, err := m.Resolve("support.acme.io"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed domain still resolves: %v", err)
	}

	// Once acme lets it go, evil can verify it with its own record.
	dns.publish(t, m, evil.ID, "support.acme.io")
	if e, err = m.VerifyDomain(ctx, evil.ID, e.ID); err != nil || !e.Verified() {
		t.Fatalf("evil verify after release: verified=%v err=%v", e.Verified(), err)
	}
}

// TestDomainsEmpty checks a tenant without domains gets an empty list, not
// null, which the platform dashboard and the Domains page iterate over.
func TestDomainsEmpty(t *testing.T) {
	m, _ := newManager(t, true)
	acme, _ := m.Create("acme", "Acme", "free")
	domains, err := m.Domains(acme.ID)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(domains); string(b) != "[]" {
		t.Fatalf("Domains of a tenant without any = %s, want []", b)
	}
}

// TestRecheckDomains checks a verified domain whose record goes away is
// unverified after RecheckFailures checks in a row, and stops being its
// tenant's address, while failed lookups and trusted domains don't count.
func TestRecheckDomains(t *testing.T) {
	m, db, dns := newManagerDNS(t, true)
	ctx := context.Background()
	acme, _ := m.Create("acme", "Acme", "free")
	d, _ := m.AddDomain(acme.ID, "support.acme.io")
	dns.publish(t, m, acme.ID, "support.acme.io")
	if _, err := m.VerifyDomain(ctx, acme.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	m.SetPrimaryDomain(acme.ID, d.ID)
	db.MustExec(`INSERT INTO custom_domains (tenant_id, hostname, token, verified_at, trusted) VALUES ($1, 'legacy.acme.io', 't', NOW(), true)`, acme.ID)

	if checked, gone := m.RecheckDomains(ctx); checked != 1 || gone != 0 {
		t.Fatalf("record present: checked %d, unverified %d", checked, gone)
	}

	// The record goes; a failing DNS server in between doesn't count.
	name, _ := VerificationRecord(d)
	delete(dns, name)
	for i := 1; i < RecheckFailures; i++ {
		m.RecheckDomains(ctx)
	}
	failing := m.lookupTXT
	m.lookupTXT = func(context.Context, string) ([]string, error) { return nil, errors.New("server misbehaving") }
	m.RecheckDomains(ctx)
	m.lookupTXT = failing
	if got, _ := m.Domain(acme.ID, d.ID); !got.Verified() || got.FailedChecks != RecheckFailures-1 {
		t.Fatalf("before the last failure: verified=%v failed=%d", got.Verified(), got.FailedChecks)
	}
	if got, err := m.Resolve("support.acme.io"); err != nil || got.ID != acme.ID {
		t.Fatal("stopped routing before enough failures")
	}

	// It comes back once: the count starts over.
	dns.publish(t, m, acme.ID, "support.acme.io")
	m.RecheckDomains(ctx)
	if got, _ := m.Domain(acme.ID, d.ID); got.FailedChecks != 0 || got.LastError != "" {
		t.Fatalf("after the record came back: %+v", got)
	}

	delete(dns, name)
	for range RecheckFailures {
		m.RecheckDomains(ctx)
	}
	got, _ := m.Domain(acme.ID, d.ID)
	if got.Verified() || !strings.Contains(got.LastError, "No TXT record") {
		t.Fatalf("after %d failures: verified=%v last_error=%q", RecheckFailures, got.Verified(), got.LastError)
	}
	if _, err := m.Resolve("support.acme.io"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an unverified domain still routes: %v", err)
	}
	if u, _ := m.RootURL(acme.ID); u != "https://acme.yourapp.com" {
		t.Fatalf("RootURL = %q, want the subdomain", u)
	}
	var trustedVerified bool
	db.Get(&trustedVerified, `SELECT verified_at IS NOT NULL FROM custom_domains WHERE hostname = 'legacy.acme.io'`)
	if !trustedVerified {
		t.Fatal("a trusted domain was re-checked")
	}
}

// A record published before the rename to ConectDesk still verifies.
func TestDomainLegacyRecord(t *testing.T) {
	m, _, dns := newManagerDNS(t, true)
	acme, _ := m.Create("acme", "Acme", "free")
	d, err := m.AddDomain(acme.ID, "help.acme.io")
	if err != nil {
		t.Fatal(err)
	}
	dns["_libredesk.help.acme.io"] = []string{"libredesk-verification=" + d.Token}
	if d, err = m.VerifyDomain(context.Background(), acme.ID, d.ID); err != nil || !d.Verified() {
		t.Fatalf("legacy record: verified=%v err=%v last_error=%q", d.Verified(), err, d.LastError)
	}
}
