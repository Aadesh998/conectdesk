package main

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
	"time"
)

func TestSolutionPages(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range solutionPages {
		if seen[p.Slug] {
			t.Errorf("duplicate slug %q", p.Slug)
		}
		seen[p.Slug] = true
		if !isSolutionPath(solutionsPrefix + p.Slug) {
			t.Errorf("isSolutionPath(%q) = false", solutionsPrefix+p.Slug)
		}
		if n := len(p.Title + " | ConnectDesk"); n > 65 {
			t.Errorf("%s: title is %d characters, over 65", p.Slug, n)
		}
		got := p.withProduct("ConnectDesk")
		if n := len(got.Description); n < 70 || n > 170 {
			t.Errorf("%s: description is %d characters, want 70-170", p.Slug, n)
		}
		if strings.Contains(got.Description+got.Lede, "{product}") {
			t.Errorf("%s: {product} left unreplaced", p.Slug)
		}
		for _, f := range got.FAQs {
			if strings.Contains(f.Q+f.A, "{product}") {
				t.Errorf("%s: {product} left in FAQ %q", p.Slug, f.Q)
			}
		}
	}
	for _, path := range []string{"/solutions/", "/solutions/nope", "/admission-crm", "/solutions/admission-crm/x"} {
		if isSolutionPath(path) {
			t.Errorf("isSolutionPath(%q) = true", path)
		}
	}
}

// TestSolutionTemplate renders every solution page, and the landing page that
// links to them, through the website's templates.
func TestSolutionTemplate(t *testing.T) {
	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"Date": func(f string) string { return time.Now().Format(f) },
	}).ParseFiles("../static/public/web-templates/solution.html", "../static/public/web-templates/website.html", "../static/public/web-templates/landing.html"))
	base := map[string]any{
		"Product": "ConnectDesk", "BaseDomain": "example.com", "BaseURL": "https://example.com", "GuideURL": "",
		"Company": "ConnectDesk", "ContactEmail": "", "PrivacyURL": "/privacy", "TermsURL": "/terms", "OGImage": "",
		"GAID": "", "SiteVerification": "", "InstagramURL": defaultInstagramURL, "InstagramHandle": "getconnectdesk",
		"DemoURL": "https://example.com/demo", "Canonical": "", "Description": "d",
	}
	for _, p := range solutionPages {
		d := map[string]any{}
		for k, v := range base {
			d[k] = v
		}
		page := p.withProduct("ConnectDesk")
		d["Page"], d["Title"], d["Others"] = page, page.Title+" | ConnectDesk", solutionPages
		d["Canonical"] = "https://example.com" + solutionsPrefix + p.Slug
		d["JSONLD"] = solutionJSONLD(d, page)
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "solution", map[string]any{"Data": d}); err != nil {
			t.Fatalf("%s: %v", p.Slug, err)
		}
		html := buf.String()
		for _, want := range []string{"<h1>" + template.HTMLEscapeString(page.H1) + "</h1>", `"@type":"FAQPage"`, `rel="canonical" href="https://example.com/solutions/` + p.Slug} {
			if !strings.Contains(html, want) {
				t.Errorf("%s: page lacks %q", p.Slug, want)
			}
		}
	}
	d := map[string]any{}
	for k, v := range base {
		d[k] = v
	}
	d["Title"], d["DemoVideoURL"], d["DemoVideoFile"] = "ConnectDesk", "", false
	d["FAQs"], d["Solutions"] = landingFAQs("ConnectDesk", "example.com"), solutionPages
	d["JSONLD"] = landingJSONLD(d, d["FAQs"].([]landingFAQ))
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "landing", map[string]any{"Data": d}); err != nil {
		t.Fatal(err)
	}
	for _, p := range solutionPages {
		if !strings.Contains(buf.String(), `href="/solutions/`+p.Slug+`"`) {
			t.Errorf("landing page doesn't link to %s", p.Slug)
		}
	}
}
