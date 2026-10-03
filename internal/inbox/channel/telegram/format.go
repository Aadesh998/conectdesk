package telegram

import (
	"html"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

// telegramTags maps the editor's tags to the ones Telegram's HTML parse mode
// knows. Anything else is dropped and only its text kept.
var telegramTags = map[string]string{
	"b": "b", "strong": "b",
	"i": "i", "em": "i",
	"u": "u", "ins": "u",
	"s": "s", "strike": "s", "del": "s",
	"code":       "code",
	"pre":        "pre",
	"blockquote": "blockquote",
	"a":          "a",
}

var extraNewlines = regexp.MustCompile(`\n{3,}`)

// FormatHTML converts the HTML agents write in the rich editor to Telegram's
// HTML subset: bold, italic, underline, strikethrough, links, code and quotes
// keep their formatting; paragraphs and lists become lines. Text is escaped,
// every tag emitted is closed, and links other than web, mail and tg: ones
// keep only their text.
func FormatHTML(in string) string {
	var (
		b     strings.Builder
		open  []string
		z     = xhtml.NewTokenizer(strings.NewReader(in))
		skip  int
		lists []int // item counters, 0 for bulleted lists
	)
	for {
		tt := z.Next()
		switch tt {
		case xhtml.ErrorToken:
			for i := len(open) - 1; i >= 0; i-- {
				b.WriteString("</" + open[i] + ">")
			}
			out := strings.ReplaceAll(b.String(), " ", " ")
			return strings.TrimSpace(extraNewlines.ReplaceAllString(out, "\n\n"))
		case xhtml.TextToken:
			if skip == 0 {
				b.WriteString(html.EscapeString(string(z.Text())))
			}
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			switch tag {
			case "script", "style":
				if tt == xhtml.StartTagToken {
					skip++
				}
				continue
			case "br":
				b.WriteString("\n")
				continue
			case "ul":
				lists = append(lists, 0)
				continue
			case "ol":
				lists = append(lists, 1)
				continue
			case "li":
				if n := len(lists); n > 0 && lists[n-1] > 0 {
					b.WriteString(strings.Repeat("  ", n-1) + itoa(lists[n-1]) + ". ")
					lists[n-1]++
				} else {
					b.WriteString(strings.Repeat("  ", max(n-1, 0)) + "• ")
				}
				continue
			}
			tgTag, ok := telegramTags[tag]
			if !ok || tt == xhtml.SelfClosingTagToken {
				continue
			}
			if tgTag == "a" {
				href := ""
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					if string(k) == "href" {
						href = string(v)
					}
				}
				if !safeLink(href) {
					// Kept open as a no-op so the matching end tag lines up.
					open = append(open, "")
					continue
				}
				b.WriteString(`<a href="` + html.EscapeString(href) + `">`)
			} else {
				b.WriteString("<" + tgTag + ">")
			}
			open = append(open, tgTag)
		case xhtml.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			switch tag {
			case "script", "style":
				if skip > 0 {
					skip--
				}
				continue
			case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6":
				b.WriteString("\n\n")
				continue
			case "li":
				b.WriteString("\n")
				continue
			case "ul", "ol":
				if len(lists) > 0 {
					lists = lists[:len(lists)-1]
				}
				if len(lists) == 0 {
					b.WriteString("\n")
				}
				continue
			}
			tgTag, ok := telegramTags[tag]
			if !ok {
				continue
			}
			// Close up to the matching open tag, so the output stays nested.
			for i := len(open) - 1; i >= 0; i-- {
				if open[i] == tgTag || (tgTag == "a" && open[i] == "") {
					for j := len(open) - 1; j >= i; j-- {
						if open[j] != "" {
							b.WriteString("</" + open[j] + ">")
						}
					}
					open = open[:i]
					break
				}
			}
			if tag == "blockquote" || tag == "pre" {
				b.WriteString("\n")
			}
		}
	}
}

func safeLink(href string) bool {
	h := strings.ToLower(strings.TrimSpace(href))
	return strings.HasPrefix(h, "https://") || strings.HasPrefix(h, "http://") ||
		strings.HasPrefix(h, "mailto:") || strings.HasPrefix(h, "tg://")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
