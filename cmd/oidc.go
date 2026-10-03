package main

import (
	"strconv"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/oidc/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// handleGetAllOIDC returns all OIDC records
func handleGetAllOIDC(r *fastglue.Request) error {
	app := r.Context.(*App)
	out, err := app.oidc.GetAll(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	// Replace secrets with dummy values.
	for i := range out {
		out[i].ClearSecrets()
	}
	return r.SendEnvelope(out)
}

// handleGetOIDC returns an OIDC record by id.
func handleGetOIDC(r *fastglue.Request) error {
	var app = r.Context.(*App)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	o, err := app.oidc.Get(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	o.ClearSecrets()
	return r.SendEnvelope(o)
}

// handleCreateOIDC creates a new OIDC record.
func handleCreateOIDC(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req = models.OIDC{}
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.GeneralError)
	}

	// Test OIDC provider URL by performing a discovery.
	if err := app.auth.TestProvider(req.ProviderURL); err != nil {
		return sendErrorEnvelope(r, err)
	}

	createdOIDC, err := app.oidc.Create(tenantID(r), req)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Reload the auth manager to update the OIDC providers.
	if err := reloadAuth(app); err != nil {
		app.lo.Error("error reloading auth", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Clear client secret before returning
	createdOIDC.ClearSecrets()

	return r.SendEnvelope(createdOIDC)
}

// handleUpdateOIDC updates an OIDC record.
func handleUpdateOIDC(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req = models.OIDC{}
	)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.GeneralError)
	}

	// Test OIDC provider URL by performing a discovery.
	if err := app.auth.TestProvider(req.ProviderURL); err != nil {
		return sendErrorEnvelope(r, err)
	}

	updatedOIDC, err := app.oidc.Update(tenantID(r), id, req)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Reload the auth manager to update the OIDC providers.
	if err := reloadAuth(app); err != nil {
		app.lo.Error("error reloading auth", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Clear client secret before returning
	updatedOIDC.ClearSecrets()

	return r.SendEnvelope(updatedOIDC)
}

// handleDeleteOIDC deletes an OIDC record.
func handleDeleteOIDC(r *fastglue.Request) error {
	var app = r.Context.(*App)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	if err = app.oidc.Delete(tenantID(r), id); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}
