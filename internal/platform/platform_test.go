package platform

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/platform/models"

	"github.com/conectdesk/conectdesk/internal/totp"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

func newManager(t *testing.T) (*Manager, *sqlx.DB) {
	t.Helper()
	db := testutil.NewDB(t, "platform")
	lo := logf.New(logf.Opts{})
	m, err := New(Opts{DB: db, Lo: &lo, EncryptionKey: "0123456789abcdef0123456789abcdef"})
	if err != nil {
		t.Fatalf("platform.New: %v", err)
	}
	return m, db
}

// addWorkspace creates a seeded workspace and returns its id.
func addWorkspace(t *testing.T, db *sqlx.DB, slug, name, status string) int {
	t.Helper()
	var id int
	if err := db.Get(&id, `INSERT INTO tenants (slug, "name", status) VALUES ($1, $2, $3::tenant_status) RETURNING id`, slug, name, status); err != nil {
		t.Fatal(err)
	}
	db.MustExec(`SELECT seed_tenant($1)`, id)
	return id
}

func addAgent(t *testing.T, db *sqlx.DB, tenantID int, email string, enabled bool) int {
	t.Helper()
	var id int
	if err := db.Get(&id, `INSERT INTO users (tenant_id, type, first_name, email, enabled) VALUES ($1, 'agent', 'Agent', $2, $3) RETURNING id`, tenantID, email, enabled); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestLoginAndSessions(t *testing.T) {
	m, db := newManager(t)
	if _, err := m.SetAdmin("not an email", "x"); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("invalid email: %v", err)
	}
	if _, err := m.SetAdmin(" Ops@Example.com ", "Str0ng!Passw0rd#1"); err != nil {
		t.Fatal(err)
	}

	for _, c := range [][2]string{{"ops@example.com", "wrong"}, {"nobody@example.com", "Str0ng!Passw0rd#1"}} {
		if _, _, err := m.Login(c[0], c[1], "1.2.3.4"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("Login(%q): %v, want ErrInvalidCredentials", c[0], err)
		}
	}
	a, token, err := m.Login("OPS@example.com", "Str0ng!Passw0rd#1", "1.2.3.4")
	if err != nil || token == "" || a.Email != "ops@example.com" {
		t.Fatalf("Login: %+v %q %v", a, token, err)
	}
	if got, err := m.SessionAdmin(token); err != nil || got.ID != a.ID {
		t.Fatalf("SessionAdmin: %+v %v", got, err)
	}
	for _, bad := range []string{"", "made-up"} {
		if _, err := m.SessionAdmin(bad); !errors.Is(err, ErrNoSession) {
			t.Fatalf("SessionAdmin(%q): %v", bad, err)
		}
	}

	// The token isn't stored, only its hash.
	var n int
	db.Get(&n, `SELECT COUNT(*) FROM platform_sessions WHERE token_hash = $1`, token)
	if n != 0 {
		t.Fatal("session token stored in the clear")
	}

	// Resetting the password keeps the admin; the old password stops working.
	if _, err := m.SetAdmin("ops@example.com", "N3w!Passw0rd#2026"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Login("ops@example.com", "Str0ng!Passw0rd#1", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password: %v", err)
	}

	m.Logout(token)
	if _, err := m.SessionAdmin(token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("after logout: %v", err)
	}

	_, token, _ = m.Login("ops@example.com", "N3w!Passw0rd#2026", "")
	db.MustExec(`UPDATE platform_sessions SET expires_at = NOW() - INTERVAL '1 second'`)
	if _, err := m.SessionAdmin(token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("expired session: %v", err)
	}

	_, token, _ = m.Login("ops@example.com", "N3w!Passw0rd#2026", "")
	if ok, err := m.RemoveAdmin("ops@example.com"); !ok || err != nil {
		t.Fatalf("RemoveAdmin: %v %v", ok, err)
	}
	if _, err := m.SessionAdmin(token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("removed admin's session still works: %v", err)
	}
}

func TestWorkspacesAndOverview(t *testing.T) {
	m, db := newManager(t)
	acme := addWorkspace(t, db, "acme", "Acme Corp", "active")
	addWorkspace(t, db, "globex", "Globex", "suspended")
	addAgent(t, db, acme, "alice@acme.io", true)
	db.MustExec(`UPDATE users SET last_active_at = NOW() WHERE email = 'alice@acme.io'`)

	for q, want := range map[string][]string{
		"":         {"globex", "acme", "default"},
		"acm":      {"acme"},
		"GLOBEX":   {"globex"},
		"alice@ac": {"acme"},
		"nothing":  {},
	} {
		got, total, err := m.Workspaces(q, "", 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		slugs := []string{}
		for _, w := range got {
			slugs = append(slugs, w.Slug)
		}
		if total != len(want) || len(slugs) != len(want) {
			t.Errorf("Workspaces(%q) = %v (total %d), want %v", q, slugs, total, want)
			continue
		}
		for i := range want {
			if slugs[i] != want[i] {
				t.Errorf("Workspaces(%q) = %v, want %v", q, slugs, want)
			}
		}
	}
	if got, _, _ := m.Workspaces("", "suspended", 1, 10); len(got) != 1 || got[0].Slug != "globex" {
		t.Errorf("suspended filter: %+v", got)
	}
	if got, total, _ := m.Workspaces("", "", 2, 2); total != 3 || len(got) != 1 || got[0].Slug != "default" {
		t.Errorf("page 2 of 2: %+v total %d", got, total)
	}
	got, _, _ := m.Workspaces("acme", "", 1, 10)
	if got[0].Agents != 1 || !got[0].LastActiveAt.Valid {
		t.Errorf("acme row: %+v", got[0])
	}

	o, err := m.Overview()
	if err != nil {
		t.Fatal(err)
	}
	if o.Workspaces != 3 || o.Active != 2 || o.Suspended != 1 {
		t.Errorf("overview: %+v", o)
	}

	agents, err := m.WorkspaceAgents(acme)
	if err != nil || len(agents) != 1 || agents[0].Email.String != "alice@acme.io" {
		t.Errorf("agents: %+v %v", agents, err)
	}
}

func TestImpersonation(t *testing.T) {
	m, db := newManager(t)
	admin, _ := m.SetAdmin("ops@example.com", "Str0ng!Passw0rd#1")
	acme := addWorkspace(t, db, "acme", "Acme", "active")
	globex := addWorkspace(t, db, "globex", "Globex", "active")
	alice := addAgent(t, db, acme, "alice@acme.io", true)
	off := addAgent(t, db, acme, "off@acme.io", false)
	gina := addAgent(t, db, globex, "gina@globex.io", true)

	for _, uid := range []int{off, gina, 999999} {
		if _, err := m.Impersonate(admin, acme, uid); !errors.Is(err, ErrAgentNotFound) {
			t.Fatalf("Impersonate(user %d): %v, want ErrAgentNotFound", uid, err)
		}
	}

	token, err := m.Impersonate(admin, acme, alice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ConsumeImpersonation(globex, token); !errors.Is(err, ErrInvalidLink) {
		t.Fatalf("link used on another workspace: %v", err)
	}
	imp, err := m.ConsumeImpersonation(acme, token)
	if err != nil || imp.UserID != alice || imp.AdminEmail != "ops@example.com" {
		t.Fatalf("consume: %+v %v", imp, err)
	}
	if _, err := m.ConsumeImpersonation(acme, token); !errors.Is(err, ErrInvalidLink) {
		t.Fatalf("link used twice: %v", err)
	}

	token, _ = m.Impersonate(admin, acme, alice)
	db.MustExec(`UPDATE platform_impersonations SET expires_at = NOW() - INTERVAL '1 second' WHERE used_at IS NULL`)
	if _, err := m.ConsumeImpersonation(acme, token); !errors.Is(err, ErrInvalidLink) {
		t.Fatalf("expired link: %v", err)
	}
}

func TestAudit(t *testing.T) {
	m, db := newManager(t)
	admin, _ := m.SetAdmin("ops@example.com", "Str0ng!Passw0rd#1")
	acme := addWorkspace(t, db, "acme", "Acme", "active")

	m.Audit(admin, ActionLogin, 0, "", nil, "1.2.3.4")
	m.Audit(admin, ActionChangePlan, acme, "acme", map[string]any{"from": "free", "to": "pro"}, "1.2.3.4")

	all, err := m.AuditLog(0, 10)
	if err != nil || len(all) != 2 || all[0].Action != ActionChangePlan {
		t.Fatalf("AuditLog: %+v %v", all, err)
	}
	only, _ := m.AuditLog(acme, 10)
	if len(only) != 1 || string(only[0].Details) != `{"to": "pro", "from": "free"}` {
		t.Fatalf("workspace audit: %+v", only)
	}

	// The entry outlives the workspace and the admin.
	db.MustExec(`DELETE FROM tenants WHERE id = $1`, acme)
	m.RemoveAdmin("ops@example.com")
	all, _ = m.AuditLog(0, 10)
	if len(all) != 2 || all[0].WorkspaceID != 0 || all[0].WorkspaceSlug != "acme" || all[0].AdminEmail != "ops@example.com" {
		t.Fatalf("after deletion: %+v", all[0])
	}
}

func TestAdminTwoFactor(t *testing.T) {
	m, _ := newManager(t)
	now := time.Unix(1700000000, 0)
	m.now = func() time.Time { return now }
	a, _ := m.SetAdmin("ops@example.com", "Str0ng!Passw0rd#1")

	secret, _ := totp.NewSecret()
	if _, err := m.EnableTwoFactor(a, secret, "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("enable with a wrong code: %v", err)
	}
	c, _ := totp.Code(secret, now)
	codes, err := m.EnableTwoFactor(a, secret, c)
	if err != nil || len(codes) != 10 {
		t.Fatalf("enable: %v", err)
	}
	if _, _, err := m.Login("ops@example.com", "Str0ng!Passw0rd#1", ""); !errors.Is(err, ErrTwoFactorRequired) {
		t.Fatalf("Login with two-factor on: %v, want ErrTwoFactorRequired", err)
	}
	a, err = m.Authenticate("ops@example.com", "Str0ng!Passw0rd#1")
	if err != nil || !a.TOTPEnabled {
		t.Fatalf("Authenticate: %+v %v", a, err)
	}
	if err := m.VerifyTwoFactor(a, c); !errors.Is(err, ErrInvalidCode) {
		t.Fatal("the setup code works again")
	}
	now = now.Add(totp.Step)
	c, _ = totp.Code(secret, now)
	if err := m.VerifyTwoFactor(a, c); err != nil {
		t.Fatalf("next code: %v", err)
	}
	if err := m.VerifyTwoFactor(a, codes[0]); err != nil {
		t.Fatalf("recovery code: %v", err)
	}
	if err := m.VerifyTwoFactor(a, codes[0]); !errors.Is(err, ErrInvalidCode) {
		t.Fatal("a recovery code works twice")
	}
	if ok, err := m.ResetTwoFactor("ops@example.com"); !ok || err != nil {
		t.Fatalf("reset: %v %v", ok, err)
	}
	if _, tok, err := m.Login("ops@example.com", "Str0ng!Passw0rd#1", ""); err != nil || tok == "" {
		t.Fatalf("login after reset: %v", err)
	}
}

func TestMailSettings(t *testing.T) {
	m, db := newManager(t)
	if ms, err := m.MailSettings(); err != nil || ms.Enabled {
		t.Fatalf("unset: %+v %v", ms, err)
	}
	if err := m.SetMailSettings(models.MailSettings{Enabled: true, Host: "smtp.mailgun.org", Port: 587, Username: "postmaster@mg.x.io", Password: "s3cret", From: "X <noreply@mg.x.io>"}); err != nil {
		t.Fatal(err)
	}
	var raw string
	db.Get(&raw, `SELECT value::text FROM platform_settings WHERE key = 'mail'`)
	if strings.Contains(raw, "s3cret") {
		t.Fatal("password stored in the clear")
	}
	// Saving without a password keeps it.
	if err := m.SetMailSettings(models.MailSettings{Enabled: true, Host: "smtp.mailgun.org", Port: 2525, From: "X <noreply@mg.x.io>"}); err != nil {
		t.Fatal(err)
	}
	ms, err := m.MailSettings()
	if err != nil || ms.Password != "s3cret" || ms.Port != 2525 {
		t.Fatalf("after saving without a password: %+v %v", ms, err)
	}
}
