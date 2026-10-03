// Package whatsapp implements a WhatsApp inbox on Meta's WhatsApp Business
// Platform (the Cloud API).
//
// A workspace connects a number through Embedded Signup on the platform's Meta
// app. Inbound messages arrive on the app's webhook and are parsed by
// ParseValue, so Receive is a no-op like the livechat channel. Replies are sent
// through the Graph API with the business token Embedded Signup issued.
package whatsapp

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/zerodha/logf"
)

const (
	ChannelWhatsApp = "whatsapp"

	// DefaultMaxMediaSizeMB caps inbound media downloads when the inbox doesn't set one.
	DefaultMaxMediaSizeMB = 20

	// maxTextLength is WhatsApp's per-message body limit. Longer replies are split.
	maxTextLength = 4096

	// maxCaptionLength is the limit on a media message's caption.
	maxCaptionLength = 1024

	// ExternalIDPrefix namespaces WhatsApp contacts in users.external_user_id so a
	// WhatsApp contact can never collide with one synced from another system.
	// The rest is the contact's WhatsApp ID, their number in international form
	// without the plus.
	ExternalIDPrefix = "whatsapp:"
)

// Config is the WhatsApp inbox configuration, stored as the inbox's JSON config.
// Everything but the message handling options is set by Embedded Signup.
type Config struct {
	// AccountID is the phone number ID, which webhooks are addressed to and
	// messages are sent from.
	AccountID string `json:"account_id"`

	// WABAID is the WhatsApp Business Account the number belongs to.
	WABAID string `json:"waba_id"`

	// DisplayPhoneNumber and VerifiedName are shown in the admin UI.
	DisplayPhoneNumber string `json:"display_phone_number"`
	VerifiedName       string `json:"verified_name"`

	// AccessToken is the business token Embedded Signup issued. It doesn't
	// expire, and is encrypted at rest by the inbox manager.
	AccessToken string `json:"access_token"`

	// TwoStepPIN is the two-step verification PIN the number was registered
	// with, needed to register it again. Encrypted at rest.
	TwoStepPIN string `json:"two_step_pin"`

	// RegistrationError is why registering the number for the Cloud API failed,
	// if it did, shown in the admin UI.
	RegistrationError string `json:"registration_error,omitempty"`

	// ConnectedBy is the Meta user who connected the number, which a
	// deauthorize or data deletion callback names.
	ConnectedBy string `json:"connected_by"`

	// MaxMediaSizeMB caps the size of an inbound media file we will download.
	MaxMediaSizeMB int `json:"max_media_size_mb"`

	// MarkSeen sends a read receipt (blue ticks) when a message is received.
	MarkSeen bool `json:"mark_seen"`
}

// WhatsApp is a Cloud API inbox.
type WhatsApp struct {
	id           int
	tenantID     int
	name         string
	from         string
	config       Config
	graph        *meta.Client
	lo           *logf.Logger
	messageStore inbox.MessageStore
	userStore    inbox.UserStore

	// templates caches the account's approved templates.
	tmu         sync.Mutex
	templates   []Template
	templatesAt time.Time
}

// Opts holds the options required for the WhatsApp inbox.
type Opts struct {
	ID int
	// TenantID is the tenant owning the inbox.
	TenantID int
	Name     string
	From     string
	Config   Config
	Graph    *meta.Client
	Lo       *logf.Logger
}

// New returns a new WhatsApp inbox.
func New(store inbox.MessageStore, userStore inbox.UserStore, opts Opts) (*WhatsApp, error) {
	cfg := opts.Config
	if cfg.AccountID == "" || cfg.AccessToken == "" {
		return nil, fmt.Errorf("whatsapp inbox %d is not connected", opts.ID)
	}
	if opts.Graph == nil || !opts.Graph.App().Configured() {
		return nil, fmt.Errorf("the platform's meta app is not configured")
	}
	if cfg.MaxMediaSizeMB <= 0 {
		cfg.MaxMediaSizeMB = DefaultMaxMediaSizeMB
	}

	return &WhatsApp{
		id:           opts.ID,
		tenantID:     opts.TenantID,
		name:         opts.Name,
		from:         opts.From,
		config:       cfg,
		graph:        opts.Graph,
		lo:           opts.Lo,
		messageStore: store,
		userStore:    userStore,
	}, nil
}

// Identifier returns the inbox's database ID.
func (w *WhatsApp) Identifier() int {
	return w.id
}

// Channel returns the channel name for this inbox.
func (w *WhatsApp) Channel() string {
	return ChannelWhatsApp
}

// Name returns the inbox name.
func (w *WhatsApp) Name() string {
	return w.name
}

// FromAddress returns the WhatsApp number this inbox sends from.
func (w *WhatsApp) FromAddress() string {
	return w.from
}

// ReplyToAddress is not applicable to WhatsApp and always returns empty.
func (w *WhatsApp) ReplyToAddress() string {
	return ""
}

