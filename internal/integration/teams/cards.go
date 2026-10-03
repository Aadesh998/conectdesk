package teams

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"html"
	"image"
	"image/color"
	"image/png"
	"regexp"
	"strconv"
	"strings"

	"github.com/conectdesk/conectdesk/internal/chatmsg"
)

// Card actions agents submit.
const (
	ActionAssign  = "assign"
	ActionResolve = "resolve"
	ActionReply   = "reply"
	ActionNote    = "note"

	inputText = "cd_text"
)

// RootCard is the first message of a conversation's thread: the update, a box
// to write in, and the actions agents take on the conversation.
func RootCard(t chatmsg.Translator, post chatmsg.Post) map[string]any {
	id := strconv.Itoa(post.ConversationID)
	submit := func(title, action string) map[string]any {
		return map[string]any{
			"type":  "Action.Submit",
			"title": title,
			"data":  map[string]any{"cd_action": action, "conversation_id": id},
		}
	}
	input := []map[string]any{{
		"type":        "Input.Text",
		"id":          inputText,
		"isMultiline": true,
		"placeholder": t.T("chat.card.placeholder"),
	}}
	return chatmsg.AdaptiveCard(post, input, []map[string]any{
		submit(t.T("chat.action.sendReply"), ActionReply),
		submit(t.T("chat.action.addNote"), ActionNote),
		submit(t.T("chat.action.assignToMe"), ActionAssign),
		submit(t.T("chat.action.resolve"), ActionResolve),
	})
}

// UpdateCard is a later update, posted in the conversation's thread.
func UpdateCard(post chatmsg.Post) map[string]any {
	return chatmsg.AdaptiveCard(post, nil, nil)
}

// CardMessage wraps a card as a message.
func CardMessage(card map[string]any) Outgoing {
	return Outgoing{Type: "message", Attachments: []map[string]any{chatmsg.CardAttachment(card)}}
}

// TextMessage is a plain message.
func TextMessage(text string) Outgoing {
	return Outgoing{Type: "message", Text: text, TextFormat: "plain"}
}

// Submit is what an agent submitted from a card.
type Submit struct {
	Action         string
	ConversationID int
	Text           string
}

// ParseSubmit reads a card submission, which arrives as a message's value.
func ParseSubmit(value json.RawMessage) (Submit, bool) {
	if len(value) == 0 {
		return Submit{}, false
	}
	var v struct {
		Action         string          `json:"cd_action"`
		ConversationID json.RawMessage `json:"conversation_id"`
		Text           string          `json:"cd_text"`
	}
	if err := json.Unmarshal(value, &v); err != nil || v.Action == "" {
		return Submit{}, false
	}
	id, err := strconv.Atoi(strings.Trim(string(v.ConversationID), `"`))
	if err != nil || id <= 0 {
		return Submit{}, false
	}
	return Submit{Action: v.Action, ConversationID: id, Text: strings.TrimSpace(v.Text)}, true
}

var (
	reMention = regexp.MustCompile(`(?s)<at[^>]*>.*?</at>`)
	reTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	reCode    = regexp.MustCompile(`(?i)^connect\s+([a-z0-9-]{4,32})$`)
)

// CleanText is the text of a message with the bot's @mention and any markup
// removed, as a person typed it.
func CleanText(text string) string {
	text = reMention.ReplaceAllString(text, "")
	text = strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n").Replace(text)
	text = reTag.ReplaceAllString(text, "")
	return strings.TrimSpace(html.UnescapeString(text))
}

// LinkCode reads "connect CODE", sent to link a channel to a workspace.
func LinkCode(text string) (string, bool) {
	m := reCode.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return "", false
	}
	return strings.ToUpper(m[1]), true
}

// Package builds the Teams app package an admin uploads to add the bot to
// their teams: the manifest and its two icons, zipped.
func Package(app App, website, domain string) ([]byte, error) {
	name := app.Name
	if name == "" {
		name = "ConectDesk"
	}
	manifest := map[string]any{
		"$schema":         "https://developer.microsoft.com/en-us/json-schemas/teams/v1.17/MicrosoftTeams.schema.json",
		"manifestVersion": "1.17",
		"version":         "1.0.0",
		"id":              app.AppID,
		"developer": map[string]any{
			"name":          name,
			"websiteUrl":    website,
			"privacyUrl":    website,
			"termsOfUseUrl": website,
		},
		"name": map[string]any{"short": chatmsg.Truncate(name, 30), "full": name},
		"description": map[string]any{
			"short": "Customer conversations from " + name + " in your channels.",
			"full":  "Posts new conversations and their updates from " + name + " to a channel, and lets agents reply, add notes, assign and resolve them from Teams.",
		},
		"icons":       map[string]any{"color": "color.png", "outline": "outline.png"},
		"accentColor": "#16A34A",
		"bots": []map[string]any{{
			"botId":              app.AppID,
			"scopes":             []string{"team"},
			"isNotificationOnly": false,
			"supportsFiles":      false,
			"commandLists": []map[string]any{{
				"scopes":   []string{"team"},
				"commands": []map[string]any{{"title": "connect", "description": "Link this channel to your workspace with the code from its admin page"}},
			}},
		}},
		"permissions":  []string{"identity", "messageTeamMembers"},
		"validDomains": []string{domain},
	}
	mb, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := []struct {
		name string
		data func() ([]byte, error)
	}{
		{"manifest.json", func() ([]byte, error) { return mb, nil }},
		{"color.png", func() ([]byte, error) { return icon(192, color.RGBA{0x16, 0xa3, 0x4a, 0xff}, color.White) }},
		{"outline.png", func() ([]byte, error) { return icon(32, color.Transparent, color.White) }},
	}
	for _, f := range files {
		b, err := f.data()
		if err != nil {
			return nil, err
		}
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(b); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// icon draws a speech bubble, the app's icon: a ring with a tail, in fg on bg.
func icon(size int, bg, fg color.Color) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	c := float64(size) / 2
	outer, inner := float64(size)*0.36, float64(size)*0.24
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-c, float64(y)+0.5-c*0.92
			d2 := dx*dx + dy*dy
			tail := float64(y) > c*1.1 && float64(y) < c*1.62 && float64(x) > c*0.62 && float64(x) < c*0.62+(c*1.62-float64(y))*0.9
			if (d2 <= outer*outer && d2 >= inner*inner) || (tail && d2 > inner*inner) {
				img.Set(x, y, fg)
			} else {
				img.Set(x, y, bg)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
