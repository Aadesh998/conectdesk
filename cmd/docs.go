package main

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"path"
	"strings"

	"github.com/conectdesk/conectdesk/internal/docs"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/platform"
	pmodels "github.com/conectdesk/conectdesk/internal/platform/models"
	"github.com/conectdesk/conectdesk/internal/tenant"
	realip "github.com/ferluci/fast-realip"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// The internal documentation is served on its own host, docs.<base_domain> by
// default, to docs viewers: accounts platform admins add that can log in to
// the documentation and open nothing else. The client guide, written for the
// people who run a workspace, is served on the same host to anyone. Their
// paths answer on no other host.
const docsSessionCookie = "docs_session"

// isDocsPath reports whether path belongs to the documentation.
func isDocsPath(path string) bool {
	for _, base := range []string{"/docs", "/guide"} {
		if path == base || strings.HasPrefix(path, base+"/") || strings.HasPrefix(path, "/api"+base+"/") {
			return true
		}
	}
	return false
}

// isDocsHost reports whether host is the documentation's host.
func isDocsHost(app *App, host string) bool {
	if app.platform == nil || app.docs == nil || app.docsHost == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.EqualFold(strings.TrimSuffix(host, "."), app.docsHost)
}

// initDocs loads the documentation and the client guide and returns their
// host, with the platform dashboard, which manages who may read the former.
func initDocs(tenants *tenant.Manager, p *platform.Manager) (*docs.Docs, *docs.Docs, string) {
	if p == nil {
		return nil, nil, ""
	}
	d, err := docs.Load()
	if err != nil {
		log.Fatalf("error loading documentation: %v", err)
	}
	g, err := docs.LoadGuide()
	if err != nil {
		log.Fatalf("error loading the client guide: %v", err)
	}
	return d, g, strings.ToLower(cmp.Or(ko.String("app.tenancy.docs_host"), "docs."+tenants.BaseDomain()))
}

// docsViewer returns the docs viewer logged in on the request.
func docsViewer(r *fastglue.Request) (pmodels.DocsViewer, error) {
	app := r.Context.(*App)
	return app.platform.DocsSessionViewer(string(r.RequestCtx.Request.Header.Cookie(docsSessionCookie)))
}

// handleDocsPage serves a documentation page to a logged in docs viewer, and
// the login form to anyone else.
func handleDocsPage(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !isDocsHost(app, string(r.RequestCtx.Host())) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	r.RequestCtx.Response.Header.Set("X-Frame-Options", "DENY")
	r.RequestCtx.Response.Header.Set("X-Robots-Tag", "noindex, nofollow")
	product := cmp.Or(ko.String("app.tenancy.product_name"), "ConectDesk")

	viewer, err := docsViewer(r)
	if err != nil {
		if !errors.Is(err, platform.ErrNoSession) {
			return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
		}
		return app.tmpl.RenderWebPage(tenant.DefaultID, r.RequestCtx, "docs-login", map[string]any{
			"Data": map[string]any{"Product": product},
		})
	}

	slug, _ := r.RequestCtx.UserValue("page").(string)
	_, idx, ok := app.docs.Page(slug)
	if !ok {
		r.RequestCtx.Redirect("/docs", fasthttp.StatusFound)
		return nil
	}
	data := docsPageData(app.docs, idx, "/docs", "Docs")
	data["Product"], data["Email"] = product, viewer.Email
	return app.tmpl.RenderWebPage(tenant.DefaultID, r.RequestCtx, "docs", map[string]any{"Data": data})
}

// docsPageData is what the docs template shows of page idx of d, served
// under base.
func docsPageData(d *docs.Docs, idx int, base, label string) map[string]any {
	data := map[string]any{
		"Base":  base,
		"Label": label,
		"Pages": d.Pages,
		"Page":  d.Pages[idx],
		"Index": idx,
	}
	if idx > 0 {
		data["Prev"] = d.Pages[idx-1]
	}
	if idx < len(d.Pages)-1 {
		data["Next"] = d.Pages[idx+1]
	}
	return data
}

