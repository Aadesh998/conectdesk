package email

import (
	"errors"
	"fmt"
	"net/mail"
	"sync"
	"time"

	"github.com/conectdesk/conectdesk/internal/inbox/models"
	notifier "github.com/conectdesk/conectdesk/internal/notification"
	"github.com/zerodha/logf"
)

// ErrNotConfigured is returned when neither a tenant nor the platform has a
// mailer switched on.
var ErrNotConfigured = errors.New("no email notification mailer is configured")

// mailerTTL bounds how long a mailer built from settings is reused, in case a
// settings change doesn't invalidate it.
const mailerTTL = 10 * time.Minute

// Settings is a tenant's email notification mailer configuration.
type Settings struct {
	Enabled bool
	// From is the address mail is sent from, "Name <address>" or a bare address.
	From string
	SMTP models.SMTPConfig
}

// TenantRouter sends each tenant's notification email through that tenant's
// own mailer when it has one switched on, else through the platform's mailer
// with the tenant's name on the From address. Messages of no tenant (tenant
// ID 0), such as signup verification, go through the platform's mailer. It
// implements notifier.Notifier.
type TenantRouter struct {
	lo         *logf.Logger
	settings   func(tenantID int) (Settings, error)
	tenantName func(tenantID int) string

	platformMu       sync.RWMutex
	platformSettings func() (Settings, error)

	mu      sync.Mutex
	mailers map[int]cachedMailer
}

type cachedMailer struct {
	email   *Email // nil when the tenant has no mailer of its own
	from    string
	expires time.Time
}

// TenantRouterOpts contains options for creating a TenantRouter.
type TenantRouterOpts struct {
	Lo *logf.Logger
	// PlatformSettings returns the platform's mailer settings, which serve
	// tenants without a mailer of their own. SetPlatformSettings can set it later.
	PlatformSettings func() (Settings, error)
	// Settings returns a tenant's mailer settings.
	Settings func(tenantID int) (Settings, error)
	// TenantName returns a tenant's display name, used on the platform's From address.
	TenantName func(tenantID int) string
}

// NewTenantRouter returns a TenantRouter.
func NewTenantRouter(opts TenantRouterOpts) *TenantRouter {
	return &TenantRouter{
		lo:               opts.Lo,
		platformSettings: opts.PlatformSettings,
		settings:         opts.Settings,
		tenantName: opts.TenantName,
		mailers:    make(map[int]cachedMailer),
	}
}

// Name returns the name of the provider.
func (r *TenantRouter) Name() string {
	return notifier.ProviderEmail
}

// Send sends the message through the mailer of its tenant.
func (r *TenantRouter) Send(msg notifier.Message) error {
	e, from, err := r.mailerFor(msg.TenantID)
	if err != nil {
		return err
	}
	em := e.prepareEmail(msg.Subject, msg.Content, msg.RecipientEmails, msg)
	em.From = from
	return e.send(em)
}

// Configured reports whether the tenant's mail goes out, through its own
// mailer or the platform's.
func (r *TenantRouter) Configured(tenantID int) bool {
	_, _, err := r.mailerFor(tenantID)
	return err == nil
}

// UsesOwnMailer reports whether the tenant sends through its own mailer.
func (r *TenantRouter) UsesOwnMailer(tenantID int) bool {
	m, err := r.own(tenantID)
	return err == nil && m.email != nil
}

// SetPlatformSettings sets where the platform's mailer settings come from.
func (r *TenantRouter) SetPlatformSettings(fn func() (Settings, error)) {
	r.platformMu.Lock()
	r.platformSettings = fn
	r.platformMu.Unlock()
	r.Invalidate(platformKey)
}

// InvalidatePlatform drops the platform's mailer so the next message rebuilds
// it from its current settings.
func (r *TenantRouter) InvalidatePlatform() {
	r.Invalidate(platformKey)
}

// Invalidate drops the tenant's mailer so the next message rebuilds it from
// its current settings.
func (r *TenantRouter) Invalidate(tenantID int) {
	r.mu.Lock()
	m, ok := r.mailers[tenantID]
	delete(r.mailers, tenantID)
	r.mu.Unlock()
	if ok && m.email != nil {
		m.email.Close()
	}
}

// Close closes every mailer's connections.
func (r *TenantRouter) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, m := range r.mailers {
		if m.email != nil {
			m.email.Close()
		}
		delete(r.mailers, id)
	}
}

// platformKey caches the platform's mailer next to the tenants'.
const platformKey = -1

// mailerFor returns the mailer and From address for the tenant's mail.
func (r *TenantRouter) mailerFor(tenantID int) (*Email, string, error) {
	if tenantID > 0 {
		own, err := r.own(tenantID)
		if err != nil {
			return nil, "", err
		}
		if own.email != nil {
			return own.email, own.from, nil
		}
	}

	platform, err := r.own(platformKey)
	if err != nil {
		return nil, "", err
	}
	if platform.email == nil {
		return nil, "", ErrNotConfigured
	}
	if tenantID <= 0 {
		return platform.email, platform.from, nil
	}
	return platform.email, fromWithName(platform.from, r.tenantName(tenantID)), nil
}

// own returns the tenant's own mailer, building it from its settings when not cached.
func (r *TenantRouter) own(tenantID int) (cachedMailer, error) {
	r.mu.Lock()
	m, ok := r.mailers[tenantID]
	r.mu.Unlock()
	if ok && time.Now().Before(m.expires) {
		return m, nil
	}
	if ok {
		r.Invalidate(tenantID)
	}

	var s Settings
	var err error
	if tenantID == platformKey {
		r.platformMu.RLock()
		fn := r.platformSettings
		r.platformMu.RUnlock()
		if fn != nil {
			s, err = fn()
		}
	} else {
		s, err = r.settings(tenantID)
	}
	if err != nil {
		return cachedMailer{}, fmt.Errorf("loading mailer settings: %w", err)
	}
	m = cachedMailer{expires: time.Now().Add(mailerTTL)}
	if s.Enabled && s.SMTP.Host != "" {
		e, err := New([]models.SMTPConfig{s.SMTP}, Opts{Lo: r.lo, FromEmail: s.From})
		if err != nil {
			r.lo.Error("error building tenant mailer", "tenant_id", tenantID, "error", err)
			return cachedMailer{}, err
		}
		m.email, m.from = e, s.From
	}

	r.mu.Lock()
	if cur, ok := r.mailers[tenantID]; ok && cur.email != nil {
		// Built concurrently; keep the first and drop this one.
		r.mu.Unlock()
		if m.email != nil {
			m.email.Close()
		}
		return cur, nil
	}
	r.mailers[tenantID] = m
	r.mu.Unlock()
	return m, nil
}

// fromWithName puts a display name on a From address, keeping its address.
func fromWithName(from, name string) string {
	if name == "" {
		return from
	}
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return from
	}
	return (&mail.Address{Name: name, Address: addr.Address}).String()
}
