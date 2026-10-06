package docs

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// TestLoad renders the embedded pages and checks each has a title, a unique
// slug and its sections with ids, and that the title isn't rendered twice.
func TestLoad(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range d.Pages {
		if p.Title == "" || p.Slug == "" {
			t.Errorf("page %q has no title or slug", p.Slug)
		}
		if seen[p.Slug] {
			t.Errorf("slug %q used twice", p.Slug)
		}
		seen[p.Slug] = true
		if strings.Contains(string(p.HTML), "<h1") {
			t.Errorf("page %q renders its title, which the page template shows", p.Slug)
		}
		for _, h := range p.Headings {
			if h.ID == "" || !strings.Contains(string(p.HTML), `id="`+h.ID+`"`) {
				t.Errorf("page %q heading %q has no matching id", p.Slug, h.Title)
			}
		}
	}
	first, i, ok := d.Page("")
	if !ok || i != 0 || first.Slug != d.Pages[0].Slug {
		t.Errorf("Page(\"\") = %q, %d, %v; want the first page", first.Slug, i, ok)
	}
	if _, _, ok := d.Page("no-such-page"); ok {
		t.Error("Page found an unknown slug")
	}
}

func TestSlugOf(t *testing.T) {
	for in, want := range map[string]string{
		"content/00-welcome.md":  "welcome",
		"content/10-inbox.md":    "inbox",
		"content/platform.md":    "platform",
		"content/a-b.md":         "a-b",
		"content/2fa-and-sso.md": "2fa-and-sso",
	} {
		if got := slugOf(in); got != want {
			t.Errorf("slugOf(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLinks checks every link from one page to another, and to a section of
// it, lands somewhere.
func TestLinks(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	link := regexp.MustCompile(`href="/docs/([^"#]*)(?:#([^"]*))?"`)
	for _, p := range d.Pages {
		for _, m := range link.FindAllStringSubmatch(string(p.HTML), -1) {
			target, _, ok := d.Page(m[1])
			if !ok {
				t.Errorf("page %q links to unknown page %q", p.Slug, m[1])
				continue
			}
			if m[2] != "" && !strings.Contains(string(target.HTML), `id="`+m[2]+`"`) {
				t.Errorf("page %q links to unknown section %q of %q", p.Slug, m[2], m[1])
			}
		}
	}
}

// TestGuide checks the client guide renders, its links land on a page and
// section of it, every screenshot it shows exists, and every screenshot in
// guide/img is shown somewhere.
func TestGuide(t *testing.T) {
	d, err := LoadGuide()
	if err != nil {
		t.Fatalf("LoadGuide: %v", err)
	}
	link := regexp.MustCompile(`href="/guide/([^"#]*)(?:#([^"]*))?"`)
	img := regexp.MustCompile(`src="/guide/img/([^"]+)"`)
	used := map[string]bool{}
	for _, p := range d.Pages {
		if p.Title == "" || strings.Contains(string(p.HTML), "<h1") {
			t.Errorf("guide page %q has no title, or renders it twice", p.Slug)
		}
		for _, m := range link.FindAllStringSubmatch(string(p.HTML), -1) {
			target, _, ok := d.Page(m[1])
			if !ok {
				t.Errorf("guide page %q links to unknown page %q", p.Slug, m[1])
				continue
			}
			if m[2] != "" && !strings.Contains(string(target.HTML), `id="`+m[2]+`"`) {
				t.Errorf("guide page %q links to unknown section %q of %q", p.Slug, m[2], m[1])
			}
		}
		for _, m := range img.FindAllStringSubmatch(string(p.HTML), -1) {
			used[m[1]] = true
			if _, err := fs.Stat(GuideImages(), m[1]); err != nil {
				t.Errorf("guide page %q shows missing screenshot %q", p.Slug, m[1])
			}
		}
	}
	files, _ := fs.ReadDir(GuideImages(), ".")
	for _, f := range files {
		if !used[f.Name()] {
			t.Errorf("screenshot %q isn't used by any guide page", f.Name())
		}
	}
}
