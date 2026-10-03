package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/attachment"
	"github.com/conectdesk/conectdesk/internal/conversation"
	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/volatiletech/null/v9"
)

const (
	// Bounds on one import run, so a mistyped limit can't walk years of history.
	maxImportConversations = 500
	maxImportMessages      = 200
	defaultImportLimit     = 50
)

// ImportOpts configures a history import.
type ImportOpts struct {
	// ConversationLimit is how many recent conversations to import, newest first.
	ConversationLimit int `json:"conversation_limit"`
	// MessageLimit is how many recent messages to import from each.
	MessageLimit int `json:"message_limit"`
}

// Chat is one conversation read from the Page's or account's history.
type Chat struct {
	Contact  models.IncomingContact
	Messages []conversation.HistoryMessage
}

// graphMessage is a message as the Conversations API returns it.
type graphMessage struct {
	ID          string `json:"id"`
	CreatedTime string `json:"created_time"`
	Message     string `json:"message"`
	From        struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"from"`
	Attachments struct {
		Data []struct {
			MimeType  string `json:"mime_type"`
			Name      string `json:"name"`
			FileURL   string `json:"file_url"`
			ImageData *struct {
				URL string `json:"url"`
			} `json:"image_data"`
			VideoData *struct {
				URL string `json:"url"`
			} `json:"video_data"`
		} `json:"data"`
	} `json:"attachments"`
}

// History reads the Page's or Instagram account's recent conversations
// through the Conversations API, oldest message first in each, and hands each
// to fn. One conversation failing to read is reported and the rest go on.
func (m *Messenger) History(ctx context.Context, opts ImportOpts, fn func(Chat) error) (seen int, errs []string, err error) {
	convLimit := clamp(opts.ConversationLimit, defaultImportLimit, maxImportConversations)
	msgLimit := clamp(opts.MessageLimit, defaultImportLimit, maxImportMessages)

	platform := "messenger"
	if m.channel == ChannelInstagram {
		platform = "instagram"
	}
	q := url.Values{
		"platform": {platform},
		"fields":   {"id,updated_time"},
		"limit":    {fmt.Sprint(min(convLimit, 50))},
	}

	for seen < convLimit {
		var page struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			Paging struct {
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := m.graph.Get(ctx, m.config.AccessToken, m.config.PageID+"/conversations", q, &page); err != nil {
			return seen, errs, fmt.Errorf("listing conversations: %w", err)
		}
		for _, c := range page.Data {
			if seen >= convLimit {
				break
			}
			if err := ctx.Err(); err != nil {
				return seen, errs, err
			}
			seen++
			chat, err := m.readChat(ctx, c.ID, msgLimit)
			if err != nil {
				m.lo.Error("error reading conversation history", "channel", m.channel, "conversation", c.ID, "error", err)
				errs = append(errs, err.Error())
				continue
			}
			if len(chat.Messages) == 0 {
				continue
			}
			if err := fn(chat); err != nil {
				errs = append(errs, err.Error())
			}
		}
		if page.Paging.Next == "" || page.Paging.Cursors.After == "" {
			break
		}
		q.Set("after", page.Paging.Cursors.After)
	}
	return seen, errs, nil
}

// readChat reads one conversation's recent messages.
func (m *Messenger) readChat(ctx context.Context, conversationID string, limit int) (Chat, error) {
	var out struct {
		Messages struct {
			Data []graphMessage `json:"data"`
		} `json:"messages"`
	}
	fields := fmt.Sprintf("messages.limit(%d){id,created_time,message,from,attachments{mime_type,name,file_url,image_data,video_data}}", limit)
	if err := m.graph.Get(ctx, m.config.AccessToken, url.PathEscape(conversationID), url.Values{"fields": {fields}}, &out); err != nil {
		return Chat{}, err
	}

	var (
		chat     Chat
		contact  string
		name     string
		messages = out.Messages.Data
	)
	// Newest first from the API; stored oldest first.
	for i := len(messages) - 1; i >= 0; i-- {
		gm := messages[i]
		outgoing := gm.From.ID == m.config.AccountID || gm.From.ID == m.config.PageID
		if !outgoing && contact == "" {
			contact = gm.From.ID
			name = gm.From.Name
			if name == "" && gm.From.Username != "" {
				name = "@" + gm.From.Username
			}
		}
		hm, ok := m.historyMessage(ctx, gm, outgoing)
		if ok {
			chat.Messages = append(chat.Messages, hm)
		}
	}
	// A conversation where the person never wrote has no contact to file it under.
	if contact == "" || !isDigits(contact) {
		return Chat{}, nil
	}

	fallback := "Facebook user"
	if m.channel == ChannelInstagram {
		fallback = "Instagram user"
	}
	first, last := meta.SplitName(name, fallback)
	chat.Contact = models.IncomingContact{
		FirstName:      first,
		LastName:       last,
		ExternalUserID: null.StringFrom(ExternalIDPrefix(m.channel) + contact),
	}
	return chat, nil
}

// historyMessage converts one message from the Conversations API.
func (m *Messenger) historyMessage(ctx context.Context, gm graphMessage, outgoing bool) (conversation.HistoryMessage, bool) {
	if gm.ID == "" {
		return conversation.HistoryMessage{}, false
	}
	sentAt, _ := time.Parse("2006-01-02T15:04:05-0700", gm.CreatedTime)

	in := models.IncomingMessage{
		TenantID:    m.tenantID,
		Channel:     m.channel,
		InboxID:     m.id,
		SourceID:    null.StringFrom(gm.ID),
		Content:     gm.Message,
		ContentType: models.ContentTypeText,
	}
	for _, a := range gm.Attachments.Data {
		link := a.FileURL
		kind := "file"
		switch {
		case a.ImageData != nil && a.ImageData.URL != "":
			link, kind = a.ImageData.URL, "image"
		case a.VideoData != nil && a.VideoData.URL != "":
			link, kind = a.VideoData.URL, "video"
		}
		if link == "" {
			continue
		}
		att, err := m.download(ctx, attachmentRef{kind: kind, url: link})
		if err != nil {
			// History media expires on Meta's side; keep the message without it.
			in.Content = strings.TrimSpace(in.Content + "\n[" + kind + "]")
			continue
		}
		if a.Name != "" {
			att.Name = a.Name
		}
		in.Attachments = append(in.Attachments, att)
	}
	if in.Content == "" && len(in.Attachments) == 0 {
		in.Content = "[unsupported message]"
	}
	metaJSON, _ := json.Marshal(map[string]any{"imported": true})
	in.Meta = metaJSON
	if in.Attachments == nil {
		in.Attachments = attachment.Attachments{}
	}
	return conversation.HistoryMessage{IncomingMessage: in, Outgoing: outgoing, SentAt: sentAt}, true
}

func clamp(v, def, max int) int {
	if v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}
