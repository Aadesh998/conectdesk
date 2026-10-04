package main

import (
	"cmp"
	"encoding/base64"
	"errors"
	"net"
	"net/mail"
	"slices"
	"strconv"
	"strings"
	"time"

	auth_ "github.com/conectdesk/conectdesk/internal/auth"
	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	notifier "github.com/conectdesk/conectdesk/internal/notification"
	"github.com/conectdesk/conectdesk/internal/platform"
	pmodels "github.com/conectdesk/conectdesk/internal/platform/models"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/conectdesk/conectdesk/internal/tenant"
	tmodels "github.com/conectdesk/conectdesk/internal/tenant/models"
	"github.com/conectdesk/conectdesk/internal/totp"
	"github.com/conectdesk/conectdesk/internal/workspace"
	realip "github.com/ferluci/fast-realip"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"rsc.io/qr"
)

// The platform admin dashboard is served on its own host, admin.<base_domain>
// by default, with its own login. Its paths answer on no other host.
const (
	platformSessionCookie = "platform_session"
	platformCSRFCookie    = "platform_csrf"
	platformCSRFHeader    = "X-Platform-CSRF"
	platformAdminKey      = "platform_admin"

	// impersonatePath is served on workspace hosts; it turns a one-time link
	// from the dashboard into a login session.
	impersonatePath = "/impersonate"
)

// isPlatformPath reports whether path belongs to the platform dashboard.
func isPlatformPath(path string) bool {
	return path == "/platform" || strings.HasPrefix(path, "/platform/") || strings.HasPrefix(path, "/api/platform/")
}

// isPlatformHost reports whether host is the platform dashboard's host.
func isPlatformHost(app *App, host string) bool {
	if app.platform == nil || app.platformHost == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.EqualFold(strings.TrimSuffix(host, "."), app.platformHost)
}

// platformAuth lets the request through only with a platform session on the
// platform host, and for changes only with the CSRF header matching its cookie.
func platformAuth(handler fastglue.FastRequestHandler) fastglue.FastRequestHandler {
	return func(r *fastglue.Request) error {
		app := r.Context.(*App)
		if !isPlatformHost(app, string(r.RequestCtx.Host())) || tenantFromReq(r).ID != 0 {
			return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
		}
		admin, err := app.platform.SessionAdmin(string(r.RequestCtx.Request.Header.Cookie(platformSessionCookie)))
		if err != nil {
			if !errors.Is(err, platform.ErrNoSession) {
				return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
			}
			return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, app.i18n.For(tenantID(r)).T("platform.notLoggedIn"), nil, envelope.PermissionError)
		}
		if !r.RequestCtx.IsGet() {
			cookie := string(r.RequestCtx.Request.Header.Cookie(platformCSRFCookie))
			if cookie == "" || cookie != string(r.RequestCtx.Request.Header.Peek(platformCSRFHeader)) {
				return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.For(tenantID(r)).T("auth.csrfTokenMismatch"), nil, envelope.PermissionError)
			}
		}
		// Admins without two-factor login can only set it up while it's required.
		if platformRequires2FA() && !admin.TOTPEnabled && !strings.HasPrefix(string(r.RequestCtx.Path()), "/api/platform/2fa") &&
			string(r.RequestCtx.Path()) != "/api/platform/me" && string(r.RequestCtx.Path()) != "/api/platform/logout" {
			return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.T("auth.twoFactorSetupRequired"),
				map[string]bool{"two_factor_setup_required": true}, envelope.PermissionError)
		}
		r.RequestCtx.SetUserValue(platformAdminKey, admin)
		return handler(r)
	}
}

func platformAdmin(r *fastglue.Request) pmodels.Admin {
	a, _ := r.RequestCtx.UserValue(platformAdminKey).(pmodels.Admin)
	return a
}