// FromNameTemplate is not applicable to WhatsApp and always returns empty.
func (w *WhatsApp) FromNameTemplate() string {
	return ""
}

// AccountID returns the phone number ID webhooks for this inbox are addressed to.
func (w *WhatsApp) AccountID() string {
	return w.config.AccountID
}

// TenantID returns the tenant owning the inbox.
func (w *WhatsApp) TenantID() int {
	return w.tenantID
}

// Receive is a no-op; inbound messages arrive over the Meta webhook.
func (w *WhatsApp) Receive(ctx context.Context) error {
	return nil
}

// Close is a no-op; the inbox holds no long-lived connections.
func (w *WhatsApp) Close() error {
	return nil
}

// Config returns the inbox's resolved configuration.
func (w *WhatsApp) Config() Config {
	return w.config
}

// MarkSeen sends a read receipt for a message if the inbox is configured to.
func (w *WhatsApp) MarkSeen(ctx context.Context, messageID string) {
	if !w.config.MarkSeen || messageID == "" {
		return
	}
	body := map[string]string{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        messageID,
	}
	if err := w.graph.PostJSON(ctx, w.config.AccessToken, w.config.AccountID+"/messages", body, nil); err != nil {
		w.lo.Warn("error marking whatsapp message as read", "inbox_id", w.id, "message_id", messageID, "error", err)
	}
}

// Send delivers an outgoing message through the Cloud API.
//
// The recipient's WhatsApp ID travels in the message's `to` recipients, set by
// the conversation manager when the reply is queued.
//
// WhatsApp only accepts a free-form message within 24 hours of the contact's
// last one. Outside that window Meta accepts the request but later reports the
// message failed (error 131047) on the status webhook; only an approved
// template can reopen the conversation.
func (w *WhatsApp) Send(message models.OutboundMessage) error {
	_, _, err := w.SendTracked(message)
	return err
}

// SendTracked sends a message like Send, and returns the recipient and the
// IDs WhatsApp gave each message it sent (a long reply goes as several), which
// the status webhook later reports delivery and read receipts against.
func (w *WhatsApp) SendTracked(message models.OutboundMessage) (string, []string, error) {
	to := firstNonEmpty(message.To)
	if !IsValidWAID(to) {
		return "", nil, fmt.Errorf("invalid whatsapp recipient %q on message %s", to, message.UUID)
	}
	var ids []string

	ctx := context.Background()

	// A template goes out as prepared when the agent sent it; its content is
	// only the rendered copy kept for the conversation.
	if pt, ok := TemplateFromMeta(message.Meta); ok {
		switch {
		case pt.HeaderMedia != "":
			if len(message.Attachments) != 1 || len(message.Attachments[0].Content) == 0 {
				return to, nil, fmt.Errorf("template %q needs one file for its header", pt.Name)
			}
			header, err := w.headerMedia(ctx, pt.HeaderMedia, message.Attachments[0].Name, message.Attachments[0].ContentType, message.Attachments[0].Content)
			if err != nil {
				return to, nil, fmt.Errorf("uploading template header: %w", err)
			}
			pt.Components = append([]map[string]any{header}, pt.Components...)
		case len(message.Attachments) > 0:
			return to, nil, fmt.Errorf("a whatsapp template can't carry attachments")
		}
		body := map[string]any{
			"messaging_product": "whatsapp",
			"recipient_type":    "individual",
			"to":                to,
			"type":              "template",
			"template":          templateBody(pt),
		}
		id, err := w.post(ctx, body)
		if err != nil {
			return to, nil, fmt.Errorf("sending whatsapp template %q: %w", pt.Name, err)
		}
		w.lo.Info("whatsapp template sent", "inbox_id", w.id, "message_uuid", message.UUID, "template", pt.Name)
		return to, appendID(ids, id), nil
	}

	// Agents compose in a rich text editor, so the body arrives as HTML.
	// WhatsApp only renders plain text.
	text := strings.TrimSpace(stringutil.HTML2Text(message.Content))

	// The first attachment carries a short reply as its caption, so "here you
	// go" stays with the file instead of arriving as a separate bubble.
	caption := ""
	if len(message.Attachments) > 0 && len(text) <= maxCaptionLength {
		caption, text = text, ""
	}

	for _, chunk := range meta.SplitText(text, maxTextLength) {
		body := map[string]any{
			"messaging_product": "whatsapp",
			"recipient_type":    "individual",
			"to":                to,
			"type":              "text",
			"text":              map[string]any{"body": chunk, "preview_url": false},
		}
		id, err := w.post(ctx, body)
		if err != nil {
			return to, ids, fmt.Errorf("sending whatsapp text: %w", err)
		}
		ids = appendID(ids, id)
	}

	for i, att := range message.Attachments {
		if len(att.Content) == 0 {
			w.lo.Warn("skipping whatsapp attachment with no content", "name", att.Name, "message_uuid", message.UUID)
			continue
		}
		attCaption := ""
		if i == 0 {
			attCaption = caption
		}
		sent, err := w.sendMedia(ctx, to, att.Name, att.ContentType, att.Content, attCaption)
		ids = append(ids, sent...)
		if err != nil {
			return to, ids, fmt.Errorf("sending whatsapp attachment %q: %w", att.Name, err)
		}
	}

	w.lo.Info("whatsapp message sent", "inbox_id", w.id, "message_uuid", message.UUID, "attachments", len(message.Attachments))
	return to, ids, nil
}

