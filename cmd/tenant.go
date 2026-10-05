package main

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"strconv"
	"strings"
	"sync"
	"time"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/image"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/media"
	"github.com/conectdesk/conectdesk/internal/platform"
	pmodels "github.com/conectdesk/conectdesk/internal/platform/models"
	"github.com/conectdesk/conectdesk/internal/setting"
	"github.com/conectdesk/conectdesk/internal/tenant"
	tmodels "github.com/conectdesk/conectdesk/internal/tenant/models"
	"github.com/conectdesk/conectdesk/internal/user"
	realip "github.com/ferluci/fast-realip"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// tenantCtxKey holds the resolved tenant on the request context.
const tenantCtxKey = "tenant"

// tenantHandler resolves the tenant a request belongs to from its host and
// stores it on the request context before any route runs. Requests for an
// unknown or suspended tenant never reach a handler.
func tenantHandler(app *App, next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// Behind a proxy that ends TLS, such as Railway's, the request arrives as
		// http; redirects are built from its scheme, so take the proxy's.
		if string(ctx.Request.Header.Peek("X-Forwarded-Proto")) == "https" {
			ctx.Request.URI().SetScheme("https")
		}

		// Load balancer health checks hit the bare host.
		if string(ctx.Path()) == "/health" {
			next(ctx)
			return
		}
		if string(ctx.Path()) == tlsAskPath {
			serveTLSAsk(app, ctx)
			return
		}

		// The platform dashboard's host has no tenant and serves only the
		// dashboard; no other host serves it.
		if isPlatformHost(app, string(ctx.Host())) {
			switch path := string(ctx.Path()); {
			case path == "/":
				ctx.Redirect("/platform", fasthttp.StatusFound)
			case isPlatformPath(path):
				next(ctx)
			default:
				writeTenantError(ctx, fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), envelope.NotFoundError)
			}
			return
		}
		// Likewise the documentation's host serves only the documentation, to
		// docs viewers, and no other host serves it.
		if isDocsHost(app, string(ctx.Host())) {
			switch path := string(ctx.Path()); {
			case path == "/":
				ctx.Redirect("/docs", fasthttp.StatusFound)
			case isDocsPath(path):
				next(ctx)
			default:
				writeTenantError(ctx, fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), envelope.NotFoundError)
			}
			return
		}
		if isPlatformPath(string(ctx.Path())) || (isDocsPath(string(ctx.Path())) && app.docsHost != "") {
			writeTenantError(ctx, fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), envelope.NotFoundError)
			return
		}

		// The bare base domain has no tenant. Workspaces are only created by
		// platform admins, so it serves no signup.
		if app.tenant.IsBaseDomain(string(ctx.Host())) {
			// The landing page, and finding a workspace to log in to.
			if p := string(ctx.Path()); p == "/" || p == "/api/v1/workspaces/lookup" || strings.HasPrefix(p, "/static/public/static/") {
				next(ctx)
				return
			}
			// The shared OAuth redirect URL relays to the workspace.
			if strings.HasPrefix(string(ctx.Path()), "/oauth/") {
				next(ctx)
				return
			}
			// Meta calls one webhook and one pair of account callbacks for every
			// workspace, and routes each event by the account it names.
			if p := string(ctx.Path()); p == "/webhook/meta" || strings.HasPrefix(p, "/meta/") {
				next(ctx)
				return
			}
			writeTenantError(ctx, fasthttp.StatusNotFound, app.i18n.T("tenant.notFound"), envelope.NotFoundError)
			return
		}

		t, err := app.tenant.Resolve(string(ctx.Host()))
		switch {
		case errors.Is(err, tenant.ErrNotFound):
			writeTenantError(ctx, fasthttp.StatusNotFound, app.i18n.T("tenant.notFound"), envelope.NotFoundError)
			return
		case err != nil:
			app.lo.Error("error resolving tenant", "host", string(ctx.Host()), "error", err)
			writeTenantError(ctx, fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), envelope.GeneralError)
			return
		case !t.Active() && t.DeletionScheduledAt.Valid:
			writeTenantError(ctx, fasthttp.StatusForbidden, app.i18n.For(t.ID).Ts("tenant.scheduledForDeletion",
				"date", t.DeletionScheduledAt.Time.UTC().Format("2 January 2006")), envelope.PermissionError)
			return
		case !t.Active():
			writeTenantError(ctx, fasthttp.StatusForbidden, app.i18n.For(t.ID).T("tenant.suspended"), envelope.PermissionError)
			return
		}

		ctx.SetUserValue(tenantCtxKey, t)
		next(ctx)
	}
}