// handlePlatformPage serves the dashboard, which logs in and loads its data
// through the platform API.
func handlePlatformPage(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !isPlatformHost(app, string(r.RequestCtx.Host())) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	r.RequestCtx.Response.Header.Set("X-Frame-Options", "DENY")
	r.RequestCtx.Response.Header.Set("Cache-Control", "no-store")
	return app.tmpl.RenderWebPage(tenant.DefaultID, r.RequestCtx, "platform", map[string]any{
		"Data": map[string]any{"BaseDomain": app.tenant.BaseDomain()},
	})
}

// handlePlatformLogin logs a platform admin in.
func handlePlatformLogin(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		ip  = realip.FromRequest(r.RequestCtx)
		req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
	)
	if !isPlatformHost(app, string(r.RequestCtx.Host())) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	admin, err := app.platform.Authenticate(req.Email, req.Password)
	if err != nil {
		if errors.Is(err, platform.ErrInvalidCredentials) {
			app.lo.Warn("failed platform admin login", "email", req.Email, "ip", ip)
			return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, app.i18n.For(tenantID(r)).T("platform.invalidCredentials"), nil, envelope.PermissionError)
		}
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	// With two-factor login the password alone opens no session.
	if admin.TOTPEnabled {
		token, err := stringutil.RandomAlphanumeric(40)
		if err == nil {
			key := "platform_mfa:" + token
			err = app.redis.HSet(ctx, key, "admin_id", admin.ID).Err()
			if err == nil {
				err = app.redis.Expire(ctx, key, twoFactorChallengeTTL).Err()
			}
		}
		if err != nil {
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
		}
		return r.SendEnvelope(map[string]any{"two_factor_required": true, "two_factor_token": token})
	}
	return finishPlatformLogin(r, admin, ip)
}

// handlePlatformLoginTwoFactor finishes a platform admin's password login with
// a code from their authenticator app or a recovery code.
func handlePlatformLoginTwoFactor(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		ip  = realip.FromRequest(r.RequestCtx)
		req struct {
			Token string `json:"two_factor_token"`
			Code  string `json:"code"`
		}
	)
	if !isPlatformHost(app, string(r.RequestCtx.Host())) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	if err := r.Decode(&req, "json"); err != nil || req.Token == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	key := "platform_mfa:" + req.Token
	idStr, err := app.redis.HGet(ctx, key, "admin_id").Result()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, app.i18n.T("auth.twoFactorExpired"), map[string]bool{"two_factor_expired": true}, envelope.PermissionError)
	}
	if n, _ := app.redis.HIncrBy(ctx, key, "attempts", 1).Result(); n > twoFactorMaxAttempts {
		app.redis.Del(ctx, key)
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, app.i18n.T("auth.twoFactorExpired"), map[string]bool{"two_factor_expired": true}, envelope.PermissionError)
	}
	id, _ := strconv.Atoi(idStr)
	admin, err := app.platform.AdminByID(id)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, app.i18n.T("auth.twoFactorExpired"), map[string]bool{"two_factor_expired": true}, envelope.PermissionError)
	}
	if err := app.platform.VerifyTwoFactor(admin, req.Code); err != nil {
		app.lo.Warn("failed platform admin two-factor code", "email", admin.Email, "ip", ip)
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, app.i18n.T("auth.twoFactorInvalidCode"), nil, envelope.PermissionError)
	}
	app.redis.Del(ctx, key)
	return finishPlatformLogin(r, admin, ip)
}

// finishPlatformLogin opens the session of an admin who has proven who they are.
func finishPlatformLogin(r *fastglue.Request, admin pmodels.Admin, ip string) error {
	app := r.Context.(*App)
	token, err := app.platform.NewSession(admin, ip)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}

	csrf, err := stringutil.RandomAlphanumeric(32)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	setPlatformCookie(r, platformSessionCookie, token, true, platform.SessionTTL)
	setPlatformCookie(r, platformCSRFCookie, csrf, false, platform.SessionTTL)
	app.platform.Audit(admin, platform.ActionLogin, 0, "", nil, ip)
	return r.SendEnvelope(map[string]any{"email": admin.Email})
}

