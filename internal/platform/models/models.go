// Package models holds the platform admin dashboard's models.
package models

import (
	"encoding/json"
	"time"

	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
)

// Admin runs the installation. Admins are not users of any workspace.
type Admin struct {
	ID           int       `db:"id" json:"id"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	Email        string    `db:"email" json:"email"`
	PasswordHash string    `db:"password_hash" json:"-"`
	LastLoginAt  null.Time `db:"last_login_at" json:"last_login_at"`
	// TOTPEnabled is set when the admin logs in with a two-factor code.
	TOTPEnabled bool `db:"totp_enabled" json:"totp_enabled"`
}

// Overview sums up every workspace.
type Overview struct {
	Workspaces         int     `db:"workspaces" json:"workspaces"`
	Active             int     `db:"active" json:"active"`
	Suspended          int     `db:"suspended" json:"suspended"`
	Signups7d          int     `db:"signups_7d" json:"signups_7d"`
	Signups30d         int     `db:"signups_30d" json:"signups_30d"`
	ActiveWorkspaces7d int     `db:"active_workspaces_7d" json:"active_workspaces_7d"`
	Agents             int     `db:"agents" json:"agents"`
	Conversations30d   int     `db:"conversations_30d" json:"conversations_30d"`
	Plans              []Count `db:"-" json:"plans"`
}

// Count is how many workspaces are on a plan.
type Count struct {
	Plan  string `db:"plan" json:"plan"`
	Count int    `db:"count" json:"count"`
}

// Workspace is a tenant as the platform lists it.
type Workspace struct {
	ID               int         `db:"id" json:"id"`
	Slug             string      `db:"slug" json:"slug"`
	Name             string      `db:"name" json:"name"`
	Status           string      `db:"status" json:"status"`
	Plan             string      `db:"plan" json:"plan"`
	CreatedAt        time.Time   `db:"created_at" json:"created_at"`
	CustomDomain     null.String `db:"custom_domain" json:"custom_domain"`
	Agents           int         `db:"agents" json:"agents"`
	Inboxes          int         `db:"inboxes" json:"inboxes"`
	Conversations30d int         `db:"conversations_30d" json:"conversations_30d"`
	LastActiveAt     null.Time   `db:"last_active_at" json:"last_active_at"`
	Total            int         `db:"total" json:"-"`
}

// WorkspaceCounts is what a workspace holds.
type WorkspaceCounts struct {
	Contacts         int `db:"contacts" json:"contacts"`
	Conversations    int `db:"conversations" json:"conversations"`
	Conversations30d int `db:"conversations_30d" json:"conversations_30d"`
	HelpCenters      int `db:"help_centers" json:"help_centers"`
}

// Agent is a workspace agent as the platform lists it.
type Agent struct {
	ID           int            `db:"id" json:"id"`
	FirstName    string         `db:"first_name" json:"first_name"`
	LastName     string         `db:"last_name" json:"last_name"`
	Email        null.String    `db:"email" json:"email"`
	Enabled      bool           `db:"enabled" json:"enabled"`
	LastActiveAt null.Time      `db:"last_active_at" json:"last_active_at"`
	LastLoginAt  null.Time      `db:"last_login_at" json:"last_login_at"`
	Roles        pq.StringArray `db:"roles" json:"roles"`
}

// AuditEntry is one thing a platform admin did.
type AuditEntry struct {
	ID            int64           `db:"id" json:"id"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
	AdminEmail    string          `db:"admin_email" json:"admin_email"`
	Action        string          `db:"action" json:"action"`
	WorkspaceID   int             `db:"workspace_id" json:"workspace_id"`
	WorkspaceSlug string          `db:"workspace_slug" json:"workspace_slug"`
	Details       json.RawMessage `db:"details" json:"details"`
	IP            string          `db:"ip" json:"ip"`
}

// Impersonation is a used impersonation link.
type Impersonation struct {
	AdminID    int    `db:"admin_id"`
	AdminEmail string `db:"admin_email"`
	UserID     int    `db:"user_id"`
}

// MailSettings is the platform's own mailer, which sends signup verification
// and the email of every tenant without a mailer of its own. Its fields match
// a tenant's notification.email.* settings.
type MailSettings struct {
	Enabled       bool   `json:"enabled"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password,omitempty"`
	AuthProtocol  string `json:"auth_protocol"`
	TLSType       string `json:"tls_type"`
	TLSSkipVerify bool   `json:"tls_skip_verify"`
	HelloHostname string `json:"hello_hostname"`
	// From is the sender, "Name <address>" or an address.
	From        string `json:"email_address"`
	MaxConns    int    `json:"max_conns"`
	IdleTimeout string `json:"idle_timeout"`
	WaitTimeout string `json:"wait_timeout"`
}