// tenantFromReq returns the tenant resolved for the request.
func tenantFromReq(r *fastglue.Request) tmodels.Tenant {
	t, _ := r.RequestCtx.UserValue(tenantCtxKey).(tmodels.Tenant)
	return t
}

// writeTenantError writes an error in the same envelope shape fastglue uses,
// since tenant resolution runs before the router.
func writeTenantError(ctx *fasthttp.RequestCtx, code int, msg, errType string) {
	b, _ := json.Marshal(map[string]any{
		"status":     "error",
		"message":    msg,
		"data":       nil,
		"error_type": errType,
	})
	ctx.SetStatusCode(code)
	ctx.SetContentType("application/json; charset=utf-8")
	ctx.SetBody(b)
}

// tenantID returns the ID of the tenant resolved for the request.
func tenantID(r *fastglue.Request) int {
	return tenantFromReq(r).ID
}

// newTenantRootURL returns a function giving the URL a tenant is served on. In
// single-tenant mode that is the app.root_url setting.
func newTenantRootURL(tenants *tenant.Manager, settings *setting.Manager) func(tenantID int) string {
	return func(tenantID int) string {
		if !tenants.Enabled() {
			u, err := settings.GetAppRootURL(tenant.DefaultID)
			if err != nil {
				return ko.String("app.root_url")
			}
			return u
		}
		u, err := tenants.RootURL(tenantID)
		if err != nil {
			return ""
		}
		return u
	}
}

// newSystemUserResolver returns a cached lookup of each tenant's system user ID.
// System users are never deleted, so an ID once found stays valid.
func newSystemUserResolver(users *user.Manager) func(tenantID int) (int, error) {
	var cache sync.Map
	return func(tenantID int) (int, error) {
		if id, ok := cache.Load(tenantID); ok {
			return id.(int), nil
		}
		u, err := users.GetSystemUser(tenantID)
		if err != nil {
			return 0, err
		}
		cache.Store(tenantID, u.ID)
		return u.ID, nil
	}
}

// tenantConstsTTL bounds how stale a tenant's cached constants can get.
const tenantConstsTTL = 30 * time.Second

type cachedConsts struct {
	c   *constants
	exp time.Time
}

