// Package search provides search functionality.
package search

import (
	"embed"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	models "github.com/conectdesk/conectdesk/internal/search/models"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
)

// Manager is the search manager
type Manager struct {
	q    queries
	lo   *logf.Logger
	i18n *i18ncat.Catalog
}

// Opts contains the options for creating a new search manager
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18ncat.Catalog
}

// queries contains all the prepared queries
type queries struct {
	SearchConversationsByRefNum       *sqlx.Stmt `query:"search-conversations-by-reference-number"`
	SearchConversationsByContactEmail *sqlx.Stmt `query:"search-conversations-by-contact-email"`
	SearchMessages                    *sqlx.Stmt `query:"search-messages"`
	SearchContacts                    *sqlx.Stmt `query:"search-contacts"`
}

// New creates a new search manager
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, lo: opts.Lo, i18n: opts.I18n}, nil
}

// Conversations searches conversations based on the query
func (s *Manager) Conversations(tenantID int, query string) ([]models.ConversationResult, error) {
	var refNumResults = make([]models.ConversationResult, 0)
	if err := s.q.SearchConversationsByRefNum.Select(&refNumResults, tenantID, query); err != nil {
		s.lo.Error("error searching conversations", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, s.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	var emailResults = make([]models.ConversationResult, 0)
	if err := s.q.SearchConversationsByContactEmail.Select(&emailResults, tenantID, query); err != nil {
		s.lo.Error("error searching conversations", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, s.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return append(refNumResults, emailResults...), nil
}

// Messages searches messages based on the query
func (s *Manager) Messages(tenantID int, query string) ([]models.MessageResult, error) {
	var results = make([]models.MessageResult, 0)
	if err := s.q.SearchMessages.Select(&results, tenantID, query); err != nil {
		s.lo.Error("error searching messages", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, s.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return results, nil
}

// Contacts searches contacts based on the query
func (s *Manager) Contacts(tenantID int, query string) ([]models.ContactResult, error) {
	var results = make([]models.ContactResult, 0)
	if err := s.q.SearchContacts.Select(&results, tenantID, query); err != nil {
		s.lo.Error("error searching contacts", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, s.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return results, nil
}
