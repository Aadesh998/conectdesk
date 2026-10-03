package models

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
)

const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
)

// Tenant is a business (workspace) sharing the installation.
type Tenant struct {
	ID           int             `db:"id" json:"id"`
	CreatedAt    time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time       `db:"updated_at" json:"updated_at"`
	Slug         string          `db:"slug" json:"slug"`
	Name         string          `db:"name" json:"name"`
	CustomDomain null.String     `db:"custom_domain" json:"custom_domain"`
	Status       string          `db:"status" json:"status"`
	Plan         string          `db:"plan" json:"plan"`
	Meta         json.RawMessage `db:"meta" json:"meta"`
	// DeletionScheduledAt is when the tenant and all its data are deleted; it
	// stays suspended until then, and can be restored.
	DeletionScheduledAt null.Time `db:"deletion_scheduled_at" json:"deletion_scheduled_at"`
}

// Usage is what a tenant's plan limits cap.
type Usage struct {
	Agents  int `db:"agents" json:"agents"`
	Inboxes int `db:"inboxes" json:"inboxes"`
}

// Onboarding is which steps of the setup checklist a tenant has done.
type Onboarding struct {
	Inbox    bool `db:"inbox" json:"inbox"`
	Teammate bool `db:"teammate" json:"teammate"`
	Widget   bool `db:"widget" json:"widget"`
	Article  bool `db:"article" json:"article"`
	Reply    bool `db:"reply" json:"reply"`
}

// Active reports whether the tenant may serve requests.
func (t Tenant) Active() bool {
	return t.Status == StatusActive
}

// meta is what Tenant.Meta holds.
type meta struct {
	// Channels are the inbox channels the platform lets the tenant use; nil
	// (no key) leaves every channel open.
	Channels *[]string `json:"channels"`
}

// AllowedChannels returns the inbox channels the platform admin enabled for
// the tenant. all is true when they haven't restricted them.
func (t Tenant) AllowedChannels() (channels []string, all bool) {
	var m meta
	if len(t.Meta) == 0 || json.Unmarshal(t.Meta, &m) != nil || m.Channels == nil {
		return nil, true
	}
	return *m.Channels, false
}

// ChannelAllowed reports whether the tenant may have inboxes of channel.
func (t Tenant) ChannelAllowed(channel string) bool {
	channels, all := t.AllowedChannels()
	return all || slices.Contains(channels, channel)
}

// Domain is a hostname a tenant has added to serve on.
type Domain struct {
	ID         int       `db:"id" json:"id"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time `db:"updated_at" json:"updated_at"`
	Hostname   string    `db:"hostname" json:"hostname"`
	Token      string    `db:"token" json:"-"`
	VerifiedAt null.Time `db:"verified_at" json:"verified_at"`
	CheckedAt  null.Time `db:"checked_at" json:"checked_at"`
	// LastError says why the last verification failed.
	LastError string `db:"last_error" json:"last_error"`
	// FailedChecks counts daily re-checks in a row that found the record gone.
	FailedChecks int `db:"failed_checks" json:"failed_checks"`
	// Primary is set when the domain is the tenant's workspace address.
	Primary bool `db:"is_primary" json:"primary"`
	// HelpCenters names the help centers served on the domain.
	HelpCenters pq.StringArray `db:"help_centers" json:"help_centers"`
}

// Verified reports whether the tenant has proven it controls the domain.
func (d Domain) Verified() bool {
	return d.VerifiedAt.Valid
}
