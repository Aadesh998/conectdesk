// Package platform backs the platform admin dashboard, where the people who
// run the installation see and manage every workspace (tenant). Platform
// admins are not users of any workspace, and everything they do is audited.
package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/mfa"
	"github.com/conectdesk/conectdesk/internal/platform/models"
	"github.com/conectdesk/conectdesk/internal/totp"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
	"golang.org/x/crypto/bcrypt"
)

var (
	//go:embed queries.sql
	efs embed.FS

	// ErrInvalidCredentials is returned for a wrong email or password.
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrNoSession is returned for a missing, unknown or expired session.
	ErrNoSession = errors.New("no platform session")
	// ErrInvalidLink is returned for an unknown, used or expired impersonation link.
	ErrInvalidLink = errors.New("invalid or expired impersonation link")
	// ErrAgentNotFound is returned when impersonating someone who isn't an
	// enabled agent of the workspace.
	ErrAgentNotFound = errors.New("agent not found in workspace")
	// ErrTwoFactorRequired is returned by Login for an admin who logs in with a code.
	ErrTwoFactorRequired = errors.New("two-factor code required")
	// ErrInvalidCode is returned for a wrong or reused two-factor code.
	ErrInvalidCode = errors.New("invalid two-factor code")
	// ErrInvalidEmail is returned for an admin email that isn't an address.
	ErrInvalidEmail = errors.New("invalid email")
	// ErrSuspended is returned when a suspended docs viewer logs in.
	ErrSuspended = errors.New("account suspended")
	// ErrNotFound is returned for an unknown docs viewer.
	ErrNotFound = errors.New("not found")

	// dummyHash is compared against for unknown emails, so a login takes as
	// long whether or not the email exists.
	dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not a password"), bcrypt.DefaultCost)
)

const (
	// SessionTTL is how long a platform login lasts.
	SessionTTL = 12 * time.Hour
	// ImpersonationTTL is how long an impersonation link works.
	ImpersonationTTL = time.Minute
)

// Audit actions.
const (
	ActionLogin       = "login"
	ActionLogout      = "logout"
	ActionSuspend     = "suspend"
	ActionActivate    = "activate"
	ActionChangePlan  = "change_plan"
	ActionDelete      = "delete"
	ActionImpersonate = "impersonate"
	ActionEnable2FA   = "enable_2fa"
	// ActionScheduleDelete suspends a workspace and deletes it after a grace period.
	ActionScheduleDelete  = "schedule_delete"
	ActionRestore         = "restore"
	ActionUpdateMail      = "update_mail"
	ActionCreate          = "create"
	ActionSetChannels     = "set_channels"
	ActionSetIntegrations = "set_integrations"
	ActionTestMail        = "test_mail"
	// ActionAddDocsViewer and ActionRemoveDocsViewer give and take away access
	// to the internal documentation.
	ActionAddDocsViewer    = "add_docs_viewer"
	ActionRemoveDocsViewer = "remove_docs_viewer"
	// Docs viewers suspended or let back in, and given a new password.
	ActionSuspendDocsViewer  = "suspend_docs_viewer"
	ActionActivateDocsViewer = "activate_docs_viewer"
	ActionResetDocsPassword  = "reset_docs_password"
	// What platform admins do to a workspace's agents.
	ActionCreateAgent        = "create_agent"
	ActionResetAgentPassword = "reset_agent_password"
	ActionSuspendAgent       = "suspend_agent"
	ActionActivateAgent      = "activate_agent"
	ActionDeleteAgent        = "delete_agent"
)

// Manager manages platform admins, their sessions and the audit log, and
// reads across workspaces.
type Manager struct {
	q             queries
	lo            *logf.Logger
	encryptionKey string
	now           func() time.Time
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB *sqlx.DB
	Lo *logf.Logger
	// EncryptionKey encrypts admins' two-factor secrets.
	EncryptionKey string
}

