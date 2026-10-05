package main

import (
	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	realip "github.com/ferluci/fast-realip"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleLogin logs in the user and returns the user.
func handleLogin(r *fastglue.Request) error {
	var (
		app      = r.Context.(*App)
		ip       = realip.FromRequest(r.RequestCtx)
		loginReq loginRequest
	)

	// Decode JSON request.
	if err := r.Decode(&loginReq, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	if loginReq.Email == "" || loginReq.Password == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	// Verify email and password.
	user, err := app.user.VerifyPassword(tenantID(r), loginReq.Email, []byte(loginReq.Password))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Check if user is enabled.
	if !user.Enabled {
		return sendErrorEnvelope(r, envelope.NewError(envelope.PermissionError, app.i18n.For(tenantID(r)).T("user.accountDisabled"), nil))
	}

	// A workspace requiring single sign-on leaves password login to admins.
	if tenantConsts(app, tenantID(r)).RequireSSO && !user.HasAdminRole() {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.For(tenantID(r)).T("auth.ssoRequired"), nil, envelope.PermissionError)
	}

	// With two-factor login the password alone opens no session: the agent
	// answers a challenge with a code next.
	if st, err := app.mfa.Status(tenantID(r), user.ID); err == nil && st.Enabled {
		token, err := startTwoFactorChallenge(app, tenantID(r), user.ID)
		if err != nil {
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
		}
		return r.SendEnvelope(map[string]any{"two_factor_required": true, "two_factor_token": token})
	}

	return finishLogin(r, user, ip)
}

// finishLogin opens the session of a user who has proven who they are.
func finishLogin(r *fastglue.Request, user umodels.User, ip string) error {
	app := r.Context.(*App)
	if err := app.auth.SaveSession(tenantID(r), amodels.User{
		ID:        user.ID,
		Email:     user.Email.String,
		FirstName: user.FirstName,
		LastName:  user.LastName,
	}, r); err != nil {
		app.lo.Error("error saving session", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	// Set CSRF cookie if not already set.
	if err := app.auth.SetCSRFCookie(r); err != nil {
		app.lo.Error("error setting csrf cookie", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}

	// Update last login time.
	if err := app.user.UpdateLastLoginAt(tenantID(r), user.ID); err != nil {
		return sendErrorEnvelope(r, err)
	}

	app.user.InvalidateAgentCache(user.ID)

	// Insert activity log.
	if err := app.activityLog.Login(tenantID(r), user.ID, user.Email.String, ip); err != nil {
		app.lo.Error("error creating login activity log", "error", err)
	}

	return r.SendEnvelope(user)
}

// handleLogout logs out the user and redirects to the dashboard.
func handleLogout(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		ip    = realip.FromRequest(r.RequestCtx)
	)

	// Insert activity log.
	if err := app.activityLog.Logout(tenantID(r), auser.ID, auser.Email, ip); err != nil {
		app.lo.Error("error creating logout activity log", "error", err)
	}

	if err := app.auth.DestroySession(r); err != nil {
		app.lo.Error("error destroying session", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	// Add no-cache headers.
	r.RequestCtx.Response.Header.Add("Cache-Control",
		"no-store, no-cache, must-revalidate, post-check=0, pre-check=0")
	r.RequestCtx.Response.Header.Add("Pragma", "no-cache")
	r.RequestCtx.Response.Header.Add("Expires", "-1")
	return r.RedirectURI("/", fasthttp.StatusFound, nil, "")
}
