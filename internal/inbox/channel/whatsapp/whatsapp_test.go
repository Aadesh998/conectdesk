package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/conectdesk/conectdesk/internal/attachment"
	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/zerodha/logf"
)

// fakeGraph records the Graph API calls an inbox makes.
type fakeGraph struct {
	mu    sync.Mutex
	calls []string
	// handler answers a call; nil answers {}.
	handler func(w http.ResponseWriter, r *http.Request, body []byte)
}

func (f *fakeGraph) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path+" "+string(body))
		f.mu.Unlock()
		if f.handler != nil {
			f.handler(w, r, body)
			return
		}
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestInbox(t *testing.T, f *fakeGraph) *WhatsApp {
	t.Helper()
	graph := meta.NewClient(meta.App{AppID: "app", AppSecret: "secret"})
	graph.SetBaseURL(f.server(t).URL)
	lo := logf.New(logf.Opts{})
	w, err := New(nil, nil, Opts{
		ID:       7,
		TenantID: 3,
		Name:     "Support",
		Config:   Config{AccountID: "PNID", AccessToken: "tok", MarkSeen: true},
		Graph:    graph,
		Lo:       &lo,
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func decodeValue(t *testing.T, s string) Value {
	t.Helper()
	var v Value
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNewRequiresConnection(t *testing.T) {
	graph := meta.NewClient(meta.App{AppID: "app", AppSecret: "secret"})
	if _, err := New(nil, nil, Opts{Config: Config{AccountID: "1"}, Graph: graph}); err == nil {
		t.Error("inbox without a token was accepted")
	}
	if _, err := New(nil, nil, Opts{Config: Config{AccountID: "1", AccessToken: "t"}, Graph: meta.NewClient(meta.App{})}); err == nil {
		t.Error("inbox without a configured meta app was accepted")
	}
}

func TestParseTextMessage(t *testing.T) {
	w := newTestInbox(t, &fakeGraph{})
	v := decodeValue(t, `{
		"messaging_product": "whatsapp",
		"metadata": {"display_phone_number": "15550001111", "phone_number_id": "PNID"},
		"contacts": [{"profile": {"name": "Asha Rao"}, "wa_id": "919876543210"}],
		"messages": [{"from": "919876543210", "id": "wamid.1", "timestamp": "1", "type": "text", "text": {"body": "Hi there"}}]
	}`)

	msgs := w.ParseValue(context.Background(), v)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages", len(msgs))
	}
	m := msgs[0]
	if m.TenantID != 3 || m.InboxID != 7 || m.Channel != ChannelWhatsApp {
		t.Errorf("routing fields: %+v", m)
	}
	if m.SourceID.String != "wamid.1" || m.Content != "Hi there" {
		t.Errorf("content: %q %q", m.SourceID.String, m.Content)
	}
	if m.Contact.FirstName != "Asha" || m.Contact.LastName != "Rao" {
		t.Errorf("name: %q %q", m.Contact.FirstName, m.Contact.LastName)
	}
	if m.Contact.ExternalUserID.String != "whatsapp:919876543210" || m.Contact.PhoneNumber.String != "919876543210" {
		t.Errorf("contact ids: %q %q", m.Contact.ExternalUserID.String, m.Contact.PhoneNumber.String)
	}
}

func TestParseSkipsReactionsAndBadSenders(t *testing.T) {
	w := newTestInbox(t, &fakeGraph{})
	v := decodeValue(t, `{
		"metadata": {"phone_number_id": "PNID"},
		"messages": [
			{"from": "919876543210", "id": "wamid.r", "type": "reaction", "reaction": {"emoji": "👍"}},
			{"from": "not-a-number", "id": "wamid.x", "type": "text", "text": {"body": "x"}},
			{"from": "919876543210", "id": "", "type": "text", "text": {"body": "x"}},
			{"from": "919876543210", "id": "wamid.b", "type": "interactive", "interactive": {"type": "button_reply", "button_reply": {"title": "Yes"}}},
			{"from": "919876543210", "id": "wamid.p", "type": "poll"}
		],
		"statuses": [{"id": "wamid.out", "status": "failed", "errors": [{"code": 131047, "title": "Re-engagement message"}]}]
	}`)

	msgs := w.ParseValue(context.Background(), v)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want the button reply and the placeholder", len(msgs))
	}
	if msgs[0].Content != "Yes" {
		t.Errorf("button reply: %q", msgs[0].Content)
	}
	if msgs[1].Content != "[unsupported message]" {
		t.Errorf("unknown type: %q", msgs[1].Content)
	}
}

// A media URL off Meta's CDNs is refused, and the message is still stored
// with a placeholder so the agent sees something arrived.
func TestMediaOffMetaHostIsRefused(t *testing.T) {
	f := &fakeGraph{}
	f.handler = func(w http.ResponseWriter, r *http.Request, body []byte) {
		w.Write([]byte(`{"url": "http://169.254.169.254/latest/meta-data", "mime_type": "image/jpeg"}`))
	}
	w := newTestInbox(t, f)
	v := decodeValue(t, `{
		"metadata": {"phone_number_id": "PNID"},
		"messages": [{"from": "919876543210", "id": "wamid.img", "type": "image", "image": {"id": "MEDIA1", "mime_type": "image/jpeg"}}]
	}`)

	msgs := w.ParseValue(context.Background(), v)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages", len(msgs))
	}
	if len(msgs[0].Attachments) != 0 || msgs[0].Content != "[image]" {
		t.Errorf("got attachments %d, content %q", len(msgs[0].Attachments), msgs[0].Content)
	}
}

func TestSendText(t *testing.T) {
	f := &fakeGraph{}
	w := newTestInbox(t, f)

	err := w.Send(models.OutboundMessage{UUID: "u1", To: []string{"919876543210"}, Content: "<p>Hello <b>there</b></p>"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || !strings.HasPrefix(f.calls[0], "POST /"+meta.DefaultGraphVersion+"/PNID/messages") {
		t.Fatalf("calls: %v", f.calls)
	}
	if !strings.Contains(f.calls[0], `"to":"919876543210"`) || !strings.Contains(f.calls[0], `"type":"text"`) {
		t.Errorf("body: %s", f.calls[0])
	}
}

func TestSendAttachmentUploadsThenSends(t *testing.T) {
	f := &fakeGraph{}
	f.handler = func(w http.ResponseWriter, r *http.Request, body []byte) {
		if strings.HasSuffix(r.URL.Path, "/media") {
			w.Write([]byte(`{"id": "UPLOADED"}`))
			return
		}
		w.Write([]byte(`{}`))
	}
	w := newTestInbox(t, f)

	err := w.Send(models.OutboundMessage{
		UUID:    "u2",
		To:      []string{"919876543210"},
		Content: "Your invoice",
		Attachments: attachment.Attachments{{
			Name: "invoice.pdf", ContentType: "application/pdf", Content: []byte("%PDF"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("want upload and send, got %v", f.calls)
	}
	send := f.calls[1]
	if !strings.Contains(send, `"type":"document"`) || !strings.Contains(send, `"id":"UPLOADED"`) ||
		!strings.Contains(send, `"caption":"Your invoice"`) || !strings.Contains(send, `"filename":"invoice.pdf"`) {
		t.Errorf("send body: %s", send)
	}
}

func TestSendRejectsBadRecipient(t *testing.T) {
	w := newTestInbox(t, &fakeGraph{})
	for _, to := range []string{"", "919876543210@c.us", "+91 98765", "abc"} {
		if err := w.Send(models.OutboundMessage{UUID: "u", To: []string{to}, Content: "x"}); err == nil {
			t.Errorf("sent to %q", to)
		}
	}
}

func TestWAID(t *testing.T) {
	if got := WAIDFromPhone("+91 (98765) 43-210"); got != "919876543210" {
		t.Errorf("WAIDFromPhone: %q", got)
	}
	if IsValidWAID("123") || IsValidWAID("1234567890123456") || !IsValidWAID("919876543210") {
		t.Error("IsValidWAID bounds")
	}
}

func TestSendTrackedReturnsIDs(t *testing.T) {
	f := &fakeGraph{}
	n := 0
	f.handler = func(w http.ResponseWriter, r *http.Request, body []byte) {
		n++
		w.Write([]byte(fmt.Sprintf(`{"messages": [{"id": "wamid.%d"}]}`, n)))
	}
	w := newTestInbox(t, f)
	to, ids, err := w.SendTracked(models.OutboundMessage{UUID: "u", To: []string{"919876543210"}, Content: strings.Repeat("word ", 1000)})
	if err != nil {
		t.Fatal(err)
	}
	// 5000 bytes goes as two messages, and each ID is kept.
	if to != "919876543210" || strings.Join(ids, ",") != "wamid.1,wamid.2" {
		t.Fatalf("got %q %v", to, ids)
	}
}

func TestReceipts(t *testing.T) {
	w := newTestInbox(t, &fakeGraph{})
	v := decodeValue(t, `{"statuses": [
		{"id": "wamid.a", "status": "delivered", "recipient_id": "919876543210"},
		{"id": "wamid.b", "status": "failed", "recipient_id": "919876543210",
		 "errors": [{"code": 131047, "title": "Re-engagement message", "message": "Re-engagement message",
		             "error_data": {"details": "Message failed to send because more than 24 hours have passed"}}]},
		{"id": "wamid.c", "status": "deleted"}
	]}`)
	rs := w.Receipts(v)
	if len(rs) != 2 {
		t.Fatalf("got %+v", rs)
	}
	if rs[0].Status != "delivered" || rs[0].ExternalIDs[0] != "wamid.a" {
		t.Errorf("delivered: %+v", rs[0])
	}
	if rs[1].Status != "failed" || !strings.Contains(rs[1].Error, "131047") || !strings.Contains(rs[1].Error, "24 hours") {
		t.Errorf("failed: %+v", rs[1])
	}
}
