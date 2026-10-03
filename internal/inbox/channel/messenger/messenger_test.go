package messenger

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/attachment"
	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/zerodha/logf"
)

type fakeGraph struct {
	mu      sync.Mutex
	calls   []string
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

func newTestInbox(t *testing.T, channel string, app meta.App, f *fakeGraph) *Messenger {
	t.Helper()
	graph := meta.NewClient(app)
	graph.SetBaseURL(f.server(t).URL)
	lo := logf.New(logf.Opts{})
	accountID := "PAGE1"
	if channel == ChannelInstagram {
		accountID = "IG1"
	}
	m, err := New(nil, nil, Opts{
		ID:       9,
		TenantID: 4,
		Channel:  channel,
		Config:   Config{AccountID: accountID, PageID: "PAGE1", AccessToken: "ptok"},
		Graph:    graph,
		Lo:       &lo,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

var app = meta.App{AppID: "app", AppSecret: "secret"}

func TestParseMessengerText(t *testing.T) {
	f := &fakeGraph{handler: func(w http.ResponseWriter, r *http.Request, body []byte) {
		// The profile lookup.
		w.Write([]byte(`{"first_name": "Ravi", "last_name": "Kumar"}`))
	}}
	m := newTestInbox(t, ChannelMessenger, app, f)

	in, err := m.ParseEvent(context.Background(), []byte(`{
		"sender": {"id": "5551"}, "recipient": {"id": "PAGE1"}, "timestamp": 1,
		"message": {"mid": "m_1", "text": "Is my order shipped?"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if in.TenantID != 4 || in.InboxID != 9 || in.Channel != ChannelMessenger {
		t.Errorf("routing: %+v", in)
	}
	if in.SourceID.String != "m_1" || in.Content != "Is my order shipped?" {
		t.Errorf("content: %+v", in)
	}
	if in.Contact.FirstName != "Ravi" || in.Contact.LastName != "Kumar" || in.Contact.ExternalUserID.String != "messenger:5551" {
		t.Errorf("contact: %+v", in.Contact)
	}

	// The name is cached: a second message makes no profile call.
	before := len(f.calls)
	if _, err := m.ParseEvent(context.Background(), []byte(`{"sender": {"id": "5551"}, "message": {"mid": "m_2", "text": "?"}}`)); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != before {
		t.Errorf("profile looked up again: %v", f.calls)
	}
}

func TestParseIgnored(t *testing.T) {
	m := newTestInbox(t, ChannelInstagram, app, &fakeGraph{})
	for name, raw := range map[string]string{
		"echo":     `{"sender": {"id": "IG1"}, "message": {"mid": "m", "text": "hi", "is_echo": true}}`,
		"deleted":  `{"sender": {"id": "77"}, "message": {"mid": "m", "is_deleted": true}}`,
		"read":     `{"sender": {"id": "77"}, "read": {"mid": "m"}}`,
		"own":      `{"sender": {"id": "IG1"}, "message": {"mid": "m", "text": "hi"}}`,
		"reaction": `{"sender": {"id": "77"}, "reaction": {"mid": "m", "action": "react"}}`,
	} {
		_, err := m.ParseEvent(context.Background(), []byte(raw))
		var ignored meta.ErrIgnored
		if !errors.As(err, &ignored) {
			t.Errorf("%s: got %v, want ignored", name, err)
		}
	}
}

func TestParseInstagramShareKeepsLink(t *testing.T) {
	m := newTestInbox(t, ChannelInstagram, app, &fakeGraph{handler: func(w http.ResponseWriter, r *http.Request, body []byte) {
		w.Write([]byte(`{"username": "asha.shop"}`))
	}})
	in, err := m.ParseEvent(context.Background(), []byte(`{
		"sender": {"id": "77"},
		"message": {"mid": "m_3", "attachments": [{"type": "share", "payload": {"url": "https://www.instagram.com/p/abc"}}]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(in.Content, "https://www.instagram.com/p/abc") {
		t.Errorf("content: %q", in.Content)
	}
	if in.Contact.FirstName != "@asha.shop" || in.Contact.ExternalUserID.String != "instagram:77" {
		t.Errorf("contact: %+v", in.Contact)
	}
}

func TestSendRetriesWithHumanAgentTag(t *testing.T) {
	windowErr := `{"error": {"message": "outside window", "code": 10, "error_subcode": 2018278}}`

	// Without human_agent the window error is returned as is.
	f := &fakeGraph{handler: func(w http.ResponseWriter, r *http.Request, body []byte) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(windowErr))
	}}
	m := newTestInbox(t, ChannelMessenger, app, f)
	err := m.Send(models.OutboundMessage{UUID: "u", To: []string{"5551"}, Content: "late reply"})
	if !meta.IsOutsideWindow(err) || len(f.calls) != 1 {
		t.Fatalf("got %v after %d calls", err, len(f.calls))
	}

	// With it, the reply is retried once with the tag.
	f = &fakeGraph{}
	f.handler = func(w http.ResponseWriter, r *http.Request, body []byte) {
		if strings.Contains(string(body), "HUMAN_AGENT") {
			w.Write([]byte(`{"message_id": "m"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(windowErr))
	}
	m = newTestInbox(t, ChannelMessenger, meta.App{AppID: "app", AppSecret: "secret", HumanAgent: true}, f)
	if err := m.Send(models.OutboundMessage{UUID: "u", To: []string{"5551"}, Content: "late reply"}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || !strings.Contains(f.calls[1], `"tag":"HUMAN_AGENT"`) || !strings.HasPrefix(f.calls[1], "POST /"+meta.DefaultGraphVersion+"/PAGE1/messages") {
		t.Fatalf("calls: %v", f.calls)
	}
}

func TestInstagramAttachmentsByURL(t *testing.T) {
	f := &fakeGraph{handler: func(w http.ResponseWriter, r *http.Request, body []byte) {
		w.Write([]byte(`{"message_id": "m_x"}`))
	}}
	m := newTestInbox(t, ChannelInstagram, app, f)

	// A file without a public link is refused before anything is sent, so the
	// text doesn't go out without it.
	err := m.Send(models.OutboundMessage{
		UUID: "u", To: []string{"77"}, Content: "see file",
		Attachments: attachment.Attachments{{Name: "a.png", ContentType: "image/png", Content: []byte("x"), PublicURL: "/uploads/a.png"}},
	})
	if !errors.Is(err, ErrInstagramAttachments) || len(f.calls) != 0 {
		t.Fatalf("got %v after %d calls", err, len(f.calls))
	}

	_, ids, err := m.SendTracked(models.OutboundMessage{
		UUID: "u", To: []string{"77"}, Content: "see file",
		Attachments: attachment.Attachments{{Name: "a.png", ContentType: "image/png", PublicURL: "https://acme.example/uploads/a.png?signature=s"}},
	})
	if err != nil || len(ids) != 2 {
		t.Fatalf("got %v %v", ids, err)
	}
	if !strings.Contains(f.calls[1], `"type":"image"`) || !strings.Contains(f.calls[1], `"url":"https://acme.example/uploads/a.png?signature=s"`) {
		t.Errorf("attachment call: %s", f.calls[1])
	}
}

func TestSendSplitsLongText(t *testing.T) {
	f := &fakeGraph{}
	m := newTestInbox(t, ChannelInstagram, app, f)
	if err := m.Send(models.OutboundMessage{UUID: "u", To: []string{"77"}, Content: strings.Repeat("word ", 300)}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 {
		t.Errorf("1500 bytes on Instagram should go as 2 messages, got %d", len(f.calls))
	}
}

func TestParseReceipt(t *testing.T) {
	m := newTestInbox(t, ChannelMessenger, app, &fakeGraph{})
	rc, ok := m.ParseReceipt([]byte(`{"sender": {"id": "5551"}, "delivery": {"mids": ["m_1", "m_2"], "watermark": 1700000000000}}`))
	if !ok || rc.Status != "delivered" || strings.Join(rc.ExternalIDs, ",") != "m_1,m_2" || rc.Recipient != "5551" {
		t.Errorf("delivery: %+v %v", rc, ok)
	}
	rc, ok = m.ParseReceipt([]byte(`{"sender": {"id": "5551"}, "read": {"watermark": 1700000000000}}`))
	if !ok || rc.Status != "read" || len(rc.ExternalIDs) != 0 || rc.Watermark.UnixMilli() != 1700000000000 {
		t.Errorf("messenger read: %+v %v", rc, ok)
	}
	rc, ok = m.ParseReceipt([]byte(`{"sender": {"id": "77"}, "read": {"mid": "ig_m"}}`))
	if !ok || rc.Status != "read" || rc.ExternalIDs[0] != "ig_m" {
		t.Errorf("instagram read: %+v %v", rc, ok)
	}
	if _, ok := m.ParseReceipt([]byte(`{"sender": {"id": "5551"}, "message": {"mid": "m", "text": "hi"}}`)); ok {
		t.Error("a message was taken for a receipt")
	}
}

func TestSendTrackedReturnsIDs(t *testing.T) {
	f := &fakeGraph{handler: func(w http.ResponseWriter, r *http.Request, body []byte) {
		w.Write([]byte(`{"recipient_id": "5551", "message_id": "m_out"}`))
	}}
	m := newTestInbox(t, ChannelMessenger, app, f)
	to, ids, err := m.SendTracked(models.OutboundMessage{UUID: "u", To: []string{"5551"}, Content: "hi"})
	if err != nil || to != "5551" || len(ids) != 1 || ids[0] != "m_out" {
		t.Fatalf("got %q %v %v", to, ids, err)
	}
}

func TestHistory(t *testing.T) {
	f := &fakeGraph{handler: func(w http.ResponseWriter, r *http.Request, body []byte) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/PAGE1/conversations"):
			if r.URL.Query().Get("platform") != "messenger" {
				t.Errorf("platform: %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"data": [{"id": "t_1"}, {"id": "t_2"}]}`))
		case strings.HasSuffix(r.URL.Path, "/t_1"):
			// Newest first, as the API returns them.
			w.Write([]byte(`{"messages": {"data": [
				{"id": "m_3", "created_time": "2026-01-02T10:00:00+0000", "message": "Thanks!", "from": {"id": "5551", "name": "Ravi Kumar"}},
				{"id": "m_2", "created_time": "2026-01-01T11:00:00+0000", "message": "It shipped today.", "from": {"id": "PAGE1", "name": "Acme"}},
				{"id": "m_1", "created_time": "2026-01-01T10:00:00+0000", "message": "Where is my order?", "from": {"id": "5551", "name": "Ravi Kumar"}}
			]}}`))
		case strings.HasSuffix(r.URL.Path, "/t_2"):
			// Only the Page wrote: no contact to file it under.
			w.Write([]byte(`{"messages": {"data": [{"id": "m_9", "created_time": "2026-01-01T10:00:00+0000", "message": "Hi", "from": {"id": "PAGE1"}}]}}`))
		default:
			w.Write([]byte(`{}`))
		}
	}}
	m := newTestInbox(t, ChannelMessenger, app, f)

	var chats []Chat
	seen, errs, err := m.History(context.Background(), ImportOpts{}, func(c Chat) error {
		chats = append(chats, c)
		return nil
	})
	if err != nil || len(errs) != 0 || seen != 2 {
		t.Fatalf("seen %d, errs %v, err %v", seen, errs, err)
	}
	if len(chats) != 1 {
		t.Fatalf("chats: %+v", chats)
	}
	c := chats[0]
	if c.Contact.FirstName != "Ravi" || c.Contact.ExternalUserID.String != "messenger:5551" {
		t.Errorf("contact: %+v", c.Contact)
	}
	if len(c.Messages) != 3 || c.Messages[0].SourceID.String != "m_1" || c.Messages[2].SourceID.String != "m_3" {
		t.Fatalf("order: %+v", c.Messages)
	}
	if c.Messages[0].Outgoing || !c.Messages[1].Outgoing {
		t.Errorf("direction: %v %v", c.Messages[0].Outgoing, c.Messages[1].Outgoing)
	}
	if !c.Messages[0].SentAt.Equal(time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("sent at: %v", c.Messages[0].SentAt)
	}
}

func TestSendTyping(t *testing.T) {
	f := &fakeGraph{}
	m := newTestInbox(t, ChannelInstagram, app, f)
	m.SendTyping("77")
	m.SendTyping("not-an-id")
	if len(f.calls) != 1 || !strings.Contains(f.calls[0], `"sender_action":"typing_on"`) {
		t.Fatalf("calls: %v", f.calls)
	}
}