// handlePlatformLogout ends the platform session.
func handlePlatformLogout(r *fastglue.Request) error {
	app := r.Context.(*App)
	app.platform.Logout(string(r.RequestCtx.Request.Header.Cookie(platformSessionCookie)))
	setPlatformCookie(r, platformSessionCookie, "", true, -1)
	setPlatformCookie(r, platformCSRFCookie, "", false, -1)
	app.platform.Audit(platformAdmin(r), platform.ActionLogout, 0, "", nil, realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(true)
}

// handlePlatformMe returns the logged in platform admin.
func handlePlatformMe(r *fastglue.Request) error {
	a := platformAdmin(r)
	return r.SendEnvelope(map[string]any{
		"email":               a.Email,
		"plans":               platformPlans(),
		"default_plan":        cmp.Or(ko.String("app.tenancy.default_plan"), "free"),
		"channels":            inboxChannels,
		"totp_enabled":        a.TOTPEnabled,
		"two_factor_required": platformRequires2FA(),
	})
}

// platformRequires2FA reports whether platform admins must use two-factor
// login, app.tenancy.platform_require_2fa, on unless set false.
func platformRequires2FA() bool {
	return !ko.Exists("app.tenancy.platform_require_2fa") || ko.Bool("app.tenancy.platform_require_2fa")
}

// handlePlatformSetupTwoFactor starts setting up the admin's two-factor login.
func handlePlatformSetupTwoFactor(r *fastglue.Request) error {
	app := r.Context.(*App)
	a := platformAdmin(r)
	secret, err := totp.NewSecret()
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}
	if err := app.redis.Set(ctx, "platform_mfa_setup:"+strconv.Itoa(a.ID), secret, twoFactorSetupTTL).Err(); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}
	uri := totp.URI(secret, "Platform admin ("+app.tenant.BaseDomain()+")", a.Email)
	code, err := qr.Encode(uri, qr.M)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}
	code.Scale = 6
	return r.SendEnvelope(map[string]any{
		"secret": secret,
		"uri":    uri,
		"qr":     "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG()),
	})
}

// handlePlatformEnableTwoFactor switches the admin's two-factor login on once
// they type a code, and returns their recovery codes.
func handlePlatformEnableTwoFactor(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		a   = platformAdmin(r)
		req struct {
			Code string `json:"code"`
		}
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	secret, err := app.redis.Get(ctx, "platform_mfa_setup:"+strconv.Itoa(a.ID)).Result()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("auth.twoFactorSetupExpired"), nil, envelope.InputError)
	}
	codes, err := app.platform.EnableTwoFactor(a, secret, req.Code)
	if err != nil {
		if errors.Is(err, platform.ErrInvalidCode) {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("auth.twoFactorInvalidCode"), nil, envelope.InputError)
		}
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}
	app.redis.Del(ctx, "platform_mfa_setup:"+strconv.Itoa(a.ID))
	app.platform.Audit(a, platform.ActionEnable2FA, 0, "", nil, realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(map[string]any{"recovery_codes": codes})
}

// handlePlatformOverview sums up every workspace.
func handlePlatformOverview(r *fastglue.Request) error {
	app := r.Context.(*App)
	o, err := app.platform.Overview()
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	return r.SendEnvelope(o)
}

// platformWorkspace is a listed workspace with its URL.
type platformWorkspace struct {
	pmodels.Workspace
	URL string `json:"url"`
}