// handleGuidePage serves a page of the client guide, to anyone.
func handleGuidePage(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !isDocsHost(app, string(r.RequestCtx.Host())) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	slug, _ := r.RequestCtx.UserValue("page").(string)
	_, idx, ok := app.guide.Page(slug)
	if !ok {
		r.RequestCtx.Redirect("/guide", fasthttp.StatusFound)
		return nil
	}
	data := docsPageData(app.guide, idx, "/guide", "Guide")
	for k, v := range websiteData(app) {
		data[k] = v
	}
	data["Public"] = true
	data["Title"] = fmt.Sprintf("%s · %s Guide", app.guide.Pages[idx].Title, data["Product"])
	// The first page is served at /guide itself.
	data["Canonical"] = data["GuideURL"]
	if idx > 0 {
		data["Canonical"] = data["GuideURL"].(string) + "/" + app.guide.Pages[idx].Slug
	}
	data["Description"] = cmp.Or(app.guide.Pages[idx].Description,
		fmt.Sprintf("%s in the %s client guide: how to set up and run your workspace.", app.guide.Pages[idx].Title, data["Product"]))
	r.RequestCtx.Response.Header.Set("X-Frame-Options", "DENY")
	setWebsiteHeaders(app, r.RequestCtx)
	return app.tmpl.RenderWebPage(tenant.DefaultID, r.RequestCtx, "docs", map[string]any{"Data": data})
}

// handleGuideImage serves one of the client guide's screenshots.
func handleGuideImage(r *fastglue.Request) error {
	app := r.Context.(*App)
	name, _ := r.RequestCtx.UserValue("file").(string)
	b, err := fs.ReadFile(docs.GuideImages(), name)
	if !isDocsHost(app, string(r.RequestCtx.Host())) || err != nil || strings.Contains(name, "/") {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	r.RequestCtx.Response.Header.Set("Cache-Control", "public, max-age=86400")
	r.RequestCtx.SetContentType(cmp.Or(mime.TypeByExtension(path.Ext(name)), "application/octet-stream"))
	r.RequestCtx.SetBody(b)
	return nil
}

// handleGuideSearchIndex returns every client guide page's sections, for
// searching the guide in the browser.
func handleGuideSearchIndex(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !isDocsHost(app, string(r.RequestCtx.Host())) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	r.RequestCtx.Response.Header.Set("Cache-Control", "no-store")
	return r.SendEnvelope(searchIndex(app.guide))
}

// handleDocsSearchIndex returns every page's sections, for searching the
// documentation in the browser.
func handleDocsSearchIndex(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !isDocsHost(app, string(r.RequestCtx.Host())) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	if _, err := docsViewer(r); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, app.i18n.T("platform.docsNotLoggedIn"), nil, envelope.PermissionError)
	}
	r.RequestCtx.Response.Header.Set("Cache-Control", "no-store")
	return r.SendEnvelope(searchIndex(app.docs))
}

// searchIndexEntry is one section of a page, for searching in the browser.
type searchIndexEntry struct {
	Page      string `json:"page"`
	PageTitle string `json:"page_title"`
	docs.Heading
}

// searchIndex is every page's title and sections.
func searchIndex(d *docs.Docs) []searchIndexEntry {
	out := []searchIndexEntry{}
	for _, p := range d.Pages {
		out = append(out, searchIndexEntry{Page: p.Slug, PageTitle: p.Title, Heading: docs.Heading{Title: p.Title, Level: 1}})
		for _, h := range p.Headings {
			out = append(out, searchIndexEntry{Page: p.Slug, PageTitle: p.Title, Heading: h})
		}
	}
	return out
}

// handleDocsLogin logs a docs viewer in.
func handleDocsLogin(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		ip  = realip.FromRequest(r.RequestCtx)
		req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
	)
	if !isDocsHost(app, string(r.RequestCtx.Host())) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	_, token, err := app.platform.DocsLogin(req.Email, req.Password, ip)
	if err != nil {
		if errors.Is(err, platform.ErrInvalidCredentials) {
			app.lo.Warn("failed docs login", "email", req.Email, "ip", ip)
			return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, app.i18n.T("platform.invalidCredentials"), nil, envelope.PermissionError)
		}
		if errors.Is(err, platform.ErrSuspended) {
			return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.T("platform.docsSuspended"), nil, envelope.PermissionError)
		}
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	setDocsCookie(r, token, false)
	return r.SendEnvelope(true)
}

// handleDocsLogout ends the docs session.
func handleDocsLogout(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !isDocsHost(app, string(r.RequestCtx.Host())) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	app.platform.DocsLogout(string(r.RequestCtx.Request.Header.Cookie(docsSessionCookie)))
	setDocsCookie(r, "", true)
	return r.SendEnvelope(true)
}

