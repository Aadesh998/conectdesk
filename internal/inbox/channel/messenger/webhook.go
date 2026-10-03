package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"path"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/attachment"
	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/volatiletech/null/v9"
)

// Event is one entry of a webhook's messaging array.
type Event struct {
	Sender struct {
		ID string `json:"id"`
	} `json:"sender"`
	Recipient struct {
		ID string `json:"id"`
	} `json:"recipient"`
	Timestamp int64 `json:"timestamp"`

	Message *struct {
		Mid           string `json:"mid"`
		Text          string `json:"text"`
		IsEcho        bool   `json:"is_echo"`
		IsDeleted     bool   `json:"is_deleted"`
		IsUnsupported bool   `json:"is_unsupported"`
		Attachments   []struct {
			Type    string `json:"type"`
			Payload struct {
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"payload"`
		} `json:"attachments"`
	} `json:"message"`

	// Delivery names the messages delivered, on Messenger.
	Delivery *struct {
		Mids      []string `json:"mids"`
		Watermark int64    `json:"watermark"`
	} `json:"delivery"`

	// Read is a read receipt: a watermark on Messenger, the message on Instagram.
	Read *struct {
		Mid       string `json:"mid"`
		Watermark int64  `json:"watermark"`
	} `json:"read"`

	Postback *struct {
		Mid     string `json:"mid"`
		Title   string `json:"title"`
		Payload string `json:"payload"`
	} `json:"postback"`
}

// downloadable attachment types carry a CDN URL to a file worth keeping.
var downloadable = map[string]bool{
	"image":         true,
	"video":         true,
	"audio":         true,
	"file":          true,
	"story_mention": true,
}

// ParseReceipt returns the delivery or read receipt a messaging event carries,
// if it is one. The sender of a receipt is the person our messages went to.
func (m *Messenger) ParseReceipt(raw json.RawMessage) (meta.Receipt, bool) {
	var ev Event
	if json.Unmarshal(raw, &ev) != nil {
		return meta.Receipt{}, false
	}
	switch {
	case ev.Delivery != nil && len(ev.Delivery.Mids) > 0:
		return meta.Receipt{ExternalIDs: ev.Delivery.Mids, Status: "delivered", Recipient: ev.Sender.ID}, true
	case ev.Read != nil && ev.Read.Mid != "":
		return meta.Receipt{ExternalIDs: []string{ev.Read.Mid}, Status: "read", Recipient: ev.Sender.ID}, true
	case ev.Read != nil && ev.Read.Watermark > 0:
		return meta.Receipt{Status: "read", Recipient: ev.Sender.ID, Watermark: time.UnixMilli(ev.Read.Watermark)}, true
	}
	return meta.Receipt{}, false
}

// ParseEvent turns one messaging event into an IncomingMessage for the
// conversation manager, downloading any attached media.
//
// It returns meta.ErrIgnored for events that are valid but not worth storing:
// echoes of our own replies, read and delivery receipts, reactions.
func (m *Messenger) ParseEvent(ctx context.Context, raw json.RawMessage) (models.IncomingMessage, error) {
	var (
		in models.IncomingMessage
		ev Event
	)
	if err := json.Unmarshal(raw, &ev); err != nil {
		return in, fmt.Errorf("decoding %s event: %w", m.channel, err)
	}

	var (
		sourceID string
		content  string
		urls     []attachmentRef
	)
	switch {
	case ev.Message != nil:
		msg := ev.Message
		// Replies sent from the Page, including ours, come back as echoes.
		if msg.IsEcho {
			return in, meta.ErrIgnored{Reason: "echo"}
		}
		if msg.IsDeleted {
			return in, meta.ErrIgnored{Reason: "deleted message"}
		}
		sourceID = msg.Mid
		content = msg.Text
		for _, a := range msg.Attachments {
			switch {
			case downloadable[a.Type] && a.Payload.URL != "":
				urls = append(urls, attachmentRef{kind: a.Type, url: a.Payload.URL})
			case a.Payload.URL != "":
				// Shared posts, reels and links: keep the link for the agent.
				content = strings.TrimSpace(content + "\n" + strings.TrimSpace(a.Payload.Title+" "+a.Payload.URL))
			}
		}
		if content == "" && len(urls) == 0 {
			content = "[unsupported message]"
		}
	case ev.Postback != nil:
		// A tap on a button in a message we sent.
		sourceID = ev.Postback.Mid
		content = ev.Postback.Title
		if content == "" {
			content = ev.Postback.Payload
		}
	default:
		return in, meta.ErrIgnored{Reason: "not a message"}
	}

	if sourceID == "" {
		return in, fmt.Errorf("%s message has no id", m.channel)
	}
	senderID := ev.Sender.ID
	// A message the account sent itself, from the app or the inbox on Meta.
	if senderID == m.config.AccountID || senderID == m.config.PageID {
		return in, meta.ErrIgnored{Reason: "own message"}
	}
	if !isDigits(senderID) {
		return in, fmt.Errorf("malformed %s sender %q", m.channel, senderID)
	}

	fallback := "Facebook user"
	if m.channel == ChannelInstagram {
		fallback = "Instagram user"
	}
	first, last := meta.SplitName(m.profileName(ctx, senderID), fallback)

	in = models.IncomingMessage{
		TenantID:    m.tenantID,
		Channel:     m.channel,
		InboxID:     m.id,
		SourceID:    null.StringFrom(sourceID),
		Content:     content,
		ContentType: models.ContentTypeText,
		Contact: models.IncomingContact{
			FirstName:      first,
			LastName:       last,
			ExternalUserID: null.StringFrom(ExternalIDPrefix(m.channel) + senderID),
		},
	}

	metaMap := map[string]any{m.channel + "_sender_id": senderID}
	for _, ref := range urls {
		att, err := m.download(ctx, ref)
		if err != nil {
			m.lo.Error("error downloading media", "channel", m.channel, "inbox_id", m.id, "source_id", sourceID, "error", err)
			metaMap["media_error"] = err.Error()
			if in.Content == "" {
				in.Content = fmt.Sprintf("[%s]", ref.kind)
			}
			continue
		}
		in.Attachments = append(in.Attachments, att)
	}

	metaJSON, err := json.Marshal(metaMap)
	if err != nil {
		return in, fmt.Errorf("marshalling message meta: %w", err)
	}
	in.Meta = metaJSON
	return in, nil
}

type attachmentRef struct {
	kind string
	url  string
}

// download fetches an inbound attachment from Meta's CDN. The URLs are signed,
// so no token is sent.
func (m *Messenger) download(ctx context.Context, ref attachmentRef) (attachment.Attachment, error) {
	maxBytes := int64(m.config.MaxMediaSizeMB) * 1024 * 1024
	content, contentType, err := m.graph.Download(ctx, ref.url, "", maxBytes)
	if err != nil {
		return attachment.Attachment{}, err
	}
	if base, _, err := mime.ParseMediaType(contentType); err == nil {
		contentType = base
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// CDN paths end in the file's name, before the signature query.
	name := ""
	if i := strings.IndexByte(ref.url, '?'); i >= 0 {
		name = path.Base(ref.url[:i])
	} else {
		name = path.Base(ref.url)
	}
	if name == "" || name == "." || name == "/" || !strings.Contains(name, ".") {
		name = m.channel + "-" + ref.kind
		if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
			name += exts[0]
		}
	}

	return attachment.Attachment{
		Name:        name,
		Size:        len(content),
		Content:     content,
		ContentType: contentType,
		Disposition: attachment.DispositionAttachment,
	}, nil
}