// handlePlatformWorkspaces returns a page of workspaces, searched and filtered.
func handlePlatformWorkspaces(r *fastglue.Request) error {
	var (
		app    = r.Context.(*App)
		args   = r.RequestCtx.QueryArgs()
		search = string(args.Peek("q"))
		status = string(args.Peek("status"))
	)
	page, _ := strconv.Atoi(string(args.Peek("page")))
	page = max(page, 1)
	const pageSize = 25
	if status != "" && status != tmodels.StatusActive && status != tmodels.StatusSuspended {
		status = ""
	}
	list, total, err := app.platform.Workspaces(search, status, page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	out := make([]platformWorkspace, 0, len(list))
	for _, w := range list {
		out = append(out, platformWorkspace{Workspace: w, URL: app.tenant.URLFor(tmodels.Tenant{Slug: w.Slug, CustomDomain: w.CustomDomain})})
	}
	return r.SendEnvelope(map[string]any{"results": out, "total": total, "page": page, "page_size": pageSize})
}

// handlePlatformWorkspace returns one workspace with its usage, agents,
// domains and the audit of what platform admins did to it.
func handlePlatformWorkspace(r *fastglue.Request) error {
	app := r.Context.(*App)
	t, ok := platformWorkspaceParam(r)
	if !ok {
		return nil
	}
	usage, err := planUsage(app, r.RequestCtx, t.ID)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	counts, err := app.platform.WorkspaceCounts(t.ID)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	agents, err := app.platform.WorkspaceAgents(t.ID)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	domains, err := app.tenant.Domains(t.ID)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	audit, err := app.platform.AuditLog(t.ID, 50)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}

	channels, allChannels := t.AllowedChannels()
	return r.SendEnvelope(map[string]any{
		"workspace":    t,
		"channels":     channels,
		"all_channels": allChannels,
		"url":          app.tenant.URLFor(t),
		"default":      t.ID == tenant.DefaultID,
		"usage":        usage.Usage,
		"limits":       usage.Limits,
		"counts":       counts,
		"fair_use":     usage.FairUse,
		"agents":       agents,
		"domains":      domains,
		"audit":        audit,
	})
}

// handlePlatformSetStatus suspends or activates a workspace.
func handlePlatformSetStatus(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			Status string `json:"status"`
		}
	)
	t, ok := platformWorkspaceParam(r)
	if !ok {
		return nil
	}
	if err := r.Decode(&req, "json"); err != nil || (req.Status != tmodels.StatusActive && req.Status != tmodels.StatusSuspended) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	if t.ID == tenant.DefaultID {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("platform.defaultWorkspace"), nil, envelope.InputError)
	}
	// Activating one scheduled for deletion would leave it scheduled; restore it.
	if t.DeletionScheduledAt.Valid {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("platform.restoreFirst"), nil, envelope.InputError)
	}
	if _, err := app.tenant.SetStatus(t.ID, req.Status); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	action := platform.ActionActivate
	if req.Status == tmodels.StatusSuspended {
		action = platform.ActionSuspend
	}
	app.platform.Audit(platformAdmin(r), action, t.ID, t.Slug, map[string]any{"from": t.Status}, realip.FromRequest(r.RequestCtx))
	return handlePlatformWorkspace(r)
}

// sendWorkspaceError answers a workspace that couldn't be created: which field
// is invalid, a taken slug, or a generic error.
func sendWorkspaceError(r *fastglue.Request, err error) error {
	app := r.Context.(*App)
	var ve workspace.ValidationError
	switch {
	case errors.As(err, &ve):
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("workspace.invalid."+ve.Field), map[string]string{"field": ve.Field}, envelope.InputError)
	case errors.Is(err, workspace.ErrSlugTaken):
		return r.SendErrorEnvelope(fasthttp.StatusConflict, app.i18n.For(tenantID(r)).T("workspace.slugTaken"), map[string]string{"field": "slug"}, envelope.ConflictError)
	}
	app.lo.Error("error creating workspace", "error", err)
	return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
}

// handlePlatformCreateWorkspace creates a workspace with its first admin, on
// a plan and with the inbox channels the platform admin picked.
func handlePlatformCreateWorkspace(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			workspace.Request
			Plan     string   `json:"plan"`
			Channels []string `json:"channels"`
			// AllChannels leaves every channel open, now and as channels are added.
			AllChannels bool `json:"all_channels"`
		}
	)
	if app.workspace == nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	if req.Plan != "" && !slices.Contains(platformPlans(), req.Plan) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("platform.unknownPlan"), nil, envelope.InputError)
	}
	opts := workspace.CreateOpts{Plan: req.Plan}
	if !req.AllChannels {
		channels, ok := platformChannels(req.Channels)
		if !ok {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("platform.unknownChannel"), nil, envelope.InputError)
		}
		opts.Channels = channels
	}
	t, err := app.workspace.CreateWith(r.RequestCtx, req.Request, opts)
	if err != nil {
		return sendWorkspaceError(r, err)
	}
	app.platform.Audit(platformAdmin(r), platform.ActionCreate, t.ID, t.Slug,
		map[string]any{"name": t.Name, "plan": t.Plan, "admin_email": strings.ToLower(strings.TrimSpace(req.Email)), "channels": opts.Channels},
		realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(map[string]any{"id": t.ID, "url": app.tenant.URLFor(t)})
}

