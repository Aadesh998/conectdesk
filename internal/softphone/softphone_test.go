package softphone

import (
	"errors"
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/zerodha/logf"
)

const key = "0123456789abcdef0123456789abcdef"

func TestNormalize(t *testing.T) {
	a, err := Normalize(Account{Username: " sip:asha01 ", Password: "pw", Domain: "Acme.Voip.Exotel.com."}, true)
	if err != nil {
		t.Fatal(err)
	}
	if a.Username != "asha01" || a.Domain != "acme.voip.exotel.com" || a.Server != "" || a.Port != DefaultPort {
		t.Fatalf("normalized to %+v", a)
	}
	a, err = Normalize(Account{Username: "asha01", Password: "pw", Domain: "acme.voip.exotel.com", Server: "wss://voip.in1.exotel.com:8443/wss"}, true)
	if err != nil || a.Server != "voip.in1.exotel.com" || a.Port != 8443 {
		t.Fatalf("normalized to %+v, %v", a, err)
	}
	for _, tc := range []struct {
		a     Account
		isNew bool
		want  error
	}{
		{Account{Username: "a b", Password: "pw", Domain: "x.example"}, true, ErrInvalidUsername},
		{Account{Username: "asha", Password: "pw", Domain: "localhost"}, true, ErrInvalidDomain},
		{Account{Username: "asha", Password: "pw", Domain: "x.example", Server: "x.example:abc"}, true, ErrInvalidServer},
		{Account{Username: "asha", Password: "pw", Domain: "x.example", Server: "bad host.example"}, true, ErrInvalidServer},
		{Account{Username: "asha", Password: "pw", Domain: "x.example", Port: 70000}, true, ErrInvalidPort},
		{Account{Username: "asha", Domain: "x.example"}, true, ErrNoPassword},
		{Account{Username: "asha", Domain: "x.example"}, false, nil},
	} {
		if _, err := Normalize(tc.a, tc.isNew); !errors.Is(err, tc.want) {
			t.Errorf("Normalize(%+v, %v) = %v, want %v", tc.a, tc.isNew, err, tc.want)
		}
	}
}

func TestAccounts(t *testing.T) {
	db := testutil.NewDB(t, "softphone")
	db.MustExec(`INSERT INTO tenants (id, slug, "name") VALUES (2, 'acme', 'Acme'), (3, 'globex', 'Globex')`)
	var alice, bob, gina int
	db.Get(&alice, `INSERT INTO users (tenant_id, type, first_name, email) VALUES (2, 'agent', 'Alice', 'alice@acme.io') RETURNING id`)
	db.Get(&bob, `INSERT INTO users (tenant_id, type, first_name, email) VALUES (2, 'agent', 'Bob', 'bob@acme.io') RETURNING id`)
	db.Get(&gina, `INSERT INTO users (tenant_id, type, first_name, email) VALUES (3, 'agent', 'Gina', 'gina@globex.io') RETURNING id`)
	lo := logf.New(logf.Opts{})
	m, err := New(Opts{DB: db, Lo: &lo, EncryptionKey: key})
	if err != nil {
		t.Fatal(err)
	}

	if _, found, err := m.Get(2, alice); err != nil || found {
		t.Fatalf("before saving: found %v, %v", found, err)
	}
	if err := m.Save(2, alice, Account{Username: "alice", Domain: "acme.voip.example"}); !errors.Is(err, ErrNoPassword) {
		t.Fatalf("new account without a password: %v", err)
	}
	if err := m.Save(2, alice, Account{Username: "alice", Password: "s3cret", Domain: "acme.voip.example"}); err != nil {
		t.Fatal(err)
	}
	var stored string
	db.Get(&stored, `SELECT password FROM agent_sip_accounts WHERE user_id = $1`, alice)
	if stored == "s3cret" || stored == "" {
		t.Fatal("password stored in the clear")
	}

	// A save without a password keeps the one saved.
	if err := m.Save(2, alice, Account{Username: "alice2", Domain: "acme.voip.example", Server: "ws.voip.example", Port: 8443}); err != nil {
		t.Fatal(err)
	}
	a, found, err := m.Get(2, alice)
	if err != nil || !found || a.Username != "alice2" || a.Password != "s3cret" || a.Server != "ws.voip.example" || a.Port != 8443 || !a.RingBrowser {
		t.Fatalf("got %+v, %v, %v", a, found, err)
	}

	if err := m.Save(2, bob, Account{Username: "ALICE2", Password: "x", Domain: "acme.voip.example"}); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("taken username: %v", err)
	}
	// Another workspace may use the same SIP username.
	if err := m.Save(3, gina, Account{Username: "alice2", Password: "x", Domain: "globex.voip.example"}); err != nil {
		t.Fatal(err)
	}
	// An agent of another workspace isn't reachable.
	if _, found, _ := m.Get(3, alice); found {
		t.Fatal("another workspace's agent's account was found")
	}

	if ok, err := m.SetRingBrowser(2, alice, false); err != nil || !ok {
		t.Fatalf("ring browser off: %v, %v", ok, err)
	}
	if a, _, _ := m.Get(2, alice); a.RingBrowser {
		t.Fatal("still rings the browser")
	}
	if ok, _ := m.SetRingBrowser(2, bob, true); ok {
		t.Fatal("switched browser calling for an agent without an account")
	}

	if err := m.Delete(2, alice); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := m.Get(2, alice); found {
		t.Fatal("account left after delete")
	}
}
