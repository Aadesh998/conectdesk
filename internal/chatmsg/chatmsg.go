// Package chatmsg turns conversation events into chat messages for Slack and
// Microsoft Teams, shared by webhooks that post to those channels and by the
// Slack and Teams apps.
package chatmsg

import (
	"encoding/json"
	"strings"

	"github.com/conectdesk/conectdesk/internal/webhook/models"
)

// maxSnippetRunes caps how much of a message is quoted in a chat post.
const maxSnippetRunes = 300

// Translator looks up the workspace's wording for a key; *i18n.I18n is one.
type Translator interface {
	T(key string) string
}

// Fact is a labelled detail shown under a post, such as the inbox.
type Fact struct {
	Name  string
	Value string
}

// Post is a chat message built from an event, before it is shaped for Slack or Teams.
type Post struct {
	Event models.WebhookEvent
	Title string
	Body  string
	Facts []Fact
	// Link opens the conversation in the desk, and LinkText labels it.
	Link     string
	LinkText string
	// ConversationID and ConversationUUID are zero for events not about a
	// conversation, such as a test.
	ConversationID   int
	ConversationUUID string
}

// conversation holds the conversation fields a post shows.
type conversation struct {
	ID              int      `json:"id"`
	UUID            string   `json:"uuid"`
	ReferenceNumber string   `json:"reference_number"`
	Subject         string   `json:"subject"`
	Status          string   `json:"status"`
	Priority        string   `json:"priority"`
	InboxName       string   `json:"inbox_name"`
	LastMessage     string   `json:"last_message"`
	Tags            []string `json:"tags"`
	Contact         struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Email     string `json:"email"`
	} `json:"contact"`
}

// message holds the message fields a post shows.
type message struct {
	UUID             string `json:"uuid"`
	Type             string `json:"type"`
	ConversationID   int    `json:"conversation_id"`
	ConversationUUID string `json:"conversation_uuid"`
	TextContent      string `json:"text_content"`
	Private          bool   `json:"private"`
	SenderID         int    `json:"sender_id"`
	Author           struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	} `json:"author"`
}

// event is a conversation event payload reduced to what a post needs.
type event struct {
	Conversation   *conversation `json:"conversation"`
	PreviousStatus string        `json:"previous_status"`
	NewStatus      string        `json:"new_status"`
	NewTags        []string      `json:"new_tags"`
	AssigneeName   string        `json:"assignee_name"`
}

// MessageInfo is what a message.created payload says about who wrote what, so
// the Slack and Teams apps can leave out notes that came from their own channel.
type MessageInfo struct {
	UUID     string
	SenderID int
	Private  bool
}

// ParseMessage reads a message.created payload.
func ParseMessage(payload []byte) (MessageInfo, error) {
	var msg message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return MessageInfo{}, err
	}
	return MessageInfo{UUID: msg.UUID, SenderID: msg.SenderID, Private: msg.Private}, nil
}

// Skips reports whether an event is left out of chat posts. Activity messages
// ("X assigned this to Y") repeat the conversation events, and message updates
// are delivery receipts, so both would only be noise in a channel.
func Skips(ev models.WebhookEvent, payload []byte) bool {
	switch ev {
	case models.EventMessageUpdated:
		return true
	case models.EventMessageCreated:
		var msg message
		if err := json.Unmarshal(payload, &msg); err != nil {
			return true
		}
		return msg.Type == "activity"
	}
	return false
}

// ConversationLink is the desk address of a conversation, or "" without one.
func ConversationLink(rootURL, uuid string) string {
	if uuid == "" || rootURL == "" {
		return ""
	}
	return strings.TrimRight(rootURL, "/") + "/inboxes/all/conversation/" + uuid
}

