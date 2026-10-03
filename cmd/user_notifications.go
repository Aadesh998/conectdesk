package main

import (
	"strconv"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

func handleGetUserNotifications(r *fastglue.Request) error {
	var (
		app    = r.Context.(*App)
		auser  = r.RequestCtx.UserValue("user").(amodels.User)
		limit  = 20
		offset = 0
	)
	if l := r.RequestCtx.QueryArgs().GetUintOrZero("limit"); l > 0 && l <= 100 {
		limit = l
	}
	if o := r.RequestCtx.QueryArgs().GetUintOrZero("offset"); o > 0 {
		offset = o
	}
	notifications, err := app.userNotification.GetAll(tenantID(r), auser.ID, limit, offset)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(notifications)
}

func handleGetUserNotificationStats(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	stats, err := app.userNotification.GetStats(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(stats)
}

func handleMarkNotificationAsRead(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	if err := app.userNotification.MarkAsRead(tenantID(r), id, auser.ID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

func handleMarkAllNotificationsAsRead(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)

	if err := app.userNotification.MarkAllAsRead(tenantID(r), auser.ID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

func handleDeleteNotification(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	if err := app.userNotification.Delete(tenantID(r), id, auser.ID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

func handleDeleteAllNotifications(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)

	if err := app.userNotification.DeleteAll(tenantID(r), auser.ID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}