type queries struct {
	UpsertAdmin                 *sqlx.Stmt `query:"upsert-admin"`
	GetAdminByEmail             *sqlx.Stmt `query:"get-admin-by-email"`
	GetAdmins                   *sqlx.Stmt `query:"get-admins"`
	DeleteAdmin                 *sqlx.Stmt `query:"delete-admin"`
	TouchAdminLogin             *sqlx.Stmt `query:"touch-admin-login"`
	InsertSession               *sqlx.Stmt `query:"insert-session"`
	GetSessionAdmin             *sqlx.Stmt `query:"get-session-admin"`
	DeleteSession               *sqlx.Stmt `query:"delete-session"`
	DeleteExpiredSessions       *sqlx.Stmt `query:"delete-expired-sessions"`
	DeleteExpiredImpersonations *sqlx.Stmt `query:"delete-expired-impersonations"`
	InsertAudit                 *sqlx.Stmt `query:"insert-audit"`
	GetAudit                    *sqlx.Stmt `query:"get-audit"`
	GetOverview                 *sqlx.Stmt `query:"get-overview"`
	GetPlanCounts               *sqlx.Stmt `query:"get-plan-counts"`
	GetWorkspaces               *sqlx.Stmt `query:"get-workspaces"`
	GetWorkspaceCounts          *sqlx.Stmt `query:"get-workspace-counts"`
	GetWorkspaceAgents          *sqlx.Stmt `query:"get-workspace-agents"`
	InsertImpersonation         *sqlx.Stmt `query:"insert-impersonation"`
	ConsumeImpersonation        *sqlx.Stmt `query:"consume-impersonation"`
	GetAdminTOTP                *sqlx.Stmt `query:"get-admin-totp"`
	EnableAdminTOTP             *sqlx.Stmt `query:"enable-admin-totp"`
	ResetAdminTOTP              *sqlx.Stmt `query:"reset-admin-totp"`
	UseAdminTOTPStep            *sqlx.Stmt `query:"use-admin-totp-step"`
	UseAdminRecoveryCode        *sqlx.Stmt `query:"use-admin-recovery-code"`
	GetPlatformSetting          *sqlx.Stmt `query:"get-platform-setting"`
	SetPlatformSetting          *sqlx.Stmt `query:"set-platform-setting"`
	UpsertDocsViewer            *sqlx.Stmt `query:"upsert-docs-viewer"`
	GetDocsViewerByEmail        *sqlx.Stmt `query:"get-docs-viewer-by-email"`
	GetDocsViewers              *sqlx.Stmt `query:"get-docs-viewers"`
	DeleteDocsViewer            *sqlx.Stmt `query:"delete-docs-viewer"`
	TouchDocsViewerLogin        *sqlx.Stmt `query:"touch-docs-viewer-login"`
	InsertDocsSession           *sqlx.Stmt `query:"insert-docs-session"`
	GetDocsSessionViewer        *sqlx.Stmt `query:"get-docs-session-viewer"`
	DeleteDocsSession           *sqlx.Stmt `query:"delete-docs-session"`
	DeleteExpiredDocsSessions   *sqlx.Stmt `query:"delete-expired-docs-sessions"`
	SetDocsViewerEnabled        *sqlx.Stmt `query:"set-docs-viewer-enabled"`
	DeleteDocsViewerSessions    *sqlx.Stmt `query:"delete-docs-viewer-sessions"`
}

// New creates and returns a new instance of the Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, lo: opts.Lo, encryptionKey: opts.EncryptionKey, now: time.Now}, nil
}

// SetAdmin creates a platform admin, or sets the password of an existing one.
// The caller checks the password is strong enough.
func (m *Manager) SetAdmin(email, password string) (models.Admin, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return models.Admin{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return models.Admin{}, err
	}
	var a models.Admin
	if err := m.q.UpsertAdmin.Get(&a, email, string(hash)); err != nil {
		m.lo.Error("error saving platform admin", "error", err)
		return a, err
	}
	return a, nil
}

// Admins returns every platform admin.
func (m *Manager) Admins() ([]models.Admin, error) {
	var out []models.Admin
	if err := m.q.GetAdmins.Select(&out); err != nil {
		m.lo.Error("error fetching platform admins", "error", err)
		return nil, err
	}
	return out, nil
}

// RemoveAdmin removes a platform admin and ends their sessions.
func (m *Manager) RemoveAdmin(email string) (bool, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return false, err
	}
	res, err := m.q.DeleteAdmin.Exec(email)
	if err != nil {
		m.lo.Error("error removing platform admin", "error", err)
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Login checks the admin's password and starts a session, returning its
// token for the session cookie. An admin with two-factor login gets
// ErrTwoFactorRequired instead: use Authenticate, VerifyTwoFactor and
// NewSession.
func (m *Manager) Login(email, password, ip string) (models.Admin, string, error) {
	a, err := m.Authenticate(email, password)
	if err != nil {
		return a, "", err
	}
	if a.TOTPEnabled {
		return a, "", ErrTwoFactorRequired
	}
	token, err := m.NewSession(a, ip)
	return a, token, err
}

// Authenticate checks the admin's password.
func (m *Manager) Authenticate(email, password string) (models.Admin, error) {
	var a models.Admin
	email, _ = normalizeEmail(email)
	err := m.q.GetAdminByEmail.Get(&a, email)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return a, ErrInvalidCredentials
	case err != nil:
		m.lo.Error("error fetching platform admin", "error", err)
		return a, err
	}
	if bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil {
		return a, ErrInvalidCredentials
	}
	return a, nil
}

