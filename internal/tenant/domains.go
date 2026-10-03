package tenant

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/tenant/models"
	"github.com/lib/pq"
)

const (
	// verifyRecordPrefix is prepended to a hostname to name its verification record.
	verifyRecordPrefix = "_conectdesk."
	// verifyValuePrefix is prepended to a domain's token in its verification record.
	verifyValuePrefix = "conectdesk-verification="
	// Records published before the rename still prove control, so domains
	// verified with them stay verified.
	legacyRecordPrefix = "_libredesk."
	legacyValuePrefix  = "libredesk-verification="
	dnsTimeout         = 10 * time.Second
)

var (
	// ErrInvalidDomain is returned for a hostname that can't be a custom domain.
	ErrInvalidDomain = errors.New("invalid domain")
	// ErrDomainExists is returned when the tenant has already added the hostname.
	ErrDomainExists = errors.New("domain already added")
	// ErrDomainTaken is returned when another tenant holds the hostname verified.
	ErrDomainTaken = errors.New("domain verified by another workspace")
	// ErrDomainNotFound is returned when the tenant has no such domain.
	ErrDomainNotFound = errors.New("domain not found")
	// ErrDomainNotVerified is returned when using a domain before verifying it.
	ErrDomainNotVerified = errors.New("domain not verified")
	// ErrDomainInUse is returned when removing a domain help centers are served on.
	ErrDomainInUse = errors.New("domain used by a help center")

	labelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

// VerificationRecord returns the name and value of the DNS TXT record that
// proves control of the domain.
func VerificationRecord(d models.Domain) (name, value string) {
	return verifyRecordPrefix + d.Hostname, verifyValuePrefix + d.Token
}

// NormalizeDomain returns the hostname of a custom domain, which may be given
// as a URL, or ErrInvalidDomain.
func (m *Manager) NormalizeDomain(domain string) (string, error) {
	host := strings.TrimSpace(domain)
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	host = normalizeHost(host)

	labels := strings.Split(host, ".")
	if len(host) > 253 || len(labels) < 2 || net.ParseIP(host) != nil {
		return "", ErrInvalidDomain
	}
	for _, l := range labels {
		if !labelRe.MatchString(l) {
			return "", ErrInvalidDomain
		}
	}
	// The base domain and its subdomains belong to the installation.
	if m.baseDomain != "" && (host == m.baseDomain || strings.HasSuffix(host, "."+m.baseDomain)) {
		return "", ErrInvalidDomain
	}
	return host, nil
}

// Domains returns the tenant's custom domains.
func (m *Manager) Domains(tenantID int) ([]models.Domain, error) {
	out := []models.Domain{}
	if err := m.q.GetDomains.Select(&out, tenantID, 0); err != nil {
		m.lo.Error("error fetching domains", "tenant_id", tenantID, "error", err)
		return nil, err
	}
	return out, nil
}

// Domain returns one of the tenant's custom domains.
func (m *Manager) Domain(tenantID, id int) (models.Domain, error) {
	var out []models.Domain
	if err := m.q.GetDomains.Select(&out, tenantID, id); err != nil {
		m.lo.Error("error fetching domain", "tenant_id", tenantID, "id", id, "error", err)
		return models.Domain{}, err
	}
	if len(out) == 0 {
		return models.Domain{}, ErrDomainNotFound
	}
	return out[0], nil
}

// AddDomain adds an unverified custom domain to the tenant.
func (m *Manager) AddDomain(tenantID int, domain string) (models.Domain, error) {
	host, err := m.NormalizeDomain(domain)
	if err != nil {
		return models.Domain{}, err
	}
	if taken, err := m.verifiedElsewhere(tenantID, host); err != nil {
		return models.Domain{}, err
	} else if taken {
		return models.Domain{}, ErrDomainTaken
	}

	token, err := newToken()
	if err != nil {
		return models.Domain{}, err
	}
	var id int
	if err := m.q.InsertDomain.Get(&id, tenantID, host, token); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Domain{}, ErrDomainExists
		}
		m.lo.Error("error inserting domain", "tenant_id", tenantID, "error", err)
		return models.Domain{}, err
	}
	return m.Domain(tenantID, id)
}