// handlePlatformSetChannels sets the inbox channels a workspace may use. Its
// enabled inboxes of channels taken away are switched off.
func handlePlatformSetChannels(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			Channels    []string `json:"channels"`
			AllChannels bool     `json:"all_channels"`
		}
	)
	t, ok := platformWorkspaceParam(r)
	if !ok {
		return nil
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	channels, ok := platformChannels(req.Channels)
	if !ok && !req.AllChannels {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("platform.unknownChannel"), nil, envelope.InputError)
	}
	if req.AllChannels {
		channels = nil
	}
	from, fromAll := t.AllowedChannels()
	if _, err := app.tenant.SetChannels(t.ID, channels, req.AllChannels); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}

	var disabled []int
	if !req.AllChannels {
		ids, err := app.inbox.DisableOtherChannels(t.ID, channels)
		if err != nil {
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
		}
		for _, id := range ids {
			if err := reloadInbox(app, t.ID, id); err != nil {
				app.lo.Error("error reloading inbox", "tenant_id", t.ID, "id", id, "error", err)
			}
		}
		disabled = ids
	}
	app.platform.Audit(platformAdmin(r), platform.ActionSetChannels, t.ID, t.Slug, map[string]any{
		"from":             map[string]any{"channels": from, "all": fromAll},
		"to":               map[string]any{"channels": channels, "all": req.AllChannels},
		"disabled_inboxes": disabled,
	}, realip.FromRequest(r.RequestCtx))
	return handlePlatformWorkspace(r)
}

// platformChannels returns channels without repeats, in inboxChannels order;
// ok is false if one isn't an inbox channel.
func platformChannels(channels []string) ([]string, bool) {
	for _, c := range channels {
		if !slices.Contains(inboxChannels, c) {
			return nil, false
		}
	}
	out := []string{}
	for _, c := range inboxChannels {
		if slices.Contains(channels, c) {
			out = append(out, c)
		}
	}
	return out, true
}

// handlePlatformSetPlan moves a workspace to another configured plan.
func handlePlatformSetPlan(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			Plan string `json:"plan"`
		}
	)
	t, ok := platformWorkspaceParam(r)
	if !ok {
		return nil
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	if !slices.Contains(platformPlans(), req.Plan) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("platform.unknownPlan"), nil, envelope.InputError)
	}
	if _, err := app.tenant.SetPlan(t.ID, req.Plan); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	app.platform.Audit(platformAdmin(r), platform.ActionChangePlan, t.ID, t.Slug, map[string]any{"from": t.Plan, "to": req.Plan}, realip.FromRequest(r.RequestCtx))
	return handlePlatformWorkspace(r)
}

// handlePlatformDelete deletes a workspace and all its data, once its slug is
// typed to confirm.
func handlePlatformDelete(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			Confirm string `json:"confirm"`
			// Now deletes at once instead of after the grace period.
			Now bool `json:"now"`
		}
	)
	t, ok := platformWorkspaceParam(r)
	if !ok {
		return nil
	}
	if t.ID == tenant.DefaultID {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("platform.defaultWorkspace"), nil, envelope.InputError)
	}
	if err := r.Decode(&req, "json"); err != nil || strings.TrimSpace(req.Confirm) != t.Slug {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("platform.confirmSlug"), nil, envelope.InputError)
	}
	if !req.Now {
		at := time.Now().Add(deletionGrace())
		if _, err := app.tenant.ScheduleDeletion(t.ID, at); err != nil {
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
		}
		app.platform.Audit(platformAdmin(r), platform.ActionScheduleDelete, t.ID, t.Slug, map[string]any{"deletes_at": at.UTC()}, realip.FromRequest(r.RequestCtx))
		return handlePlatformWorkspace(r)
	}
	if err := deleteTenantData(app.tenant, app.media, t.ID); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	app.platform.Audit(platformAdmin(r), platform.ActionDelete, 0, t.Slug, map[string]any{"name": t.Name, "plan": t.Plan}, realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(true)
}

