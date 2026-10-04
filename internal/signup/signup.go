// Package signup creates a new tenant: its row, its default data and its
// first admin, all in one transaction.
package signup

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	rmodels "github.com/conectdesk/conectdesk/internal/role/models"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/conectdesk/conectdesk/internal/tenant"
	tmodels "github.com/conectdesk/conectdesk/internal/tenant/models"
	"github.com/conectdesk/conectdesk/internal/user"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/zerodha/logf"
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrSlugTaken is returned when the workspace slug is in use or reserved.
	ErrSlugTaken = errors.New("workspace slug is taken")

	slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,61}[a-z0-9])$`)

	// reservedSlugs can't be workspaces: they are, or may become, the platform's own hosts.
	reservedSlugs = map[string]bool{
		"www": true, "app": true, "api": true, "admin": true, "mail": true, "smtp": true,
		"imap": true, "ftp": true, "static": true, "cdn": true, "assets": true, "status": true,
		"help": true, "support": true, "docs": true, "blog": true, "billing": true, "signup": true,
		"login": true, "auth": true, "dashboard": true, "default": true, "system": true, "root": true,
	}
)

// ValidationError describes which field of a request is invalid.
type ValidationError struct {
	Field string
}

func (e ValidationError) Error() string { return "invalid " + e.Field }

// Request is a new workspace and its first admin.
type Request struct {
	Company   string `json:"company"`
	Slug      string `json:"slug"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

// Manager creates tenants. Only platform admins, from the platform dashboard
// or the command line, create them: there is no public signup.
type Manager struct {
	db      *sqlx.DB
	lo      *logf.Logger
	tenants *tenant.Manager
	plan    string
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB      *sqlx.DB
	Lo      *logf.Logger
	Tenants *tenant.Manager
	// Plan is the plan new tenants start on.
	Plan string
}

// New returns a signup Manager.
func New(opts Opts) *Manager {
	return &Manager{db: opts.DB, lo: opts.Lo, tenants: opts.Tenants, plan: opts.Plan}
}

// Normalize trims the request and lowercases the slug and email.
func (r *Request) Normalize() {
	r.Company = strings.TrimSpace(r.Company)
	r.Slug = strings.ToLower(strings.TrimSpace(r.Slug))
	r.FirstName = strings.TrimSpace(r.FirstName)
	r.LastName = strings.TrimSpace(r.LastName)
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
}

// Validate checks the request, returning a ValidationError naming the bad field.
func (r Request) Validate() error {
	switch {
	case r.Company == "" || len(r.Company) > 140:
		return ValidationError{"company"}
	case !slugRe.MatchString(r.Slug) || strings.Contains(r.Slug, "--"):
		return ValidationError{"slug"}
	case r.FirstName == "" || len(r.FirstName) > 140 || len(r.LastName) > 140:
		return ValidationError{"name"}
	case !stringutil.ValidEmail(r.Email) || len(r.Email) > 254:
		return ValidationError{"email"}
	case !user.IsStrongPassword(r.Password):
		return ValidationError{"password"}
	}
	return nil
}

// Create validates the request and creates the tenant. Nothing is kept if
// any step fails.
func (m *Manager) Create(ctx context.Context, r Request) (tmodels.Tenant, error) {
	return m.CreateWith(ctx, r, CreateOpts{})
}

// CreateOpts are what a platform admin sets on a workspace they create.
type CreateOpts struct {
	// Plan is the workspace's plan; empty is the default plan.
	Plan string
	// Channels are the inbox channels the workspace may use; nil allows all.
	Channels []string
}