// VerifyDomain looks up the domain's verification record and marks it verified
// when the record carries its token. A failed check isn't an error: the domain
// comes back unverified with LastError saying why.
func (m *Manager) VerifyDomain(ctx context.Context, tenantID, id int) (models.Domain, error) {
	d, err := m.Domain(tenantID, id)
	if err != nil || d.Verified() {
		return d, err
	}
	if taken, err := m.verifiedElsewhere(tenantID, d.Hostname); err != nil {
		return d, err
	} else if taken {
		return d, ErrDomainTaken
	}

	checkErr, _ := m.checkRecord(ctx, d)
	if _, err := m.q.RecordDomainCheck.Exec(tenantID, id, checkErr); err != nil {
		// Another tenant verified the hostname between the check and the update.
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return d, ErrDomainTaken
		}
		m.lo.Error("error recording domain check", "tenant_id", tenantID, "id", id, "error", err)
		return d, err
	}
	if checkErr == "" {
		m.Invalidate()
	}
	return m.Domain(tenantID, id)
}

// checkRecord returns why the domain's verification record doesn't prove
// control of it, or "" when it does. transient is set when the lookup itself
// failed, which says nothing about the record.
func (m *Manager) checkRecord(ctx context.Context, d models.Domain) (msg string, transient bool) {
	name, value := VerificationRecord(d)
	msg, transient = m.lookupRecord(ctx, name, value)
	if msg != "" && !transient {
		if legacy, _ := m.lookupRecord(ctx, legacyRecordPrefix+d.Hostname, legacyValuePrefix+d.Token); legacy == "" {
			return "", false
		}
	}
	return msg, transient
}

// lookupRecord checks that the TXT records at name include value.
func (m *Manager) lookupRecord(ctx context.Context, name, value string) (msg string, transient bool) {
	ctx, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()

	records, err := m.lookupTXT(ctx, name)
	var dnsErr *net.DNSError
	switch {
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return fmt.Sprintf("No TXT record found at %s.", name), false
	case err != nil:
		m.lo.Warn("error looking up domain verification record", "name", name, "error", err)
		return fmt.Sprintf("Looking up %s failed, try again in a few minutes.", name), true
	case slices.Contains(records, value):
		return "", false
	case len(records) == 0:
		return fmt.Sprintf("No TXT record found at %s.", name), false
	default:
		return fmt.Sprintf("The TXT record at %s doesn't have the value %s.", name, value), false
	}
}

// RemoveDomain removes one of the tenant's domains. A domain help centers are
// served on can't be removed; the workspace address falls back to the
// subdomain when it was the domain.
func (m *Manager) RemoveDomain(tenantID, id int) error {
	d, err := m.Domain(tenantID, id)
	if err != nil {
		return err
	}
	if len(d.HelpCenters) > 0 {
		return ErrDomainInUse
	}
	if d.Primary {
		if err := m.SetPrimaryDomain(tenantID, 0); err != nil {
			return err
		}
	}
	if _, err := m.q.DeleteDomain.Exec(tenantID, id); err != nil {
		m.lo.Error("error deleting domain", "tenant_id", tenantID, "id", id, "error", err)
		return err
	}
	m.Invalidate()
	return nil
}

// SetPrimaryDomain makes a verified domain the tenant's workspace address, used
// in links and emails. Domain id 0 makes the tenant's subdomain its address again.
func (m *Manager) SetPrimaryDomain(tenantID, id int) error {
	host := sql.NullString{}
	if id != 0 {
		d, err := m.Domain(tenantID, id)
		if err != nil {
			return err
		}
		if !d.Verified() {
			return ErrDomainNotVerified
		}
		host = sql.NullString{String: d.Hostname, Valid: true}
	}
	if _, err := m.q.SetPrimaryDomain.Exec(tenantID, host); err != nil {
		m.lo.Error("error setting primary domain", "tenant_id", tenantID, "error", err)
		return err
	}
	m.Invalidate()
	return nil
}

