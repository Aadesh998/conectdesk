package main

import (
	"strings"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// landingOr serves the website on the bare base domain, with its box to
// find a workspace and log in to it, and next on every other host.
func landingOr(next fastglue.FastRequestHandler) fastglue.FastRequestHandler {
	return func(r *fastglue.Request) error {
		app := r.Context.(*App)
		if !app.tenant.Enabled() || !app.tenant.IsBaseDomain(string(r.RequestCtx.Host())) {
			return next(r)
		}
		video := ko.String("app.tenancy.demo_video_url")
		d := websiteData(app)
		// DemoURL is where "Book a demo" goes, e.g. a calendar or a wa.me
		// link; the buttons are hidden without it.
		d["DemoURL"] = ko.String("app.tenancy.demo_url")
		// DemoVideoURL is the product walkthrough; a video file plays in a
		// <video>, anything else (Cloudflare Stream, YouTube) is embedded.
		// The section is hidden without it.
		d["DemoVideoURL"], d["DemoVideoFile"] = video, isVideoFile(video)
		d["Title"] = d["Product"].(string) + ": Admission CRM for Coaching Institutes"
		d["Canonical"] = d["BaseURL"].(string) + "/"
		d["Description"] = "Calls, WhatsApp and Instagram in one inbox for coaching institutes. Missed-call replies, AI answers in Hindi and English, and a daily admissions report."
		faqs := landingFAQs(d["Product"].(string), d["BaseDomain"].(string))
		d["FAQs"], d["JSONLD"] = faqs, landingJSONLD(d, faqs)
		setWebsiteHeaders(app, r.RequestCtx)
		return app.tmpl.RenderWebPage(tenant.DefaultID, r.RequestCtx, "landing", map[string]any{"Data": d})
	}
}

// isVideoFile reports whether the URL points straight at a video file rather
// than a player page.
func isVideoFile(u string) bool {
	p := strings.ToLower(u)
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	return strings.HasSuffix(p, ".mp4") || strings.HasSuffix(p, ".webm") || strings.HasSuffix(p, ".m4v")
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
