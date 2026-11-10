package main

import (
	"cmp"
	"errors"
	"fmt"
	"html"
	"net/mail"
	"strconv"
	"strings"

	"github.com/conectdesk/conectdesk/internal/envelope"
	notifier "github.com/conectdesk/conectdesk/internal/notification"
	"github.com/conectdesk/conectdesk/internal/platform"
	tmodels "github.com/conectdesk/conectdesk/internal/tenant/models"
	"github.com/conectdesk/conectdesk/internal/user"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	realip "github.com/ferluci/fast-realip"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// Platform admins manage a workspace's agents from the dashboard: add one,
// give one a new password, suspend, reactivate or delete one. New passwords
// are generated unless the admin types one, emailed to the person, and shown
// to the admin once, to share another way when the email doesn't go out.

// credentialsResp is what the dashboard gets back after a password is set.
type credentialsResp struct {
	// Password is the new password, shown to the admin this once.
	Password string `json:"password"`
	// Emailed is set when the password went out by email; EmailError says why
	// it didn't.
	Emailed    bool   `json:"emailed"`
	EmailError string `json:"email_error,omitempty"`
}

// newPassword returns the typed password when it is strong, or a generated one
// for an empty one.
func newPassword(typed string) (string, error) {
	if typed == "" {
		return user.GeneratePassword()
	}
	if !user.IsStrongPassword(typed) {
		return "", errWeakPassword
	}
	return typed, nil
}

var errWeakPassword = errors.New("weak password")

// sendCredentials emails a person the password to log in at loginURL with,
// through the workspace's mailer (the platform's for tenantID 0 or a workspace
// without one), and reports whether it went out.
func sendCredentials(app *App, tenantID int, to, name, loginURL, password, reason string) credentialsResp {
	out := credentialsResp{Password: password}
	product := cmp.Or(ko.String("app.tenancy.product_name"), "ConnectDesk")
	greeting := "Hi,"
	if name = strings.TrimSpace(name); name != "" {
		greeting = "Hi " + html.EscapeString(name) + ","
	}
	content := fmt.Sprintf(`<p>%s</p>
<p>%s</p>
<p>Log in at <a href="%s">%s</a> with:</p>
<p>Email: <strong>%s</strong><br>Password: <strong style="font-family:monospace;font-size:1.1em">%s</strong></p>
<p>Keep this password to yourself. If you didn't expect this email, tell your administrator.</p>`,
		greeting, html.EscapeString(reason), html.EscapeString(loginURL), html.EscapeString(loginURL),
		html.EscapeString(to), html.EscapeString(password))
	err := app.notifier.SendSync(notifier.Message{
		TenantID:        tenantID,
		RecipientEmails: []string{to},
		Subject:         "Your " + product + " login",
		Content:         content,
		Provider:        notifier.ProviderEmail,
	})
	if err != nil {
		app.lo.Error("error emailing credentials", "tenant_id", tenantID, "error", err)
		out.EmailError = err.Error()
		return out
	}
	out.Emailed = true
	return out
}

// platformAgentParam returns the workspace and the agent the request names,
// sending the error itself when there's none. The System user and AI
// assistants aren't agents here.
func platformAgentParam(r *fastglue.Request) (tmodels.Tenant, umodels.User, bool) {
	app := r.Context.(*App)
	t, ok := platformWorkspaceParam(r)
	if !ok {
		return t, umodels.User{}, false
	}
	notFound := func() (tmodels.Tenant, umodels.User, bool) {
		r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("platform.noSuchAgent"), nil, envelope.NotFoundError)
		return t, umodels.User{}, false
	}
	uid, err := strconv.Atoi(r.RequestCtx.UserValue("uid").(string))
	if err != nil || uid <= 0 {
		return notFound()
	}
	a, err := app.user.GetAgent(t.ID, uid, "")
	if err != nil || a.Type != umodels.UserTypeAgent {
		return notFound()
	}
	if sys, err := app.user.GetSystemUser(t.ID); err == nil && sys.ID == a.ID {
		return notFound()
	}
	return t, a, true
}

