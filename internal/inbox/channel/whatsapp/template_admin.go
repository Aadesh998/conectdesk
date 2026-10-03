package whatsapp

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Limits Meta puts on a template.
const (
	maxTemplateName   = 512
	maxTemplateBody   = 1024
	maxTemplateHeader = 60
	maxTemplateFooter = 60
	maxButtonText     = 25
	maxQuickReplies   = 10
	maxCallToActions  = 2
)

var (
	templateNameRe = regexp.MustCompile(`^[a-z0-9_]+$`)
	languageRe     = regexp.MustCompile(`^[a-z]{2,3}(_[A-Z]{2})?$`)
	numberedRe     = regexp.MustCompile(`\{\{\s*(\d+)\s*\}\}`)
)

// ManagedTemplate is a template as the admin page lists it, whatever its status.
type ManagedTemplate struct {
	TemplateSummary
	ID             string `json:"id"`
	Status         string `json:"status"`
	RejectedReason string `json:"rejected_reason"`
	Quality        string `json:"quality"`
	HeaderFormat   string `json:"header_format"`
}

// TemplateDraft is a new template an admin submits for Meta's review.
// Variables are numbered, {{1}}, {{2}}, and each needs a sample value: Meta
// reviews the template with them.
type TemplateDraft struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	// Category is MARKETING or UTILITY. Authentication templates carry
	// one-time codes and aren't for a helpdesk.
	Category string `json:"category"`

	// HeaderFormat is "", TEXT, IMAGE, VIDEO or DOCUMENT.
	HeaderFormat  string `json:"header_format"`
	HeaderText    string `json:"header_text"`
	HeaderExample string `json:"header_example"`
	// HeaderHandle is the uploaded sample of a media header, set by the caller.
	HeaderHandle string `json:"-"`

	Body         string        `json:"body"`
	BodyExamples []string      `json:"body_examples"`
	Footer       string        `json:"footer"`
	Buttons      []DraftButton `json:"buttons"`
}

// DraftButton is a button on a new template.
type DraftButton struct {
	// Type is QUICK_REPLY, URL or PHONE_NUMBER.
	Type string `json:"type"`
	Text string `json:"text"`
	// URL may end in {{1}}, filled in when the template is sent.
	URL        string `json:"url"`
	URLExample string `json:"url_example"`
	Phone      string `json:"phone"`
}

// ListTemplates returns every template on the WhatsApp Business Account,
// approved or not, for the admin page.
func (w *WhatsApp) ListTemplates(ctx context.Context) ([]ManagedTemplate, error) {
	if w.config.WABAID == "" {
		return nil, fmt.Errorf("whatsapp inbox %d has no business account", w.id)
	}
	var (
		out   = []ManagedTemplate{}
		after string
	)
	for i := 0; i < maxTemplatePages; i++ {
		q := url.Values{
			"fields": {"id,name,language,status,category,parameter_format,components,rejected_reason,quality_score"},
			"limit":  {"100"},
		}
		if after != "" {
			q.Set("after", after)
		}
		var page struct {
			Data []struct {
				Template
				RejectedReason string `json:"rejected_reason"`
				QualityScore   struct {
					Score string `json:"score"`
				} `json:"quality_score"`
			} `json:"data"`
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
			mt := ManagedTemplate{
				TemplateSummary: Summarize(t.Template),
				ID:              t.ID,
				Status:          t.Status,
				Quality:         t.QualityScore.Score,
			}
			if t.RejectedReason != "" && t.RejectedReason != "NONE" {
				mt.RejectedReason = t.RejectedReason
			}
			for _, c := range t.Components {
				if strings.EqualFold(c.Type, "HEADER") {
					mt.HeaderFormat = strings.ToUpper(c.Format)
				}
			}
			out = append(out, mt)
		}
		if page.Paging.Next == "" || page.Paging.Cursors.After == "" {
			break
		}
		after = page.Paging.Cursors.After
	}
	return out, nil
}

// CreateTemplate submits a template for Meta's review and returns its ID and
// status, usually PENDING; many are approved within minutes.
func (w *WhatsApp) CreateTemplate(ctx context.Context, d TemplateDraft) (string, string, error) {
	components, err := BuildTemplateComponents(d)
	if err != nil {
		return "", "", err
	}
	body := map[string]any{
		"name":       d.Name,
		"language":   d.Language,
		"category":   d.Category,
		"components": components,
	}
	var out struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Category string `json:"category"`
	}
	if err := w.graph.PostJSON(ctx, w.config.AccessToken, w.config.WABAID+"/message_templates", body, &out); err != nil {
		return "", "", err
	}
	w.InvalidateTemplates()
	return out.ID, out.Status, nil
}

