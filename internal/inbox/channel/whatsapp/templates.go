package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// templateCacheTTL is how long an inbox reuses the template list before
	// asking Meta again. Approvals take minutes to hours, so this is plenty.
	templateCacheTTL = 5 * time.Minute

	// maxTemplatePages bounds pagination through a WABA's templates.
	maxTemplatePages = 10

	// maxHeaderParam and maxParam are WhatsApp's limits on a parameter's length.
	maxHeaderParam = 60
	maxParam       = 1024

	// MetaTemplateKey is the message meta key a prepared template travels
	// under from the reply to Send.
	MetaTemplateKey = "whatsapp_template"

	// WindowDuration is how long after a contact's last message a business may
	// send them free-form messages. After it, only a template.
	WindowDuration = 24 * time.Hour
)

var placeholderRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)

// Template is a message template on the WhatsApp Business Account, as Meta
// returns it.
type Template struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Language        string              `json:"language"`
	Status          string              `json:"status"`
	Category        string              `json:"category"`
	ParameterFormat string              `json:"parameter_format"`
	Components      []TemplateComponent `json:"components"`
}

// TemplateComponent is a template's header, body, footer or buttons.
type TemplateComponent struct {
	Type    string `json:"type"`
	Format  string `json:"format"`
	Text    string `json:"text"`
	Buttons []struct {
		Type string `json:"type"`
		Text string `json:"text"`
		URL  string `json:"url"`
	} `json:"buttons"`
}

// TemplateSummary is a template as the reply box shows it: its text and the
// values an agent fills in.
type TemplateSummary struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Category string `json:"category"`
	Header   string `json:"header"`
	Body     string `json:"body"`
	Footer   string `json:"footer"`
	// HeaderParams, BodyParams and ButtonParams name the placeholders to fill,
	// in order: "1", "2" for numbered templates, or the names of named ones.
	HeaderParams []string `json:"header_params"`
	BodyParams   []string `json:"body_params"`
	ButtonParams []string `json:"button_params"`
	// Buttons are the labels of the template's buttons, for the preview.
	Buttons []string `json:"buttons"`
	// HeaderMedia is image, video or document for a template whose header is
	// a file the agent attaches when sending it.
	HeaderMedia string `json:"header_media,omitempty"`
	// Supported is false for templates ConectDesk can't send yet: those with a
	// media or location header, or buttons such as copy code or catalog.
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
}

// TemplateRequest is the values an agent fills into a template.
type TemplateRequest struct {
	Name     string   `json:"name"`
	Language string   `json:"language"`
	Header   []string `json:"header"`
	Body     []string `json:"body"`
	Buttons  []string `json:"buttons"`
}

// PreparedTemplate is a template message ready for the Cloud API, stored on
// the outgoing message's meta so Send posts exactly what the agent saw.
type PreparedTemplate struct {
	Name       string           `json:"name"`
	Language   string           `json:"language"`
	Components []map[string]any `json:"components,omitempty"`
	// HeaderMedia is set for a media header, sent with the message's one
	// attachment: image, video or document.
	HeaderMedia string `json:"header_media,omitempty"`
}

