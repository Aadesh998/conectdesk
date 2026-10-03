package main

import (
	"strconv"

	cmodels "github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

func handleGetStatuses(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
	)
	out, err := app.status.GetAll(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(out)
}

func handleCreateStatus(r *fastglue.Request) error {
	var (
		app    = r.Context.(*App)
		status = cmodels.Status{}
	)
	if err := r.Decode(&status, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}

	if status.Name == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.empty", "name", "`name`"), nil, envelope.InputError)
	}

	createdStatus, err := app.status.Create(tenantID(r), status.Name, status.Category)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(createdStatus)
}

func handleDeleteStatus(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
	)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	err = app.status.Delete(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

func handleUpdateStatus(r *fastglue.Request) error {
	var (
		app    = r.Context.(*App)
		status = cmodels.Status{}
	)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	if err := r.Decode(&status, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}

	if status.Name == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.empty", "name", "`name`"), nil, envelope.InputError)
	}

	updatedStatus, err := app.status.Update(tenantID(r), id, status.Name, status.Category)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(updatedStatus)
}
