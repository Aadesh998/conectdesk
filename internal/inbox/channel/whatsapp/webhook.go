package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"strings"

	"github.com/conectdesk/conectdesk/internal/attachment"
	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/volatiletech/null/v9"
)

// Value is the value of a "messages" change on the WhatsApp webhook.
type Value struct {
	MessagingProduct string `json:"messaging_product"`
	Metadata         struct {
		DisplayPhoneNumber string `json:"display_phone_number"`
		PhoneNumberID      string `json:"phone_number_id"`
	} `json:"metadata"`
	Contacts []struct {
		Profile struct {
			Name string `json:"name"`
		} `json:"profile"`
		WaID string `json:"wa_id"`
	} `json:"contacts"`
	Messages []Message `json:"messages"`
	Statuses []Status  `json:"statuses"`
}

// Message is one inbound WhatsApp message.
type Message struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`

	Text *struct {
		Body string `json:"body"`
	} `json:"text"`
	Image    *Media `json:"image"`
	Video    *Media `json:"video"`
	Audio    *Media `json:"audio"`
	Document *Media `json:"document"`
	Sticker  *Media `json:"sticker"`
	Location *struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Name      string  `json:"name"`
		Address   string  `json:"address"`
	} `json:"location"`
	Button *struct {
		Text string `json:"text"`
	} `json:"button"`
	Interactive *struct {
		Type        string `json:"type"`
		ButtonReply *struct {
			Title string `json:"title"`
		} `json:"button_reply"`
		ListReply *struct {
			Title string `json:"title"`
		} `json:"list_reply"`
	} `json:"interactive"`
	Contacts []struct {
		Name struct {
			FormattedName string `json:"formatted_name"`
		} `json:"name"`
		Phones []struct {
			Phone string `json:"phone"`
		} `json:"phones"`
	} `json:"contacts"`
}

// Media is an inbound media object, downloaded by its ID.
type Media struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
}

// Status is a delivery status for a message we sent.
type Status struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	RecipientID string `json:"recipient_id"`
	Errors      []struct {
		Code      int    `json:"code"`
		Title     string `json:"title"`
		Message   string `json:"message"`
		ErrorData struct {
			Details string `json:"details"`
		} `json:"error_data"`
	} `json:"errors"`
}

// Receipts returns the delivery statuses in a webhook change: sent,
// delivered, read, or failed with Meta's reason, e.g. a reply sent after the
// 24-hour window (131047).
func (w *WhatsApp) Receipts(v Value) []meta.Receipt {
	var out []meta.Receipt
	for _, s := range v.Statuses {
		switch s.Status {
		case "sent", "delivered", "read", "failed":
		default:
			// "deleted" and "warning" say nothing about delivery.
			continue
		}
		r := meta.Receipt{ExternalIDs: []string{s.ID}, Status: s.Status, Recipient: s.RecipientID}
		if s.Status == "failed" {
			var reasons []string
			for _, e := range s.Errors {
				reason := strings.TrimSpace(e.Title)
				if e.Message != "" && e.Message != e.Title {
					reason = strings.TrimSpace(reason + ": " + e.Message)
				}
				if detail := strings.TrimSpace(e.ErrorData.Details); detail != "" {
					reason += " (" + detail + ")"
				}
				reasons = append(reasons, fmt.Sprintf("%d %s", e.Code, reason))
			}
			r.Error = strings.Join(reasons, "; ")
			w.lo.Warn("whatsapp message failed", "inbox_id", w.id, "message_id", s.ID, "error", r.Error)
		}
		out = append(out, r)
	}
	return out
}

// ParseValue turns the messages in a webhook change into IncomingMessages for
// the conversation manager, downloading any attached media. Messages with
// nothing to store, such as reactions, are left out.
func (w *WhatsApp) ParseValue(ctx context.Context, v Value) []models.IncomingMessage {

	names := map[string]string{}
	for _, c := range v.Contacts {
		names[c.WaID] = c.Profile.Name
	}

	var out []models.IncomingMessage
	for _, msg := range v.Messages {
		in, err := w.buildIncomingMessage(ctx, msg, names[msg.From])
		if err != nil {
			w.lo.Debug("skipping whatsapp message", "inbox_id", w.id, "message_id", msg.ID, "reason", err)
			continue
		}
		out = append(out, in)
	}
	return out
}

func (w *WhatsApp) buildIncomingMessage(ctx context.Context, msg Message, profileName string) (models.IncomingMessage, error) {
	var in models.IncomingMessage

	if msg.ID == "" {
		return in, fmt.Errorf("message has no id")
	}
	if !IsValidWAID(msg.From) {
		return in, fmt.Errorf("malformed sender %q", msg.From)
	}

	content, media := messageBody(msg)
	if content == "" && media == nil {
		return in, meta.ErrIgnored{Reason: "message type " + msg.Type}
	}

	first, last := meta.SplitName(profileName, "+"+msg.From)
	in = models.IncomingMessage{
		TenantID:    w.tenantID,
		Channel:     ChannelWhatsApp,
		InboxID:     w.id,
		SourceID:    null.StringFrom(msg.ID),
		Content:     content,
		ContentType: models.ContentTypeText,
		Contact: models.IncomingContact{
			FirstName:      first,
			LastName:       last,
			PhoneNumber:    null.StringFrom(msg.From),
			ExternalUserID: null.StringFrom(ExternalIDPrefix + msg.From),
		},
	}

	metaMap := map[string]any{"whatsapp_wa_id": msg.From}

	// A failed download is logged and the message is still stored with its
	// caption, because losing the text too would hide it from the agent.
	if media != nil {
		att, err := w.downloadMedia(ctx, media)
		if err != nil {
			w.lo.Error("error downloading whatsapp media", "inbox_id", w.id, "message_id", msg.ID, "error", err)
			metaMap["whatsapp_media_error"] = err.Error()
			if in.Content == "" {
				in.Content = fmt.Sprintf("[%s]", meta.MediaKind(media.MimeType))
			}
		} else {
			in.Attachments = attachment.Attachments{att}
		}
	}

	metaJSON, err := json.Marshal(metaMap)
	if err != nil {
		return in, fmt.Errorf("marshalling whatsapp message meta: %w", err)
	}
	in.Meta = metaJSON
	return in, nil
}

// messageBody returns a message's text and the media to download, if any.
// Kinds a helpdesk can't render get a placeholder so the agent can see
// something arrived and follow up.
func messageBody(msg Message) (string, *Media) {
	switch msg.Type {
	case "text":
		if msg.Text != nil {
			return msg.Text.Body, nil
		}
	case "image":
		return caption(msg.Image), msg.Image
	case "video":
		return caption(msg.Video), msg.Video
	case "audio":
		return "", msg.Audio
	case "document":
		return caption(msg.Document), msg.Document
	case "sticker":
		return "", msg.Sticker
	case "location":
		if l := msg.Location; l != nil {
			label := strings.TrimSpace(strings.Join([]string{l.Name, l.Address}, " "))
			return strings.TrimSpace(fmt.Sprintf("[location] %s https://maps.google.com/?q=%f,%f", label, l.Latitude, l.Longitude)), nil
		}
	case "contacts":
		var parts []string
		for _, c := range msg.Contacts {
			entry := c.Name.FormattedName
			for _, p := range c.Phones {
				entry += " " + p.Phone
			}
			parts = append(parts, strings.TrimSpace(entry))
		}
		return "[contact] " + strings.Join(parts, ", "), nil
	case "button":
		if msg.Button != nil {
			return msg.Button.Text, nil
		}
	case "interactive":
		if i := msg.Interactive; i != nil {
			if i.ButtonReply != nil {
				return i.ButtonReply.Title, nil
			}
			if i.ListReply != nil {
				return i.ListReply.Title, nil
			}
		}
	case "reaction", "system", "request_welcome":
		// Not messages from the contact worth a conversation.
		return "", nil
	}
	return "[unsupported message]", nil
}

func caption(m *Media) string {
	if m == nil {
		return ""
	}
	return m.Caption
}

// downloadMedia resolves a media ID and downloads the file.
func (w *WhatsApp) downloadMedia(ctx context.Context, m *Media) (attachment.Attachment, error) {
	if m.ID == "" {
		return attachment.Attachment{}, fmt.Errorf("media has no id")
	}
	mediaURL, mimeType, err := w.mediaURL(ctx, m.ID)
	if err != nil {
		return attachment.Attachment{}, fmt.Errorf("resolving media url: %w", err)
	}

	maxBytes := int64(w.config.MaxMediaSizeMB) * 1024 * 1024
	content, respType, err := w.graph.Download(ctx, mediaURL, w.config.AccessToken, maxBytes)
	if err != nil {
		return attachment.Attachment{}, err
	}

	contentType := firstNonEmpty([]string{m.MimeType, mimeType, respType, "application/octet-stream"})
	// Strip any "; codecs=..." parameters so the stored type stays a bare MIME type.
	if base, _, err := mime.ParseMediaType(contentType); err == nil {
		contentType = base
	}

	name := strings.TrimSpace(m.Filename)
	if name == "" {
		name = "whatsapp-" + m.ID
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