// Build turns an event payload, as JSON, into a post in the workspace's
// language. rootURL is the workspace's address, which links back to the conversation.
func Build(t Translator, ev models.WebhookEvent, payload []byte, rootURL string) (Post, error) {
	post := Post{Event: ev, LinkText: t.T("chat.openConversation")}

	switch ev {
	case models.EventWebhookTest:
		post.Title = t.T("chat.connected.title")
		post.Body = t.T("chat.connected.body")
		return post, nil

	case models.EventMessageCreated:
		var msg message
		if err := json.Unmarshal(payload, &msg); err != nil {
			return post, err
		}
		author := FullName(msg.Author.FirstName, msg.Author.LastName)
		switch {
		case msg.Private:
			post.Title = Format(t.T("chat.internalNote"), "name", orDefault(author, t.T("chat.anAgent")))
		case msg.Type == "outgoing":
			post.Title = Format(t.T("chat.replySent"), "name", orDefault(author, t.T("chat.anAgent")))
		default:
			post.Title = Format(t.T("chat.newMessage"), "name", orDefault(author, t.T("chat.aContact")))
		}
		post.Body = Snippet(msg.TextContent)
		post.ConversationID = msg.ConversationID
		post.ConversationUUID = msg.ConversationUUID
		post.Link = ConversationLink(rootURL, msg.ConversationUUID)
		return post, nil
	}

	var e event
	if err := json.Unmarshal(payload, &e); err != nil {
		return post, err
	}
	// conversation.created carries the conversation itself, not under a key.
	conv := e.Conversation
	if conv == nil {
		conv = &conversation{}
		if err := json.Unmarshal(payload, conv); err != nil {
			return post, err
		}
	}

	ref := t.T("chat.conversation")
	if conv.ReferenceNumber != "" {
		ref = Format(t.T("chat.conversationRef"), "ref", conv.ReferenceNumber)
	}
	contact := FullName(conv.Contact.FirstName, conv.Contact.LastName)
	if contact == "" {
		contact = conv.Contact.Email
	}

	switch ev {
	case models.EventConversationCreated:
		post.Title = Format(t.T("chat.newConversation"), "name", orDefault(contact, t.T("chat.aContact")))
		post.Body = Snippet(conv.LastMessage)
	case models.EventConversationStatusChanged:
		if e.PreviousStatus != "" {
			post.Title = Format(t.T("chat.statusMoved"), "ref", ref, "from", e.PreviousStatus, "to", e.NewStatus)
		} else {
			post.Title = Format(t.T("chat.statusNow"), "ref", ref, "status", e.NewStatus)
		}
	case models.EventConversationTagsChanged:
		if len(e.NewTags) == 0 {
			post.Title = Format(t.T("chat.tagsRemoved"), "ref", ref)
		} else {
			post.Title = Format(t.T("chat.tagged"), "ref", ref, "tags", strings.Join(e.NewTags, ", "))
		}
	case models.EventConversationAssigned:
		post.Title = Format(t.T("chat.assigned"), "ref", ref, "name", orDefault(e.AssigneeName, t.T("chat.anAgent")))
	case models.EventConversationUnassigned:
		post.Title = Format(t.T("chat.unassigned"), "ref", ref)
	default:
		// Automation rules post with an event name of their own choosing.
		post.Title = Format(t.T("chat.customEvent"), "ref", ref, "event", string(ev))
		post.Body = Snippet(conv.LastMessage)
	}

	if conv.Subject != "" {
		post.Facts = append(post.Facts, Fact{t.T("chat.fact.subject"), conv.Subject})
	}
	if contact != "" && ev != models.EventConversationCreated {
		post.Facts = append(post.Facts, Fact{t.T("chat.fact.contact"), contact})
	}
	if conv.InboxName != "" {
		post.Facts = append(post.Facts, Fact{t.T("chat.fact.inbox"), conv.InboxName})
	}
	if conv.Status != "" && ev != models.EventConversationStatusChanged {
		post.Facts = append(post.Facts, Fact{t.T("chat.fact.status"), conv.Status})
	}
	if conv.Priority != "" {
		post.Facts = append(post.Facts, Fact{t.T("chat.fact.priority"), conv.Priority})
	}
	post.ConversationID = conv.ID
	post.ConversationUUID = conv.UUID
	post.Link = ConversationLink(rootURL, conv.UUID)
	return post, nil
}