// DeleteTemplate deletes a template. With an ID only that language is
// deleted; without one, every language of the name.
func (w *WhatsApp) DeleteTemplate(ctx context.Context, name, id string) error {
	q := url.Values{"name": {name}}
	if id != "" {
		q.Set("hsm_id", id)
	}
	if err := w.graph.DeleteQuery(ctx, w.config.AccessToken, w.config.WABAID+"/message_templates", q); err != nil {
		return err
	}
	w.InvalidateTemplates()
	return nil
}

// UploadTemplateSample uploads the sample file a media header template is
// reviewed with.
func (w *WhatsApp) UploadTemplateSample(ctx context.Context, name, contentType string, content []byte) (string, error) {
	return w.graph.UploadHandle(ctx, w.config.AccessToken, name, contentType, content)
}

// InvalidateTemplates drops the cached template list, after a template was
// created or deleted, or Meta reported a review.
func (w *WhatsApp) InvalidateTemplates() {
	w.tmu.Lock()
	w.templates = nil
	w.tmu.Unlock()
}

// BuildTemplateComponents checks a draft against Meta's rules and returns the
// components to submit, samples included.
func BuildTemplateComponents(d TemplateDraft) ([]map[string]any, error) {
	d.Name = strings.TrimSpace(d.Name)
	if !templateNameRe.MatchString(d.Name) || len(d.Name) > maxTemplateName {
		return nil, fmt.Errorf("the name may only have lowercase letters, digits and underscores")
	}
	if !languageRe.MatchString(d.Language) {
		return nil, fmt.Errorf("unknown language %q", d.Language)
	}
	if d.Category != "MARKETING" && d.Category != "UTILITY" {
		return nil, fmt.Errorf("the category must be marketing or utility")
	}

	var components []map[string]any

	switch f := strings.ToUpper(d.HeaderFormat); f {
	case "":
	case "TEXT":
		text := strings.TrimSpace(d.HeaderText)
		if text == "" || len(text) > maxTemplateHeader {
			return nil, fmt.Errorf("the header must be 1 to %d characters", maxTemplateHeader)
		}
		vars, err := numberedVars(text)
		if err != nil {
			return nil, fmt.Errorf("header: %w", err)
		}
		if len(vars) > 1 {
			return nil, fmt.Errorf("the header can have one variable at most")
		}
		c := map[string]any{"type": "HEADER", "format": "TEXT", "text": text}
		if len(vars) == 1 {
			if strings.TrimSpace(d.HeaderExample) == "" {
				return nil, fmt.Errorf("give a sample value for the header's {{1}}")
			}
			c["example"] = map[string]any{"header_text": []string{strings.TrimSpace(d.HeaderExample)}}
		}
		components = append(components, c)
	case "IMAGE", "VIDEO", "DOCUMENT":
		if d.HeaderHandle == "" {
			return nil, fmt.Errorf("upload a sample %s for the header", strings.ToLower(f))
		}
		components = append(components, map[string]any{
			"type": "HEADER", "format": f,
			"example": map[string]any{"header_handle": []string{d.HeaderHandle}},
		})
	default:
		return nil, fmt.Errorf("unknown header format %q", d.HeaderFormat)
	}

	body := strings.TrimSpace(d.Body)
	if body == "" || len(body) > maxTemplateBody {
		return nil, fmt.Errorf("the message must be 1 to %d characters", maxTemplateBody)
	}
	vars, err := numberedVars(body)
	if err != nil {
		return nil, fmt.Errorf("message: %w", err)
	}
	// Meta rejects a template that starts or ends with a variable.
	trimmed := strings.TrimSpace(numberedRe.ReplaceAllString(body, "\x00"))
	if strings.HasPrefix(trimmed, "\x00") || strings.HasSuffix(trimmed, "\x00") {
		return nil, fmt.Errorf("the message can't start or end with a variable")
	}
	bc := map[string]any{"type": "BODY", "text": body}
	if len(vars) > 0 {
		if len(d.BodyExamples) != len(vars) {
			return nil, fmt.Errorf("give a sample value for each of the message's %d variable(s)", len(vars))
		}
		examples := make([]string, len(vars))
		for i, e := range d.BodyExamples {
			if e = strings.TrimSpace(e); e == "" {
				return nil, fmt.Errorf("give a sample value for {{%d}}", i+1)
			}
			examples[i] = e
		}
		bc["example"] = map[string]any{"body_text": [][]string{examples}}
	}
	components = append(components, bc)

	if footer := strings.TrimSpace(d.Footer); footer != "" {
		if len(footer) > maxTemplateFooter {
			return nil, fmt.Errorf("the footer can be %d characters at most", maxTemplateFooter)
		}
		if numberedRe.MatchString(footer) {
			return nil, fmt.Errorf("the footer can't have variables")
		}
		components = append(components, map[string]any{"type": "FOOTER", "text": footer})
	}

	if len(d.Buttons) > 0 {
		buttons, err := buildButtons(d.Buttons)
		if err != nil {
			return nil, err
		}
		components = append(components, map[string]any{"type": "BUTTONS", "buttons": buttons})
	}
	return components, nil
}