// DomainVerified reports whether the tenant holds the hostname verified. With
// multi-tenancy off the installation's operator owns every host, so the domain
// is recorded as verified, and keeps working if multi-tenancy is turned on later.
func (m *Manager) DomainVerified(tenantID int, domain string) (bool, error) {
	host, err := m.NormalizeDomain(domain)
	if err != nil {
		return false, nil
	}
	if !m.enabled {
		token, err := newToken()
		if err != nil {
			return false, err
		}
		if _, err := m.q.TrustDomain.Exec(tenantID, host, token); err != nil {
			m.lo.Error("error recording trusted domain", "tenant_id", tenantID, "host", host, "error", err)
			return false, err
		}
		return true, nil
	}
	domains, err := m.Domains(tenantID)
	if err != nil {
		return false, err
	}
	for _, d := range domains {
		if d.Hostname == host {
			return d.Verified(), nil
		}
	}
	return false, nil
}

// ServesHost reports whether the installation serves host: the base domain, a
// subdomain of an existing tenant, or a verified custom domain. A TLS server
// issuing certificates on demand asks this before issuing one.
func (m *Manager) ServesHost(host string) (bool, error) {
	host = normalizeHost(host)
	if m.IsBaseDomain(host) {
		return true, nil
	}
	if slug, ok := SubdomainSlug(host, m.baseDomain); ok {
		_, err := m.GetBySlug(slug)
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return err == nil, err
	}
	var ok bool
	if err := m.q.VerifiedDomainExists.Get(&ok, host); err != nil {
		m.lo.Error("error checking verified domain", "host", host, "error", err)
		return false, err
	}
	return ok, nil
}

func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (m *Manager) verifiedElsewhere(tenantID int, host string) (bool, error) {
	var taken bool
	if err := m.q.DomainVerifiedElsewhere.Get(&taken, tenantID, host); err != nil {
		m.lo.Error("error checking domain", "host", host, "error", err)
		return false, err
	}
	return taken, nil
}

// newTXTLookup returns a TXT lookup against server, or the system resolver
// when server is empty.
func newTXTLookup(server string) func(ctx context.Context, name string) ([]string, error) {
	r := net.DefaultResolver
	if server != "" {
		r = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, server)
			},
		}
	}
	return r.LookupTXT
}

// RecheckFailures is how many re-checks in a row must find a verified
// domain's record gone before it is unverified.
const RecheckFailures = 3

// RecheckDomains looks up the verification record of every verified domain
// again. A domain whose record is gone RecheckFailures checks in a row is
// unverified, so it stops routing, and stops being its tenant's address. A
// lookup that fails, as opposed to finding no record, doesn't count.
func (m *Manager) RecheckDomains(ctx context.Context) (checked, unverified int) {
	var rows []struct {
		TenantID     int    `db:"tenant_id"`
		ID           int    `db:"id"`
		Hostname     string `db:"hostname"`
		Token        string `db:"token"`
		FailedChecks int    `db:"failed_checks"`
	}
	if err := m.q.GetDomainsToRecheck.Select(&rows); err != nil {
		m.lo.Error("error fetching domains to re-check", "error", err)
		return 0, 0
	}
	for _, r := range rows {
		if ctx.Err() != nil {
			break
		}
		msg, transient := m.checkRecord(ctx, models.Domain{Hostname: r.Hostname, Token: r.Token})
		if transient {
			continue
		}
		checked++
		var gone bool
		if err := m.q.RecordDomainRecheck.Get(&gone, r.TenantID, r.ID, msg, RecheckFailures); err != nil {
			m.lo.Error("error recording domain re-check", "tenant_id", r.TenantID, "hostname", r.Hostname, "error", err)
			continue
		}
		if msg != "" {
			m.lo.Warn("verified domain's record is gone", "tenant_id", r.TenantID, "hostname", r.Hostname, "failed_checks", r.FailedChecks+1)
		}
		if gone {
			m.q.ClearPrimaryDomainIf.Exec(r.TenantID, r.Hostname)
			m.lo.Warn("domain unverified: its record stayed gone", "tenant_id", r.TenantID, "hostname", r.Hostname)
			unverified++
		}
	}
	if unverified > 0 {
		m.Invalidate()
	}
	return checked, unverified
}

// RunDomainRechecks re-checks verified domains every interval until ctx is done.
func (m *Manager) RunDomainRechecks(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.RecheckDomains(ctx)
		}
	}
}
