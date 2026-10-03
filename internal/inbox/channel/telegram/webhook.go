package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"mime"
	"path"
	"strconv"
	"strings"

	"github.com/conectdesk/conectdesk/internal/attachment"
	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/volatiletech/null/v9"
)

// VerifySecret compares the X-Telegram-Bot-Api-Secret-Token header with the
// inbox secret in constant time. An inbox with no secret rejects everything.
func VerifySecret(provided, expected string) bool {
	if expected == "" || provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// Update is the part of a Bot API update we consume.
type Update struct {
	UpdateID        int64    `json:"update_id"`
	Message         *Message `json:"message"`
	BusinessMessage *Message `json:"business_message"`
	CallbackQuery   *struct {
		ID   string `json:"id"`
		From *User  `json:"from"`
		Data string `json:"data"`
		// Message is the bot's message whose button was tapped.
		Message *Message `json:"message"`
	} `json:"callback_query"`
	BusinessConnection *struct {
		ID        string `json:"id"`
		User      User   `json:"user"`
		CanReply  bool   `json:"can_reply"`
		IsEnabled bool   `json:"is_enabled"`
	} `json:"business_connection"`
}

// User is a Telegram user.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

func (u *User) displayName() string {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" && u.Username != "" {
		name = "@" + u.Username
	}
	return name
}

// Message is an incoming Telegram message.
type Message struct {
	MessageID int64 `json:"message_id"`
	Date      int64 `json:"date"`
	From      *User `json:"from"`
	Chat      struct {
		ID    int64  `json:"id"`
		Type  string `json:"type"`
		Title string `json:"title"`
	} `json:"chat"`
	BusinessConnectionID string `json:"business_connection_id"`
	Text                 string `json:"text"`
	Caption              string `json:"caption"`
	Photo                []struct {
		FileID   string `json:"file_id"`
		FileSize int64  `json:"file_size"`
	} `json:"photo"`
	Document  *File `json:"document"`
	Video     *File `json:"video"`
	Audio     *File `json:"audio"`
	Voice     *File `json:"voice"`
	VideoNote *File `json:"video_note"`
	Sticker   *struct {
		FileID string `json:"file_id"`
		Emoji  string `json:"emoji"`
	} `json:"sticker"`
	Location *struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"location"`
	Contact *struct {
		PhoneNumber string `json:"phone_number"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
	} `json:"contact"`
}

// File is a file attached to a message.
type File struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

// BusinessUpdate returns the Telegram Business connection an update reports:
// the business linked the bot (enabled) or unlinked it. ok is false for any
// other update.
func BusinessUpdate(u Update) (link BusinessLink, enabled, ok bool) {
	bc := u.BusinessConnection
	if bc == nil {
		return BusinessLink{}, false, false
	}
	return BusinessLink{
		ConnectionID: bc.ID,
		UserID:       bc.User.ID,
		Name:         bc.User.displayName(),
		Username:     bc.User.Username,
		CanReply:     bc.CanReply,
	}, bc.IsEnabled, true
}

// ParseUpdate turns a webhook update into an IncomingMessage for the
// conversation manager: a message to the bot, a message to the linked
// business account, or a tap on a reply button. It returns meta.ErrIgnored
// for updates with nothing to store.
func (t *Telegram) ParseUpdate(ctx context.Context, u Update) (models.IncomingMessage, error) {
	switch {
	case u.CallbackQuery != nil:
		return t.parseCallback(ctx, u)
	case u.BusinessMessage != nil:
		return t.parseMessage(ctx, u.BusinessMessage, true)
	case u.Message != nil:
		return t.parseMessage(ctx, u.Message, false)
	}
	return models.IncomingMessage{}, meta.ErrIgnored{Reason: "not a message"}
}

// chatTarget is who a conversation is with: the person in a private chat, or
// the group, and the ID replies are addressed to.
type chatTarget struct {
	recipient string
	name      string
	group     bool
}

// target works out a message's chat, or why it is skipped. Support happens
// one to one unless the inbox takes group chats.
func (t *Telegram) target(msg *Message, business bool) (chatTarget, error) {
	if msg.From == nil || msg.From.IsBot {
		return chatTarget{}, meta.ErrIgnored{Reason: "not from a person"}
	}
	chatID := strconv.FormatInt(msg.Chat.ID, 10)
	switch msg.Chat.Type {
	case "private":
	case "group", "supergroup":
		if !t.config.AllowGroups || business {
			return chatTarget{}, meta.ErrIgnored{Reason: "group chat"}
		}
		return chatTarget{recipient: chatID, name: msg.Chat.Title, group: true}, nil
	default:
		return chatTarget{}, meta.ErrIgnored{Reason: "chat type " + msg.Chat.Type}
	}
	if business {
		link := t.config.Business
		if link == nil || link.ConnectionID != msg.BusinessConnectionID {
			return chatTarget{}, meta.ErrIgnored{Reason: "unknown business connection"}
		}
		// The owner's own messages, sent from their phone, come back too.
		if msg.From.ID == link.UserID {
			return chatTarget{}, meta.ErrIgnored{Reason: "business owner's message"}
		}
		chatID = BusinessChatPrefix + chatID
	}
	return chatTarget{recipient: chatID, name: msg.From.displayName()}, nil
}

