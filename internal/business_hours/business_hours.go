// Package businesshours handles the management of business hours and holidays.
package businesshours

import (
	"database/sql"
	"embed"
	"errors"

	"github.com/conectdesk/conectdesk/internal/business_hours/models"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs                      embed.FS
	ErrBusinessHoursNotFound = errors.New("business hours not found")
)

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
	GetBusinessHours    *sqlx.Stmt `query:"get-business-hours"`
	GetAllBusinessHours *sqlx.Stmt `query:"get-all-business-hours"`
	InsertBusinessHours *sqlx.Stmt `query:"insert-business-hours"`
	DeleteBusinessHours *sqlx.Stmt `query:"delete-business-hours"`
	UpdateBusinessHours *sqlx.Stmt `query:"update-business-hours"`
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

// Get retrieves business hours.
func (m *Manager) Get(tenantID, id int) (models.BusinessHours, error) {
	var businessHours models.BusinessHours
	if err := m.q.GetBusinessHours.Get(&businessHours, tenantID, id); err != nil {
		if err == sql.ErrNoRows {
			return businessHours, ErrBusinessHoursNotFound
		}
		return businessHours, err
	}
	return businessHours, nil
}

// GetAll retrieves all business hours.
func (m *Manager) GetAll(tenantID int) ([]models.BusinessHours, error) {
	var hours = make([]models.BusinessHours, 0)
	if err := m.q.GetAllBusinessHours.Select(&hours, tenantID); err != nil {
		m.lo.Error("error fetching business hours", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return hours, nil
}

// Create creates new business hours.
func (m *Manager) Create(tenantID int, name string, description null.String, isAlwaysOpen bool, workingHrs, holidays types.JSONText) (models.BusinessHours, error) {
	var result models.BusinessHours
	if err := m.q.InsertBusinessHours.Get(&result, tenantID, name, description, isAlwaysOpen, workingHrs, holidays); err != nil {
		m.lo.Error("error inserting business hours", "error", err)
		return models.BusinessHours{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return result, nil
}

// Delete deletes business hours by ID.
func (m *Manager) Delete(tenantID, id int) error {
	if _, err := m.q.DeleteBusinessHours.Exec(tenantID, id); err != nil {
		if dbutil.IsForeignKeyError(err) {
			return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
		m.lo.Error("error deleting business hours", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// Update updates business hours by ID.
func (m *Manager) Update(tenantID, id int, name string, description null.String, isAlwaysOpen bool, workingHrs, holidays types.JSONText) (models.BusinessHours, error) {
	var result models.BusinessHours
	if err := m.q.UpdateBusinessHours.Get(&result, tenantID, id, name, description, isAlwaysOpen, workingHrs, holidays); err != nil {
		m.lo.Error("error updating business hours", "error", err)
		return models.BusinessHours{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return result, nil
}
