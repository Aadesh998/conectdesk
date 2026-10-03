// Package tenant manages tenants (the businesses sharing one installation) and
// resolves which tenant a request belongs to from its host.
package tenant

import (
	"cmp"
	"context"
	"database/sql"
	"embed"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/tenant/models"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS

	// ErrNotFound is returned when no tenant matches.
	ErrNotFound = errors.New("tenant not found")
	// ErrSlugTaken is returned when creating a tenant with a slug already in use.
	ErrSlugTaken = errors.New("tenant slug already taken")
	// ErrDefaultTenant is returned for changes the default tenant doesn't allow.
	ErrDefaultTenant = errors.New("the default tenant can't be changed this way")
)

const (
	cacheTTL         = time.Minute
	negativeCacheTTL = 10 * time.Second
	// maxCacheEntries bounds the cache, since hosts come from the Host header.
	maxCacheEntries = 10000
)

// Manager resolves and manages tenants.
type Manager struct {
	q          queries
	lo         *logf.Logger
	enabled    bool
	baseDomain string
	// urlPort is the base domain's port in URLs, e.g. ":9000", or "".
	urlPort   string
	urlScheme string
	lookupTXT func(ctx context.Context, name string) ([]string, error)

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	tenant  models.Tenant
	found   bool
	expires time.Time
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB *sqlx.DB
	Lo *logf.Logger
	// Enabled turns on multi-tenant host resolution. When off, every request
	// belongs to the default tenant.
	Enabled bool
	// BaseDomain is the domain tenant subdomains hang off, e.g. "yourapp.com"
	// makes "acme.yourapp.com" resolve to the tenant with slug "acme".
	BaseDomain string
	// URLScheme is the scheme of tenant URLs, "https" when empty.
	URLScheme string
	// DNSServer is the "host:port" of the DNS server domain verification asks,
	// the system resolver when empty.
	DNSServer string
	// LookupTXT replaces the DNS lookup of domain verification, for tests.
	LookupTXT func(ctx context.Context, name string) ([]string, error)
}

type queries struct {
	GetTenant          *sqlx.Stmt `query:"get-tenant"`
	GetTenantBySlug    *sqlx.Stmt `query:"get-tenant-by-slug"`
	GetTenantByDomain  *sqlx.Stmt `query:"get-tenant-by-domain"`
	GetAllTenants      *sqlx.Stmt `query:"get-all-tenants"`
	InsertTenant       *sqlx.Stmt `query:"insert-tenant"`
	UpdateTenantStatus *sqlx.Stmt `query:"update-tenant-status"`
	UpdateTenantPlan   *sqlx.Stmt `query:"update-tenant-plan"`

	ScheduleTenantDeletion   *sqlx.Stmt `query:"schedule-tenant-deletion"`
	CancelTenantDeletion     *sqlx.Stmt `query:"cancel-tenant-deletion"`
	GetTenantsDueForDeletion *sqlx.Stmt `query:"get-tenants-due-for-deletion"`
	GetTenantMediaNames      *sqlx.Stmt `query:"get-tenant-media-names"`
	GetTenantUsage           *sqlx.Stmt `query:"get-tenant-usage"`
	GetOnboarding            *sqlx.Stmt `query:"get-onboarding"`
	DeleteTenant             *sqlx.Stmt `query:"delete-tenant"`

	GetDomains              *sqlx.Stmt `query:"get-domains"`
	InsertDomain            *sqlx.Stmt `query:"insert-domain"`
	DomainVerifiedElsewhere *sqlx.Stmt `query:"domain-verified-elsewhere"`
	RecordDomainCheck       *sqlx.Stmt `query:"record-domain-check"`
	DeleteDomain            *sqlx.Stmt `query:"delete-domain"`
	SetPrimaryDomain        *sqlx.Stmt `query:"set-primary-domain"`
	VerifiedDomainExists    *sqlx.Stmt `query:"verified-domain-exists"`
	TrustDomain             *sqlx.Stmt `query:"trust-domain"`
	GetDomainsToRecheck     *sqlx.Stmt `query:"get-domains-to-recheck"`
	RecordDomainRecheck     *sqlx.Stmt `query:"record-domain-recheck"`
	ClearPrimaryDomainIf    *sqlx.Stmt `query:"clear-primary-domain-if"`
}

// New creates and returns a new instance of the Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	lookup := opts.LookupTXT
	if lookup == nil {
		lookup = newTXTLookup(opts.DNSServer)
	}
	// A base domain may carry a port, e.g. "localhost:9000" for local
	// development: hosts match without it, and URLs keep it.
	urlPort := ""
	if _, port, err := net.SplitHostPort(strings.TrimSpace(opts.BaseDomain)); err == nil && port != "" {
		urlPort = ":" + port
	}
	return &Manager{
		q:          q,
		lookupTXT:  lookup,
		lo:         opts.Lo,
		enabled:    opts.Enabled,
		baseDomain: normalizeHost(opts.BaseDomain),
		urlPort:    urlPort,
		urlScheme:  cmp.Or(opts.URLScheme, "https"),
		cache:      make(map[string]cacheEntry),
	}, nil
}