// setDocsCookie sets the docs session cookie, or deletes it. It is Lax, not
// Strict like the platform's, so a link to a page opens it logged in; the
// only changes the docs host takes are logging in and out.
func setDocsCookie(r *fastglue.Request, value string, del bool) {
	c := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(c)
	c.SetKey(docsSessionCookie)
	c.SetValue(value)
	c.SetPath("/")
	c.SetHTTPOnly(true)
	c.SetSecure(!ko.Bool("app.server.disable_secure_cookies"))
	c.SetSameSite(fasthttp.CookieSameSiteLaxMode)
	if del {
		c.SetExpire(fasthttp.CookieExpireDelete)
	} else {
		c.SetMaxAge(int(platform.DocsSessionTTL.Seconds()))
	}
	r.RequestCtx.Response.Header.SetCookie(c)
}

// handlePlatformDocsViewers returns the docs viewers and where they log in.
func handlePlatformDocsViewers(r *fastglue.Request) error {
	app := r.Context.(*App)
	viewers, err := app.platform.DocsViewers()
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}
	url := ""
	if app.docsHost != "" {
		url = app.tenant.HostURL(app.docsHost) + "/docs"
	}
	return r.SendEnvelope(map[string]any{"viewers": viewers, "url": url})
}

// handlePlatformSetDocsViewer adds a docs viewer, or gives one a new password:
// the typed one, or a generated one when it's empty. With send_email the
// password is emailed to them through the platform's mailer.
func handlePlatformSetDocsViewer(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			Email     string `json:"email"`
			Password  string `json:"password"`
			SendEmail bool   `json:"send_email"`
		}
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	password, err := newPassword(req.Password)
	if err != nil {
		return sendPasswordError(r, err)
	}
	existing, err := app.platform.DocsViewers()
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}
	v, err := app.platform.SetDocsViewer(req.Email, password)
	if err != nil {
		if errors.Is(err, platform.ErrInvalidEmail) {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("validation.invalidEmail"), nil, envelope.InputError)
		}
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}
	action, reason := platform.ActionAddDocsViewer, "A platform administrator gave you access to the internal documentation."
	for _, e := range existing {
		if e.Email == v.Email {
			action, reason = platform.ActionResetDocsPassword, "A platform administrator set a new password for your documentation login. Your old password no longer works."
		}
	}
	out := credentialsResp{Password: password}
	if req.SendEmail {
		out = sendCredentials(app, 0, v.Email, "", app.tenant.HostURL(app.docsHost)+"/docs", password, reason)
	}
	app.platform.Audit(platformAdmin(r), action, 0, "", map[string]any{"email": v.Email, "emailed": out.Emailed}, realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(out)
}

// handlePlatformSetDocsViewerStatus suspends a docs viewer, logging them out,
// or lets them back in.
func handlePlatformSetDocsViewerStatus(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			Email   string `json:"email"`
			Enabled bool   `json:"enabled"`
		}
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	if err := app.platform.SetDocsViewerEnabled(req.Email, req.Enabled); err != nil {
		if errors.Is(err, platform.ErrNotFound) {
			return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("platform.docsViewerNotFound"), nil, envelope.NotFoundError)
		}
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}
	action := platform.ActionActivateDocsViewer
	if !req.Enabled {
		action = platform.ActionSuspendDocsViewer
	}
	app.platform.Audit(platformAdmin(r), action, 0, "", map[string]any{"email": strings.ToLower(strings.TrimSpace(req.Email))}, realip.FromRequest(r.RequestCtx))
	return handlePlatformDocsViewers(r)
}

// handlePlatformRemoveDocsViewer removes a docs viewer, logging them out.
func handlePlatformRemoveDocsViewer(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			Email string `json:"email"`
		}
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	ok, err := app.platform.RemoveDocsViewer(req.Email)
	switch {
	case errors.Is(err, platform.ErrInvalidEmail) || (err == nil && !ok):
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("platform.docsViewerNotFound"), nil, envelope.NotFoundError)
	case err != nil:
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.T("globals.messages.somethingWentWrong"), nil))
	}
	app.platform.Audit(platformAdmin(r), platform.ActionRemoveDocsViewer, 0, "", map[string]any{"email": strings.ToLower(strings.TrimSpace(req.Email))}, realip.FromRequest(r.RequestCtx))
	return handlePlatformDocsViewers(r)
}
