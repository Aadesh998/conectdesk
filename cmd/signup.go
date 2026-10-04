package main

import (
	"cmp"
	"errors"
	"strings"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/signup"
	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// sendSignupError answers a workspace that couldn't be created: which field
// is invalid, a taken slug, or a generic error.
func sendSignupError(r *fastglue.Request, err error) error {
	app := r.Context.(*App)
	var ve signup.ValidationError
	switch {
	case errors.As(err, &ve):
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("signup.invalid."+ve.Field), map[string]string{"field": ve.Field}, envelope.InputError)
	case errors.Is(err, signup.ErrSlugTaken):
		return r.SendErrorEnvelope(fasthttp.StatusConflict, app.i18n.For(tenantID(r)).T("signup.slugTaken"), map[string]string{"field": "slug"}, envelope.ConflictError)
	}
	app.lo.Error("error creating workspace", "error", err)
	return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
}

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