// IsBaseDomain reports whether host is the bare base domain, which belongs to
// no tenant, in multi-tenant mode.
func (m *Manager) IsBaseDomain(host string) bool {
	return m.enabled && m.baseDomain != "" && normalizeHost(host) == m.baseDomain
}

// Enabled reports whether multi-tenant mode is on.
func (m *Manager) Enabled() bool {
	return m.enabled
}

// Resolve returns the tenant a request for host belongs to. In single-tenant
// mode that is always the default tenant. Otherwise a subdomain of the base
// domain resolves by slug and any other host by custom domain. The bare base
// domain belongs to no tenant.
func (m *Manager) Resolve(host string) (models.Tenant, error) {
	if !m.enabled {
		return m.cached("#default", func() (models.Tenant, error) { return m.Get(DefaultID) })
	}

	host = normalizeHost(host)
	if host == "" || host == m.baseDomain {
		return models.Tenant{}, ErrNotFound
	}
	if slug, ok := SubdomainSlug(host, m.baseDomain); ok {
		return m.cached(host, func() (models.Tenant, error) { return m.getBy(m.q.GetTenantBySlug, slug) })
	}
	return m.cached(host, func() (models.Tenant, error) { return m.getBy(m.q.GetTenantByDomain, host) })
}

// RootURL returns the URL the tenant is served on: its custom domain if it has
// one, else its subdomain of the base domain. It is only meaningful in
// multi-tenant mode.
func (m *Manager) RootURL(id int) (string, error) {
	t, err := m.cached("#id:"+strconv.Itoa(id), func() (models.Tenant, error) { return m.Get(id) })
	if err != nil {
		return "", err
	}
	return m.rootURL(t), nil
}

// BaseDomain returns the domain tenant subdomains hang off.
func (m *Manager) BaseDomain() string {
	return m.baseDomain
}

// BaseURL returns the URL of the bare base domain, where signup is served.
func (m *Manager) BaseURL() string {
	return m.urlScheme + "://" + m.baseDomain + m.urlPort
}

// URLFor returns the URL the tenant is served on in multi-tenant mode.
func (m *Manager) URLFor(t models.Tenant) string {
	return m.rootURL(t)
}

func (m *Manager) rootURL(t models.Tenant) string {
	if t.CustomDomain.Valid && t.CustomDomain.String != "" {
		return m.urlScheme + "://" + normalizeHost(t.CustomDomain.String)
	}
	return m.urlScheme + "://" + t.Slug + "." + m.baseDomain + m.urlPort
}

// GetCached returns the tenant with the given ID, from the resolution cache
// when it is there.
func (m *Manager) GetCached(id int) (models.Tenant, error) {
	return m.cached("#id:"+strconv.Itoa(id), func() (models.Tenant, error) { return m.Get(id) })
}

// Get returns the tenant with the given ID.
func (m *Manager) Get(id int) (models.Tenant, error) {
	return m.getBy(m.q.GetTenant, id)
}

// GetAll returns every tenant.
func (m *Manager) GetAll() ([]models.Tenant, error) {
	var out = make([]models.Tenant, 0)
	if err := m.q.GetAllTenants.Select(&out); err != nil {
		m.lo.Error("error fetching tenants", "error", err)
		return nil, err
	}
	return out, nil
}

// ActiveIDs returns the IDs of the tenants that may be served, for background
// workers that run for every tenant.
func (m *Manager) ActiveIDs() ([]int, error) {
	all, err := m.GetAll()
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(all))
	for _, t := range all {
		if t.Active() {
			ids = append(ids, t.ID)
		}
	}
	return ids, nil
}

// GetBySlug returns the tenant with the given slug.
func (m *Manager) GetBySlug(slug string) (models.Tenant, error) {
	return m.getBy(m.q.GetTenantBySlug, strings.ToLower(strings.TrimSpace(slug)))
}

// Delete deletes a tenant and all its data. The default tenant can't be deleted.
func (m *Manager) Delete(id int) error {
	if id == DefaultID {
		return ErrDefaultTenant
	}
	res, err := m.q.DeleteTenant.Exec(id)
	if err != nil {
		m.lo.Error("error deleting tenant", "tenant_id", id, "error", err)
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	m.Invalidate()
	return nil
}

// Usage returns how many agents and inboxes a tenant has, as plan limits count them.
func (m *Manager) Usage(id int) (models.Usage, error) {
	var u models.Usage
	if err := m.q.GetTenantUsage.Get(&u, id); err != nil {
		m.lo.Error("error fetching tenant usage", "tenant_id", id, "error", err)
		return u, err
	}
	return u, nil
}

// Onboarding returns which steps of the setup checklist the tenant has done.
func (m *Manager) Onboarding(id int) (models.Onboarding, error) {
	var o models.Onboarding
	if err := m.q.GetOnboarding.Get(&o, id); err != nil {
		m.lo.Error("error fetching tenant onboarding", "tenant_id", id, "error", err)
		return o, err
	}
	return o, nil
}

// Create inserts a new tenant.
func (m *Manager) Create(slug, name, plan string) (models.Tenant, error) {
	var t models.Tenant
	if err := m.q.InsertTenant.Get(&t, slug, name, plan); err != nil {
		if dbutil.IsUniqueViolationError(err) {
			return t, ErrSlugTaken
		}
		m.lo.Error("error inserting tenant", "error", err)
		return t, err
	}
	m.Invalidate()
	return t, nil
}

// SetStatus activates or suspends a tenant.
func (m *Manager) SetStatus(id int, status string) (models.Tenant, error) {
	var t models.Tenant
	if err := m.q.UpdateTenantStatus.Get(&t, id, status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return t, ErrNotFound
		}
		m.lo.Error("error updating tenant status", "error", err)
		return t, err
	}
	m.Invalidate()
	return t, nil
}

