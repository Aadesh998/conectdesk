// Package csat contains the logic for managing CSAT.
package csat

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/conectdesk/conectdesk/internal/csat/models"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs                  embed.FS
	ErrCSATAlreadyExists = errors.New("CSAT already exists")
)

const (
	csatURL = "%s/csat/%s"
)

// Manager manages CSAT.
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
	Insert *sqlx.Stmt `query:"insert"`
	Get    *sqlx.Stmt `query:"get"`
	Update *sqlx.Stmt `query:"update"`
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

// Create creates a new CSAT for the given conversation ID, returning ErrCSATAlreadyExists if one already exists.
func (m *Manager) Create(tenantID, conversationID int) (models.CSATResponse, error) {
	var (
		uuid string
		rsp  models.CSATResponse
	)
	err := m.q.Insert.QueryRow(tenantID, conversationID).Scan(&uuid)
	if err != nil {
		if err == sql.ErrNoRows {
			return rsp, ErrCSATAlreadyExists
		}
		m.lo.Error("error creating CSAT", "error", err)
		return rsp, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return m.Get(tenantID, uuid)
}

// Get retrieves the CSAT for the given UUID.
func (m *Manager) Get(tenantID int, uuid string) (models.CSATResponse, error) {
	var csat models.CSATResponse
	err := m.q.Get.Get(&csat, tenantID, uuid)
	if err != nil {
		if err == sql.ErrNoRows {
			return csat, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("validation.notFoundCsatSurvey"), nil)
		}
		m.lo.Error("error getting CSAT", "error", err)
		return csat, err
	}
	return csat, nil
}

// UpdateResponse updates the CSAT response for the given csat.
func (m *Manager) UpdateResponse(tenantID int, uuid string, score int, feedback string, meta json.RawMessage) error {
	csat, err := m.Get(tenantID, uuid)
	if err != nil {
		return err
	}

	if csat.ResponseTimestamp.Valid {
		return envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("csat.alreadySubmitted"), nil)
	}

	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}

	_, err = m.q.Update.Exec(tenantID, uuid, score, feedback, meta)
	if err != nil {
		m.lo.Error("error updating CSAT", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// MakePublicURL returns the public URL for the given CSAT UUID.
func (m *Manager) MakePublicURL(appBaseURL, uuid string) string {
	return fmt.Sprintf(csatURL, appBaseURL, uuid)
}
