package user

import (
	"os"
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/knadh/goyesql/v2"
)

// TestPhoneQueries checks an agent can set the number they take calls on,
// and the ringing list has only enabled agents with one.
func TestPhoneQueries(t *testing.T) {
	db := testutil.NewDB(t, "user_phone")
	b, err := os.ReadFile("queries.sql")
	if err != nil {
		t.Fatal(err)
	}
	q, err := goyesql.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	var a, off, contact int
	db.Get(&a, `INSERT INTO users (tenant_id, type, first_name, email) VALUES (1, 'agent', 'Asha', 'asha@acme.example') RETURNING id`)
	db.Get(&off, `INSERT INTO users (tenant_id, type, first_name, email, enabled, phone_number) VALUES (1, 'agent', 'Off', 'off@acme.example', false, '+919800000002') RETURNING id`)
	db.Get(&contact, `INSERT INTO users (tenant_id, type, first_name, phone_number) VALUES (1, 'contact', 'C', '+919800000003') RETURNING id`)

	db.MustExec(q["update-own-phone"].Query, 1, a, "+919800000001")
	// A contact isn't changed by the agent query.
	db.MustExec(q["update-own-phone"].Query, 1, contact, "")

	var agents []PhoneAgent
	if err := db.Select(&agents, q["get-agents-with-phone"].Query, 1); err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].ID != a || agents[0].PhoneNumber != "+919800000001" {
		t.Fatalf("agents: %+v", agents)
	}
	var phone string
	db.Get(&phone, `SELECT phone_number FROM users WHERE id = $1`, contact)
	if phone != "+919800000003" {
		t.Fatalf("contact phone changed to %q", phone)
	}
}