// tenantConsts returns the constants of a tenant: its root URL, and the
// branding and upload limits from its settings. In single-tenant mode, and for
// the default tenant, those are the installation's constants.
func tenantConsts(app *App, tenantID int) *constants {
	base := app.consts.Load().(*constants)
	if !app.tenant.Enabled() {
		return base
	}
	if v, ok := app.tenantConsts.Load(tenantID); ok && time.Now().Before(v.(cachedConsts).exp) {
		return v.(cachedConsts).c
	}

	c := *base
	c.AppBaseURL = app.rootURL(tenantID)
	if b, err := app.setting.GetByPrefix(tenantID, "app"); err == nil {
		var s struct {
			FaviconURL        *string  `json:"app.favicon_url"`
			LogoURL           *string  `json:"app.logo_url"`
			SiteName          *string  `json:"app.site_name"`
			AllowedExtensions []string `json:"app.allowed_file_upload_extensions"`
			MaxUploadSizeMB   *int     `json:"app.max_file_upload_size"`
			Require2FA        *bool    `json:"app.require_2fa"`
			RequireSSO        *bool    `json:"app.require_sso"`
		}
		if err := json.Unmarshal(b, &s); err == nil {
			if s.FaviconURL != nil {
				c.FaviconURL = *s.FaviconURL
			}
			if s.LogoURL != nil {
				c.LogoURL = *s.LogoURL
			}
			if s.SiteName != nil {
				c.SiteName = *s.SiteName
			}
			if s.AllowedExtensions != nil {
				c.AllowedUploadFileExtensions = s.AllowedExtensions
			}
			if s.MaxUploadSizeMB != nil {
				c.MaxFileUploadSizeMB = *s.MaxUploadSizeMB
			}
			if s.Require2FA != nil {
				c.Require2FA = *s.Require2FA
			}
			if s.RequireSSO != nil {
				c.RequireSSO = *s.RequireSSO
			}
		}
	}
	// A workspace can't raise its own per-file limit over its plan's.
	if planMax := planLimit(app, tenantID, "max_file_upload_mb"); planMax > 0 && c.MaxFileUploadSizeMB > planMax {
		c.MaxFileUploadSizeMB = planMax
	}
	app.tenantConsts.Store(tenantID, cachedConsts{c: &c, exp: time.Now().Add(tenantConstsTTL)})
	return &c
}

// tenantTemplateFuncs returns the template functions that differ per tenant,
// or nil when they don't. The functions read the tenant's constants when
// called, so a tenant's template set can be built once and kept.
func tenantTemplateFuncs(app *App) func(tenantID int) template.FuncMap {
	return func(tenantID int) template.FuncMap {
		if !app.tenant.Enabled() {
			return nil
		}
		return template.FuncMap{
			"RootURL":    func() string { return tenantConsts(app, tenantID).AppBaseURL },
			"FaviconURL": func() string { return tenantConsts(app, tenantID).FaviconURL },
			"LogoURL":    func() string { return tenantConsts(app, tenantID).LogoURL },
			"SiteName":   func() string { return tenantConsts(app, tenantID).SiteName },
			// Pages and emails of the tenant are in its own language.
			"L": func() any { return app.i18n.For(tenantID) },
		}
	}
}

// Resources capped by plan limits.
const (
	planAgents  = "agents"
	planInboxes = "inboxes"
)

// planLimit returns the tenant's plan limit named key, e.g. "max_agents", from
// app.tenancy.plans.<plan>.<key>. 0 means unlimited, which is also what a plan
// not listed, the default tenant (the installation's own) and single-tenant
// mode get.
func planLimit(app *App, tenantID int, key string) int {
	if !app.tenant.Enabled() || tenantID == tenant.DefaultID || tenantID == 0 {
		return 0
	}
	t, err := app.tenant.GetCached(tenantID)
	if err != nil {
		return 0
	}
	return ko.Int("app.tenancy.plans." + t.Plan + "." + key)
}

// checkTenantRate counts an authenticated request against its workspace's
// api_requests_per_minute. Over the limit it has written the 429 response.
func checkTenantRate(r *fastglue.Request) error {
	app := r.Context.(*App)
	tid := tenantID(r)
	return app.rateLimit.CheckTenant(r.RequestCtx, tid, planLimit(app, tid, "api_requests_per_minute"))
}