// NewSession starts a session for an admin who has proven who they are, and
// returns its token for the session cookie.
func (m *Manager) NewSession(a models.Admin, ip string) (string, error) {
	token, hash, err := newToken()
	if err != nil {
		return "", err
	}
	m.q.DeleteExpiredSessions.Exec()
	m.q.DeleteExpiredImpersonations.Exec()
	if _, err := m.q.InsertSession.Exec(a.ID, hash, time.Now().Add(SessionTTL), ip); err != nil {
		m.lo.Error("error saving platform session", "error", err)
		return "", err
	}
	m.q.TouchAdminLogin.Exec(a.ID)
	return token, nil
}

// EnableTwoFactor switches the admin's two-factor login on with a secret their
// app shows code for, and returns their recovery codes, shown this once.
func (m *Manager) EnableTwoFactor(a models.Admin, secret, code string) ([]string, error) {
	step, ok := totp.Verify(secret, code, m.now())
	if !ok {
		return nil, ErrInvalidCode
	}
	enc, err := crypto.Encrypt(secret, m.encryptionKey)
	if err != nil {
		return nil, err
	}
	codes, hashes, err := mfa.NewRecoveryCodeSet()
	if err != nil {
		return nil, err
	}
	if _, err := m.q.EnableAdminTOTP.Exec(a.ID, enc, pq.Array(hashes), int64(step)); err != nil {
		m.lo.Error("error enabling platform admin two-factor login", "error", err)
		return nil, err
	}
	return codes, nil
}

// VerifyTwoFactor checks a code from the admin's app, or one of their
// recovery codes, which then can't be used again. A code is accepted once.
func (m *Manager) VerifyTwoFactor(a models.Admin, code string) error {
	var row struct {
		Secret            string    `db:"secret"`
		EnabledAt         null.Time `db:"enabled_at"`
		RecoveryCodesLeft int       `db:"recovery_codes_left"`
	}
	if err := m.q.GetAdminTOTP.Get(&row, a.ID); err != nil || !row.EnabledAt.Valid {
		return ErrInvalidCode
	}
	if mfa.IsRecoveryCode(code) {
		var left int
		if err := m.q.UseAdminRecoveryCode.Get(&left, a.ID, mfa.HashRecoveryCode(code)); err != nil {
			return ErrInvalidCode
		}
		return nil
	}
	secret, err := crypto.Decrypt(row.Secret, m.encryptionKey)
	if err != nil {
		m.lo.Error("error decrypting platform admin two-factor secret", "error", err)
		return err
	}
	step, ok := totp.Verify(secret, code, m.now())
	if !ok {
		return ErrInvalidCode
	}
	var id int
	if err := m.q.UseAdminTOTPStep.Get(&id, a.ID, int64(step)); err != nil {
		return ErrInvalidCode
	}
	return nil
}

// ResetTwoFactor switches an admin's two-factor login off, for a lost phone;
// they set it up again at their next login.
func (m *Manager) ResetTwoFactor(email string) (bool, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return false, err
	}
	res, err := m.q.ResetAdminTOTP.Exec(email)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// AdminByID returns the admin with the id, for finishing a two-factor login.
func (m *Manager) AdminByID(id int) (models.Admin, error) {
	admins, err := m.Admins()
	if err != nil {
		return models.Admin{}, err
	}
	for _, a := range admins {
		if a.ID == id {
			return a, nil
		}
	}
	return models.Admin{}, ErrInvalidCredentials
}

// SessionAdmin returns the admin a session token belongs to.
func (m *Manager) SessionAdmin(token string) (models.Admin, error) {
	var a models.Admin
	if token == "" {
		return a, ErrNoSession
	}
	if err := m.q.GetSessionAdmin.Get(&a, hashToken(token)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return a, ErrNoSession
		}
		m.lo.Error("error fetching platform session", "error", err)
		return a, err
	}
	return a, nil
}

// Logout ends a session.
func (m *Manager) Logout(token string) error {
	_, err := m.q.DeleteSession.Exec(hashToken(token))
	return err
}

// Audit records what an admin did, on a workspace when workspaceID isn't 0.
// Failing to audit is logged, not returned, once the action has happened.
func (m *Manager) Audit(a models.Admin, action string, workspaceID int, workspaceSlug string, details map[string]any, ip string) {
	if details == nil {
		details = map[string]any{}
	}
	b, _ := json.Marshal(details)
	if _, err := m.q.InsertAudit.Exec(a.ID, a.Email, action, workspaceID, workspaceSlug, b, ip); err != nil {
		m.lo.Error("error recording platform audit", "action", action, "workspace_id", workspaceID, "error", err)
	}
}

// AuditLog returns the latest audit entries, of one workspace when workspaceID isn't 0.
func (m *Manager) AuditLog(workspaceID, limit int) ([]models.AuditEntry, error) {
	out := []models.AuditEntry{}
	if err := m.q.GetAudit.Select(&out, workspaceID, limit); err != nil {
		m.lo.Error("error fetching platform audit", "error", err)
		return nil, err
	}
	return out, nil
}