// handlePlatformRestore restores a workspace scheduled for deletion.
func handlePlatformRestore(r *fastglue.Request) error {
	app := r.Context.(*App)
	t, ok := platformWorkspaceParam(r)
	if !ok {
		return nil
	}
	if _, err := app.tenant.CancelDeletion(t.ID); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("platform.notScheduled"), nil, envelope.InputError)
	}
	app.platform.Audit(platformAdmin(r), platform.ActionRestore, t.ID, t.Slug, nil, realip.FromRequest(r.RequestCtx))
	return handlePlatformWorkspace(r)
}

// handlePlatformImpersonate returns a one-time link that logs the admin into
// the workspace as one of its agents.
func handlePlatformImpersonate(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			UserID int `json:"user_id"`
		}
	)
	t, ok := platformWorkspaceParam(r)
	if !ok {
		return nil
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	admin := platformAdmin(r)
	token, err := app.platform.Impersonate(admin, t.ID, req.UserID)
	if err != nil {
		if errors.Is(err, platform.ErrAgentNotFound) {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("platform.agentNotFound"), nil, envelope.InputError)
		}
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	app.platform.Audit(admin, platform.ActionImpersonate, t.ID, t.Slug, map[string]any{"user_id": req.UserID}, realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(map[string]any{
		"url":        app.tenant.URLFor(t) + impersonatePath + "?token=" + token,
		"expires_in": int(platform.ImpersonationTTL.Seconds()),
	})
}

// handlePlatformAudit returns the latest platform audit entries.
func handlePlatformAudit(r *fastglue.Request) error {
	app := r.Context.(*App)
	id, _ := strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("workspace_id")))
	entries, err := app.platform.AuditLog(id, 200)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	return r.SendEnvelope(entries)
}

