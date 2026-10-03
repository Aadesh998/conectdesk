// Package tag handles the management of tags.
package tag

import (
	"embed"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/tag/models"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
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
	GetAllTags *sqlx.Stmt `query:"get-all-tags"`
	InsertTag  *sqlx.Stmt `query:"insert-tag"`
	DeleteTag  *sqlx.Stmt `query:"delete-tag"`
	UpdateTag  *sqlx.Stmt `query:"update-tag"`
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

// GetAll retrieves all tags.
func (t *Manager) GetAll(tenantID int) ([]models.Tag, error) {
	var tags = make([]models.Tag, 0)
	if err := t.q.GetAllTags.Select(&tags, tenantID); err != nil {
		t.lo.Error("error fetching tags", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, t.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return tags, nil
}

// Create creates a new tag.
func (t *Manager) Create(tenantID int, name string) (models.Tag, error) {
	var tag models.Tag
	if err := t.q.InsertTag.Get(&tag, tenantID, name); err != nil {
		if dbutil.IsUniqueViolationError(err) {
			return tag, envelope.NewError(envelope.ConflictError, t.i18n.For(tenantID).T("errors.alreadyExistsTag"), nil)
		}
		t.lo.Error("error inserting tag", "error", err)
		return tag, envelope.NewError(envelope.GeneralError, t.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return tag, nil
}

// Delete deletes a tag by ID.
func (t *Manager) Delete(tenantID, id int) error {
	if _, err := t.q.DeleteTag.Exec(tenantID, id); err != nil {
		t.lo.Error("error deleting tag", "error", err)
		return envelope.NewError(envelope.GeneralError, t.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// Update updates a tag by id.
func (t *Manager) Update(tenantID, id int, name string) (models.Tag, error) {
	var tag models.Tag
	if err := t.q.UpdateTag.Get(&tag, tenantID, id, name); err != nil {
		t.lo.Error("error updating tag", "error", err)
		return tag, envelope.NewError(envelope.GeneralError, t.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return tag, nil
}
