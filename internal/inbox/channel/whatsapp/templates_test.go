package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/attachment"
	"github.com/conectdesk/conectdesk/internal/conversation/models"
)

func decodeTemplate(t *testing.T, s string) Template {
	t.Helper()
	var tmpl Template
	if err := json.Unmarshal([]byte(s), &tmpl); err != nil {
		t.Fatal(err)
	}
	return tmpl
}

const orderTemplate = `{
	"name": "order_update", "language": "en_US", "status": "APPROVED", "category": "UTILITY",
	"parameter_format": "POSITIONAL",
	"components": [
		{"type": "HEADER", "format": "TEXT", "text": "Order {{1}}"},
		{"type": "BODY", "text": "Hi {{1}}, order {{2}} ships {{3}}. Thanks {{1}}!"},
		{"type": "FOOTER", "text": "Reply STOP to opt out"},
		{"type": "BUTTONS", "buttons": [
			{"type": "QUICK_REPLY", "text": "Stop"},
			{"type": "URL", "text": "Track", "url": "https://shop.example/track/{{1}}"}
		]}
	]
}`

func TestSummarize(t *testing.T) {
	s := Summarize(decodeTemplate(t, orderTemplate))
	if !s.Supported {
		t.Fatalf("unsupported: %s", s.Reason)
	}
	if strings.Join(s.HeaderParams, ",") != "1" || strings.Join(s.BodyParams, ",") != "1,2,3" || strings.Join(s.ButtonParams, ",") != "1" {
		t.Errorf("params: %v %v %v", s.HeaderParams, s.BodyParams, s.ButtonParams)
	}
	if s.Footer != "Reply STOP to opt out" || len(s.Buttons) != 2 {
		t.Errorf("footer/buttons: %q %v", s.Footer, s.Buttons)
	}

	for _, tc := range []struct{ tmpl, reason string }{
		{`{"name":"x","components":[{"type":"HEADER","format":"LOCATION"},{"type":"BODY","text":"hi"}]}`, "header_location"},
		{`{"name":"x","components":[{"type":"BODY","text":"{{1}}"},{"type":"BUTTONS","buttons":[{"type":"OTP","text":"Copy"}]}]}`, "button_otp"},
		{`{"name":"x","components":[{"type":"CAROUSEL"}]}`, "component_carousel"},
	} {
		s := Summarize(decodeTemplate(t, tc.tmpl))
		if s.Supported || s.Reason != tc.reason {
			t.Errorf("%s: supported=%v reason=%q", tc.reason, s.Supported, s.Reason)
		}
	}
}

