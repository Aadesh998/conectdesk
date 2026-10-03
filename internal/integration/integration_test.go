package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/integration/slack"
	"github.com/conectdesk/conectdesk/internal/integration/teams"
	"github.com/conectdesk/conectdesk/internal/testutil"
	wmodels "github.com/conectdesk/conectdesk/internal/webhook/models"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

const testKey = "0123456789abcdef0123456789abcdef"

// fakeChat stands in for Slack's Web API and the Bot Framework, recording what
// the bot posts.
type fakeChat struct {
	mu    sync.Mutex
	posts []map[string]any
	paths []string
	n     int
}

func (f *fakeChat) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	if strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token") {
		w.Write([]byte(`{"access_token":"bot","expires_in":3600}`))
		return
	}
	var post map[string]any
	json.Unmarshal(body, &post)
	f.posts = append(f.posts, post)
	f.paths = append(f.paths, r.URL.EscapedPath())
	f.n++
	switch {
	case strings.HasSuffix(r.URL.Path, "/chat.postMessage"):
		if r.Header.Get("Authorization") != "Bearer xoxb-secret" {
			w.Write([]byte(`{"ok":false,"error":"invalid_auth"}`))
			return
		}
		w.Write([]byte(`{"ok":true,"ts":"100.` + string(rune('0'+f.n)) + `"}`))
	case strings.HasSuffix(r.URL.Path, "/v3/conversations"):
		w.Write([]byte(`{"id":"19:chan@thread.tacv2;messageid=555"}`))
	default:
		w.Write([]byte(`{"id":"1"}`))
	}
}

func (f *fakeChat) snapshot() ([]map[string]any, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.posts...), append([]string(nil), f.paths...)
}

