// Package template manages templates including creation, retrieval and rendering.
package template

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"sync"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/template/models"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs                   embed.FS
	ErrTemplateNotFound   = errors.New("template not found")
	TypeEmailOutgoing     = "email_outgoing"
	TypeEmailNotification = "email_notification"
)

// Manager handles template-related operations.
type Manager struct {
	mutex   sync.RWMutex
	tpls    *template.Template
	webTpls *template.Template
	funcMap template.FuncMap
	q       queries
	lo      *logf.Logger
	i18n    *i18ncat.Catalog

	// tenantFuncs returns the template functions that differ per tenant, or nil.
	tenantFuncs func(tenantID int) template.FuncMap
	// baseTpls and baseWebTpls are never executed, so they can be cloned into
	// a tenant's template sets, which are cached in tenantTpls.
	baseTpls    *template.Template
	baseWebTpls *template.Template
	tenantTpls  map[tenantSetKey]*template.Template
}

// tenantSetKey identifies a tenant's copy of the email or web template set.
type tenantSetKey struct {
	tenantID int
	web      bool
}

// queries contains prepared SQL queries.
type queries struct {
	InsertTemplate     *sqlx.Stmt `query:"insert"`
	UpdateTemplate     *sqlx.Stmt `query:"update"`
	DeleteTemplate     *sqlx.Stmt `query:"delete"`
	GetDefaultTemplate *sqlx.Stmt `query:"get-default"`
	GetAllTemplates    *sqlx.Stmt `query:"get-all"`
	GetTemplate        *sqlx.Stmt `query:"get-template"`
	GetByName          *sqlx.Stmt `query:"get-by-name"`
	IsBuiltIn          *sqlx.Stmt `query:"is-builtin"`
}

// New creates and returns a new instance of the Manager.
func New(lo *logf.Logger, db *sqlx.DB, webTpls *template.Template, tpls *template.Template, funcMap template.FuncMap, i18n *i18ncat.Catalog) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, db, efs); err != nil {
		return nil, err
	}
	m := &Manager{
		mutex:   sync.RWMutex{},
		funcMap: funcMap,
		q:       q,
		lo:      lo,
		i18n:    i18n,
	}
	if err := m.setTemplates(webTpls, tpls); err != nil {
		return nil, err
	}
	return m, nil
}

// SetTenantFuncs sets the source of template functions that differ per tenant.
func (m *Manager) SetTenantFuncs(fn func(tenantID int) template.FuncMap) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.tenantFuncs = fn
	m.tenantTpls = make(map[tenantSetKey]*template.Template)
}

// setTemplates installs the template sets and keeps unexecuted copies to clone
// tenant sets from. Caller must hold the write lock or own m.
func (m *Manager) setTemplates(webTpls, tpls *template.Template) error {
	var err error
	if m.baseTpls, err = tpls.Clone(); err != nil {
		return fmt.Errorf("cloning email templates: %w", err)
	}
	if m.baseWebTpls, err = webTpls.Clone(); err != nil {
		return fmt.Errorf("cloning web templates: %w", err)
	}
	m.tpls, m.webTpls = tpls, webTpls
	m.tenantTpls = make(map[tenantSetKey]*template.Template)
	return nil
}