// handlePlatformCreateAgent adds an agent to a workspace with a password,
// generated unless one is given, and emails it to them.
func handlePlatformCreateAgent(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			FirstName string   `json:"first_name"`
			LastName  string   `json:"last_name"`
			Email     string   `json:"email"`
			Roles     []string `json:"roles"`
			Password  string   `json:"password"`
			SendEmail bool     `json:"send_email"`
		}
	)
	t, ok := platformWorkspaceParam(r)
	if !ok {
		return nil
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	req.FirstName, req.LastName = strings.TrimSpace(req.FirstName), strings.TrimSpace(req.LastName)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.FirstName == "" || len(req.FirstName) > 140 || len(req.LastName) > 140 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("platform.agentNameRequired"), nil, envelope.InputError)
	}
	if a, err := mail.ParseAddress(req.Email); err != nil || a.Address != req.Email {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("validation.invalidEmail"), nil, envelope.InputError)
	}
	if len(req.Roles) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("platform.agentRoleRequired"), nil, envelope.InputError)
	}
	password, err := newPassword(req.Password)
	if err != nil {
		return sendPasswordError(r, err)
	}
	if err := checkPlanLimit(app, t.ID, planAgents, 1); err != nil {
		return sendErrorEnvelope(r, err)
	}
	a, err := app.user.CreateAgent(t.ID, req.FirstName, req.LastName, req.Email, req.Roles)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := app.user.SetAgentPassword(t.ID, a.ID, password); err != nil {
		return sendErrorEnvelope(r, err)
	}
	out := credentialsResp{Password: password}
	if req.SendEmail {
		out = sendCredentials(app, t.ID, req.Email, req.FirstName, app.tenant.URLFor(t), password,
			"A platform administrator created your account in the "+t.Name+" workspace.")
	}
	app.platform.Audit(platformAdmin(r), platform.ActionCreateAgent, t.ID, t.Slug,
		map[string]any{"email": req.Email, "roles": req.Roles, "emailed": out.Emailed}, realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(out)
}

// handlePlatformResetAgentPassword gives an agent a new password, generated
// unless one is given, and emails it to them.
func handlePlatformResetAgentPassword(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		ip  = realip.FromRequest(r.RequestCtx)
		req struct {
			Password  string `json:"password"`
			SendEmail bool   `json:"send_email"`
		}
	)
	t, a, ok := platformAgentParam(r)
	if !ok {
		return nil
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	password, err := newPassword(req.Password)
	if err != nil {
		return sendPasswordError(r, err)
	}
	if err := app.user.SetAgentPassword(t.ID, a.ID, password); err != nil {
		return sendErrorEnvelope(r, err)
	}
	admin := platformAdmin(r)
	// The workspace's own activity log shows it too, done by its System user.
	if sys, err := app.user.GetSystemUser(t.ID); err == nil {
		app.activityLog.PasswordSet(t.ID, sys.ID, "Platform support "+admin.Email, ip, a.ID, a.Email.String)
	}
	out := credentialsResp{Password: password}
	if req.SendEmail && a.Email.String != "" {
		out = sendCredentials(app, t.ID, a.Email.String, a.FirstName, app.tenant.URLFor(t), password,
			"A platform administrator set a new password for your account in the "+t.Name+" workspace. Your old password no longer works.")
	}
	app.platform.Audit(admin, platform.ActionResetAgentPassword, t.ID, t.Slug,
		map[string]any{"email": a.Email.String, "emailed": out.Emailed}, ip)
	return r.SendEnvelope(out)
}

// handlePlatformSetAgentStatus suspends an agent, logging them out at their
// next request, or reactivates them.
func handlePlatformSetAgentStatus(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			Enabled bool `json:"enabled"`
		}
	)
	t, a, ok := platformAgentParam(r)
	if !ok {
		return nil
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	if err := app.user.ToggleEnabled(t.ID, a.ID, umodels.UserTypeAgent, req.Enabled); err != nil {
		return sendErrorEnvelope(r, err)
	}
	app.user.InvalidateAgentCache(a.ID)
	if !req.Enabled {
		app.wsHub.KickUser(a.ID)
	}
	action := platform.ActionActivateAgent
	if !req.Enabled {
		action = platform.ActionSuspendAgent
	}
	app.platform.Audit(platformAdmin(r), action, t.ID, t.Slug, map[string]any{"email": a.Email.String}, realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(true)
}

// handlePlatformDeleteAgent deletes an agent, unassigning their open
// conversations, as deleting one under Admin, Agents does.
func handlePlatformDeleteAgent(r *fastglue.Request) error {
	app := r.Context.(*App)
	t, a, ok := platformAgentParam(r)
	if !ok {
		return nil
	}
	if err := app.user.SoftDeleteAgent(t.ID, a.ID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	app.user.InvalidateAgentCache(a.ID)
	app.wsHub.KickUser(a.ID)
	if err := app.conversation.UnassignOpen(t.ID, a.ID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	app.platform.Audit(platformAdmin(r), platform.ActionDeleteAgent, t.ID, t.Slug, map[string]any{"email": a.Email.String}, realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(true)
}

// sendPasswordError sends a password that isn't strong enough as an input error.
func sendPasswordError(r *fastglue.Request, err error) error {
	app := r.Context.(*App)
	if errors.Is(err, errWeakPassword) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("platform.weakPassword"), nil, envelope.InputError)
	}
	return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
}