func newTestManager(t *testing.T, db *sqlx.DB, srv *httptest.Server) *Manager {
	t.Helper()
	lo := logf.New(logf.Opts{})
	tc := teams.New(teams.App{AppID: "a", AppPassword: "p"}, srv.Client(), srv.URL)
	tc.AllowServiceURLs(func(u string) bool { return strings.HasPrefix(u, srv.URL) })
	m, err := New(Opts{
		DB: db, Lo: &lo, I18n: testutil.NewI18n(t), EncryptionKey: testKey, Workers: 2, QueueSize: 10,
		RootURL: func(int) string { return "https://desk.test" },
		Slack:   slack.New(slack.App{ClientID: "c", ClientSecret: "s", SigningSecret: "x"}, srv.Client(), srv.URL+"/api"),
		Teams:   tc,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func newConversation(t *testing.T, db *sqlx.DB) int {
	t.Helper()
	var contact, inbox, status, id int
	db.MustExec(`DELETE FROM conversations`)
	if err := db.Get(&contact, `INSERT INTO users (tenant_id, type, email, first_name, last_name) VALUES (1, 'contact', 'jane@x.test', 'Jane', 'Doe') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&inbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Mail', 'email', '{}') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&status, `SELECT id FROM conversation_statuses WHERE tenant_id = 1 AND name = 'Open'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&id, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
		VALUES (1, $1, $2, $3, '42') RETURNING id`, contact, inbox, status); err != nil {
		t.Fatal(err)
	}
	return id
}

// waitPosts waits for the workers to post n messages.
func waitPosts(t *testing.T, f *fakeChat, n int) ([]map[string]any, []string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if posts, paths := f.snapshot(); len(posts) >= n {
			return posts, paths
		}
		time.Sleep(20 * time.Millisecond)
	}
	posts, _ := f.snapshot()
	t.Fatalf("got %d posts, want %d: %v", len(posts), n, posts)
	return nil, nil
}

// TestSlackThreads checks a conversation's first event starts a thread with
// the action buttons, later ones reply in it, notes written in the thread
// aren't posted back, and channels not subscribed to an event get nothing.
func TestSlackThreads(t *testing.T) {
	db := testutil.NewDB(t, "integration_slack")
	f := &fakeChat{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	m := newTestManager(t, db, srv)
	convID := newConversation(t, db)

	in, err := m.Connect(1, Connect{Provider: ProviderSlack, TeamID: "T1", TeamName: "Acme", ChannelID: "C1", ChannelName: "#support",
		AccessToken: "xoxb-secret", BotUserID: "UBOT"})
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	db.Get(&stored, `SELECT access_token FROM chat_integrations WHERE id = $1`, in.ID)
	if stored == "xoxb-secret" || stored == "" {
		t.Errorf("slack token stored as %q", stored)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Run(ctx)

	conv := map[string]any{"id": convID, "uuid": "c-1", "reference_number": "42", "inbox_name": "Mail",
		"last_message": "Where is my order?", "contact": map[string]any{"first_name": "Jane"}}
	m.HandleEvent(1, wmodels.EventConversationCreated, conv)
	posts, _ := waitPosts(t, f, 1)
	b, _ := json.Marshal(posts[0])
	if posts[0]["thread_ts"] != nil || !strings.Contains(string(b), slack.ActionAssign) || !strings.Contains(string(b), "New conversation from Jane") {
		t.Fatalf("first post isn't a thread root with buttons: %s", b)
	}
	ref, _ := m.Thread(in, convID)
	if got, _ := m.ThreadConversation(in, ref); ref == "" || got != convID {
		t.Fatalf("thread %q maps to %d", ref, got)
	}

	msg := map[string]any{"uuid": "m-1", "type": "incoming", "conversation_id": convID, "conversation_uuid": "c-1",
		"text_content": "Any news?", "author": map[string]any{"first_name": "Jane"}}
	m.HandleEvent(1, wmodels.EventMessageCreated, msg)
	posts, _ = waitPosts(t, f, 2)
	if posts[1]["thread_ts"] != ref {
		t.Errorf("reply went to thread %v, want %s", posts[1]["thread_ts"], ref)
	}

	// A note written in the thread isn't posted back; the next one is.
	m.ExpectNote(in.ID, "c-1", 9)
	note := map[string]any{"uuid": "m-2", "type": "outgoing", "private": true, "sender_id": 9, "conversation_id": convID, "conversation_uuid": "c-1"}
	m.HandleEvent(1, wmodels.EventMessageCreated, note)
	m.HandleEvent(1, wmodels.EventMessageCreated, note)
	posts, _ = waitPosts(t, f, 3)
	time.Sleep(100 * time.Millisecond)
	if posts, _ = f.snapshot(); len(posts) != 3 {
		t.Errorf("got %d posts, want the thread's note posted once", len(posts))
	}

	// Tags aren't in the default events.
	m.HandleEvent(1, wmodels.EventConversationTagsChanged, map[string]any{"conversation": conv, "new_tags": []string{"x"}})
	time.Sleep(150 * time.Millisecond)
	if posts, _ = f.snapshot(); len(posts) != 3 {
		t.Errorf("a channel not subscribed to tag changes got one")
	}

	// A revoked token turns the workspace's channels off.
	db.MustExec(`UPDATE chat_integrations SET access_token = '' WHERE id = $1`, in.ID)
	m.HandleEvent(1, wmodels.EventConversationAssigned, map[string]any{"conversation": conv, "assignee_name": "Sam"})
	time.Sleep(200 * time.Millisecond)
	if got, _ := m.Get(1, in.ID); got.IsActive {
		t.Error("a channel whose token Slack rejected is still on")
	}
}

// TestTeamsThreads checks Teams gets a thread created on the channel, then replies in it.
func TestTeamsThreads(t *testing.T) {
	db := testutil.NewDB(t, "integration_teams")
	f := &fakeChat{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	m := newTestManager(t, db, srv)
	convID := newConversation(t, db)

	in, err := m.Connect(1, Connect{Provider: ProviderTeams, TeamID: "ms-tenant", ChannelID: "19:chan@thread.tacv2", ChannelName: "Support",
		ServiceURL: srv.URL + "/amer/"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Run(ctx)

	conv := map[string]any{"id": convID, "uuid": "c-1", "reference_number": "42"}
	m.HandleEvent(1, wmodels.EventConversationCreated, conv)
	m.HandleEvent(1, wmodels.EventConversationStatusChanged, map[string]any{"conversation": conv, "previous_status": "Open", "new_status": "Resolved"})
	posts, paths := waitPosts(t, f, 2)
	if paths[0] != "/amer/v3/conversations" {
		t.Errorf("thread created at %s", paths[0])
	}
	if !strings.Contains(paths[1], "/amer/v3/conversations/19%3Achan%40thread.tacv2%3Bmessageid%3D555/activities") {
		t.Errorf("update posted at %s", paths[1])
	}
	b, _ := json.Marshal(posts[0])
	if !strings.Contains(string(b), `"cd_action":"assign"`) {
		t.Errorf("thread root has no actions: %s", b)
	}
	if ref, _ := m.Thread(in, convID); ref != "19:chan@thread.tacv2;messageid=555" {
		t.Errorf("thread = %q", ref)
	}
}

// TestConnectOnce checks a channel stays with the workspace that connected it,
// and connecting it again refreshes it.
func TestConnectOnce(t *testing.T) {
	db := testutil.NewDB(t, "integration_connect")
	f := &fakeChat{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	m := newTestManager(t, db, srv)
	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'other', 'Other') ON CONFLICT DO NOTHING`)

	c := Connect{Provider: ProviderSlack, TeamID: "T1", ChannelID: "C1", ChannelName: "#a", AccessToken: "xoxb-1", BotUserID: "U"}
	first, err := m.Connect(1, c)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(1, first.ID, []string{"conversation.created"}, false)
	c.AccessToken = "xoxb-2"
	again, err := m.Connect(1, c)
	if err != nil || again.ID != first.ID || !again.IsActive || again.AccessToken != "xoxb-2" || len(again.Events) != 1 {
		t.Errorf("reconnect = %+v, %v", again, err)
	}
	if _, err := m.Connect(2, c); err != ErrChannelTaken {
		t.Errorf("another workspace took the channel: %v", err)
	}
	if _, err := m.Update(1, first.ID, []string{"not.an.event"}, true); err == nil {
		t.Error("saved an unknown event")
	}
}
