package oidc

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"

	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/oidc/models"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs         embed.FS
	redirectURL = "/api/v1/oidc/%d/finish"
)

// Manager handles oidc-related operations.
type Manager struct {
	q             queries
	lo            *logf.Logger
	i18n          *i18ncat.Catalog
	rootURL       func(tenantID int) string
	encryptionKey string
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB            *sqlx.DB
	Lo            *logf.Logger
	I18n          *i18ncat.Catalog
	EncryptionKey string
}

// queries contains prepared SQL queries.
type queries struct {
	GetAllOIDC *sqlx.Stmt `query:"get-all-oidc"`
	GetOIDC    *sqlx.Stmt `query:"get-oidc"`
	InsertOIDC *sqlx.Stmt `query:"insert-oidc"`
	UpdateOIDC *sqlx.Stmt `query:"update-oidc"`
	DeleteOIDC *sqlx.Stmt `query:"delete-oidc"`

	GetAllEnabledOIDC *sqlx.Stmt `query:"get-all-enabled-oidc"`
}

// New creates and returns a new instance of the oidc Manager. rootURL returns
// the URL a tenant is served on, which provider redirect URLs hang off.
func New(opts Opts, rootURL func(tenantID int) string) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{
		q:             q,
		lo:            opts.Lo,
		i18n:          opts.I18n,
		rootURL:       rootURL,
		encryptionKey: opts.EncryptionKey,
	}, nil
}