// SetPlan changes a tenant's plan.
func (m *Manager) SetPlan(id int, plan string) (models.Tenant, error) {
	var t models.Tenant
	if err := m.q.UpdateTenantPlan.Get(&t, id, plan); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return t, ErrNotFound
		}
		m.lo.Error("error updating tenant plan", "error", err)
		return t, err
	}
	m.Invalidate()
	return t, nil
}

// ScheduleDeletion suspends a tenant and schedules it and all its data for
// deletion at at. Until then CancelDeletion restores it.
func (m *Manager) ScheduleDeletion(id int, at time.Time) (models.Tenant, error) {
	var t models.Tenant
	if err := m.q.ScheduleTenantDeletion.Get(&t, id, at); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return t, ErrDefaultTenant
		}
		m.lo.Error("error scheduling tenant deletion", "error", err)
		return t, err
	}
	m.Invalidate()
	return t, nil
}

// CancelDeletion restores a tenant scheduled for deletion, and activates it.
func (m *Manager) CancelDeletion(id int) (models.Tenant, error) {
	var t models.Tenant
	if err := m.q.CancelTenantDeletion.Get(&t, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return t, ErrNotFound
		}
		m.lo.Error("error cancelling tenant deletion", "error", err)
		return t, err
	}
	m.Invalidate()
	return t, nil
}

// DueForDeletion returns the tenants whose deletion grace period has ended.
func (m *Manager) DueForDeletion() ([]models.Tenant, error) {
	var out []models.Tenant
	if err := m.q.GetTenantsDueForDeletion.Select(&out); err != nil {
		m.lo.Error("error fetching tenants due for deletion", "error", err)
		return nil, err
	}
	return out, nil
}

// MediaNames returns the names of the tenant's stored files, which deleting
// the tenant's rows leaves in the store.
func (m *Manager) MediaNames(id int) ([]string, error) {
	var out []string
	if err := m.q.GetTenantMediaNames.Select(&out, id); err != nil {
		return nil, err
	}
	return out, nil
}

// Invalidate drops every cached resolution, so changes to tenants take effect
// on the next request.
func (m *Manager) Invalidate() {
	m.mu.Lock()
	m.cache = make(map[string]cacheEntry)
	m.mu.Unlock()
}

func (m *Manager) getBy(stmt *sqlx.Stmt, arg any) (models.Tenant, error) {
	var t models.Tenant
	if err := stmt.Get(&t, arg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return t, ErrNotFound
		}
		m.lo.Error("error fetching tenant", "error", err)
		return t, err
	}
	return t, nil
}

// cached returns the cached resolution for key, loading it on a miss. Misses
// are cached briefly too, so requests for unknown hosts don't each hit the DB.
func (m *Manager) cached(key string, load func() (models.Tenant, error)) (models.Tenant, error) {
	now := time.Now()
	m.mu.RLock()
	e, ok := m.cache[key]
	m.mu.RUnlock()
	if ok && now.Before(e.expires) {
		if !e.found {
			return models.Tenant{}, ErrNotFound
		}
		return e.tenant, nil
	}

	t, err := load()
	if err != nil && !errors.Is(err, ErrNotFound) {
		return t, err
	}

	e = cacheEntry{tenant: t, found: err == nil, expires: now.Add(cacheTTL)}
	if !e.found {
		e.expires = now.Add(negativeCacheTTL)
	}
	m.mu.Lock()
	if len(m.cache) >= maxCacheEntries {
		m.cache = make(map[string]cacheEntry)
	}
	m.cache[key] = e
	m.mu.Unlock()
	return t, err
}

// SubdomainSlug returns the tenant slug when host is a direct subdomain of
// baseDomain, e.g. "acme" for "acme.yourapp.com". Nested subdomains don't match.
func SubdomainSlug(host, baseDomain string) (string, bool) {
	if baseDomain == "" {
		return "", false
	}
	slug, ok := strings.CutSuffix(host, "."+baseDomain)
	if !ok || slug == "" || strings.Contains(slug, ".") {
		return "", false
	}
	return slug, true
}

// normalizeHost lowercases host and strips any port and trailing dot.
func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(strings.ToLower(host), ".")
}
