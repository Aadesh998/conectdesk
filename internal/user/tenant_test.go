package user

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/user/models"
)

// TestTenantIsolation checks users of one tenant are invisible to another.
func TestTenantIsolation(t *testing.T) {
	u, db := newTestManager(t)
	const acme, foo = 2, 3
	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'acme', 'Acme'), (3, 'foo', 'Foo')`)
	db.MustExec(`INSERT INTO roles (tenant_id, name, description, permissions) VALUES
		(2, 'Agent', '', '{}'), (3, 'Agent', '', '{}')`)

	isNotFound := func(err error) bool {
		e, ok := err.(envelope.Error)
		return ok && e.ErrorType == envelope.NotFoundError
	}

	// The same person can be an agent of two tenants.
	ravi, err := u.CreateAgent(acme, "Ravi", "", "ravi@example.com", []string{"Agent"})
	if err != nil {
		t.Fatalf("CreateAgent acme: %v", err)
	}
	raviFoo, err := u.CreateAgent(foo, "Ravi", "", "ravi@example.com", []string{"Agent"})
	if err != nil {
		t.Fatalf("CreateAgent foo, same email: %v", err)
	}
	if ravi.ID == raviFoo.ID || ravi.TenantID != acme || raviFoo.TenantID != foo {
		t.Fatalf("got %d/%d in tenants %d/%d", ravi.ID, raviFoo.ID, ravi.TenantID, raviFoo.TenantID)
	}
	if len(ravi.Roles) != 1 || ravi.Roles[0] != "Agent" {
		t.Fatalf("acme agent roles: %v", ravi.Roles)
	}

	// Lookups by ID, email and cache never cross tenants.
	if _, err := u.GetAgent(foo, ravi.ID, ""); !isNotFound(err) {
		t.Fatalf("foo reading acme agent by ID: %v, want not found", err)
	}
	if _, err := u.GetAgentCachedOrLoad(acme, ravi.ID); err != nil {
		t.Fatalf("warming cache: %v", err)
	}
	if _, err := u.GetAgentCachedOrLoad(foo, ravi.ID); !isNotFound(err) {
		t.Fatalf("foo reading cached acme agent: %v, want not found", err)
	}
	got, err := u.Get(foo, 0, "ravi@example.com", []string{models.UserTypeAgent})
	if err != nil || got.ID != raviFoo.ID {
		t.Fatalf("foo lookup by email: %d, %v; want %d", got.ID, err, raviFoo.ID)
	}

	// Writes addressed to another tenant's user change nothing.
	if err := u.UpdateAgent(foo, ravi.ID, "Hacked", "", "", []string{"Agent"}, true, "", ""); !isNotFound(err) {
		t.Fatalf("UpdateAgent of another tenant's agent: %v, want not found", err)
	}
	if err := u.SoftDeleteAgent(foo, ravi.ID); !isNotFound(err) && err != nil {
		t.Fatalf("SoftDeleteAgent: %v", err)
	}
	u.InvalidateAllAgentCache()
	again, err := u.GetAgent(acme, ravi.ID, "")
	if err != nil || again.FirstName != "Ravi" || len(again.Roles) != 1 {
		t.Fatalf("acme agent after foo's writes: %+v, %v", again, err)
	}

	// Contacts with the same email resolve separately per tenant.
	c1 := newContact("ann@example.com", "", "Ann", "")
	c2 := newContact("ann@example.com", "", "Ann", "")
	if err := u.ResolveContact(acme, c1, models.ContactSync); err != nil {
		t.Fatal(err)
	}
	if err := u.ResolveContact(foo, c2, models.ContactSync); err != nil {
		t.Fatal(err)
	}
	if c1.ID == c2.ID {
		t.Fatal("contacts of two tenants resolved to the same user")
	}
	if _, err := u.GetContactOrVisitor(foo, c1.ID, ""); !isNotFound(err) {
		t.Fatalf("foo reading acme contact: %v, want not found", err)
	}
	if _, err := u.ExportContactData(foo, c1.ID); err != nil {
		t.Fatalf("export: %v", err)
	}
	var contact []byte
	contact, _ = u.ExportContactData(foo, c1.ID)
	if string(contact) != `{"contact": null, "conversations": []}` {
		t.Fatalf("foo exporting acme contact got %s", contact)
	}

	// API keys only authenticate on their own tenant.
	key, secret, err := u.GenerateAPIKey(acme, ravi.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.ValidateAPIKey(acme, key, secret); err != nil {
		t.Fatalf("own tenant API key: %v", err)
	}
	if _, err := u.ValidateAPIKey(foo, key, secret); err == nil {
		t.Fatal("acme API key authenticated on foo")
	}

	// Listing only shows the tenant's own users.
	list, err := u.GetAgents(foo)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range list {
		if a.ID == ravi.ID {
			t.Fatal("foo's agent list includes acme's agent")
		}
	}
	if len(list) != 1 {
		t.Fatalf("foo agents: %d, want 1", len(list))
	}

	// Notes can't be attached across tenants.
	if _, err := u.CreateNote(foo, c1.ID, raviFoo.ID, "note"); err == nil {
		t.Fatal("foo attached a note to acme's contact")
	}
}