// SlackButton is an interactive button the Slack app adds under a post.
type SlackButton struct {
	Text     string
	ActionID string
	Value    string
	// Style is "", "primary" or "danger".
	Style string
}

// SlackBlocks lays a post out as Slack Block Kit blocks: the title and quote,
// the facts, then the buttons and a link to the conversation.
func SlackBlocks(post Post, buttons ...SlackButton) []map[string]any {
	text := "*" + SlackEscape(post.Title) + "*"
	if post.Body != "" {
		text += "\n>" + strings.ReplaceAll(SlackEscape(post.Body), "\n", "\n>")
	}
	blocks := []map[string]any{{
		"type": "section",
		"text": map[string]any{"type": "mrkdwn", "text": text},
	}}
	if len(post.Facts) > 0 {
		var facts []string
		for _, f := range post.Facts {
			facts = append(facts, "*"+SlackEscape(f.Name)+":* "+SlackEscape(f.Value))
		}
		blocks = append(blocks, map[string]any{
			"type":     "context",
			"elements": []map[string]any{{"type": "mrkdwn", "text": strings.Join(facts, "  |  ")}},
		})
	}
	var elements []map[string]any
	for _, b := range buttons {
		el := map[string]any{
			"type":      "button",
			"text":      map[string]any{"type": "plain_text", "text": b.Text},
			"action_id": b.ActionID,
			"value":     b.Value,
		}
		if b.Style != "" {
			el["style"] = b.Style
		}
		elements = append(elements, el)
	}
	if post.Link != "" {
		elements = append(elements, map[string]any{
			"type":      "button",
			"text":      map[string]any{"type": "plain_text", "text": post.LinkText},
			"url":       post.Link,
			"action_id": "cd_open",
		})
	}
	if len(elements) > 0 {
		blocks = append(blocks, map[string]any{"type": "actions", "elements": elements})
	}
	return blocks
}

// AdaptiveCard lays a post out as an Adaptive Card for Teams. extraBody goes
// under the facts, and actions before the link to the conversation.
func AdaptiveCard(post Post, extraBody []map[string]any, actions []map[string]any) map[string]any {
	body := []map[string]any{{
		"type": "TextBlock", "text": post.Title, "weight": "Bolder", "size": "Medium", "wrap": true,
	}}
	if post.Body != "" {
		body = append(body, map[string]any{"type": "TextBlock", "text": post.Body, "wrap": true, "isSubtle": true})
	}
	if len(post.Facts) > 0 {
		var facts []map[string]string
		for _, f := range post.Facts {
			facts = append(facts, map[string]string{"title": f.Name, "value": f.Value})
		}
		body = append(body, map[string]any{"type": "FactSet", "facts": facts})
	}
	body = append(body, extraBody...)
	card := map[string]any{
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body":    body,
	}
	if post.Link != "" {
		actions = append(actions, map[string]any{"type": "Action.OpenUrl", "title": post.LinkText, "url": post.Link})
	}
	if len(actions) > 0 {
		card["actions"] = actions
	}
	return card
}

// CardAttachment wraps an Adaptive Card as a Teams message attachment.
func CardAttachment(card map[string]any) map[string]any {
	return map[string]any{
		"contentType": "application/vnd.microsoft.card.adaptive",
		"contentUrl":  nil,
		"content":     card,
	}
}

// Format fills {name} placeholders in a translated string. Unlike i18n's Ts it
// leaves braces in the values alone, so a customer's text is shown as written.
func Format(s string, params ...string) string {
	for i := 0; i+1 < len(params); i += 2 {
		s = strings.ReplaceAll(s, "{"+params[i]+"}", params[i+1])
	}
	return s
}

// SlackEscape escapes the characters Slack's mrkdwn treats as control characters.
func SlackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// Snippet trims a message to the length quoted in a post.
func Snippet(s string) string {
	return Truncate(strings.TrimSpace(s), maxSnippetRunes)
}

// Truncate cuts s to at most n runes, marking the cut with an ellipsis.
func Truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n])) + "…"
	}
	return s
}

// FullName joins a first and last name.
func FullName(first, last string) string {
	return strings.TrimSpace(first + " " + last)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