func buildButtons(in []DraftButton) ([]map[string]any, error) {
	var (
		out                 = []map[string]any{}
		quick, cta          int
		lastWasQuick, mixed bool
	)
	for i, b := range in {
		text := strings.TrimSpace(b.Text)
		if text == "" || len(text) > maxButtonText {
			return nil, fmt.Errorf("button %d's label must be 1 to %d characters", i+1, maxButtonText)
		}
		switch b.Type {
		case "QUICK_REPLY":
			quick++
			if i > 0 && !lastWasQuick {
				mixed = true
			}
			lastWasQuick = true
			out = append(out, map[string]any{"type": "QUICK_REPLY", "text": text})
		case "URL":
			cta++
			if i > 0 && lastWasQuick {
				mixed = true
			}
			lastWasQuick = false
			u := strings.TrimSpace(b.URL)
			parsed, err := url.Parse(strings.Replace(u, "{{1}}", "x", 1))
			if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
				return nil, fmt.Errorf("button %d needs a full web address", i+1)
			}
			btn := map[string]any{"type": "URL", "text": text, "url": u}
			if strings.Contains(u, "{{") {
				if !strings.HasSuffix(u, "{{1}}") || strings.Count(u, "{{") != 1 {
					return nil, fmt.Errorf("button %d's address can only end in {{1}}", i+1)
				}
				if strings.TrimSpace(b.URLExample) == "" {
					return nil, fmt.Errorf("give a sample address for button %d", i+1)
				}
				btn["example"] = []string{strings.TrimSpace(b.URLExample)}
			}
			out = append(out, btn)
		case "PHONE_NUMBER":
			cta++
			if i > 0 && lastWasQuick {
				mixed = true
			}
			lastWasQuick = false
			phone := strings.TrimSpace(b.Phone)
			if !strings.HasPrefix(phone, "+") || !IsValidWAID(WAIDFromPhone(phone)) {
				return nil, fmt.Errorf("button %d needs a phone number with its country code, e.g. +91…", i+1)
			}
			out = append(out, map[string]any{"type": "PHONE_NUMBER", "text": text, "phone_number": phone})
		default:
			return nil, fmt.Errorf("unknown button type %q", b.Type)
		}
	}
	if quick > maxQuickReplies {
		return nil, fmt.Errorf("a template can have %d quick replies at most", maxQuickReplies)
	}
	if cta > maxCallToActions {
		return nil, fmt.Errorf("a template can have %d link or call buttons at most", maxCallToActions)
	}
	// Meta groups quick replies apart from link and call buttons.
	if mixed && quick > 0 && cta > 0 {
		return nil, fmt.Errorf("put the quick replies together, before or after the link and call buttons")
	}
	return out, nil
}

// numberedVars returns a text's variables, which must be {{1}}, {{2}}, … in order.
func numberedVars(text string) ([]int, error) {
	if placeholderRe.MatchString(text) && !numberedRe.MatchString(text) {
		return nil, fmt.Errorf("variables must be numbered: {{1}}, {{2}}, …")
	}
	var out []int
	for i, m := range numberedRe.FindAllStringSubmatch(text, -1) {
		n, _ := strconv.Atoi(m[1])
		if n != i+1 {
			return nil, fmt.Errorf("variables must go {{1}}, {{2}}, … in order, each used once")
		}
		out = append(out, n)
	}
	if len(numberedRe.FindAllString(text, -1)) != len(placeholderRe.FindAllString(text, -1)) {
		return nil, fmt.Errorf("variables must be numbered: {{1}}, {{2}}, …")
	}
	return out, nil
}
