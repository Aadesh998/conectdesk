// Package retention deletes what workspaces choose not to keep: resolved
// conversations once they are older than the workspace's retention period,
// and, if it asks, contacts left without any conversation.
package retention

import (
	"context"
	"embed"
	"time"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
)

const (
	// MinDays is the shortest retention period a workspace can set.
	MinDays = 7
	// MaxDays is the longest, ten years.
	MaxDays = 3650

	defaultBatchSize = 500
)

// Policy is a tenant's retention policy. Tenants without one keep everything.
type Policy struct {
	TenantID int `db:"tenant_id"`
	// Days is how long a resolved conversation is kept after its last activity.
	Days int `db:"days"`
	// DeleteContacts also deletes contacts left without conversations and
	// inactive for Days.
	DeleteContacts bool `db:"delete_contacts"`
}

// Result is what applying a policy deleted.
type Result struct {
	Conversations int
	Contacts      int
	// Avatars are the stored avatar files of the deleted contacts, which
	// deleting the rows leaves in the store.
	Avatars []string
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB *sqlx.DB
	Lo *logf.Logger
	// BatchSize is how many rows one statement deletes; 500 by default.
	BatchSize int
}

type queries struct {
	GetPolicies         *sqlx.Stmt `query:"get-policies"`
	DeleteConversations *sqlx.Stmt `query:"delete-conversations"`
	DeleteContacts      *sqlx.Stmt `query:"delete-contacts"`
}

// Manager applies retention policies.
type Manager struct {
	q         queries
	lo        *logf.Logger
	batchSize int
}

// New returns a Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}
	return &Manager{q: q, lo: opts.Lo, batchSize: opts.BatchSize}, nil
}

// Policies returns the policies of every tenant that has one.
func (m *Manager) Policies() ([]Policy, error) {
	var out []Policy
	if err := m.q.GetPolicies.Select(&out); err != nil {
		m.lo.Error("error fetching retention policies", "error", err)
		return nil, err
	}
	return out, nil
}

// Apply deletes what the policy no longer keeps, in batches so no statement
// holds locks for long. Conversations go first, so their contacts can go in
// the same run.
func (m *Manager) Apply(ctx context.Context, p Policy) (Result, error) {
	var res Result
	if p.Days <= 0 {
		return res, nil
	}
	for ctx.Err() == nil {
		r, err := m.q.DeleteConversations.Exec(p.TenantID, p.Days, m.batchSize)
		if err != nil {
			m.lo.Error("error deleting expired conversations", "tenant_id", p.TenantID, "error", err)
			return res, err
		}
		n, _ := r.RowsAffected()
		res.Conversations += int(n)
		if int(n) < m.batchSize {
			break
		}
	}
	if !p.DeleteContacts {
		return res, ctx.Err()
	}
	for ctx.Err() == nil {
		var avatars []string
		if err := m.q.DeleteContacts.Select(&avatars, p.TenantID, p.Days, m.batchSize); err != nil {
			m.lo.Error("error deleting expired contacts", "tenant_id", p.TenantID, "error", err)
			return res, err
		}
		res.Contacts += len(avatars)
		for _, a := range avatars {
			if a != "" {
				res.Avatars = append(res.Avatars, a)
			}
		}
		if len(avatars) < m.batchSize {
			break
		}
	}
	return res, ctx.Err()
}

// Run applies every tenant's policy every interval, calling done with what
// each deleted, until ctx is cancelled.
func (m *Manager) Run(ctx context.Context, interval time.Duration, done func(Policy, Result)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if policies, err := m.Policies(); err == nil {
			for _, p := range policies {
				// An error is logged; what was deleted before it is still reported.
				res, _ := m.Apply(ctx, p)
				if res.Conversations > 0 || res.Contacts > 0 {
					m.lo.Info("applied retention policy", "tenant_id", p.TenantID, "days", p.Days,
						"conversations", res.Conversations, "contacts", res.Contacts)
					done(p, res)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