// Overview sums up every workspace.
func (m *Manager) Overview() (models.Overview, error) {
	var o models.Overview
	if err := m.q.GetOverview.Get(&o); err != nil {
		m.lo.Error("error fetching platform overview", "error", err)
		return o, err
	}
	if err := m.q.GetPlanCounts.Select(&o.Plans); err != nil {
		return o, err
	}
	return o, nil
}

// Workspaces returns a page of workspaces matching search and status, newest
// first, and how many match in all.
func (m *Manager) Workspaces(search, status string, page, pageSize int) ([]models.Workspace, int, error) {
	out := []models.Workspace{}
	if err := m.q.GetWorkspaces.Select(&out, strings.TrimSpace(search), status, pageSize, (page-1)*pageSize); err != nil {
		m.lo.Error("error fetching workspaces", "error", err)
		return nil, 0, err
	}
	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}
	return out, total, nil
}

// WorkspaceCounts returns what a workspace holds.
func (m *Manager) WorkspaceCounts(id int) (models.WorkspaceCounts, error) {
	var c models.WorkspaceCounts
	if err := m.q.GetWorkspaceCounts.Get(&c, id); err != nil {
		m.lo.Error("error fetching workspace counts", "workspace_id", id, "error", err)
		return c, err
	}
	return c, nil
}

// WorkspaceAgents returns a workspace's agents.
func (m *Manager) WorkspaceAgents(id int) ([]models.Agent, error) {
	out := []models.Agent{}
	if err := m.q.GetWorkspaceAgents.Select(&out, id); err != nil {
		m.lo.Error("error fetching workspace agents", "workspace_id", id, "error", err)
		return nil, err
	}
	return out, nil
}

// Impersonate creates a one-time link token that logs the admin into the
// workspace as the agent. It works for ImpersonationTTL.
func (m *Manager) Impersonate(a models.Admin, workspaceID, userID int) (string, error) {
	agents, err := m.WorkspaceAgents(workspaceID)
	if err != nil {
		return "", err
	}
	found := false
	for _, ag := range agents {
		if ag.ID == userID && ag.Enabled {
			found = true
		}
	}
	if !found {
		return "", ErrAgentNotFound
	}

	token, hash, err := newToken()
	if err != nil {
		return "", err
	}
	if _, err := m.q.InsertImpersonation.Exec(a.ID, workspaceID, userID, hash, time.Now().Add(ImpersonationTTL)); err != nil {
		m.lo.Error("error saving impersonation", "error", err)
		return "", err
	}
	return token, nil
}

// ConsumeImpersonation uses up an impersonation link of the workspace.
func (m *Manager) ConsumeImpersonation(workspaceID int, token string) (models.Impersonation, error) {
	var i models.Impersonation
	if token == "" {
		return i, ErrInvalidLink
	}
	if err := m.q.ConsumeImpersonation.Get(&i, hashToken(token), workspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return i, ErrInvalidLink
		}
		m.lo.Error("error using impersonation", "error", err)
		return i, err
	}
	return i, nil
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", ErrInvalidEmail
	}
	return email, nil
}

// newToken returns a random token and the hash it is stored as.
func newToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	t := base64.RawURLEncoding.EncodeToString(b)
	return t, hashToken(t), nil
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// MailSettings returns the platform's mailer settings, the password decrypted.
// Unset, they are zero: switched off.
func (m *Manager) MailSettings() (models.MailSettings, error) {
	var ms models.MailSettings
	var raw []byte
	if err := m.q.GetPlatformSetting.Get(&raw, "mail"); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ms, nil
		}
		return ms, err
	}
	if err := json.Unmarshal(raw, &ms); err != nil {
		return ms, err
	}
	if ms.Password != "" && crypto.IsEncrypted(ms.Password) {
		p, err := crypto.Decrypt(ms.Password, m.encryptionKey)
		if err != nil {
			return ms, err
		}
		ms.Password = p
	}
	return ms, nil
}

// SetMailSettings saves the platform's mailer settings. An empty password
// keeps the one saved.
func (m *Manager) SetMailSettings(ms models.MailSettings) error {
	if ms.Password == "" {
		cur, err := m.MailSettings()
		if err != nil {
			return err
		}
		ms.Password = cur.Password
	}
	if ms.Password != "" {
		enc, err := crypto.Encrypt(ms.Password, m.encryptionKey)
		if err != nil {
			return err
		}
		ms.Password = enc
	}
	b, err := json.Marshal(ms)
	if err != nil {
		return err
	}
	_, err = m.q.SetPlatformSetting.Exec("mail", b)
	return err
}