// CreateWith is Create with a plan and allowed channels, for workspaces a
// platform admin creates.
func (m *Manager) CreateWith(ctx context.Context, r Request, opts CreateOpts) (tmodels.Tenant, error) {
	r.Normalize()
	if err := r.Validate(); err != nil {
		return tmodels.Tenant{}, err
	}
	if reservedSlugs[r.Slug] {
		return tmodels.Tenant{}, ErrSlugTaken
	}
	adminHash, err := bcrypt.GenerateFromPassword([]byte(r.Password), bcrypt.DefaultCost)
	if err != nil {
		return tmodels.Tenant{}, fmt.Errorf("hashing password: %w", err)
	}

	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return tmodels.Tenant{}, err
	}
	defer tx.Rollback()

	t, err := m.createTenant(tx, r, string(adminHash), opts)
	if err != nil {
		return tmodels.Tenant{}, err
	}
	if err := tx.Commit(); err != nil {
		return tmodels.Tenant{}, err
	}
	m.created(t)
	return t, nil
}

// createTenant creates the tenant with its default data, its system user and
// its admin, whose password is already hashed, inside tx.
func (m *Manager) createTenant(tx *sqlx.Tx, r Request, adminHash string, opts CreateOpts) (tmodels.Tenant, error) {
	meta := map[string]any{}
	if opts.Channels != nil {
		meta["channels"] = opts.Channels
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return tmodels.Tenant{}, err
	}

	// Nobody logs in as the system user; it only authors automatic actions.
	sysPassword, err := stringutil.RandomAlphanumeric(32)
	if err != nil {
		return tmodels.Tenant{}, err
	}
	sysHash, err := bcrypt.GenerateFromPassword([]byte(sysPassword), bcrypt.DefaultCost)
	if err != nil {
		return tmodels.Tenant{}, fmt.Errorf("hashing password: %w", err)
	}

	var t tmodels.Tenant
	if err := tx.Get(&t, `
		INSERT INTO tenants (slug, "name", "plan", meta) VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at, slug, "name", custom_domain, status, "plan", meta, deletion_scheduled_at`,
		r.Slug, r.Company, cmp.Or(opts.Plan, m.plan), string(metaJSON)); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return tmodels.Tenant{}, ErrSlugTaken
		}
		return tmodels.Tenant{}, fmt.Errorf("inserting tenant: %w", err)
	}

	if _, err := tx.Exec(`SELECT seed_tenant($1)`, t.ID); err != nil {
		return tmodels.Tenant{}, fmt.Errorf("seeding tenant: %w", err)
	}

	// The tenant's own name and URL replace the seeded defaults.
	if _, err := tx.Exec(`
		UPDATE settings SET value = CASE key
			WHEN 'app.site_name' THEN to_jsonb($2::text)
			WHEN 'app.root_url' THEN to_jsonb($3::text)
			WHEN 'app.favicon_url' THEN to_jsonb($3::text || '/favicon.ico')
		END, updated_at = NOW()
		WHERE tenant_id = $1 AND key IN ('app.site_name', 'app.root_url', 'app.favicon_url')`,
		t.ID, r.Company, m.tenants.URLFor(t)); err != nil {
		return tmodels.Tenant{}, fmt.Errorf("setting tenant settings: %w", err)
	}

	if _, err := tx.Exec(`
		WITH u AS (
			INSERT INTO users (tenant_id, email, type, first_name, last_name, password)
			VALUES ($1, $2, $3, 'System', '', $4), ($1, $5, $3, $6, $7, $8)
			RETURNING id, email
		)
		INSERT INTO user_roles (tenant_id, user_id, role_id)
		SELECT $1, u.id, r.id FROM u JOIN roles r ON r.tenant_id = $1 AND r.name = $9`,
		t.ID, umodels.SystemUserEmail, umodels.UserTypeAgent, sysHash,
		r.Email, r.FirstName, r.LastName, adminHash, rmodels.RoleAdmin); err != nil {
		return tmodels.Tenant{}, fmt.Errorf("creating users: %w", err)
	}
	return t, nil
}

// created runs after a tenant is committed.
func (m *Manager) created(t tmodels.Tenant) {
	m.tenants.Invalidate()
	m.lo.Info("tenant created", "tenant_id", t.ID, "slug", t.Slug)
}
