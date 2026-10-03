// Package priority handles the management of conversation priorities.
package priority

import (
	"embed"

	"github.com/conectdesk/conectdesk/internal/conversation/priority/models"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
)

// Manager handles changes to priorities.
type Manager struct {
	q    queries
	lo   *logf.Logger
	i18n *i18ncat.Catalog
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18ncat.Catalog
}

// queries contains prepared SQL queries.
type queries struct {
	GetAll *sqlx.Stmt `query:"get-all"`
	Get    *sqlx.Stmt `query:"get"`
}

// New creates and returns a new instance of the Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{
		q:    q,
		lo:   opts.Lo,
		i18n: opts.I18n,
	}, nil
}

// GetAll retrieves all priorities.
func (m *Manager) GetAll(tenantID int) ([]models.Priority, error) {
	var priorities = make([]models.Priority, 0)
	if err := m.q.GetAll.Select(&priorities, tenantID); err != nil {
		m.lo.Error("error fetching priorities", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return priorities, nil
}

// Get retrieves a priority by ID.
func (m *Manager) Get(tenantID, id int) (models.Priority, error) {
	var priority models.Priority
	if err := m.q.Get.Get(&priority, tenantID, id); err != nil {
		m.lo.Error("error fetching priority", "error", err)
		return priority, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return priority, nil
}
