// Package docs holds the internal documentation served on docs.<base_domain>:
// Markdown pages embedded in the binary, rendered to HTML once at startup.
package docs

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

//go:embed content/*.md
var content embed.FS

// Heading is a section of a page, linked from the sidebar and found by search.
type Heading struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Level int    `json:"level"`
	// Text is the section's plain text, for search.
	Text string `json:"text"`
}

// Page is one Markdown file. Files are ordered by name, so a numeric prefix
// such as "10-" sets a page's place; the slug drops it.
type Page struct {
	Slug     string
	Title    string
	HTML     template.HTML
	Headings []Heading
}

// Docs is the rendered documentation.
type Docs struct {
	Pages  []Page
	bySlug map[string]int
}

// Load renders every embedded page.
func Load() (*Docs, error) {
	names, err := fs.Glob(content, "content/*.md")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		// The pages are ours, so they may use inline HTML such as <kbd>.
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	d := &Docs{bySlug: map[string]int{}}
	for _, name := range names {
		src, err := content.ReadFile(name)
		if err != nil {
			return nil, err
		}
		p, err := render(md, src)
		if err != nil {
			return nil, fmt.Errorf("rendering %s: %w", name, err)
		}
		p.Slug = slugOf(name)
		if p.Title == "" {
			p.Title = p.Slug
		}
		d.bySlug[p.Slug] = len(d.Pages)
		d.Pages = append(d.Pages, p)
	}
	if len(d.Pages) == 0 {
		return nil, fmt.Errorf("no documentation pages")
	}
	return d, nil
}

// Page returns the page with the slug, the first page for "".
func (d *Docs) Page(slug string) (Page, int, bool) {
	if slug == "" {
		return d.Pages[0], 0, true
	}
	i, ok := d.bySlug[slug]
	if !ok {
		return Page{}, 0, false
	}
	return d.Pages[i], i, true
}

func slugOf(name string) string {
	s := strings.TrimSuffix(path.Base(name), ".md")
	if i := strings.IndexByte(s, '-'); i > 0 && strings.Trim(s[:i], "0123456789") == "" {
		s = s[i+1:]
	}
	return s
}

// render turns a page's Markdown into HTML, its title (the first level-one
// heading) and its level two and three headings with their text.
func render(md goldmark.Markdown, src []byte) (Page, error) {
	var p Page
	doc := md.Parser().Parse(text.NewReader(src))

	var (
		cur   *Heading
		title ast.Node
	)
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*ast.Heading); ok {
			text := plainText(h, src)
			if h.Level == 1 && title == nil {
				// The page shows its title itself.
				p.Title, title = text, h
				continue
			}
			if h.Level <= 3 {
				id, _ := h.AttributeString("id")
				idStr, _ := id.([]byte)
				p.Headings = append(p.Headings, Heading{ID: string(idStr), Title: text, Level: h.Level})
				cur = &p.Headings[len(p.Headings)-1]
				continue
			}
		}
		if cur != nil && len(cur.Text) < 4000 {
			cur.Text += " " + plainText(n, src)
		}
	}

	if title != nil {
		doc.RemoveChild(doc, title)
	}
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return p, err
	}
	p.HTML = template.HTML(buf.String())
	return p, nil
}

// plainText is the text of a node and its children.
func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(src))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *ast.CodeSpan:
			for cc := t.FirstChild(); cc != nil; cc = cc.NextSibling() {
				if tt, ok := cc.(*ast.Text); ok {
					b.Write(tt.Segment.Value(src))
				}
			}
			return ast.WalkSkipChildren, nil
		}
		if c.Type() == ast.TypeBlock && c != n {
			b.WriteByte(' ')
		}
		return ast.WalkContinue, nil
	})
	return strings.Join(strings.Fields(b.String()), " ")
}