func (t *Telegram) parseMessage(ctx context.Context, msg *Message, business bool) (models.IncomingMessage, error) {
	var in models.IncomingMessage
	tgt, err := t.target(msg, business)
	if err != nil {
		return in, err
	}

	content := msg.Text
	if content == "" {
		content = msg.Caption
	}
	// /start opens the chat with the bot; keep it so the agent sees the person
	// arrived, but as words.
	if content == "/start" || strings.HasPrefix(content, "/start ") {
		content = "[started the chat]"
	}

	var file *File
	switch {
	case len(msg.Photo) > 0:
		// Sizes come smallest first; take the largest.
		p := msg.Photo[len(msg.Photo)-1]
		file = &File{FileID: p.FileID, MimeType: "image/jpeg", FileSize: p.FileSize}
	case msg.Document != nil:
		file = msg.Document
	case msg.Video != nil:
		file = msg.Video
	case msg.Audio != nil:
		file = msg.Audio
	case msg.Voice != nil:
		file = msg.Voice
	case msg.VideoNote != nil:
		file = msg.VideoNote
		if file.MimeType == "" {
			file.MimeType = "video/mp4"
		}
	case msg.Sticker != nil:
		content = strings.TrimSpace("[sticker] " + msg.Sticker.Emoji)
	case msg.Location != nil:
		content = fmt.Sprintf("[location] https://maps.google.com/?q=%f,%f", msg.Location.Latitude, msg.Location.Longitude)
	case msg.Contact != nil:
		content = strings.TrimSpace(fmt.Sprintf("[contact] %s %s %s", msg.Contact.FirstName, msg.Contact.LastName, msg.Contact.PhoneNumber))
	}
	if content == "" && file == nil {
		content = "[unsupported message]"
	}
	// In a group, say who wrote each message.
	if tgt.group && content != "" {
		content = msg.From.displayName() + ": " + content
	}

	in = t.incoming(tgt, fmt.Sprintf("%s:%d", tgt.recipient, msg.MessageID), content)
	metaMap := map[string]any{"telegram_chat_id": tgt.recipient, "telegram_username": msg.From.Username}
	if file != nil {
		att, err := t.download(ctx, file)
		if err != nil {
			t.lo.Error("error downloading telegram file", "inbox_id", t.id, "error", err)
			metaMap["media_error"] = err.Error()
			if in.Content == "" {
				in.Content = "[" + meta.MediaKind(file.MimeType) + "]"
			}
		} else {
			in.Attachments = attachment.Attachments{att}
		}
	}
	b, err := json.Marshal(metaMap)
	if err != nil {
		return in, err
	}
	in.Meta = b
	return in, nil
}

// parseCallback turns a tap on a quick reply button into the customer's
// answer, and acknowledges the tap so Telegram stops its spinner.
func (t *Telegram) parseCallback(ctx context.Context, u Update) (models.IncomingMessage, error) {
	cb := u.CallbackQuery
	if cb.ID != "" {
		if err := t.api.AnswerCallback(ctx, cb.ID); err != nil {
			t.lo.Debug("error answering telegram button tap", "inbox_id", t.id, "error", err)
		}
	}
	label, ok := strings.CutPrefix(cb.Data, callbackPrefix)
	if !ok || cb.Message == nil || cb.From == nil {
		return models.IncomingMessage{}, meta.ErrIgnored{Reason: "unknown button"}
	}
	// The tapped message is the bot's; the chat and the tapper come from the query.
	msg := *cb.Message
	msg.From = cb.From
	tgt, err := t.target(&msg, msg.BusinessConnectionID != "")
	if err != nil {
		return models.IncomingMessage{}, err
	}
	content := label
	if tgt.group {
		content = cb.From.displayName() + ": " + label
	}
	in := t.incoming(tgt, "cb:"+cb.ID, content)
	in.Meta, _ = json.Marshal(map[string]any{"telegram_chat_id": tgt.recipient, "telegram_button": label})
	return in, nil
}

// incoming builds the message for a chat. Telegram message IDs are only
// unique within a chat, so the source ID carries the bot and the chat.
func (t *Telegram) incoming(tgt chatTarget, id, content string) models.IncomingMessage {
	fallback := "Telegram user"
	if tgt.group {
		fallback = "Telegram group"
	}
	first, last := meta.SplitName(tgt.name, fallback)
	if tgt.group {
		// A group's title is one name, not a first and last.
		first, last = strings.TrimSpace(tgt.name), ""
		if first == "" {
			first = fallback
		}
	}
	return models.IncomingMessage{
		TenantID:    t.tenantID,
		Channel:     ChannelTelegram,
		InboxID:     t.id,
		SourceID:    null.StringFrom(fmt.Sprintf("telegram:%d:%s", t.config.BotID, id)),
		Content:     content,
		ContentType: models.ContentTypeText,
		Contact: models.IncomingContact{
			FirstName:      first,
			LastName:       last,
			ExternalUserID: null.StringFrom(ExternalIDPrefix + tgt.recipient),
		},
	}
}

func (t *Telegram) download(ctx context.Context, f *File) (attachment.Attachment, error) {
	maxBytes := int64(t.config.MaxMediaSizeMB) * 1024 * 1024
	content, filePath, err := t.api.Download(ctx, f.FileID, maxBytes)
	if err != nil {
		return attachment.Attachment{}, err
	}
	contentType := f.MimeType
	if contentType == "" {
		contentType = mime.TypeByExtension(path.Ext(filePath))
	}
	if base, _, err := mime.ParseMediaType(contentType); err == nil {
		contentType = base
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	name := f.FileName
	if name == "" {
		name = path.Base(filePath)
	}
	return attachment.Attachment{
		Name:        name,
		Size:        len(content),
		Content:     content,
		ContentType: contentType,
		Disposition: attachment.DispositionAttachment,
	}, nil
}
