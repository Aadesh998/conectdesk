package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/conectdesk/conectdesk/internal/user"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/zerodha/logf"
)

const password = "Str0ng!Passw0rd#2026"

func TestCreate(t *testing.T) {
	db := testutil.NewDB(t, "workspace")
	lo := logf.New(logf.Opts{})
	tenants, err := tenant.New(tenant.Opts{DB: db, Lo: &lo, Enabled: true, BaseDomain: "yourapp.test"})
	if err != nil {
		t.Fatal(err)
	}
	m := New(Opts{DB: db, Lo: &lo, Tenants: tenants, Plan: "free"})
	ctx := context.Background()

	req := Request{Company: " Acme Inc ", Slug: "Acme", FirstName: "Ann", LastName: "Owner", Email: "Ann@Acme.test", Password: password}
	acme, err := m.Create(ctx, req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if acme.Slug != "acme" || acme.Name != "Acme Inc" || acme.Plan != "free" || !acme.Active() {
		t.Fatalf("tenant = %+v", acme)
	}

	// The new tenant has the same defaults as the default tenant.
	for _, table := range []string{"roles", "conversation_statuses", "conversation_priorities", "templates", "business_hours", "sla_policies", "ai_prompts", "ai_providers"} {
		var def, got int
		db.Get(&def, `SELECT COUNT(*) FROM `+table+` WHERE tenant_id = 1`)
		db.Get(&got, `SELECT COUNT(*) FROM `+table+` WHERE tenant_id = $1`, acme.ID)
		if got == 0 || got != def {
			t.Errorf("%s: acme has %d rows, default tenant %d", table, got, def)
		}
	}
	var siteName, rootURL string
	db.Get(&siteName, `SELECT value #>> '{}' FROM settings WHERE tenant_id = $1 AND key = 'app.site_name'`, acme.ID)
	db.Get(&rootURL, `SELECT value #>> '{}' FROM settings WHERE tenant_id = $1 AND key = 'app.root_url'`, acme.ID)
	if siteName != "Acme Inc" || rootURL != "https://acme.yourapp.test" {
		t.Fatalf("settings: site name %q, root URL %q", siteName, rootURL)
	}

	// The admin can log in on the new tenant, and it has a system user.
	users, err := user.New(testutil.NewI18n(t), user.Opts{DB: db, Lo: &lo})
	if err != nil {
		t.Fatal(err)
	}
	admin, err := users.VerifyPassword(acme.ID, "ann@acme.test", []byte(password))
	if err != nil {
		t.Fatalf("admin login: %v", err)
	}
	if len(admin.Roles) != 1 || admin.Roles[0] != "Admin" {
		t.Fatalf("admin roles: %v", admin.Roles)
	}
	if _, err := users.GetSystemUser(acme.ID); err != nil {
		t.Fatalf("system user: %v", err)
	}
	if _, err := users.VerifyPassword(tenant.DefaultID, "ann@acme.test", []byte(password)); err == nil {
		t.Fatal("acme's admin logs in on the default tenant")
	}

	// Taken, reserved and malformed slugs.
	for _, slug := range []string{"acme", "www", "default"} {
		r := req
		r.Slug = slug
		if _, err := m.Create(ctx, r); !errors.Is(err, ErrSlugTaken) {
			t.Errorf("slug %q: %v, want ErrSlugTaken", slug, err)
		}
	}
	for field, mut := range map[string]func(*Request){
		"slug":     func(r *Request) { r.Slug = "a" },
		"email":    func(r *Request) { r.Email = "not-an-email" },
		"password": func(r *Request) { r.Password = "short" },
		"company":  func(r *Request) { r.Company = " " },
	} {
		r := req
		r.Slug = "newco"
		mut(&r)
		var ve ValidationError
		if _, err := m.Create(ctx, r); !errors.As(err, &ve) || ve.Field != field {
			t.Errorf("%s: %v, want ValidationError{%s}", field, err, field)
		}
	}
	var n int
	db.Get(&n, `SELECT COUNT(*) FROM tenants WHERE slug = 'newco'`)
	if n != 0 {
		t.Fatal("an invalid request created a tenant")
	}

	// The same email can sign up a second workspace.
	r := req
	r.Slug = "acme-eu"
	if _, err := m.Create(ctx, r); err != nil {
		t.Fatalf("second workspace for the same email: %v", err)
	}
	var admins int
	db.Get(&admins, `SELECT COUNT(*) FROM users WHERE email = 'ann@acme.test' AND type = $1`, umodels.UserTypeAgent)
	if admins != 2 {
		t.Fatalf("admins with the email: %d, want 2", admins)
	}
}