// funcsFor returns the template functions for the tenant.
func (m *Manager) funcsFor(tenantID int) template.FuncMap {
	var overrides template.FuncMap
	if m.tenantFuncs != nil {
		overrides = m.tenantFuncs(tenantID)
	}
	if len(overrides) == 0 {
		return m.funcMap
	}
	out := make(template.FuncMap, len(m.funcMap)+len(overrides))
	for k, v := range m.funcMap {
		out[k] = v
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

// setFor returns the tenant's copy of the email or web template set, the
// shared set when the tenant's functions don't differ. Caller must hold the
// read lock.
func (m *Manager) setFor(tenantID int, web bool) (*template.Template, error) {
	shared := m.tpls
	if web {
		shared = m.webTpls
	}
	var overrides template.FuncMap
	if m.tenantFuncs != nil {
		overrides = m.tenantFuncs(tenantID)
	}
	if len(overrides) == 0 {
		return shared, nil
	}

	key := tenantSetKey{tenantID: tenantID, web: web}
	m.mutex.RUnlock()
	m.mutex.Lock()
	defer func() {
		m.mutex.Unlock()
		m.mutex.RLock()
	}()
	if t, ok := m.tenantTpls[key]; ok {
		return t, nil
	}
	base := m.baseTpls
	if web {
		base = m.baseWebTpls
	}
	t, err := base.Clone()
	if err != nil {
		return nil, err
	}
	t.Funcs(overrides)
	m.tenantTpls[key] = t
	return t, nil
}

// Update updates a new template with the given name, and body.
func (m *Manager) Update(tenantID, id int, t models.Template) (models.Template, error) {
	var result models.Template
	if err := m.q.UpdateTemplate.Get(&result, tenantID, id, t.Name, t.Body, t.IsDefault, t.Subject, t.Type); err != nil {
		m.lo.Error("error updating template", "error", err)
		return models.Template{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return result, nil
}

// Create creates a template.
func (m *Manager) Create(tenantID int, t models.Template) (models.Template, error) {
	if t.IsDefault {
		t.Type = TypeEmailOutgoing
	}
	var result models.Template
	if err := m.q.InsertTemplate.Get(&result, tenantID, t.Name, t.Body, t.IsDefault, t.Subject, t.Type); err != nil {
		if dbutil.IsUniqueViolationError(err) && t.IsDefault {
			return models.Template{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("template.defaultTemplateAlreadyExists"), nil)
		}
		m.lo.Error("error inserting template", "error", err)
		return models.Template{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return result, nil
}

// GetAll returns all templates by type.
func (m *Manager) GetAll(tenantID int, typ string) ([]models.Template, error) {
	var templates = make([]models.Template, 0)
	if err := m.q.GetAllTemplates.Select(&templates, tenantID, typ); err != nil {
		m.lo.Error("error fetching templates", "error", err)
		return templates, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return templates, nil
}

// Get returns a template by id.
func (m *Manager) Get(tenantID, id int) (models.Template, error) {
	var templates = models.Template{}
	if err := m.q.GetTemplate.Get(&templates, tenantID, id); err != nil {
		if err == sql.ErrNoRows {
			return templates, envelope.NewError(envelope.NotFoundError, m.i18n.For(tenantID).T("validation.notFoundTemplate"), nil)
		}
		m.lo.Error("error fetching template", "error", err)
		return templates, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return templates, nil
}

// Delete deletes a template by id.
func (m *Manager) Delete(tenantID, id int) error {
	// Do not allow deletion of built-in templates.
	isBuiltIn, err := m.isBuiltIn(tenantID, id)
	if err != nil {
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if isBuiltIn {
		return envelope.NewError(envelope.PermissionError, m.i18n.For(tenantID).T("template.cannotDeleteBuiltInTemplate"), nil)
	}
	if _, err := m.q.DeleteTemplate.Exec(tenantID, id); err != nil {
		m.lo.Error("error deleting template", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// isBuiltIn returns true if the template is built-in.
func (m *Manager) isBuiltIn(tenantID, id int) (bool, error) {
	var isBuiltIn bool
	if err := m.q.IsBuiltIn.Get(&isBuiltIn, tenantID, id); err != nil {
		m.lo.Error("error fetching template", "error", err)
		return false, fmt.Errorf("error fetching template(%d) by id: %w", id, err)
	}
	return isBuiltIn, nil
}

// getDefaultOutgoingEmailTemplate returns the default outgoing email template.
func (m *Manager) getDefaultOutgoingEmailTemplate(tenantID int) (models.Template, error) {
	var template models.Template
	if err := m.q.GetDefaultTemplate.Get(&template, tenantID); err != nil {
		if err == sql.ErrNoRows {
			return template, ErrTemplateNotFound
		}
		m.lo.Error("error fetching default template", "error", err)
		return template, fmt.Errorf("error fetching default template: %w", err)
	}
	return template, nil
}

// getByName returns a template by name.
func (m *Manager) getByName(tenantID int, name string) (models.Template, error) {
	var template models.Template
	if err := m.q.GetByName.Get(&template, tenantID, name); err != nil {
		if err == sql.ErrNoRows {
			return template, ErrTemplateNotFound
		}
		m.lo.Error("error fetching default template", "error", err)
		return template, fmt.Errorf("error fetching template(%s) by name: %w", name, err)
	}
	return template, nil
}

// Reload reloads the templates and function map.
func (m *Manager) Reload(webTpls, tpls *template.Template, funcMap template.FuncMap) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.funcMap = funcMap
	return m.setTemplates(webTpls, tpls)
}