// handleImpersonate is opened on a workspace host with a one-time link from
// the platform dashboard. It logs the browser in as the link's agent and
// records it in the workspace's activity log.
func handleImpersonate(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
		ip  = realip.FromRequest(r.RequestCtx)
	)
	if app.platform == nil || tid == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	imp, err := app.platform.ConsumeImpersonation(tid, string(r.RequestCtx.QueryArgs().Peek("token")))
	if err != nil {
		if errors.Is(err, platform.ErrInvalidLink) {
			return r.SendErrorEnvelope(fasthttp.StatusGone, app.i18n.For(tenantID(r)).T("platform.invalidLink"), nil, envelope.PermissionError)
		}
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	agent, err := app.user.GetAgent(tid, imp.UserID, "")
	if err != nil || !agent.Enabled {
		return r.SendErrorEnvelope(fasthttp.StatusGone, app.i18n.For(tenantID(r)).T("platform.invalidLink"), nil, envelope.PermissionError)
	}

	if err := app.auth.SaveSession(tid, amodels.User{
		ID:        agent.ID,
		Email:     agent.Email.String,
		FirstName: agent.FirstName,
		LastName:  agent.LastName,
	}, r, map[string]any{auth_.SessImpersonatedBy: imp.AdminEmail}); err != nil {
		app.lo.Error("error saving impersonation session", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	if err := app.auth.SetCSRFCookie(r); err != nil {
		app.lo.Error("error setting csrf cookie", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	if err := app.activityLog.PlatformImpersonation(tid, agent.ID, agent.Email.String, imp.AdminEmail, ip); err != nil {
		app.lo.Error("error recording impersonation activity", "error", err)
	}
	r.RequestCtx.Response.Header.Set("Referrer-Policy", "no-referrer")
	r.RequestCtx.Redirect("/", fasthttp.StatusFound)
	return nil
}

// platformWorkspaceParam returns the workspace named by the id route
// parameter. When there is none it has sent the error response, and ok is false.
func platformWorkspaceParam(r *fastglue.Request) (tmodels.Tenant, bool) {
	app := r.Context.(*App)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err == nil && id > 0 {
		t, err := app.tenant.Get(id)
		if err == nil {
			return t, true
		}
		if !errors.Is(err, tenant.ErrNotFound) {
			sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
			return t, false
		}
	}
	r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).T("tenant.notFound"), nil, envelope.NotFoundError)
	return tmodels.Tenant{}, false
}

// platformPlans returns the configured plan names.
func platformPlans() []string {
	plans := ko.MapKeys("app.tenancy.plans")
	slices.Sort(plans)
	return plans
}

func setPlatformCookie(r *fastglue.Request, name, value string, httpOnly bool, ttl time.Duration) {
	c := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(c)
	c.SetKey(name)
	c.SetValue(value)
	c.SetPath("/")
	c.SetHTTPOnly(httpOnly)
	c.SetSecure(!ko.Bool("app.server.disable_secure_cookies"))
	c.SetSameSite(fasthttp.CookieSameSiteStrictMode)
	if ttl < 0 {
		c.SetExpire(fasthttp.CookieExpireDelete)
	} else {
		c.SetMaxAge(int(ttl.Seconds()))
	}
	r.RequestCtx.Response.Header.SetCookie(c)
}

// handlePlatformGetMail returns the platform's mailer settings, without the
// password.
func handlePlatformGetMail(r *fastglue.Request) error {
	app := r.Context.(*App)
	ms, err := app.platform.MailSettings()
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}
	hasPassword := ms.Password != ""
	ms.Password = ""
	return r.SendEnvelope(map[string]any{"settings": ms, "has_password": hasPassword})
}

// handlePlatformSetMail saves the platform's mailer settings. An empty
// password keeps the saved one.
func handlePlatformSetMail(r *fastglue.Request) error {
	app := r.Context.(*App)
	var ms pmodels.MailSettings
	if err := r.Decode(&ms, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	ms.Host = strings.TrimSpace(ms.Host)
	ms.Username = strings.TrimSpace(ms.Username)
	ms.From = strings.TrimSpace(ms.From)
	if ms.Enabled {
		if ms.Host == "" || ms.Port <= 0 || ms.Port > 65535 {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("platform.mailHostRequired"), nil, envelope.InputError)
		}
		if _, err := mail.ParseAddress(ms.From); err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("validation.invalidFromAddress"), nil, envelope.InputError)
		}
	}
	if err := app.platform.SetMailSettings(ms); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}
	app.mailer.InvalidatePlatform()
	app.platform.Audit(platformAdmin(r), platform.ActionUpdateMail, 0, "",
		map[string]any{"enabled": ms.Enabled, "host": ms.Host, "from": ms.From}, realip.FromRequest(r.RequestCtx))
	return handlePlatformGetMail(r)
}

// handlePlatformTestMail sends a test email through the platform's mailer and
// reports what the mail server said.
func handlePlatformTestMail(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			To string `json:"to"`
		}
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	to, err := mail.ParseAddress(strings.TrimSpace(req.To))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("validation.invalidEmail"), nil, envelope.InputError)
	}
	err = app.notifier.SendSync(notifier.Message{
		TenantID:        0,
		RecipientEmails: []string{to.Address},
		Subject:         "Test email from the platform",
		Content:         "<p>This is a test email from your platform's mailer. The email of workspaces without their own mail settings goes out this way.</p>",
		Provider:        notifier.ProviderEmail,
	})
	app.platform.Audit(platformAdmin(r), platform.ActionTestMail, 0, "", map[string]any{"to": to.Address, "ok": err == nil}, realip.FromRequest(r.RequestCtx))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadGateway, err.Error(), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(true)
}
