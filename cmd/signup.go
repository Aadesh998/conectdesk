package main

import (
	"cmp"
	"errors"
	"net/url"
	"strings"

	"github.com/conectdesk/conectdesk/internal/envelope"
	notifier "github.com/conectdesk/conectdesk/internal/notification"
	"github.com/conectdesk/conectdesk/internal/signup"
	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// signupPaths are the only paths served on the bare base domain, where no
// tenant exists yet.
var signupPaths = []string{"/signup", "/signup/verify", "/api/v1/signup", "/api/v1/signup/slug", "/api/v1/signup/verify"}

// isSignupPath reports whether path is served on the base domain.
func isSignupPath(path string) bool {
	for _, p := range signupPaths {
		if path == p {
			return true
		}
	}
	return strings.HasPrefix(path, "/static/public/static/")
}

// signupAllowed reports whether the request may sign up a workspace: signup
// is on and the request is for the bare base domain, not a workspace.
func signupAllowed(r *fastglue.Request) bool {
	app := r.Context.(*App)
	return app.signup != nil && app.tenant.IsBaseDomain(string(r.RequestCtx.Host()))
}

// handleSignupPage renders the workspace signup form.
func handleSignupPage(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !signupAllowed(r) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	return app.tmpl.RenderWebPage(tenant.DefaultID, r.RequestCtx, "signup", map[string]any{
		"Data": map[string]any{
			"Title":      app.i18n.For(tenantID(r)).T("signup.title"),
			"BaseDomain": ko.String("app.tenancy.base_domain"),
		},
	})
}

// handleSignup creates a workspace and its admin, and returns its URL.
func handleSignup(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req signup.Request
	)
	if !signupAllowed(r) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	if !signupVerifyEmail() {
		t, err := app.signup.Create(r.RequestCtx, req)
		if err != nil {
			return sendSignupError(r, err)
		}
		return r.SendEnvelope(map[string]any{"url": app.tenant.URLFor(t)})
	}

	// The workspace is only created once the email is verified.
	if !app.mailer.Configured(0) {
		app.lo.Error("signup needs email verification but the platform's mailer is off; set it up in the platform dashboard (Settings > Email) or set app.tenancy.signup_verify_email = false")
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, app.i18n.For(tenantID(r)).T("signup.unavailable"), nil, envelope.GeneralError)
	}
	token, err := app.signup.Start(r.RequestCtx, req)
	if err != nil {
		return sendSignupError(r, err)
	}
	req.Normalize()
	content, err := app.tmpl.RenderInMemoryTemplate(tenant.DefaultID, "verify-signup", map[string]any{
		"Company":        req.Company,
		"WorkspaceHost":  req.Slug + "." + ko.String("app.tenancy.base_domain"),
		"VerifyURL":      app.tenant.BaseURL() + "/signup/verify?token=" + url.QueryEscape(token),
		"ExpiresInHours": int(app.signup.TokenTTL().Hours()),
	})
	if err != nil {
		app.lo.Error("error rendering signup verification email", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	// Sent by the platform's mailer: the workspace doesn't exist yet.
	if err := app.notifier.Send(notifier.Message{
		TenantID:        0,
		RecipientEmails: []string{req.Email},
		Subject:         app.i18n.For(tenantID(r)).T("signup.verifySubject"),
		Content:         content,
		Provider:        notifier.ProviderEmail,
	}); err != nil {
		app.lo.Error("error sending signup verification email", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("signup.emailFailed"), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(map[string]any{"verify": true, "email": req.Email})
}

// signupVerifyEmail reports whether signups must verify their email before
// their workspace is created. It is on unless turned off.
func signupVerifyEmail() bool {
	return !ko.Exists("app.tenancy.signup_verify_email") || ko.Bool("app.tenancy.signup_verify_email")
}

// sendSignupError answers a failed signup: which field is invalid, a taken
// slug, or a generic error.
func sendSignupError(r *fastglue.Request, err error) error {
	app := r.Context.(*App)
	var ve signup.ValidationError
	switch {
	case errors.As(err, &ve):
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("signup.invalid."+ve.Field), map[string]string{"field": ve.Field}, envelope.InputError)
	case errors.Is(err, signup.ErrSlugTaken):
		return r.SendErrorEnvelope(fasthttp.StatusConflict, app.i18n.For(tenantID(r)).T("signup.slugTaken"), map[string]string{"field": "slug"}, envelope.ConflictError)
	case errors.Is(err, signup.ErrInvalidToken):
		return r.SendErrorEnvelope(fasthttp.StatusGone, app.i18n.For(tenantID(r)).T("signup.invalidLink"), nil, envelope.InputError)
	}
	app.lo.Error("error signing up tenant", "error", err)
	return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
}

// handleSignupVerifyPage shows the pending signup of an emailed link with a
// button that creates it. Opening the link alone creates nothing, so mail
// scanners that follow links can't use it up.
func handleSignupVerifyPage(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !signupAllowed(r) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	data := map[string]any{"Title": app.i18n.For(tenantID(r)).T("signup.title")}
	p, err := app.signup.Pending(r.RequestCtx, string(r.RequestCtx.QueryArgs().Peek("token")))
	switch {
	case errors.Is(err, signup.ErrInvalidToken):
		data["Invalid"] = true
	case err != nil:
		app.lo.Error("error fetching pending signup", "error", err)
		data["Invalid"] = true
	default:
		data["Company"] = p.Company
		data["Email"] = p.Email
		data["WorkspaceHost"] = p.Slug + "." + ko.String("app.tenancy.base_domain")
	}
	r.RequestCtx.Response.Header.Set("Referrer-Policy", "no-referrer")
	return app.tmpl.RenderWebPage(tenant.DefaultID, r.RequestCtx, "signup-verify", map[string]any{"Data": data})
}

// handleSignupVerify creates the workspace of a verified signup.
func handleSignupVerify(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !signupAllowed(r) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := r.Decode(&req, "json"); err != nil || req.Token == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	t, err := app.signup.Confirm(r.RequestCtx, req.Token)
	if err != nil {
		return sendSignupError(r, err)
	}
	return r.SendEnvelope(map[string]any{"url": app.tenant.URLFor(t)})
}

// handleSignupSlugAvailable reports whether a workspace slug is free.
func handleSignupSlugAvailable(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !signupAllowed(r) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	ok, err := app.signup.SlugAvailable(r.RequestCtx, string(r.RequestCtx.QueryArgs().Peek("slug")))
	if err != nil {
		app.lo.Error("error checking slug availability", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(map[string]bool{"available": ok})
}

// landingOr serves the landing page on the bare base domain, and next on
// every other host.
func landingOr(next fastglue.FastRequestHandler) fastglue.FastRequestHandler {
	return func(r *fastglue.Request) error {
		app := r.Context.(*App)
		if !app.tenant.Enabled() || !app.tenant.IsBaseDomain(string(r.RequestCtx.Host())) {
			return next(r)
		}
		return app.tmpl.RenderWebPage(tenant.DefaultID, r.RequestCtx, "landing", map[string]any{
			"Data": map[string]any{
				"Product":       cmp.Or(ko.String("app.tenancy.product_name"), "ConectDesk"),
				"BaseDomain":    ko.String("app.tenancy.base_domain"),
				"SignupEnabled": app.signup != nil,
				// DemoURL is where "Book a demo" goes, e.g. a calendar or a
				// wa.me link; the button is hidden without it.
				"DemoURL": ko.String("app.tenancy.demo_url"),
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
