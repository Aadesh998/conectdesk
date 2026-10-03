package slack

import (
	"encoding/json"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/conectdesk/conectdesk/internal/chatmsg"
)

// Action IDs of the buttons and modal the app posts.
const (
	ActionAssign  = "cd_assign"
	ActionResolve = "cd_resolve"
	ActionReply   = "cd_reply"
	// CallbackReply is the reply modal, submitted as a view_submission.
	CallbackReply = "cd_reply_modal"

	blockMode = "cd_mode"
	blockText = "cd_text"
	// ModeReply sends the text to the customer; ModeNote adds an internal note.
	ModeReply = "reply"
	ModeNote  = "note"
)

// EventEnvelope is what the Events API posts: a URL check, or an event.
type EventEnvelope struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	TeamID    string `json:"team_id"`
	Event     Event  `json:"event"`
}

// Event is a message posted in a channel the bot is in.
type Event struct {
	Type     string `json:"type"`
	Subtype  string `json:"subtype"`
	Channel  string `json:"channel"`
	User     string `json:"user"`
	BotID    string `json:"bot_id"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
}

// IsThreadReply reports whether the event is a person's reply in a thread,
// the only messages the app acts on. Edits, joins and bots' posts are left alone.
func (e Event) IsThreadReply(botUserID string) bool {
	return e.Type == "message" && e.Subtype == "" && e.BotID == "" && e.User != "" &&
		e.User != botUserID && e.ThreadTS != "" && e.ThreadTS != e.TS
}

// Interaction is a button click (block_actions) or a modal submitted (view_submission).
type Interaction struct {
	Type      string `json:"type"`
	TriggerID string `json:"trigger_id"`
	// ResponseURL answers the person who clicked a button.
	ResponseURL string `json:"response_url"`
	User        struct {
		ID string `json:"id"`
	} `json:"user"`
	Team struct {
		ID string `json:"id"`
	} `json:"team"`
	Channel struct {
		ID string `json:"id"`
	} `json:"channel"`
	Container struct {
		MessageTS string `json:"message_ts"`
		ThreadTS  string `json:"thread_ts"`
	} `json:"container"`
	Actions []struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value"`
	} `json:"actions"`
	View struct {
		CallbackID      string `json:"callback_id"`
		PrivateMetadata string `json:"private_metadata"`
		State           struct {
			Values map[string]map[string]struct {
				Value          string `json:"value"`
				SelectedOption struct {
					Value string `json:"value"`
				} `json:"selected_option"`
			} `json:"values"`
		} `json:"state"`
	} `json:"view"`
}

// ParseInteraction reads the payload field of an interaction request.
func ParseInteraction(payload string) (Interaction, error) {
	var in Interaction
	err := json.Unmarshal([]byte(payload), &in)
	return in, err
}

// ReplyInput is what an agent entered in the reply modal.
type ReplyInput struct {
	IntegrationID  int
	ConversationID int
	// Channel and ThreadTS are where the modal was opened from.
	Channel  string
	ThreadTS string
	Mode     string
	Text     string
}

// ReplyInput reads the reply modal's fields and the conversation it is for.
func (in Interaction) ReplyInput() (ReplyInput, bool) {
	var r ReplyInput
	parts := strings.Split(in.View.PrivateMetadata, "|")
	if len(parts) != 4 {
		return r, false
	}
	var err1, err2 error
	r.IntegrationID, err1 = strconv.Atoi(parts[0])
	r.ConversationID, err2 = strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return r, false
	}
	r.Channel, r.ThreadTS = parts[2], parts[3]
	r.Mode = in.View.State.Values[blockMode]["mode"].SelectedOption.Value
	r.Text = strings.TrimSpace(in.View.State.Values[blockText]["text"].Value)
	if r.Mode != ModeNote {
		r.Mode = ModeReply
	}
	return r, true
}

// Translator looks up the workspace's wording for a key.
type Translator = chatmsg.Translator

