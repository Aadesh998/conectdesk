package main

import (
	"strconv"
	"strings"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/context_link/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

func handleGetContextLinks(r *fastglue.Request) error {
	var app = r.Context.(*App)
	links, err := app.contextLink.GetAll(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	for i := range links {
		if links[i].Secret != "" {
			links[i].Secret = strings.Repeat(stringutil.PasswordDummy, 10)
		}
	}
	return r.SendEnvelope(links)
}

func handleGetContextLink(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	contextLink, err := app.contextLink.Get(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if contextLink.Secret != "" {
		contextLink.Secret = strings.Repeat(stringutil.PasswordDummy, 10)
	}
	return r.SendEnvelope(contextLink)
}

func handleCreateContextLink(r *fastglue.Request) error {
	var (
		app         = r.Context.(*App)
		contextLink = models.ContextLink{}
	)
	if err := r.Decode(&contextLink, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	if err := validateContextLink(app, tenantID(r), &contextLink); err != nil {
		return sendErrorEnvelope(r, err)
	}

	result, err := app.contextLink.Create(tenantID(r), contextLink)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	result.Secret = strings.Repeat(stringutil.PasswordDummy, 10)
	return r.SendEnvelope(result)
}

func handleUpdateContextLink(r *fastglue.Request) error {
	var (
		app         = r.Context.(*App)
		contextLink = models.ContextLink{}
		id, _       = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	if err := r.Decode(&contextLink, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	if err := validateContextLink(app, tenantID(r), &contextLink); err != nil {
		return sendErrorEnvelope(r, err)
	}

	result, err := app.contextLink.Update(tenantID(r), id, contextLink)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	result.Secret = strings.Repeat(stringutil.PasswordDummy, 10)
	return r.SendEnvelope(result)
}

func handleDeleteContextLink(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	if err := app.contextLink.Delete(tenantID(r), id); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

func handleToggleContextLink(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	result, err := app.contextLink.Toggle(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	result.Secret = strings.Repeat(stringutil.PasswordDummy, 10)
	return r.SendEnvelope(result)
}

// handleGetActiveContextLinks returns active context links for agents.
func handleGetActiveContextLinks(r *fastglue.Request) error {
	var app = r.Context.(*App)
	links, err := app.contextLink.GetActive(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(links)
}

// handleGetContextLinkURL generates the full URL with substituted variables and optional JWT.
func handleGetContextLinkURL(r *fastglue.Request) error {
	var (
		app              = r.Context.(*App)
		auser            = r.RequestCtx.UserValue("user").(amodels.User)
		id, _            = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		conversationUUID = string(r.RequestCtx.QueryArgs().Peek("conversation_uuid"))
	)
	if id <= 0 || conversationUUID == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	agent, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conv, err := enforceConversationAccess(app, tenantID(r), conversationUUID, agent)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	contextLink, err := app.contextLink.Get(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	url, err := app.contextLink.GenerateURL(contextLink, conv.Contact, conversationUUID, auser)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(url)
}

func validateContextLink(app *App, tenantID int, contextLink *models.ContextLink) error {
	if contextLink.Name == "" {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "`name`"), nil)
	}
	if contextLink.URLTemplate == "" {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "`url_template`"), nil)
	}
	if contextLink.Secret != "" && !strings.Contains(contextLink.Secret, stringutil.PasswordDummy) && len(contextLink.Secret) != 32 {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("contextLink.secretLengthError"), nil)
	}
	if contextLink.TokenExpirySeconds <= 0 {
		contextLink.TokenExpirySeconds = 1200
	}
	return nil
}
