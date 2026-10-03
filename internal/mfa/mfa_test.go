package mfa

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/conectdesk/conectdesk/internal/totp"
	"github.com/zerodha/logf"
)

const key = "0123456789abcdef0123456789abcdef"

func TestTwoFactor(t *testing.T) {
	db := testutil.NewDB(t, "mfa")
	db.MustExec(`INSERT INTO tenants (id, slug, "name") VALUES (2, 'acme', 'Acme'), (3, 'globex', 'Globex')`)
	var alice, gina int
	db.Get(&alice, `INSERT INTO users (tenant_id, type, first_name, email) VALUES (2, 'agent', 'Alice', 'alice@acme.io') RETURNING id`)
	db.Get(&gina, `INSERT INTO users (tenant_id, type, first_name, email) VALUES (3, 'agent', 'Gina', 'gina@globex.io') RETURNING id`)
	lo := logf.New(logf.Opts{})
	m, err := New(Opts{DB: db, Lo: &lo, EncryptionKey: key})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0)
	m.now = func() time.Time { return now }

	if s, _ := m.Status(2, alice); s.Enabled {
		t.Fatal("enabled before setup")
	}
	if _, err := m.Verify(2, alice, "123456"); !errors.Is(err, ErrNotEnabled) {
		t.Fatalf("verify before setup: %v", err)
	}

	secret, _ := totp.NewSecret()
	if _, err := m.Enable(2, alice, secret, "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("enable with a wrong code: %v", err)
	}
	code, _ := totp.Code(secret, now)
	codes, err := m.Enable(2, alice, secret, code)
	if err != nil || len(codes) != RecoveryCodeCount {
		t.Fatalf("enable: %v, %d codes", err, len(codes))
	}
	var stored string
	db.Get(&stored, `SELECT totp_secret FROM users WHERE id = $1`, alice)
	if stored == secret || stored == "" {
		t.Fatal("secret stored in the clear")
	}

	// The code that enabled it can't log in; the next step's can, once, even racing.
	if _, err := m.Verify(2, alice, code); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("setup code reused: %v", err)
	}
	now = now.Add(totp.Step)
	next, _ := totp.Code(secret, now)
	var ok atomic.Int32
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.Verify(2, alice, next); err == nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 {
		t.Fatalf("a code was accepted %d times", ok.Load())
	}

	// Another workspace can't use acme's user with the right code.
	now = now.Add(totp.Step)
	third, _ := totp.Code(secret, now)
	if _, err := m.Verify(3, alice, third); err == nil {
		t.Fatal("acme's user verified through globex")
	}

	// Recovery codes work once each, typed any way.
	used, err := m.Verify(2, alice, "  "+codes[0]+" ")
	if err != nil || !used {
		t.Fatalf("recovery code: used=%v err=%v", used, err)
	}
	if _, err := m.Verify(2, alice, codes[0]); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("recovery code reused: %v", err)
	}
	if s, _ := m.Status(2, alice); s.RecoveryCodesLeft != RecoveryCodeCount-1 {
		t.Fatalf("codes left = %d", s.RecoveryCodesLeft)
	}
	fresh, err := m.NewRecoveryCodes(2, alice)
	if err != nil || len(fresh) != RecoveryCodeCount {
		t.Fatalf("new codes: %v", err)
	}
	if _, err := m.Verify(2, alice, codes[1]); !errors.Is(err, ErrInvalidCode) {
		t.Fatal("an old recovery code still works after replacing them")
	}

	if err := m.Disable(2, alice); err != nil {
		t.Fatal(err)
	}
	if s, _ := m.Status(2, alice); s.Enabled || s.RecoveryCodesLeft != 0 {
		t.Fatalf("after disable: %+v", s)
	}
	if _, err := m.NewRecoveryCodes(2, alice); !errors.Is(err, ErrNotEnabled) {
		t.Fatalf("recovery codes without 2FA: %v", err)
	}
	if _, err := m.Status(2, gina); !errors.Is(err, ErrNotEnabled) {
		t.Fatalf("globex's agent looked up through acme: %v", err)
	}
}
