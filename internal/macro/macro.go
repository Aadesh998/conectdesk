// Package macro provides functionality for managing templated text responses and actions.
package macro

import (
	"database/sql"
	"embed"
	"encoding/json"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/macro/models"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
)

// Manager is the macro manager.
type Manager struct {
	q    queries
	lo   *logf.Logger
	i18n *i18ncat.Catalog
}

// Predefined queries.
type queries struct {
	Get            *sqlx.Stmt `query:"get"`
	GetAll         *sqlx.Stmt `query:"get-all"`
	Create         *sqlx.Stmt `query:"create"`
	Update         *sqlx.Stmt `query:"update"`
	Delete         *sqlx.Stmt `query:"delete"`
	IncrUsageCount *sqlx.Stmt `query:"increment-usage-count"`
}

// Opts contains the dependencies for the macro manager.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18ncat.Catalog
}

// New initializes a macro manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs)
	if err != nil {
		return nil, err
	}
	return &Manager{q: q, lo: opts.Lo, i18n: opts.I18n}, nil
}

// Get returns a macro by ID.
func (m *Manager) Get(tenantID, id int) (models.Macro, error) {
	macro := models.Macro{}
	if err := m.q.Get.Get(&macro, tenantID, id); err != nil {
		if err == sql.ErrNoRows {
			return macro, envelope.NewError(envelope.NotFoundError, m.i18n.For(tenantID).T("validation.notFoundMacro"), nil)
		}
		m.lo.Error("error getting macro", "error", err)
		return macro, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return macro, nil
}

// Create adds a new macro.
func (m *Manager) Create(tenantID int, name, messageContent string, userID, teamID *int, visibility string, visibleWhen []string, actions json.RawMessage) (models.Macro, error) {
	var createdMacro models.Macro
	err := m.q.Create.Get(&createdMacro, tenantID, name, messageContent, userID, teamID, visibility, pq.StringArray(visibleWhen), actions)
	if err != nil {
		m.lo.Error("error creating macro", "error", err)
		return models.Macro{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return createdMacro, nil
}

// Update modifies an existing macro.
func (m *Manager) Update(tenantID, id int, name, messageContent string, userID, teamID *int, visibility string, visibleWhen []string, actions json.RawMessage) (models.Macro, error) {
	var updatedMacro models.Macro
	err := m.q.Update.Get(&updatedMacro, tenantID, id, name, messageContent, userID, teamID, visibility, pq.StringArray(visibleWhen), actions)
	if err != nil {
		m.lo.Error("error updating macro", "error", err)
		return models.Macro{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return updatedMacro, nil
}

// GetAll returns all macros.
func (m *Manager) GetAll(tenantID int) ([]models.Macro, error) {
	macros := make([]models.Macro, 0)
	err := m.q.GetAll.Select(&macros, tenantID)
	if err != nil {
		m.lo.Error("error fetching macros", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return macros, nil
}

// Delete deletes a macro by ID.
func (m *Manager) Delete(tenantID, id int) error {
	result, err := m.q.Delete.Exec(tenantID, id)
	if err != nil {
		m.lo.Error("error deleting macro", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return envelope.NewError(envelope.NotFoundError, m.i18n.For(tenantID).T("validation.notFoundMacro"), nil)
	}
	return nil
}

// IncrementUsageCount increments the usage count of a macro.
func (m *Manager) IncrementUsageCount(tenantID, id int) error {
	if _, err := m.q.IncrUsageCount.Exec(tenantID, id); err != nil {
		m.lo.Error("error incrementing usage count", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}
