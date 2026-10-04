package main

import (
	"cmp"
	"strings"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// landingOr serves the bare base domain's page, a box to find a workspace
// and log in to it, and next on every other host.
func landingOr(next fastglue.FastRequestHandler) fastglue.FastRequestHandler {
	return func(r *fastglue.Request) error {
		app := r.Context.(*App)
		if !app.tenant.Enabled() || !app.tenant.IsBaseDomain(string(r.RequestCtx.Host())) {
			return next(r)
		}
		return app.tmpl.RenderWebPage(tenant.DefaultID, r.RequestCtx, "landing", map[string]any{
			"Data": map[string]any{
				"Product":    cmp.Or(ko.String("app.tenancy.product_name"), "ConectDesk"),
				"BaseDomain": ko.String("app.tenancy.base_domain"),
			},
		})
	}
}

// handleWorkspaceLookup returns the URL of the workspace with the slug, for
// logging in from the landing page.
func handleWorkspaceLookup(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !app.tenant.Enabled() || !app.tenant.IsBaseDomain(string(r.RequestCtx.Host())) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	slug := strings.ToLower(strings.TrimSpace(string(r.RequestCtx.QueryArgs().Peek("slug"))))
	t, err := app.tenant.GetBySlug(slug)
	if err != nil || slug == "" {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("tenant.notFound"), nil, envelope.NotFoundError)
	}
	return r.SendEnvelope(map[string]any{"url": app.tenant.URLFor(t)})
}
