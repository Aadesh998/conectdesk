package i18ncat

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/knadh/go-i18n"
)

func pack(code, hello string) *i18n.I18n {
	i, _ := i18n.New([]byte(`{"_.code":"` + code + `","_.name":"x","hello":"` + hello + `"}`))
	return i
}

func TestCatalog(t *testing.T) {
	var loads atomic.Int32
	def := "en-US"
	c := New(func(lang string) (*i18n.I18n, error) {
		loads.Add(1)
		switch lang {
		case "en-US":
			return pack("en-US", "Hello"), nil
		case "es-ES":
			return pack("es-ES", "Hola"), nil
		case "de-DE":
			return pack("de-DE", "Hallo"), nil
		}
		return nil, errors.New("no such language")
	}, func() string { return def })

	// Before tenant languages are known, everyone gets the installation's.
	if got := c.For(2).T("hello"); got != "Hello" {
		t.Fatalf("before SetTenantLang: %q", got)
	}
	c.SetTenantLang(func(tenantID int) string { return map[int]string{2: "es-ES", 3: "xx-XX"}[tenantID] })

	for _, tc := range []struct {
		tenant int
		want   string
	}{{2, "Hola"}, {3, "Hello"}, {4, "Hello"}, {0, "Hello"}} {
		if got := c.For(tc.tenant).T("hello"); got != tc.want {
			t.Errorf("For(%d) = %q, want %q", tc.tenant, got, tc.want)
		}
	}
	if c.T("hello") != "Hello" || c.Code() != "en-US" {
		t.Fatal("the catalog itself isn't the installation's language")
	}

	// Packs are loaded once.
	n := loads.Load()
	c.For(2).T("hello")
	if loads.Load() != n {
		t.Fatal("pack loaded again")
	}

	// The installation's language can change.
	def = "de-DE"
	c.Reload()
	if c.T("hello") != "Hallo" || c.For(2).T("hello") != "Hola" {
		t.Fatalf("after changing the installation language: %q / %q", c.T("hello"), c.For(2).T("hello"))
	}

	s := Single(pack("fr-FR", "Bonjour"))
	if s.For(7).T("hello") != "Bonjour" || s.T("hello") != "Bonjour" {
		t.Fatal("Single catalog")
	}
}
