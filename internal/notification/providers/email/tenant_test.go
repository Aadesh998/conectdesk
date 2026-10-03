package email

import (
	"errors"
	"testing"

	"github.com/conectdesk/conectdesk/internal/inbox/models"
	"github.com/zerodha/logf"
)

func TestTenantRouter(t *testing.T) {
	smtp := models.SMTPConfig{Host: "127.0.0.1", Port: 2525, MaxConns: 1, IdleTimeout: "5s", PoolWaitTimeout: "5s"}
	platform := Settings{Enabled: true, From: "Desk <noreply@yourapp.test>", SMTP: smtp}
	settings := map[int]Settings{
		2: {Enabled: true, From: "Acme Support <support@acme.test>", SMTP: smtp},
		3: {Enabled: false, From: "ignored@foo.test", SMTP: smtp},
	}
	loads := 0
	lo := logf.New(logf.Opts{})
	r := NewTenantRouter(TenantRouterOpts{
		Lo:               &lo,
		PlatformSettings: func() (Settings, error) { return platform, nil },
		Settings: func(id int) (Settings, error) {
			loads++
			return settings[id], nil
		},
		TenantName: func(id int) string { return map[int]string{2: "Acme", 3: "Foo"}[id] },
	})
	defer r.Close()

	for _, c := range []struct {
		tenant int
		own    bool
		from   string
	}{
		{2, true, "Acme Support <support@acme.test>"}, // its own mailer
		{3, false, `"Foo" <noreply@yourapp.test>`},    // the platform's, with its name
		{0, false, "Desk <noreply@yourapp.test>"},     // no tenant, such as signup
		{9, false, "Desk <noreply@yourapp.test>"},     // a tenant without a name keeps the address
	} {
		e, from, err := r.mailerFor(c.tenant)
		if err != nil || e == nil {
			t.Fatalf("tenant %d: %v", c.tenant, err)
		}
		if from != c.from || (c.tenant > 0 && r.UsesOwnMailer(c.tenant) != c.own) {
			t.Errorf("tenant %d: from %q own %v; want %q own %v", c.tenant, from, r.UsesOwnMailer(c.tenant), c.from, c.own)
		}
	}

	// Mailers are cached until invalidated.
	before := loads
	r.mailerFor(2)
	if loads != before {
		t.Error("mailer rebuilt without a settings change")
	}
	settings[2] = Settings{Enabled: false}
	r.Invalidate(2)
	if r.UsesOwnMailer(2) {
		t.Error("tenant 2 still uses its own mailer after switching it off")
	}
	if _, from, _ := r.mailerFor(2); from != `"Acme" <noreply@yourapp.test>` {
		t.Errorf("tenant 2 fallback from = %q", from)
	}

	// New platform settings apply once invalidated.
	platform.From = "YourApp <hello@yourapp.test>"
	r.InvalidatePlatform()
	if _, from, _ := r.mailerFor(0); from != "YourApp <hello@yourapp.test>" {
		t.Errorf("after changing the platform sender: %q", from)
	}

	// With the platform's mailer off too, nothing is configured.
	platform = Settings{Enabled: false}
	r.InvalidatePlatform()
	if r.Configured(3) || r.Configured(0) {
		t.Error("configured with no mailer anywhere")
	}
	if _, _, err := r.mailerFor(3); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}