func TestPreparePositional(t *testing.T) {
	pt, text, err := Prepare(decodeTemplate(t, orderTemplate), TemplateRequest{
		Header:  []string{"#42"},
		Body:    []string{"Asha", "#42", "tomorrow\nmorning"},
		Buttons: []string{"42"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "Order #42\n\nHi Asha, order #42 ships tomorrow morning. Thanks Asha!\n\nReply STOP to opt out"
	if text != want {
		t.Errorf("text:\n%q\nwant\n%q", text, want)
	}

	b, _ := json.Marshal(pt)
	got := string(b)
	for _, frag := range []string{
		`"name":"order_update"`, `"language":"en_US"`,
		`{"parameters":[{"text":"#42","type":"text"}],"type":"header"}`,
		// A newline in a value is sent as a space: WhatsApp rejects it otherwise.
		`{"text":"tomorrow morning","type":"text"}`,
		// The URL button is the second button, index 1.
		`"index":"1","parameters":[{"text":"42","type":"text"}],"sub_type":"url","type":"button"`,
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("missing %s in %s", frag, got)
		}
	}
	if strings.Contains(got, "parameter_name") {
		t.Error("positional template sent parameter names")
	}
}

func TestPrepareNamed(t *testing.T) {
	tmpl := decodeTemplate(t, `{"name":"welcome","language":"en","parameter_format":"NAMED",
		"components":[{"type":"BODY","text":"Hello {{first_name}}, welcome to {{shop}}."}]}`)
	pt, text, err := Prepare(tmpl, TemplateRequest{Body: []string{"Ravi", "Acme"}})
	if err != nil {
		t.Fatal(err)
	}
	if text != "Hello Ravi, welcome to Acme." {
		t.Errorf("text: %q", text)
	}
	b, _ := json.Marshal(pt)
	if !strings.Contains(string(b), `"parameter_name":"first_name"`) {
		t.Errorf("named params: %s", b)
	}
}

func TestPrepareRejectsBadValues(t *testing.T) {
	tmpl := decodeTemplate(t, orderTemplate)
	for name, req := range map[string]TemplateRequest{
		"missing value": {Header: []string{"#1"}, Body: []string{"a", "b"}, Buttons: []string{"1"}},
		"empty value":   {Header: []string{"#1"}, Body: []string{"a", "  ", "c"}, Buttons: []string{"1"}},
		"long header":   {Header: []string{strings.Repeat("x", 61)}, Body: []string{"a", "b", "c"}, Buttons: []string{"1"}},
		"no button":     {Header: []string{"#1"}, Body: []string{"a", "b", "c"}},
	} {
		if _, _, err := Prepare(tmpl, req); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, _, err := Prepare(decodeTemplate(t, `{"name":"x","components":[{"type":"HEADER","format":"LOCATION"}]}`), TemplateRequest{}); err == nil {
		t.Error("unsupported template prepared")
	}
}

func TestRenderHTMLEscapes(t *testing.T) {
	if got := RenderHTML("Hi <b>you</b>\nline two\n\nnext"); got != "<p>Hi &lt;b&gt;you&lt;/b&gt;<br>line two</p><p>next</p>" {
		t.Errorf("got %q", got)
	}
}

func TestWindowOpen(t *testing.T) {
	now := time.Now()
	if !WindowOpen(now.Add(-23*time.Hour), now) || WindowOpen(now.Add(-25*time.Hour), now) || WindowOpen(time.Time{}, now) {
		t.Error("window bounds")
	}
}

func TestSendTemplate(t *testing.T) {
	f := &fakeGraph{}
	w := newTestInbox(t, f)
	meta, _ := json.Marshal(map[string]any{
		"echo_id":       "x",
		MetaTemplateKey: PreparedTemplate{Name: "order_update", Language: "en_US", Components: []map[string]any{{"type": "body"}}},
	})
	if err := w.Send(models.OutboundMessage{UUID: "u", To: []string{"919876543210"}, Content: "<p>rendered</p>", Meta: meta}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls: %v", f.calls)
	}
	for _, frag := range []string{`"type":"template"`, `"name":"order_update"`, `"language":{"code":"en_US"}`} {
		if !strings.Contains(f.calls[0], frag) {
			t.Errorf("missing %s in %s", frag, f.calls[0])
		}
	}
	// The rendered copy is for the conversation, never sent as text.
	if strings.Contains(f.calls[0], "rendered") {
		t.Errorf("sent the rendered copy: %s", f.calls[0])
	}
}

func TestTemplatesCachedAndFiltered(t *testing.T) {
	f := &fakeGraph{}
	f.handler = func(w http.ResponseWriter, r *http.Request, body []byte) {
		w.Write([]byte(`{"data": [
			{"name": "a", "language": "en", "status": "APPROVED", "components": []},
			{"name": "b", "language": "en", "status": "REJECTED", "components": []}
		]}`))
	}
	w := newTestInbox(t, f)
	w.config.WABAID = "W1"

	for range 2 {
		ts, err := w.Templates(context.Background(), false)
		if err != nil {
			t.Fatal(err)
		}
		if len(ts) != 1 || ts[0].Name != "a" {
			t.Fatalf("templates: %+v", ts)
		}
	}
	if len(f.calls) != 1 || !strings.Contains(f.calls[0], "/W1/message_templates") {
		t.Errorf("want one listing call, got %v", f.calls)
	}
	if _, err := w.FindTemplate(context.Background(), "b", "en"); err != ErrTemplateNotFound {
		t.Errorf("rejected template found: %v", err)
	}
}

func TestMediaHeaderTemplate(t *testing.T) {
	tmpl := decodeTemplate(t, `{"name":"invoice","language":"en","components":[
		{"type":"HEADER","format":"DOCUMENT"},{"type":"BODY","text":"Your invoice {{1}} is attached."}]}`)
	s := Summarize(tmpl)
	if !s.Supported || s.HeaderMedia != "document" {
		t.Fatalf("summary: %+v", s)
	}
	pt, text, err := Prepare(tmpl, TemplateRequest{Body: []string{"#7"}})
	if err != nil || pt.HeaderMedia != "document" || text != "Your invoice #7 is attached." {
		t.Fatalf("prepare: %+v %q %v", pt, text, err)
	}

	if CheckHeaderMedia("image", "image/webp") == nil || CheckHeaderMedia("image", "image/png") != nil || CheckHeaderMedia("document", "application/pdf") != nil {
		t.Error("CheckHeaderMedia")
	}

	// Sending uploads the file, then sends it as the template's header.
	f := &fakeGraph{}
	f.handler = func(w http.ResponseWriter, r *http.Request, body []byte) {
		if strings.HasSuffix(r.URL.Path, "/media") {
			w.Write([]byte(`{"id": "MEDIA9"}`))
			return
		}
		w.Write([]byte(`{"messages": [{"id": "wamid.t"}]}`))
	}
	w := newTestInbox(t, f)
	meta, _ := json.Marshal(map[string]any{MetaTemplateKey: pt})
	_, ids, err := w.SendTracked(models.OutboundMessage{
		UUID: "u", To: []string{"919876543210"}, Meta: meta,
		Attachments: attachment.Attachments{{Name: "inv.pdf", ContentType: "application/pdf", Content: []byte("%PDF")}},
	})
	if err != nil || len(ids) != 1 {
		t.Fatalf("send: %v %v", ids, err)
	}
	if len(f.calls) != 2 || !strings.Contains(f.calls[1], `"type":"header"`) || !strings.Contains(f.calls[1], `"id":"MEDIA9"`) || !strings.Contains(f.calls[1], `"filename":"inv.pdf"`) {
		t.Fatalf("calls: %v", f.calls)
	}

	// Without its file it isn't sent at all.
	if _, _, err := w.SendTracked(models.OutboundMessage{UUID: "u", To: []string{"919876543210"}, Meta: meta}); err == nil {
		t.Error("sent a media header template without its file")
	}
}

func TestBuildTemplateComponents(t *testing.T) {
	good := TemplateDraft{
		Name: "order_ready", Language: "en_US", Category: "UTILITY",
		HeaderFormat: "TEXT", HeaderText: "Order {{1}}", HeaderExample: "#42",
		Body: "Hi {{1}}, your order {{2}} is ready.", BodyExamples: []string{"Asha", "#42"},
		Footer: "Acme",
		Buttons: []DraftButton{
			{Type: "URL", Text: "Track", URL: "https://acme.example/t/{{1}}", URLExample: "https://acme.example/t/42"},
			{Type: "PHONE_NUMBER", Text: "Call us", Phone: "+919876543210"},
		},
	}
	cs, err := BuildTemplateComponents(good)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(cs)
	for _, frag := range []string{`"header_text":["#42"]`, `"body_text":[["Asha","#42"]]`, `"example":["https://acme.example/t/42"]`, `"phone_number":"+919876543210"`} {
		if !strings.Contains(string(b), frag) {
			t.Errorf("missing %s in %s", frag, b)
		}
	}

	bad := map[string]func(d *TemplateDraft){
		"uppercase name":       func(d *TemplateDraft) { d.Name = "Order" },
		"authentication":       func(d *TemplateDraft) { d.Category = "AUTHENTICATION" },
		"missing example":      func(d *TemplateDraft) { d.BodyExamples = []string{"Asha"} },
		"out of order":         func(d *TemplateDraft) { d.Body = "Hi {{2}}, order {{1}} is ready."; d.BodyExamples = []string{"a", "b"} },
		"ends with variable":   func(d *TemplateDraft) { d.Body = "Your code is {{1}}"; d.BodyExamples = []string{"1"} },
		"named variable":       func(d *TemplateDraft) { d.Body = "Hi {{name}}, ready."; d.BodyExamples = nil },
		"footer variable":      func(d *TemplateDraft) { d.Footer = "Hi {{1}}" },
		"media without sample": func(d *TemplateDraft) { d.HeaderFormat = "IMAGE" },
		"url mid variable":     func(d *TemplateDraft) { d.Buttons[0].URL = "https://a.example/{{1}}/x" },
		"phone no plus":        func(d *TemplateDraft) { d.Buttons[1].Phone = "919876543210" },
		"mixed buttons": func(d *TemplateDraft) {
			d.Buttons = []DraftButton{{Type: "QUICK_REPLY", Text: "a"}, {Type: "URL", Text: "b", URL: "https://x.example"}, {Type: "QUICK_REPLY", Text: "c"}}
		},
	}
	for name, mutate := range bad {
		d := good
		d.Buttons = append([]DraftButton(nil), good.Buttons...)
		mutate(&d)
		if _, err := BuildTemplateComponents(d); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	d := good
	d.HeaderFormat, d.HeaderHandle = "IMAGE", "4::aW1n"
	if cs, err := BuildTemplateComponents(d); err != nil || !strings.Contains(fmt.Sprint(cs[0]), "4::aW1n") {
		t.Errorf("image header: %v %v", cs, err)
	}
}
