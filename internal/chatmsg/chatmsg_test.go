package chatmsg

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/conectdesk/conectdesk/internal/webhook/models"
	"github.com/knadh/go-i18n"
)

// english loads the English language pack, which holds every chat string.
func english(t *testing.T) *i18n.I18n {
	t.Helper()
	b, err := os.ReadFile("../../i18n/en-US.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := i18n.New(b)
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func testConversation() map[string]any {
	return map[string]any{
		"id":               7,
		"uuid":             "c-1",
		"reference_number": "42",
		"subject":          "Refund {please}",
		"status":           "Open",
		"priority":         "High",
		"inbox_name":       "Support",
		"last_message":     "My order never arrived",
		"contact":          map[string]any{"first_name": "Jane", "last_name": "Doe"},
	}
}

func TestBuild(t *testing.T) {
	en := english(t)
	link := "https://desk.test/inboxes/all/conversation/c-1"
	cases := []struct {
		event   models.WebhookEvent
		payload any
		title   string
		link    string
	}{
		{models.EventConversationCreated, testConversation(), "New conversation from Jane Doe", link},
		{models.EventConversationStatusChanged, map[string]any{"previous_status": "Open", "new_status": "Resolved", "conversation": testConversation()},
			"Conversation #42 moved from Open to Resolved", link},
		{models.EventConversationAssigned, map[string]any{"assignee_name": "Sam Agent", "conversation": testConversation()},
			"Conversation #42 assigned to Sam Agent", link},
		{models.EventConversationTagsChanged, map[string]any{"new_tags": []string{"billing", "vip"}, "conversation": testConversation()},
			"Conversation #42 tagged billing, vip", link},
		{models.EventConversationUnassigned, map[string]any{"conversation": testConversation()}, "Conversation #42 is unassigned", link},
		{models.EventMessageCreated, map[string]any{"type": "incoming", "conversation_id": 7, "conversation_uuid": "c-1", "text_content": "hello", "author": map[string]any{"first_name": "Jane"}},
			"New message from Jane", link},
		{models.EventMessageCreated, map[string]any{"type": "outgoing", "private": true, "conversation_id": 7, "conversation_uuid": "c-1", "author": map[string]any{"first_name": "Sam"}},
			"Internal note from Sam", link},
		{models.EventMessageCreated, map[string]any{"type": "outgoing", "conversation_id": 7, "conversation_uuid": "c-1"},
			"Reply sent by an agent", link},
		{"sla.escalated", map[string]any{"conversation": testConversation()}, "Conversation #42: sla.escalated", link},
		{models.EventWebhookTest, map[string]any{"id": 1}, "ConectDesk is connected", ""},
	}
	for _, c := range cases {
		raw, _ := json.Marshal(c.payload)
		post, err := Build(en, c.event, raw, "https://desk.test/")
		if err != nil {
			t.Fatalf("%s: %v", c.event, err)
		}
		if post.Title != c.title {
			t.Errorf("%s: title = %q, want %q", c.event, post.Title, c.title)
		}
		if post.Link != c.link {
			t.Errorf("%s: link = %q, want %q", c.event, post.Link, c.link)
		}
		if c.link != "" && (post.ConversationID != 7 || post.ConversationUUID != "c-1") {
			t.Errorf("%s: conversation = %d %q", c.event, post.ConversationID, post.ConversationUUID)
		}
	}
}

// TestBuildKeepsBraces checks a customer's text isn't read as placeholders.
func TestBuildKeepsBraces(t *testing.T) {
	raw, _ := json.Marshal(testConversation())
	post, err := Build(english(t), models.EventConversationCreated, raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if post.Facts[0].Value != "Refund {please}" {
		t.Errorf("subject = %q", post.Facts[0].Value)
	}
}

// TestEveryKeyIsTranslated checks each chat string Build uses is in the pack,
// since a missing one would show the key itself in the channel.
func TestEveryKeyIsTranslated(t *testing.T) {
	en := english(t)
	var missing []string
	rec := recorder{en: en, missing: &missing}
	for _, ev := range []models.WebhookEvent{models.EventConversationCreated, models.EventConversationStatusChanged,
		models.EventConversationTagsChanged, models.EventConversationAssigned, models.EventConversationUnassigned,
		models.EventMessageCreated, models.EventWebhookTest, "custom.event"} {
		raw, _ := json.Marshal(map[string]any{"previous_status": "a", "conversation": testConversation()})
		if _, err := Build(rec, ev, raw, "https://desk.test"); err != nil {
			t.Fatal(err)
		}
	}
	if len(missing) > 0 {
		t.Errorf("keys missing from en-US.json: %v", missing)
	}
}

type recorder struct {
	en      *i18n.I18n
	missing *[]string
}

func (r recorder) T(key string) string {
	s := r.en.T(key)
	if s == key {
		*r.missing = append(*r.missing, key)
	}
	return s
}

func TestSkips(t *testing.T) {
	activity, _ := json.Marshal(map[string]any{"type": "activity"})
	incoming, _ := json.Marshal(map[string]any{"type": "incoming"})
	if !Skips(models.EventMessageCreated, activity) {
		t.Error("activity message is posted to chat")
	}
	if Skips(models.EventMessageCreated, incoming) {
		t.Error("incoming message is not posted to chat")
	}
	if !Skips(models.EventMessageUpdated, incoming) {
		t.Error("message update is posted to chat")
	}
}

func TestSlackBlocks(t *testing.T) {
	post := Post{Title: "A <b> & c", Body: "line one\nline two", Facts: []Fact{{"Inbox", "Support"}},
		Link: "https://desk.test/c", LinkText: "Open conversation"}
	blocks := SlackBlocks(post, SlackButton{Text: "Resolve", ActionID: "cd_resolve", Value: "7", Style: "primary"})
	text := blocks[0]["text"].(map[string]any)["text"].(string)
	if want := "*A &lt;b&gt; &amp; c*\n>line one\n>line two"; text != want {
		t.Errorf("section text = %q, want %q", text, want)
	}
	b, _ := json.Marshal(blocks)
	for _, want := range []string{`"action_id":"cd_resolve"`, `"style":"primary"`, `"url":"https://desk.test/c"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("blocks miss %s: %s", want, b)
		}
	}
	if len(blocks) != 3 {
		t.Errorf("got %d blocks, want section, context and actions", len(blocks))
	}
}

func TestAdaptiveCard(t *testing.T) {
	post := Post{Title: "T", Link: "https://desk.test/c", LinkText: "Open conversation", Facts: []Fact{{"Inbox", "Support"}}}
	card := AdaptiveCard(post, []map[string]any{{"type": "Input.Text", "id": "cd_text"}},
		[]map[string]any{{"type": "Action.Submit", "title": "Resolve"}})
	actions := card["actions"].([]map[string]any)
	if len(actions) != 2 || actions[1]["type"] != "Action.OpenUrl" {
		t.Errorf("actions = %v", actions)
	}
	if body := card["body"].([]map[string]any); len(body) != 3 || body[2]["type"] != "Input.Text" {
		t.Errorf("body = %v", body)
	}
}

// TestBuildTranslated checks a workspace in another language gets its posts
// in that language, as its pack is laid over the English one.
func TestBuildTranslated(t *testing.T) {
	pack := english(t)
	b, err := os.ReadFile("../../i18n/de-DE.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := pack.Load(b); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"previous_status": "Open", "new_status": "Resolved", "conversation": testConversation()})
	post, err := Build(pack, models.EventConversationStatusChanged, raw, "https://desk.test")
	if err != nil {
		t.Fatal(err)
	}
	if post.Title != "Gespräch #42 von Open zu Resolved verschoben" || post.LinkText != "Gespräch öffnen" {
		t.Errorf("german post = %q / %q", post.Title, post.LinkText)
	}
}