// checkStorageLimit returns an error when storing adding more bytes would take
// the tenant over its plan's max_storage_mb.
func checkStorageLimit(app *App, tenantID int, adding int64) error {
	maxMB := planLimit(app, tenantID, "max_storage_mb")
	if maxMB <= 0 {
		return nil
	}
	used, err := app.media.StorageUsed(tenantID)
	if err != nil {
		return envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if used+adding > int64(maxMB)<<20 {
		return envelope.NewError(envelope.PermissionError, app.i18n.For(tenantID).Ts("tenant.planLimitStorage", "max", formatMB(maxMB)), nil)
	}
	return nil
}

// formatMB formats a size in megabytes for messages, e.g. "500 MB" or "2 GB".
func formatMB(mb int) string {
	if mb >= 1024 && mb%1024 == 0 {
		return strconv.Itoa(mb/1024) + " GB"
	}
	return strconv.Itoa(mb) + " MB"
}

// checkPlanLimit returns an error when the tenant's plan doesn't allow one
// more agent or inbox. Limits come from app.tenancy.plans.<plan>; a plan not
// listed, a zero limit, the default tenant (the installation's own) and
// single-tenant mode are unlimited.
func checkPlanLimit(app *App, tenantID int, resource string, adding int) error {
	if !app.tenant.Enabled() || tenantID == tenant.DefaultID {
		return nil
	}
	t, err := app.tenant.Get(tenantID)
	if err != nil {
		return envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	max := ko.Int("app.tenancy.plans." + t.Plan + ".max_" + resource)
	if max <= 0 {
		return nil
	}
	usage, err := app.tenant.Usage(tenantID)
	if err != nil {
		return envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	used, msg := usage.Agents, "tenant.planLimitAgents"
	if resource == planInboxes {
		used, msg = usage.Inboxes, "tenant.planLimitInboxes"
	}
	if used+adding > max {
		return envelope.NewError(envelope.PermissionError, app.i18n.For(tenantID).Ts(msg, "max", strconv.Itoa(max)), nil)
	}
	return nil
}

// inboxChannels are the inbox channels a platform admin can enable or disable
// for each workspace.
var inboxChannels = []string{
	inbox.ChannelEmail, inbox.ChannelLiveChat, inbox.ChannelWhatsApp, inbox.ChannelMessenger,
	inbox.ChannelInstagram, inbox.ChannelTelegram, inbox.ChannelVoice,
}

// checkChannelAllowed returns an error when the platform admin hasn't enabled
// channel for the tenant. Without multi-tenancy every channel is allowed.
func checkChannelAllowed(app *App, tenantID int, channel string) error {
	if !app.tenant.Enabled() {
		return nil
	}
	t, err := app.tenant.Get(tenantID)
	if err != nil {
		return envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if !t.ChannelAllowed(channel) {
		return envelope.NewError(envelope.PermissionError, app.i18n.For(tenantID).T("tenant.channelNotAllowed"), nil)
	}
	return nil
}

// Integrations a platform admin can enable or disable for each workspace.
const (
	integrationWebhooks     = "webhooks"
	integrationSlack        = "slack"
	integrationTeams        = "teams"
	integrationContextLinks = "context_links"
)

// integrationNames are the integrations, in the order the admin settings list them.
var integrationNames = []string{integrationWebhooks, integrationSlack, integrationTeams, integrationContextLinks}

// integrationAllowed reports whether the platform admin enabled integration
// for the tenant. Without multi-tenancy every integration is allowed; a
// tenant that can't be read is treated as not allowed.
func integrationAllowed(app *App, tenantID int, integration string) bool {
	return tenantIntegrationAllowed(app.tenant, tenantID, integration)
}

// tenantIntegrationAllowed is integrationAllowed for code without the App.
func tenantIntegrationAllowed(tenants *tenant.Manager, tenantID int, integration string) bool {
	if !tenants.Enabled() {
		return true
	}
	t, err := tenants.GetCached(tenantID)
	return err == nil && t.IntegrationAllowed(integration)
}

// checkIntegrationAllowed returns an error when the platform admin hasn't
// enabled integration for the tenant.
func checkIntegrationAllowed(app *App, tenantID int, integration string) error {
	if !integrationAllowed(app, tenantID, integration) {
		return envelope.NewError(envelope.PermissionError, app.i18n.For(tenantID).T("tenant.integrationNotAllowed"), nil)
	}
	return nil
}

// withIntegration refuses the request when the platform admin hasn't enabled
// integration for the tenant.
func withIntegration(integration string, handler fastglue.FastRequestHandler) fastglue.FastRequestHandler {
	return func(r *fastglue.Request) error {
		app := r.Context.(*App)
		if err := checkIntegrationAllowed(app, tenantID(r), integration); err != nil {
			return sendErrorEnvelope(r, err)
		}
		return handler(r)
	}
}

// deletionGrace is how long a deleted workspace is kept, suspended and
// restorable, before its data is deleted: app.tenancy.deletion_grace_days, 30
// by default.
func deletionGrace() time.Duration {
	days := ko.Int("app.tenancy.deletion_grace_days")
	if days <= 0 {
		days = 30
	}
	return time.Duration(days) * 24 * time.Hour
}

// deleteTenantData deletes a tenant with all its rows and its stored files,
// which the rows' deletion alone would leave in the store.
func deleteTenantData(tenants *tenant.Manager, med *media.Manager, id int) error {
	if med != nil {
		names, err := tenants.MediaNames(id)
		if err != nil {
			return err
		}
		for _, name := range names {
			med.Delete(id, image.ThumbPrefix+name)
			med.Delete(id, name)
		}
	}
	return tenants.Delete(id)
}

// runTenantDeletions deletes, every hour, the tenants whose deletion grace
// period has ended.
func runTenantDeletions(ctx context.Context, app *App) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		due, err := app.tenant.DueForDeletion()
		if err == nil {
			for _, w := range due {
				if err := deleteTenantData(app.tenant, app.media, w.ID); err != nil {
					app.lo.Error("error deleting tenant after its grace period", "tenant_id", w.ID, "error", err)
					continue
				}
				app.lo.Info("deleted tenant after its grace period", "tenant_id", w.ID, "slug", w.Slug)
				if app.platform != nil {
					app.platform.Audit(pmodels.Admin{Email: "system"}, platform.ActionDelete, 0, w.Slug,
						map[string]any{"name": w.Name, "scheduled_at": w.DeletionScheduledAt}, "")
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// handleDeleteWorkspace lets a workspace admin delete their own workspace:
// it is suspended at once and deleted with all its data after the grace
// period, during which the platform can restore it. It takes the admin's
// password and the workspace address typed to confirm.
func handleDeleteWorkspace(r *fastglue.Request) error {
	var (
		app  = r.Context.(*App)
		tid  = tenantID(r)
		sess = r.RequestCtx.UserValue("user").(amodels.User)
		req  struct {
			Password string `json:"password"`
			Confirm  string `json:"confirm"`
		}
	)
	t := tenantFromReq(r)
	if !app.tenant.Enabled() || tid == tenant.DefaultID {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("platform.defaultWorkspace"), nil, envelope.InputError)
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	if strings.TrimSpace(req.Confirm) != t.Slug {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("platform.confirmSlug"), nil, envelope.InputError)
	}
	user, err := app.user.VerifyPassword(tid, sess.Email, []byte(req.Password))
	if err != nil || user.ID != sess.ID {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.For(tid).T("auth.wrongPassword"), nil, envelope.PermissionError)
	}
	if !user.HasAdminRole() {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.For(tid).T("status.deniedPermission"), nil, envelope.PermissionError)
	}
	at := time.Now().Add(deletionGrace())
	if _, err := app.tenant.ScheduleDeletion(tid, at); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	if app.platform != nil {
		app.platform.Audit(pmodels.Admin{Email: user.Email.String + " (workspace admin)"}, platform.ActionScheduleDelete, tid, t.Slug,
			map[string]any{"deletes_at": at.UTC()}, realip.FromRequest(r.RequestCtx))
	}
	app.lo.Info("workspace scheduled for deletion by its admin", "tenant_id", tid, "user_id", user.ID, "deletes_at", at)
	return r.SendEnvelope(map[string]any{"deletion_scheduled_at": at.UTC()})
}
