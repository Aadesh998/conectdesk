// Package i18ncat holds every language pack and picks each tenant's own, so
// what the server writes (error messages, system notes, emails) is in the
// workspace's language. Without a tenant it acts as the installation's
// language, like a single *i18n.I18n.
package i18ncat

import (
	"strings"
	"sync"

	"github.com/knadh/go-i18n"
)

// Catalog serves language packs per language and per tenant.
type Catalog struct {
	load func(lang string) (*i18n.I18n, error)

	mu          sync.RWMutex
	byLang      map[string]*i18n.I18n
	langOf      func(tenantID int) string
	defaultLang func() string
	single      *i18n.I18n
}

// New returns a Catalog loading packs with load, a pack for a language code
// such as "de-DE"; defaultLang returns the installation's language.
func New(load func(lang string) (*i18n.I18n, error), defaultLang func() string) *Catalog {
	return &Catalog{load: load, byLang: make(map[string]*i18n.I18n), defaultLang: defaultLang}
}

// Single returns a Catalog that serves one pack to every tenant, for tests and
// callers with a single language.
func Single(i *i18n.I18n) *Catalog {
	return &Catalog{single: i}
}

// SetTenantLang sets the lookup of a tenant's language code. Until it is set,
// every tenant gets the installation's language.
func (c *Catalog) SetTenantLang(fn func(tenantID int) string) {
	c.mu.Lock()
	c.langOf = fn
	c.mu.Unlock()
}

// For returns the tenant's language pack.
func (c *Catalog) For(tenantID int) *i18n.I18n {
	if c.single != nil {
		return c.single
	}
	c.mu.RLock()
	langOf := c.langOf
	c.mu.RUnlock()
	lang := ""
	if langOf != nil && tenantID > 0 {
		lang = langOf(tenantID)
	}
	return c.Lang(lang)
}

// Lang returns the pack for a language code, the installation's for "" or a
// language that can't be loaded.
func (c *Catalog) Lang(lang string) *i18n.I18n {
	if c.single != nil {
		return c.single
	}
	lang = strings.TrimSpace(lang)
	if lang == "" {
		lang = c.defaultLang()
	}
	c.mu.RLock()
	i, ok := c.byLang[lang]
	c.mu.RUnlock()
	if ok {
		return i
	}

	i, err := c.load(lang)
	if err != nil || i == nil {
		if def := c.defaultLang(); lang != def {
			return c.Lang(def)
		}
		return i
	}
	c.mu.Lock()
	if cur, ok := c.byLang[lang]; ok {
		i = cur
	} else {
		c.byLang[lang] = i
	}
	c.mu.Unlock()
	return i
}

// Default returns the installation's language pack.
func (c *Catalog) Default() *i18n.I18n {
	return c.Lang("")
}

// Reload drops the loaded packs, so a changed installation language or pack
// takes effect.
func (c *Catalog) Reload() {
	c.mu.Lock()
	c.byLang = make(map[string]*i18n.I18n)
	c.mu.Unlock()
}

// T translates key in the installation's language.
func (c *Catalog) T(key string) string { return c.Default().T(key) }

// Ts translates key with params in the installation's language.
func (c *Catalog) Ts(key string, params ...string) string { return c.Default().Ts(key, params...) }

// Tc translates key for count n in the installation's language.
func (c *Catalog) Tc(key string, n int) string { return c.Default().Tc(key, n) }

// P returns key's plural form in the installation's language.
func (c *Catalog) P(key string) string { return c.Default().P(key) }

// S returns key's singular form in the installation's language.
func (c *Catalog) S(key string) string { return c.Default().S(key) }

// JSON returns the installation's language pack.
func (c *Catalog) JSON() []byte { return c.Default().JSON() }

// Code returns the installation's language code.
func (c *Catalog) Code() string { return c.Default().Code() }