// Get returns an oidc by id.
func (o *Manager) Get(tenantID, id int) (models.OIDC, error) {
	var oidc models.OIDC
	if err := o.q.GetOIDC.Get(&oidc, tenantID, id); err != nil {
		if err == sql.ErrNoRows {
			return oidc, envelope.NewError(envelope.NotFoundError, o.i18n.For(tenantID).T("validation.notFoundOidcProvider"), nil)
		}

		o.lo.Error("error fetching oidc", "error", err)
		return oidc, envelope.NewError(envelope.GeneralError, o.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	o.decryptOIDC(&oidc)

	oidc.SetProviderLogo()
	oidc.RedirectURI = fmt.Sprintf(o.rootURL(tenantID)+redirectURL, oidc.ID)
	return oidc, nil
}

// GetAll retrieves all oidc.
func (o *Manager) GetAll(tenantID int) ([]models.OIDC, error) {
	var oidc = make([]models.OIDC, 0)
	if err := o.q.GetAllOIDC.Select(&oidc, tenantID); err != nil {
		o.lo.Error("error fetching oidc", "error", err)
		return oidc, envelope.NewError(envelope.GeneralError, o.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	o.decryptOIDCSlice(oidc)

	// Set logo and redirect URL for each record
	rootURL := o.rootURL(tenantID)
	for i := range oidc {
		oidc[i].RedirectURI = fmt.Sprintf(rootURL+redirectURL, oidc[i].ID)
		oidc[i].SetProviderLogo()
	}
	return oidc, nil
}

// GetAllEnabled returns the enabled providers of every tenant, each with the
// redirect URL of its own tenant.
func (o *Manager) GetAllEnabled() ([]models.OIDC, error) {
	var oidc = make([]models.OIDC, 0)
	if err := o.q.GetAllEnabledOIDC.Select(&oidc); err != nil {
		o.lo.Error("error fetching enabled oidc", "error", err)
		return oidc, envelope.NewError(envelope.GeneralError, o.i18n.T("globals.messages.somethingWentWrong"), nil)
	}
	o.decryptOIDCSlice(oidc)
	for i := range oidc {
		oidc[i].RedirectURI = fmt.Sprintf(o.rootURL(oidc[i].TenantID)+redirectURL, oidc[i].ID)
		oidc[i].SetProviderLogo()
	}
	return oidc, nil
}

// Create adds a new oidc.
func (o *Manager) Create(tenantID int, oidc models.OIDC) (models.OIDC, error) {
	if err := o.validateCredentials(oidc); err != nil {
		return models.OIDC{}, err
	}

	// Encrypt sensitive fields before saving
	encryptedClientID, encryptedClientSecret, err := o.encryptOIDC(oidc.ClientID, oidc.ClientSecret)
	if err != nil {
		return models.OIDC{}, envelope.NewError(envelope.GeneralError, o.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	var createdOIDC models.OIDC
	if err := o.q.InsertOIDC.Get(&createdOIDC, tenantID, oidc.Name, oidc.Provider, oidc.ProviderURL, encryptedClientID, encryptedClientSecret, oidc.LogoURL); err != nil {
		o.lo.Error("error inserting oidc", "error", err)
		return models.OIDC{}, envelope.NewError(envelope.GeneralError, o.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	o.decryptOIDC(&createdOIDC)

	return createdOIDC, nil
}

// Update updates a oidc by id.
func (o *Manager) Update(tenantID, id int, oidc models.OIDC) (models.OIDC, error) {
	current, err := o.Get(tenantID, id)
	if err != nil {
		return models.OIDC{}, err
	}

	// A masked secret keeps the stored one.
	if strings.Contains(oidc.ClientSecret, stringutil.PasswordDummy) {
		oidc.ClientSecret = current.ClientSecret
	}
	if err := o.validateCredentials(oidc); err != nil {
		return models.OIDC{}, err
	}

	// Encrypt sensitive fields before updating
	encryptedClientID, encryptedClientSecret, err := o.encryptOIDC(oidc.ClientID, oidc.ClientSecret)
	if err != nil {
		return models.OIDC{}, envelope.NewError(envelope.GeneralError, o.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	var updatedOIDC models.OIDC
	if err := o.q.UpdateOIDC.Get(&updatedOIDC, tenantID, id, oidc.Name, oidc.Provider, oidc.ProviderURL, encryptedClientID, encryptedClientSecret, oidc.Enabled, oidc.LogoURL); err != nil {
		o.lo.Error("error updating oidc", "error", err)
		return models.OIDC{}, envelope.NewError(envelope.GeneralError, o.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	o.decryptOIDC(&updatedOIDC)

	return updatedOIDC, nil
}

// Delete deletes a oidc by its id.
func (o *Manager) Delete(tenantID, id int) error {
	if _, err := o.q.DeleteOIDC.Exec(tenantID, id); err != nil {
		o.lo.Error("error deleting oidc", "error", err)
		return envelope.NewError(envelope.GeneralError, o.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// validateCredentials rejects a blank client id or secret; either one fails every token exchange, locking out OIDC-only agents with nothing to show the admin went wrong.
func (o *Manager) validateCredentials(oidc models.OIDC) error {
	if strings.TrimSpace(oidc.ClientID) == "" {
		return envelope.NewError(envelope.InputError, o.i18n.Ts("globals.messages.empty", "name", "`client_id`"), nil)
	}
	if strings.TrimSpace(oidc.ClientSecret) == "" {
		return envelope.NewError(envelope.InputError, o.i18n.Ts("globals.messages.empty", "name", "`client_secret`"), nil)
	}
	return nil
}

// encryptOIDC encrypts sensitive OIDC fields (ClientID and ClientSecret).
// Returns the encrypted values and any error encountered.
func (o *Manager) encryptOIDC(clientID, clientSecret string) (encClientID, encClientSecret string, err error) {
	encClientID, err = crypto.Encrypt(clientID, o.encryptionKey)
	if err != nil {
		o.lo.Error("error encrypting client_id", "error", err)
		return "", "", err
	}

	encClientSecret, err = crypto.Encrypt(clientSecret, o.encryptionKey)
	if err != nil {
		o.lo.Error("error encrypting client_secret", "error", err)
		return "", "", err
	}

	return encClientID, encClientSecret, nil
}

// Decrypt failures clear the field so the app stays usable across encryption_key rotation.
func (o *Manager) decryptOIDC(oidc *models.OIDC) {
	if oidc.ClientID != "" {
		decrypted, err := crypto.Decrypt(oidc.ClientID, o.encryptionKey)
		if err != nil {
			o.lo.Error("error decrypting client_id, clearing field", "error", err, "oidc_id", oidc.ID)
			oidc.ClientID = ""
		} else {
			oidc.ClientID = decrypted
		}
	}

	if oidc.ClientSecret != "" {
		decrypted, err := crypto.Decrypt(oidc.ClientSecret, o.encryptionKey)
		if err != nil {
			o.lo.Error("error decrypting client_secret, clearing field", "error", err, "oidc_id", oidc.ID)
			oidc.ClientSecret = ""
		} else {
			oidc.ClientSecret = decrypted
		}
	}
}

func (o *Manager) decryptOIDCSlice(oidcs []models.OIDC) {
	for i := range oidcs {
		o.decryptOIDC(&oidcs[i])
	}
}