// post sends one message and returns the ID WhatsApp gave it.
func (w *WhatsApp) post(ctx context.Context, body map[string]any) (string, error) {
	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := w.graph.PostJSON(ctx, w.config.AccessToken, w.config.AccountID+"/messages", body, &out); err != nil {
		return "", err
	}
	if len(out.Messages) == 0 {
		return "", nil
	}
	return out.Messages[0].ID, nil
}

func appendID(ids []string, id string) []string {
	if id == "" {
		return ids
	}
	return append(ids, id)
}

// headerMedia uploads a template's header file and returns the header component.
func (w *WhatsApp) headerMedia(ctx context.Context, kind, name, contentType string, content []byte) (map[string]any, error) {
	id, err := w.upload(ctx, name, contentType, content)
	if err != nil {
		return nil, err
	}
	media := map[string]any{"id": id}
	if kind == "document" {
		media["filename"] = name
	}
	return map[string]any{
		"type":       "header",
		"parameters": []map[string]any{{"type": kind, kind: media}},
	}, nil
}

// templateBody is the Cloud API's template object for a prepared template.
func templateBody(pt PreparedTemplate) map[string]any {
	t := map[string]any{
		"name":     pt.Name,
		"language": map[string]string{"code": pt.Language},
	}
	if len(pt.Components) > 0 {
		t["components"] = pt.Components
	}
	return t
}

// sendMedia uploads a file to the Cloud API and sends it as a message.
func (w *WhatsApp) sendMedia(ctx context.Context, to, name, contentType string, content []byte, caption string) ([]string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	mediaID, err := w.upload(ctx, name, contentType, content)
	if err != nil {
		return nil, err
	}

	kind := outboundMediaType(contentType)
	media := map[string]any{"id": mediaID}
	if caption != "" && kind != "audio" {
		media["caption"] = caption
	}
	if kind == "document" {
		media["filename"] = name
	}
	body := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to,
		"type":              kind,
		kind:                media,
	}
	id, err := w.post(ctx, body)
	if err != nil {
		return nil, err
	}
	ids := appendID(nil, id)
	// Audio takes no caption, so send it as its own message.
	if caption != "" && kind == "audio" {
		text := map[string]any{
			"messaging_product": "whatsapp",
			"to":                to,
			"type":              "text",
			"text":              map[string]any{"body": caption},
		}
		id, err := w.post(ctx, text)
		return appendID(ids, id), err
	}
	return ids, nil
}

// upload uploads a file to the Cloud API for sending and returns its media ID.
func (w *WhatsApp) upload(ctx context.Context, name, contentType string, content []byte) (string, error) {
	var uploaded struct {
		ID string `json:"id"`
	}
	fields := map[string]string{"messaging_product": "whatsapp", "type": contentType}
	file := meta.File{Field: "file", Name: name, ContentType: contentType, Content: content}
	if err := w.graph.PostMultipart(ctx, w.config.AccessToken, w.config.AccountID+"/media", fields, file, &uploaded); err != nil {
		return "", fmt.Errorf("uploading media: %w", err)
	}
	return uploaded.ID, nil
}

// outboundMediaType picks the WhatsApp message type for a file. Only some
// formats play inline; anything else goes as a document so it still arrives.
func outboundMediaType(contentType string) string {
	switch contentType {
	case "image/jpeg", "image/png":
		return "image"
	case "video/mp4", "video/3gpp":
		return "video"
	case "audio/aac", "audio/amr", "audio/mpeg", "audio/mp4", "audio/ogg":
		return "audio"
	default:
		return "document"
	}
}

// mediaURL resolves an inbound media ID to its download URL.
func (w *WhatsApp) mediaURL(ctx context.Context, mediaID string) (string, string, error) {
	var out struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
	}
	if err := w.graph.Get(ctx, w.config.AccessToken, url.PathEscape(mediaID), nil, &out); err != nil {
		return "", "", err
	}
	return out.URL, out.MimeType, nil
}

// IsValidWAID reports whether s looks like a WhatsApp ID: the international
// number without the plus, 7 to 15 digits.
func IsValidWAID(s string) bool {
	if len(s) < 7 || len(s) > 15 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// WAIDFromPhone builds a WhatsApp ID from a phone number, stripping the plus
// and the punctuation people type into a phone field.
func WAIDFromPhone(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func firstNonEmpty(ss []string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