// RootBlocks lays out the first message of a conversation's thread, with the
// buttons agents act on it with.
func RootBlocks(t Translator, post chatmsg.Post) []map[string]any {
	id := strconv.Itoa(post.ConversationID)
	return chatmsg.SlackBlocks(post,
		chatmsg.SlackButton{Text: t.T("chat.action.assignToMe"), ActionID: ActionAssign, Value: id},
		chatmsg.SlackButton{Text: t.T("chat.action.reply"), ActionID: ActionReply, Value: id, Style: "primary"},
		chatmsg.SlackButton{Text: t.T("chat.action.resolve"), ActionID: ActionResolve, Value: id},
	)
}

// ReplyModal is the modal an agent writes a reply or an internal note in.
// Its private metadata names the connection, conversation and thread.
func ReplyModal(t Translator, integrationID, conversationID int, channel, threadTS, title string) map[string]any {
	option := func(value, text string) map[string]any {
		return map[string]any{"value": value, "text": map[string]any{"type": "plain_text", "text": text}}
	}
	reply := option(ModeReply, t.T("chat.modal.toCustomer"))
	blocks := []map[string]any{}
	if title != "" {
		blocks = append(blocks, map[string]any{
			"type":     "context",
			"elements": []map[string]any{{"type": "mrkdwn", "text": chatmsg.SlackEscape(title)}},
		})
	}
	blocks = append(blocks,
		map[string]any{
			"type": "input", "block_id": blockMode,
			"label": map[string]any{"type": "plain_text", "text": t.T("chat.modal.sendAs")},
			"element": map[string]any{
				"type": "radio_buttons", "action_id": "mode", "initial_option": reply,
				"options": []map[string]any{reply, option(ModeNote, t.T("chat.modal.internalNote"))},
			},
		},
		map[string]any{
			"type": "input", "block_id": blockText,
			"label":   map[string]any{"type": "plain_text", "text": t.T("chat.modal.message")},
			"element": map[string]any{"type": "plain_text_input", "action_id": "text", "multiline": true},
		},
	)
	return map[string]any{
		"type":             "modal",
		"callback_id":      CallbackReply,
		"private_metadata": strconv.Itoa(integrationID) + "|" + strconv.Itoa(conversationID) + "|" + channel + "|" + threadTS,
		"title":            plainText(chatmsg.Truncate(t.T("chat.modal.title"), 24)),
		"submit":           plainText(chatmsg.Truncate(t.T("chat.modal.send"), 24)),
		"close":            plainText(chatmsg.Truncate(t.T("chat.modal.cancel"), 24)),
		"blocks":           blocks,
	}
}

// ModalError answers a view_submission by showing msg under the text field.
func ModalError(msg string) map[string]any {
	return map[string]any{"response_action": "errors", "errors": map[string]string{blockText: msg}}
}

func plainText(s string) map[string]any {
	return map[string]any{"type": "plain_text", "text": s}
}

var (
	// reLink is a link in Slack's markup: <https://x|label> or <https://x>.
	reLink = regexp.MustCompile(`<((?:https?|mailto):[^|>]+)(?:\|([^>]+))?>`)
	// reMention is a user, channel or group mention: <@U1>, <#C1|name>, <!here>.
	reMention = regexp.MustCompile(`<[@#!]([^|>]+)(?:\|([^>]+))?>`)
)

// TextToHTML turns a Slack message into the HTML a desk message is stored as:
// links keep their address, mentions their name, and Slack's escapes are undone
// before the text is escaped for HTML.
func TextToHTML(text string) string {
	text = reLink.ReplaceAllStringFunc(text, func(m string) string {
		sub := reLink.FindStringSubmatch(m)
		if sub[2] != "" && sub[2] != sub[1] {
			return sub[2] + " (" + sub[1] + ")"
		}
		return sub[1]
	})
	text = reMention.ReplaceAllStringFunc(text, func(m string) string {
		sub := reMention.FindStringSubmatch(m)
		if sub[2] != "" {
			return "@" + sub[2]
		}
		return "@" + sub[1]
	})
	text = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&").Replace(text)
	return PlainToHTML(text)
}

// PlainToHTML escapes plain text for HTML, one paragraph per blank-line block.
func PlainToHTML(text string) string {
	var out []string
	for _, para := range strings.Split(strings.TrimSpace(text), "\n\n") {
		if para = strings.TrimSpace(para); para != "" {
			out = append(out, "<p>"+strings.ReplaceAll(html.EscapeString(para), "\n", "<br>")+"</p>")
		}
	}
	return strings.Join(out, "")
}
