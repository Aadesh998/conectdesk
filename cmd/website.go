package main

import (
	"cmp"
	"fmt"
	"html/template"
	"strings"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// The website is what search engines see of the installation: the landing page
// and its legal pages on the bare base domain, and the public client guide on
// the docs host. The workspaces themselves are kept out of search.

// legalUpdated is when the privacy policy and terms last changed; update it
// with them.
const legalUpdated = "30 October 2026"

// legalPages are the website's legal pages, by path.
var legalPages = map[string]string{
	"/privacy": "Privacy policy",
	"/terms":   "Terms of service",
}

// landingFAQ is a question on the landing page, also published as FAQPage
// structured data.
type landingFAQ struct {
	Q, A string
	// SignIn ends the answer with a link that opens the sign-in dialog.
	SignIn bool
}

// signInAnswer ends the answer of a SignIn question.
const signInAnswer = "Sign in from the home page, enter your workspace name, and you'll be taken to your login page."

func landingFAQs(product, baseDomain string) []landingFAQ {
	return []landingFAQ{
		{Q: "Who is " + product + " for?", A: "Coaching institutes and training centres with a small admissions team, usually 3 to 30 people: JEE and NEET coaching, IELTS and study-abroad consultants, skill and computer courses, and tuition centres that get enquiries by phone and WhatsApp."},
		{Q: "Which channels does it work with?", A: "Phone calls, WhatsApp Business, Instagram, Facebook Messenger, Telegram, a chat widget for your website, and email. All of them land in the same inbox."},
		{Q: "What happens when we miss a call?", A: "The caller gets a WhatsApp reply automatically, with a separate message outside business hours, and a call-back task is created for your team. When a counsellor completes a call with that number, the task closes itself. Call-backs left too long are escalated."},
		{Q: "We already have a WhatsApp bot. Why do we need this?", A: product + " isn't another bot. It handles what happens around the bot: parents who call instead of messaging, people who want to talk to a real person, follow-ups, and giving the owner a clear view of every lead."},
		{Q: "Does the AI understand Hindi?", A: "Yes. The assistant replies in Hindi, English or Hinglish, answers from the fees, batch and course details you give it, and hands the conversation to a counsellor when it should."},
		{Q: "Are calls recorded?", A: "Yes. Recorded calls are transcribed, and a short summary with the next step is saved to the contact, so you can see what happened without listening to the whole call."},
		{Q: "How do I log in to my workspace?", A: "Each institute has its own address, such as yourinstitute." + baseDomain + ".", SignIn: true},
	}
}

// websiteData is what every website page shows: the product, where its
// pages are, and the analytics and search console settings.
func websiteData(app *App) map[string]any {
	product := cmp.Or(ko.String("app.tenancy.product_name"), "ConectDesk")
	base := app.tenant.BaseURL()
	guide := ""
	if app.docsHost != "" && app.guide != nil {
		guide = app.tenant.HostURL(app.docsHost) + "/guide"
	}
	return map[string]any{
		"Product":      product,
		"BaseDomain":   ko.String("app.tenancy.base_domain"),
		"BaseURL":      base,
		"GuideURL":     guide,
		"Company":      cmp.Or(ko.String("app.tenancy.company_name"), product),
		"ContactEmail": ko.String("app.tenancy.contact_email"),
		"PrivacyURL":   cmp.Or(ko.String("app.legal.privacy_policy_url"), base+"/privacy"),
		"TermsURL":     base + "/terms",
		"OGImage":      cmp.Or(ko.String("app.tenancy.og_image_url"), base+"/static/public/static/og-image.jpg"),
		// GAID is the Google Analytics 4 measurement ID; no analytics without it.
		"GAID": ko.String("app.tenancy.ga_measurement_id"),
		// SiteVerification is the Search Console HTML tag token, for verifying
		// a URL-prefix property instead of the domain's DNS.
		"SiteVerification": ko.String("app.tenancy.google_site_verification"),
	}
}

// landingJSONLD returns the landing page's structured data: the organisation,
// the site, the product and its FAQ.
func landingJSONLD(d map[string]any, faqs []landingFAQ) template.JS {
	base := d["BaseURL"].(string)
	org := map[string]any{
		"@type": "Organization",
		"@id":   base + "/#organization",
		"name":  d["Product"],
		"url":   base + "/",
		"logo":  base + "/static/public/static/favicon-192.png",
	}
	if d["Company"] != d["Product"] {
		org["legalName"] = d["Company"]
	}
	if email := d["ContactEmail"].(string); email != "" {
		org["email"] = email
	}
	if links := strings.Fields(ko.String("app.tenancy.social_links")); len(links) > 0 {
		org["sameAs"] = links
	}
	product := map[string]any{
		"@type":                  "SoftwareApplication",
		"name":                   d["Product"],
		"url":                    base + "/",
		"description":            d["Description"],
		"applicationCategory":    "BusinessApplication",
		"applicationSubCategory": "Admissions CRM",
		"operatingSystem":        "Web",
		"image":                  d["OGImage"],
		"inLanguage":             "en-IN",
		"countriesSupported":     "IN",
		"publisher":              map[string]any{"@id": base + "/#organization"},
		"featureList": []string{
			"Calls, WhatsApp, Instagram, Messenger, Telegram and email in one inbox",
			"Automatic WhatsApp reply to missed calls",
			"Call-back tasks with escalation",
			"Call recording, transcripts and summaries",
			"AI assistant in Hindi, English and Hinglish",
			"Lead board from enquiry to admission",
			"Daily admissions report on WhatsApp",
		},
	}
	if g := d["GuideURL"].(string); g != "" {
		product["softwareHelp"] = map[string]any{"@type": "CreativeWork", "url": g}
	}
	site := map[string]any{
		"@type":      "WebSite",
		"@id":        base + "/#website",
		"url":        base + "/",
		"name":       d["Product"],
		"inLanguage": "en-IN",
		"publisher":  map[string]any{"@id": base + "/#organization"},
	}
	questions := make([]any, 0, len(faqs))
	for _, f := range faqs {
		a := f.A
		if f.SignIn {
			a += " " + signInAnswer
		}
		questions = append(questions, map[string]any{
			"@type":          "Question",
			"name":           f.Q,
			"acceptedAnswer": map[string]any{"@type": "Answer", "text": a},
		})
	}
	faq := map[string]any{"@type": "FAQPage", "@id": base + "/#faq", "mainEntity": questions}
	return jsonLD(map[string]any{"@context": schemaOrgContext, "@graph": []any{org, site, product, faq}})
}

// websiteOrigin returns the origin of the website served on host, the base
// domain's or the docs host's, and "" on any other host.
func websiteOrigin(app *App, host string) string {
	switch {
	case app.tenant.IsBaseDomain(host):
		return app.tenant.BaseURL()
	case isDocsHost(app, host):
		return app.tenant.HostURL(app.docsHost)
	}
	return ""
}

// websiteURLs returns the indexable pages of the website served on host.
func websiteURLs(app *App, host string) []sitemapURL {
	origin := websiteOrigin(app, host)
	if isDocsHost(app, host) {
		urls := make([]sitemapURL, 0, len(app.guide.Pages))
		for i, p := range app.guide.Pages {
			loc := origin + "/guide"
			if i > 0 {
				loc += "/" + p.Slug
			}
			urls = append(urls, sitemapURL{Loc: loc})
		}
		return urls
	}
	urls := []sitemapURL{{Loc: origin + "/"}}
	for _, p := range []string{"/privacy", "/terms"} {
		if p == "/privacy" && ko.String("app.legal.privacy_policy_url") != "" {
			continue
		}
		urls = append(urls, sitemapURL{Loc: origin + p})
	}
	return urls
}

// serveWebsiteRobots writes robots.txt for the website on host, and reports
// false on any other host.
func serveWebsiteRobots(r *fastglue.Request) bool {
	app := r.Context.(*App)
	origin := websiteOrigin(app, string(r.RequestCtx.Host()))
	if origin == "" {
		return false
	}
	r.RequestCtx.SetContentType("text/plain; charset=utf-8")
	fmt.Fprint(r.RequestCtx, "User-agent: *\nDisallow: /api/\n")
	if app.tenant.IsBaseDomain(string(r.RequestCtx.Host())) {
		fmt.Fprint(r.RequestCtx, "Disallow: /oauth/\nDisallow: /meta/\nDisallow: /webhook/\n")
	}
	fmt.Fprintf(r.RequestCtx, "\nSitemap: %s/sitemap.xml\n", origin)
	return true
}

// serveWebsiteSitemap writes the sitemap of the website on host, and reports
// false on any other host.
func serveWebsiteSitemap(r *fastglue.Request) (bool, error) {
	app := r.Context.(*App)
	host := string(r.RequestCtx.Host())
	if websiteOrigin(app, host) == "" {
		return false, nil
	}
	return true, sendXML(r, urlset{Xmlns: sitemapNamespace, URLs: websiteURLs(app, host)})
}

// setWebsiteHeaders sets the security headers every website page sends.
func setWebsiteHeaders(app *App, ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("X-Content-Type-Options", "nosniff")
	ctx.Response.Header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	if strings.HasPrefix(app.tenant.BaseURL(), "https://") {
		ctx.Response.Header.Set("Strict-Transport-Security", "max-age=31536000")
	}
}

// handleLegalPage serves the privacy policy or the terms on the base domain.
// The privacy policy redirects to app.legal.privacy_policy_url when that is
// set.
func handleLegalPage(r *fastglue.Request) error {
	app := r.Context.(*App)
	path := string(r.RequestCtx.Path())
	if !app.tenant.Enabled() || !app.tenant.IsBaseDomain(string(r.RequestCtx.Host())) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	if u := ko.String("app.legal.privacy_policy_url"); path == "/privacy" && u != "" {
		r.RequestCtx.Redirect(u, fasthttp.StatusFound)
		return nil
	}
	d := websiteData(app)
	d["Page"], d["Title"], d["Updated"] = strings.TrimPrefix(path, "/"), legalPages[path], legalUpdated
	d["Canonical"] = d["BaseURL"].(string) + path
	d["Description"] = fmt.Sprintf("The %s of %s.", strings.ToLower(legalPages[path]), d["Product"])
	setWebsiteHeaders(app, r.RequestCtx)
	return app.tmpl.RenderWebPage(tenant.DefaultID, r.RequestCtx, "website", map[string]any{"Data": d})
}

// serveWebsiteNotFound answers a page the website doesn't have with an HTML
// 404 page, and anything else, such as an API call, with the JSON error.
func serveWebsiteNotFound(app *App, ctx *fasthttp.RequestCtx, msg string) {
	if !(ctx.IsGet() || ctx.IsHead()) || strings.HasPrefix(string(ctx.Path()), "/api/") {
		writeTenantError(ctx, fasthttp.StatusNotFound, msg, envelope.NotFoundError)
		return
	}
	d := websiteData(app)
	d["Page"], d["Title"] = "notfound", "Page not found"
	d["Home"] = "/"
	if isDocsHost(app, string(ctx.Host())) {
		d["Home"] = "/guide"
	}
	setWebsiteHeaders(app, ctx)
	if err := app.tmpl.RenderWebPage(tenant.DefaultID, ctx, "website", map[string]any{"Data": d}); err != nil {
		app.lo.Error("error rendering the not found page", "error", err)
		writeTenantError(ctx, fasthttp.StatusNotFound, msg, envelope.NotFoundError)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusNotFound)
}

// isWebsitePath reports whether path is one the base domain and the docs host
// serve besides their pages: crawler files and the website's static files.
func isWebsitePath(path string) bool {
	return path == "/robots.txt" || path == "/sitemap.xml" || path == "/favicon.ico" || strings.HasPrefix(path, "/static/public/static/")
}
