// Package signup creates a new tenant: its row, its default data and its
// first admin, all in one transaction.
package signup

import (
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

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
	// ErrInvalidToken is returned for an unknown, used or expired verification token.
	ErrInvalidToken = errors.New("invalid or expired verification link")

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

// Request is a signup form.
type Request struct {
	Company   string `json:"company"`
	Slug      string `json:"slug"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

// PendingSignup is a signup waiting for its email to be verified.
type PendingSignup struct {
	Slug      string    `db:"slug" json:"slug"`
	Company   string    `db:"company" json:"company"`
	Email     string    `db:"email" json:"email"`
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`
}

// Manager creates tenants.
type Manager struct {
	db       *sqlx.DB
	lo       *logf.Logger
	tenants  *tenant.Manager
	plan     string
	tokenTTL time.Duration
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB      *sqlx.DB
	Lo      *logf.Logger
	Tenants *tenant.Manager
	// Plan is the plan new tenants start on.
	Plan string
	// TokenTTL is how long a verification link stays valid, 24 hours when zero.
	TokenTTL time.Duration
}

// New returns a signup Manager.
func New(opts Opts) *Manager {
	return &Manager{db: opts.DB, lo: opts.Lo, tenants: opts.Tenants, plan: opts.Plan, tokenTTL: cmp.Or(opts.TokenTTL, 24*time.Hour)}
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

// Create validates the request and creates the tenant at once, without
// verifying its email. Nothing is kept if any step fails.
func (m *Manager) Create(ctx context.Context, r Request) (tmodels.Tenant, error) {
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

	// A signup waiting for its email to be verified holds its slug.
	var held bool
	if err := tx.Get(&held, `SELECT EXISTS(SELECT 1 FROM pending_signups WHERE slug = $1 AND expires_at > NOW())`, r.Slug); err != nil {
		return tmodels.Tenant{}, err
	}
	if held {
		return tmodels.Tenant{}, ErrSlugTaken
	}

	t, err := m.createTenant(tx, r, string(adminHash))
	if err != nil {
		return tmodels.Tenant{}, err
	}
	if err := tx.Commit(); err != nil {
		return tmodels.Tenant{}, err
	}
	m.created(t)
	return t, nil
}

// Start validates the request and holds it as a pending signup until its
// email is verified. It returns the token to email; only its hash is stored.
// Signing up again with the same email and slug replaces the earlier one.
func (m *Manager) Start(ctx context.Context, r Request) (string, error) {
	r.Normalize()
	if err := r.Validate(); err != nil {
		return "", err
	}
	if reservedSlugs[r.Slug] {
		return "", ErrSlugTaken
	}
	adminHash, err := bcrypt.GenerateFromPassword([]byte(r.Password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	token, err := stringutil.RandomAlphanumeric(48)
	if err != nil {
		return "", err
	}

	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	// Expired signups release their slugs; a repeat signup replaces its own.
	if _, err := tx.Exec(`DELETE FROM pending_signups WHERE expires_at <= NOW() OR (slug = $1 AND email = $2)`, r.Slug, r.Email); err != nil {
		return "", err
	}
	var taken bool
	if err := tx.Get(&taken, `SELECT EXISTS(SELECT 1 FROM tenants WHERE slug = $1)`, r.Slug); err != nil {
		return "", err
	}
	if taken {
		return "", ErrSlugTaken
	}
	if _, err := tx.Exec(`
		INSERT INTO pending_signups (expires_at, token_hash, slug, company, first_name, last_name, email, password_hash)
		VALUES (NOW() + $1 * INTERVAL '1 second', $2, $3, $4, $5, $6, $7, $8)`,
		int(m.tokenTTL.Seconds()), hashToken(token), r.Slug, r.Company, r.FirstName, r.LastName, r.Email, string(adminHash)); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return "", ErrSlugTaken
		}
		return "", fmt.Errorf("inserting pending signup: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	m.lo.Info("signup pending email verification", "slug", r.Slug)
	return token, nil
}

// Pending returns the unexpired pending signup for a token, without using it up.
func (m *Manager) Pending(ctx context.Context, token string) (PendingSignup, error) {
	var p PendingSignup
	err := m.db.GetContext(ctx, &p, `
		SELECT slug, company, email, expires_at FROM pending_signups
		WHERE token_hash = $1 AND expires_at > NOW()`, hashToken(token))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrInvalidToken
	}
	return p, err
}

// Confirm verifies a pending signup's token and creates its tenant. The token
// can only be used once.
func (m *Manager) Confirm(ctx context.Context, token string) (tmodels.Tenant, error) {
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return tmodels.Tenant{}, err
	}
	defer tx.Rollback()

	var p struct {
		ID           int    `db:"id"`
		Slug         string `db:"slug"`
		Company      string `db:"company"`
		FirstName    string `db:"first_name"`
		LastName     string `db:"last_name"`
		Email        string `db:"email"`
		PasswordHash string `db:"password_hash"`
	}
	if err := tx.Get(&p, `
		DELETE FROM pending_signups WHERE token_hash = $1 AND expires_at > NOW()
		RETURNING id, slug, company, first_name, last_name, email, password_hash`, hashToken(token)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tmodels.Tenant{}, ErrInvalidToken
		}
		return tmodels.Tenant{}, err
	}

	t, err := m.createTenant(tx, Request{Company: p.Company, Slug: p.Slug, FirstName: p.FirstName, LastName: p.LastName, Email: p.Email}, p.PasswordHash)
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
func (m *Manager) createTenant(tx *sqlx.Tx, r Request, adminHash string) (tmodels.Tenant, error) {
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
		INSERT INTO tenants (slug, "name", "plan") VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at, slug, "name", custom_domain, status, "plan", meta, deletion_scheduled_at`,
		r.Slug, r.Company, m.plan); err != nil {
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
	m.lo.Info("tenant signed up", "tenant_id", t.ID, "slug", t.Slug)
}

// SlugAvailable reports whether a workspace slug can be signed up: it isn't
// reserved, taken, or held by a signup waiting for email verification.
func (m *Manager) SlugAvailable(ctx context.Context, slug string) (bool, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugRe.MatchString(slug) || reservedSlugs[slug] {
		return false, nil
	}
	var taken bool
	err := m.db.GetContext(ctx, &taken, `
		SELECT EXISTS(SELECT 1 FROM tenants WHERE slug = $1)
			OR EXISTS(SELECT 1 FROM pending_signups WHERE slug = $1 AND expires_at > NOW())`, slug)
	return !taken, err
}

// TokenTTL is how long an emailed verification link stays valid.
func (m *Manager) TokenTTL() time.Duration {
	return m.tokenTTL
}

// hashToken returns the SHA-256 of a token, as stored.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