// Templates returns the WhatsApp Business Account's approved templates,
// cached for a few minutes. refresh skips the cache.
func (w *WhatsApp) Templates(ctx context.Context, refresh bool) ([]Template, error) {
	w.tmu.Lock()
	defer w.tmu.Unlock()
	if !refresh && w.templates != nil && time.Since(w.templatesAt) < templateCacheTTL {
		return w.templates, nil
	}
	if w.config.WABAID == "" {
		return nil, fmt.Errorf("whatsapp inbox %d has no business account", w.id)
	}

	var (
		out   = []Template{}
		after string
	)
	for i := 0; i < maxTemplatePages; i++ {
		q := url.Values{
			"fields": {"id,name,language,status,category,parameter_format,components"},
			"limit":  {"100"},
			"status": {"APPROVED"},
		}
		if after != "" {
			q.Set("after", after)
		}
		var page struct {
			Data   []Template `json:"data"`
			Paging struct {
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := w.graph.Get(ctx, w.config.AccessToken, w.config.WABAID+"/message_templates", q, &page); err != nil {
			return nil, err
		}
		for _, t := range page.Data {
			// The status filter is a hint on some versions; keep only approved.
			if t.Status == "APPROVED" {
				out = append(out, t)
			}
		}
		if page.Paging.Next == "" || page.Paging.Cursors.After == "" {
			break
		}
		after = page.Paging.Cursors.After
	}

	w.templates, w.templatesAt = out, time.Now()
	return out, nil
}

// FindTemplate returns an approved template by name and language.
func (w *WhatsApp) FindTemplate(ctx context.Context, name, language string) (Template, error) {
	templates, err := w.Templates(ctx, false)
	if err != nil {
		return Template{}, err
	}
	for _, t := range templates {
		if t.Name == name && t.Language == language {
			return t, nil
		}
	}
	// A template approved in the last few minutes isn't in the cache yet.
	if templates, err = w.Templates(ctx, true); err != nil {
		return Template{}, err
	}
	for _, t := range templates {
		if t.Name == name && t.Language == language {
			return t, nil
		}
	}
	return Template{}, ErrTemplateNotFound
}

// ErrTemplateNotFound is returned for a template that isn't approved on the account.
var ErrTemplateNotFound = fmt.Errorf("template not found or not approved")

// Summarize describes a template for the reply box.
func Summarize(t Template) TemplateSummary {
	s := TemplateSummary{
		Name:         t.Name,
		Language:     t.Language,
		Category:     t.Category,
		HeaderParams: []string{},
		BodyParams:   []string{},
		ButtonParams: []string{},
		Buttons:      []string{},
		Supported:    true,
	}
	for _, c := range t.Components {
		switch strings.ToUpper(c.Type) {
		case "HEADER":
			switch f := strings.ToUpper(c.Format); f {
			case "", "TEXT":
			case "IMAGE", "VIDEO", "DOCUMENT":
				s.HeaderMedia = strings.ToLower(f)
				continue
			default:
				s.Supported, s.Reason = false, "header_"+strings.ToLower(f)
				continue
			}
			s.Header = c.Text
			s.HeaderParams = placeholders(c.Text)
		case "BODY":
			s.Body = c.Text
			s.BodyParams = placeholders(c.Text)
		case "FOOTER":
			s.Footer = c.Text
		case "BUTTONS":
			for _, b := range c.Buttons {
				s.Buttons = append(s.Buttons, b.Text)
				switch strings.ToUpper(b.Type) {
				case "QUICK_REPLY", "PHONE_NUMBER":
				case "URL":
					if ps := placeholders(b.URL); len(ps) > 0 {
						s.ButtonParams = append(s.ButtonParams, ps[0])
					}
				default:
					s.Supported, s.Reason = false, "button_"+strings.ToLower(b.Type)
				}
			}
		default:
			s.Supported, s.Reason = false, "component_"+strings.ToLower(c.Type)
		}
	}
	return s
}

// Prepare checks an agent's values against a template and returns the
// message for the Cloud API with the text it will show the contact, which is
// stored as the message's content so the conversation reads like the chat.
func Prepare(t Template, req TemplateRequest) (PreparedTemplate, string, error) {
	s := Summarize(t)
	if !s.Supported {
		return PreparedTemplate{}, "", fmt.Errorf("template %q can't be sent from the helpdesk (%s)", t.Name, s.Reason)
	}

	header, err := cleanParams(req.Header, s.HeaderParams, maxHeaderParam)
	if err != nil {
		return PreparedTemplate{}, "", fmt.Errorf("header: %w", err)
	}
	body, err := cleanParams(req.Body, s.BodyParams, maxParam)
	if err != nil {
		return PreparedTemplate{}, "", fmt.Errorf("body: %w", err)
	}
	buttons, err := cleanParams(req.Buttons, s.ButtonParams, maxParam)
	if err != nil {
		return PreparedTemplate{}, "", fmt.Errorf("buttons: %w", err)
	}

	named := strings.EqualFold(t.ParameterFormat, "NAMED")
	params := func(names, values []string) []map[string]any {
		out := make([]map[string]any, len(values))
		for i, v := range values {
			p := map[string]any{"type": "text", "text": v}
			if named {
				p["parameter_name"] = names[i]
			}
			out[i] = p
		}
		return out
	}

	pt := PreparedTemplate{Name: t.Name, Language: t.Language, HeaderMedia: s.HeaderMedia}
	if len(header) > 0 {
		pt.Components = append(pt.Components, map[string]any{"type": "header", "parameters": params(s.HeaderParams, header)})
	}
	if len(body) > 0 {
		pt.Components = append(pt.Components, map[string]any{"type": "body", "parameters": params(s.BodyParams, body)})
	}
	// URL buttons are addressed by their position among all buttons.
	next := 0
	for _, c := range t.Components {
		if strings.ToUpper(c.Type) != "BUTTONS" {
			continue
		}
		for i, b := range c.Buttons {
			if strings.ToUpper(b.Type) != "URL" || len(placeholders(b.URL)) == 0 {
				continue
			}
			pt.Components = append(pt.Components, map[string]any{
				"type":       "button",
				"sub_type":   "url",
				"index":      strconv.Itoa(i),
				"parameters": []map[string]any{{"type": "text", "text": buttons[next]}},
			})
			next++
		}
	}

	var parts []string
	if s.Header != "" {
		parts = append(parts, fill(s.Header, s.HeaderParams, header))
	}
	parts = append(parts, fill(s.Body, s.BodyParams, body))
	if s.Footer != "" {
		parts = append(parts, s.Footer)
	}
	return pt, strings.Join(parts, "\n\n"), nil
}

// CheckHeaderMedia checks a file can be a template's media header: WhatsApp
// takes JPEG and PNG images, MP4 and 3GP videos, and any document.
func CheckHeaderMedia(kind, contentType string) error {
	switch kind {
	case "image":
		if contentType != "image/jpeg" && contentType != "image/png" {
			return fmt.Errorf("the header needs a JPEG or PNG image")
		}
	case "video":
		if contentType != "video/mp4" && contentType != "video/3gpp" {
			return fmt.Errorf("the header needs an MP4 or 3GP video")
		}
	case "document":
	default:
		return fmt.Errorf("this template has no media header")
	}
	return nil
}

// RenderHTML turns a template's plain text into the HTML messages are stored as.
func RenderHTML(text string) string {
	var b strings.Builder
	for _, para := range strings.Split(text, "\n\n") {
		b.WriteString("<p>")
		b.WriteString(strings.ReplaceAll(html.EscapeString(para), "\n", "<br>"))
		b.WriteString("</p>")
	}
	return b.String()
}

// TemplateFromMeta returns the prepared template carried on a message's meta, if any.
func TemplateFromMeta(meta json.RawMessage) (PreparedTemplate, bool) {
	if len(meta) == 0 {
		return PreparedTemplate{}, false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(meta, &m) != nil || len(m[MetaTemplateKey]) == 0 {
		return PreparedTemplate{}, false
	}
	var pt PreparedTemplate
	if json.Unmarshal(m[MetaTemplateKey], &pt) != nil || pt.Name == "" {
		return PreparedTemplate{}, false
	}
	return pt, true
}

// WindowOpen reports whether a contact who last wrote at lastIncoming can
// still be sent a free-form message.
func WindowOpen(lastIncoming time.Time, now time.Time) bool {
	return !lastIncoming.IsZero() && now.Sub(lastIncoming) < WindowDuration
}

// placeholders lists a text's placeholders in order of first appearance.
func placeholders(text string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, m := range placeholderRe.FindAllStringSubmatch(text, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

// cleanParams checks the values given for a component's placeholders.
// WhatsApp refuses a parameter with a newline or tab, so those become spaces.
func cleanParams(values, names []string, max int) ([]string, error) {
	if len(values) != len(names) {
		return nil, fmt.Errorf("want %d value(s), got %d", len(names), len(values))
	}
	out := make([]string, len(values))
	for i, v := range values {
		v = strings.Join(strings.Fields(v), " ")
		if v == "" {
			return nil, fmt.Errorf("{{%s}} is empty", names[i])
		}
		if len(v) > max {
			return nil, fmt.Errorf("{{%s}} is longer than %d characters", names[i], max)
		}
		out[i] = v
	}
	return out, nil
}

// fill replaces a text's placeholders with their values.
func fill(text string, names, values []string) string {
	byName := map[string]string{}
	for i, n := range names {
		if i < len(values) {
			byName[n] = values[i]
		}
	}
	return placeholderRe.ReplaceAllStringFunc(text, func(m string) string {
		name := placeholderRe.FindStringSubmatch(m)[1]
		if v, ok := byName[name]; ok {
			return v
		}
		return m
	})
}
