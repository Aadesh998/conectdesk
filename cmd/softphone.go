package main

import (
	"errors"
	"strconv"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/softphone"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// softphoneRequest is an agent's SIP account as an admin enters it. An empty
// password keeps the one saved.
type softphoneRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Domain   string `json:"domain"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
}

// softphoneError is a SIP account that couldn't be saved, in words.
func softphoneError(app *App, tid int, err error) error {
	key := ""
	switch {
	case errors.Is(err, softphone.ErrInvalidUsername):
		key = "softphone.invalidUsername"
	case errors.Is(err, softphone.ErrInvalidDomain):
		key = "softphone.invalidDomain"
	case errors.Is(err, softphone.ErrInvalidServer):
		key = "softphone.invalidServer"
	case errors.Is(err, softphone.ErrInvalidPort):
		key = "softphone.invalidPort"
	case errors.Is(err, softphone.ErrNoPassword):
		key = "softphone.passwordRequired"
	case errors.Is(err, softphone.ErrUsernameTaken):
		key = "softphone.usernameTaken"
	default:
		return envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil)
	}
	return envelope.NewError(envelope.InputError, app.i18n.For(tid).T(key), nil)
}

// softphoneAgentID returns the agent an admin route names.
func softphoneAgentID(r *fastglue.Request) (int, error) {
	app := r.Context.(*App)
	tid := tenantID(r)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id <= 0 {
		return 0, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("globals.messages.badRequest"), nil)
	}
	agent, err := app.user.GetAgent(tid, id, "")
	if err != nil {
		return 0, err
	}
	return agent.ID, nil
}

// handleGetAgentSoftphone returns an agent's SIP account for admins, without
// its password.
func handleGetAgentSoftphone(r *fastglue.Request) error {
	app := r.Context.(*App)
	tid := tenantID(r)
	id, err := softphoneAgentID(r)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	acc, found, err := app.softphone.Get(tid, id)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	if !found {
		return r.SendEnvelope(map[string]any{"configured": false})
	}
	return r.SendEnvelope(map[string]any{
		"configured":   true,
		"username":     acc.Username,
		"domain":       acc.Domain,
		"server":       acc.Server,
		"port":         acc.Port,
		"ring_browser": acc.RingBrowser,
	})
}

// handleUpdateAgentSoftphone sets an agent's SIP account.
func handleUpdateAgentSoftphone(r *fastglue.Request) error {
	app := r.Context.(*App)
	tid := tenantID(r)
	id, err := softphoneAgentID(r)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	var req softphoneRequest
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if err := app.softphone.Save(tid, id, softphone.Account{
		Username: req.Username, Password: req.Password, Domain: req.Domain, Server: req.Server, Port: req.Port,
	}); err != nil {
		return sendErrorEnvelope(r, softphoneError(app, tid, err))
	}
	return handleGetAgentSoftphone(r)
}

// handleDeleteAgentSoftphone removes an agent's SIP account: their calls
// ring their phone again.
func handleDeleteAgentSoftphone(r *fastglue.Request) error {
	app := r.Context.(*App)
	tid := tenantID(r)
	id, err := softphoneAgentID(r)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := app.softphone.Delete(tid, id); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	return r.SendEnvelope(true)
}

// handleGetOwnSoftphone returns the agent's own SIP account, password
// included, for their browser to sign in with.
func handleGetOwnSoftphone(r *fastglue.Request) error {
	app := r.Context.(*App)
	tid := tenantID(r)
	user := r.RequestCtx.UserValue("user").(amodels.User)
	acc, found, err := app.softphone.Get(tid, user.ID)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	r.RequestCtx.Response.Header.Set("Cache-Control", "no-store")
	if !found {
		return r.SendEnvelope(map[string]any{"configured": false})
	}
	return r.SendEnvelope(map[string]any{
		"configured":   true,
		"username":     acc.Username,
		"password":     acc.Password,
		"domain":       acc.Domain,
		"server":       acc.Server,
		"port":         acc.Port,
		"ring_browser": acc.RingBrowser,
	})
}

// handleUpdateOwnSoftphone switches whether the agent's calls ring their
// browser or their phone.
func handleUpdateOwnSoftphone(r *fastglue.Request) error {
	app := r.Context.(*App)
	tid := tenantID(r)
	user := r.RequestCtx.UserValue("user").(amodels.User)
	var req struct {
		RingBrowser bool `json:"ring_browser"`
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	ok, err := app.softphone.SetRingBrowser(tid, user.ID, req.RingBrowser)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("softphone.notConfigured"), nil, envelope.InputError)
	}
	return r.SendEnvelope(true)
}
