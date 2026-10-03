// Package messenger implements the Facebook Messenger and Instagram inboxes,
// which Meta runs on the same Messenger Platform: the same webhook shape, the
// same Send API and the same 24-hour reply window.
//
// A workspace connects a Page, or the Instagram professional account linked to
// one, with Facebook Login on the platform's Meta app. The inbox keeps the
// Page's access token, which doesn't expire because it was read with a
// long-lived user token. Inbound messages arrive on the app's webhook and are
// parsed by ParseEvent, so Receive is a no-op.
package messenger

import (
	"context"
	"errors"
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
	ChannelMessenger = "messenger"
	ChannelInstagram = "instagram"

	// DefaultMaxMediaSizeMB caps inbound media downloads when the inbox doesn't set one.
	DefaultMaxMediaSizeMB = 20

	// Per-message text limits: 2000 characters on Messenger, 1000 bytes on
	// Instagram. Both are counted in bytes here, which is the stricter reading.
	maxMessengerText = 2000
	maxInstagramText = 1000

	// maxProfileCache bounds the per-inbox cache of contact names.
	maxProfileCache = 5000
)

// ErrInstagramAttachments is returned when an Instagram reply's file has no
// public link: the Instagram Send API only takes an attachment by URL.
var ErrInstagramAttachments = errors.New("instagram attachments need a public link to the file")

// Config is the Messenger or Instagram inbox configuration, stored as the
// inbox's JSON config. Everything but MaxMediaSizeMB is set by the connect flow.
type Config struct {
	// AccountID is what webhooks are addressed to: the Page ID for Messenger,
	// the Instagram account ID for Instagram.
	AccountID string `json:"account_id"`

	// PageID is the Page that sends replies. For Instagram it is the Page the
	// account is linked to.
	PageID   string `json:"page_id"`
	PageName string `json:"page_name"`

	// Username is the Instagram handle, shown in the admin UI.
	Username string `json:"username"`

	// AccessToken is the Page access token, encrypted at rest by the inbox manager.
	AccessToken string `json:"access_token"`

	// ConnectedBy is the Meta user who connected the account, which a
	// deauthorize or data deletion callback names.
	ConnectedBy string `json:"connected_by"`

	// MaxMediaSizeMB caps the size of an inbound media file we will download.
	MaxMediaSizeMB int `json:"max_media_size_mb"`
}

// Messenger is a Messenger or Instagram inbox.
type Messenger struct {
	id           int
	tenantID     int
	channel      string
	name         string
	from         string
	config       Config
	graph        *meta.Client
	lo           *logf.Logger
	messageStore inbox.MessageStore
	userStore    inbox.UserStore

	// profiles caches contact names by sender ID, so a busy chat doesn't look
	// the person up on every message.
	profiles sync.Map
	nProfile int
	pmu      sync.Mutex
}

// Opts holds the options required for the inbox.
type Opts struct {
	ID int
	// TenantID is the tenant owning the inbox.
	TenantID int
	// Channel is ChannelMessenger or ChannelInstagram.
	Channel string
	Name    string
	From    string
	Config  Config
	Graph   *meta.Client
	Lo      *logf.Logger
}

// New returns a new Messenger or Instagram inbox.
func New(store inbox.MessageStore, userStore inbox.UserStore, opts Opts) (*Messenger, error) {
	if opts.Channel != ChannelMessenger && opts.Channel != ChannelInstagram {
		return nil, fmt.Errorf("unknown channel %q", opts.Channel)
	}
	cfg := opts.Config
	if cfg.AccountID == "" || cfg.PageID == "" || cfg.AccessToken == "" {
		return nil, fmt.Errorf("%s inbox %d is not connected", opts.Channel, opts.ID)
	}
	if opts.Graph == nil || !opts.Graph.App().Configured() {
		return nil, fmt.Errorf("the platform's meta app is not configured")
	}
	if cfg.MaxMediaSizeMB <= 0 {
		cfg.MaxMediaSizeMB = DefaultMaxMediaSizeMB
	}

	return &Messenger{
		id:           opts.ID,
		tenantID:     opts.TenantID,
		channel:      opts.Channel,
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
func (m *Messenger) Identifier() int {
	return m.id
}

// Channel returns ChannelMessenger or ChannelInstagram.
func (m *Messenger) Channel() string {
	return m.channel
}

// Name returns the inbox name.
func (m *Messenger) Name() string {
	return m.name
}

// FromAddress returns the Page name or Instagram handle.
func (m *Messenger) FromAddress() string {
	return m.from
}

// ReplyToAddress is not applicable and always returns empty.
func (m *Messenger) ReplyToAddress() string {
	return ""
}

// FromNameTemplate is not applicable and always returns empty.
func (m *Messenger) FromNameTemplate() string {
	return ""
}

// AccountID returns the Page or Instagram account ID webhooks are addressed to.
func (m *Messenger) AccountID() string {
	return m.config.AccountID
}

// TenantID returns the tenant owning the inbox.
func (m *Messenger) TenantID() int {
	return m.tenantID
}

// Receive is a no-op; inbound messages arrive over the Meta webhook.
func (m *Messenger) Receive(ctx context.Context) error {
	return nil
}

// Close is a no-op; the inbox holds no long-lived connections.
func (m *Messenger) Close() error {
	return nil
}

// Config returns the inbox's resolved configuration.
func (m *Messenger) Config() Config {
	return m.config
}

// ExternalIDPrefix namespaces a channel's contacts in users.external_user_id.
// The rest is the page- or Instagram-scoped ID of the person.
func ExternalIDPrefix(channel string) string {
	return channel + ":"
}

// Send delivers an outgoing message through the Send API.
//
// The recipient's scoped ID travels in the message's `to` recipients, set by
// the conversation manager when the reply is queued.
//
// A reply is only allowed within 24 hours of the person's last message. With
// meta.human_agent on, one that misses the window is retried with the
// HUMAN_AGENT tag, which extends it to 7 days for a person answering by hand.
func (m *Messenger) Send(message models.OutboundMessage) error {
	_, _, err := m.SendTracked(message)
	return err
}

// SendTracked sends a message like Send, and returns the recipient and the
// message IDs the Send API gave each part, which delivery and read receipts
// later name.
func (m *Messenger) SendTracked(message models.OutboundMessage) (string, []string, error) {
	var ids []string
	recipient := ""
	for _, to := range message.To {
		if to = strings.TrimSpace(to); to != "" {
			recipient = to
			break
		}
	}
	if recipient == "" || !isDigits(recipient) {
		return "", nil, fmt.Errorf("invalid %s recipient %q on message %s", m.channel, recipient, message.UUID)
	}
	// Checked before anything is sent, so the text doesn't go out without its file.
	if m.channel == ChannelInstagram {
		for _, att := range message.Attachments {
			if !isPublicURL(att.PublicURL) {
				return recipient, nil, ErrInstagramAttachments
			}
		}
	}

	ctx := context.Background()
	text := strings.TrimSpace(stringutil.HTML2Text(message.Content))

	limit := maxMessengerText
	if m.channel == ChannelInstagram {
		limit = maxInstagramText
	}
	for _, chunk := range meta.SplitText(text, limit) {
		body := map[string]any{
			"recipient": map[string]string{"id": recipient},
			"message":   map[string]string{"text": chunk},
		}
		id, err := m.send(ctx, body)
		if err != nil {
			return recipient, ids, fmt.Errorf("sending %s text: %w", m.channel, err)
		}
		ids = appendID(ids, id)
	}

	for _, att := range message.Attachments {
		var (
			id  string
			err error
		)
		if m.channel == ChannelInstagram {
			// Instagram fetches the file itself from the signed link.
			id, err = m.send(ctx, map[string]any{
				"recipient": map[string]string{"id": recipient},
				"message": map[string]any{"attachment": map[string]any{
					"type":    attachmentKind(att.ContentType),
					"payload": map[string]string{"url": att.PublicURL},
				}},
			})
		} else {
			if len(att.Content) == 0 {
				m.lo.Warn("skipping attachment with no content", "channel", m.channel, "name", att.Name, "message_uuid", message.UUID)
				continue
			}
			id, err = m.sendAttachment(ctx, recipient, att.Name, att.ContentType, att.Content)
		}
		if err != nil {
			return recipient, ids, fmt.Errorf("sending %s attachment %q: %w", m.channel, att.Name, err)
		}
		ids = appendID(ids, id)
	}

	m.lo.Info("message sent", "channel", m.channel, "inbox_id", m.id, "message_uuid", message.UUID, "attachments", len(message.Attachments))
	return recipient, ids, nil
}

// SendTyping shows "typing…" to the person while an agent writes.
func (m *Messenger) SendTyping(recipient string) {
	if !isDigits(recipient) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	body := map[string]any{"recipient": map[string]string{"id": recipient}, "sender_action": "typing_on"}
	if err := m.graph.PostJSON(ctx, m.config.AccessToken, m.config.PageID+"/messages", body, nil); err != nil {
		m.lo.Debug("error sending typing indicator", "channel", m.channel, "inbox_id", m.id, "error", err)
	}
}

// sendResult is the Send API's answer.
type sendResult struct {
	MessageID string `json:"message_id"`
}

func appendID(ids []string, id string) []string {
	if id == "" {
		return ids
	}
	return append(ids, id)
}

// send posts one Send API call as a standard reply, retrying it with the
// HUMAN_AGENT tag if it missed the 24-hour window and the app may use the tag.
func (m *Messenger) send(ctx context.Context, body map[string]any) (string, error) {
	var out sendResult
	body["messaging_type"] = "RESPONSE"
	err := m.graph.PostJSON(ctx, m.config.AccessToken, m.config.PageID+"/messages", body, &out)
	if err == nil || !meta.IsOutsideWindow(err) || !m.graph.App().HumanAgent {
		return out.MessageID, err
	}
	body["messaging_type"] = "MESSAGE_TAG"
	body["tag"] = "HUMAN_AGENT"
	err = m.graph.PostJSON(ctx, m.config.AccessToken, m.config.PageID+"/messages", body, &out)
	return out.MessageID, err
}

// sendAttachment uploads a file with the message that carries it (Messenger only).
func (m *Messenger) sendAttachment(ctx context.Context, recipient, name, contentType string, content []byte) (string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	kind := attachmentKind(contentType)

	fields := map[string]string{
		"recipient":      fmt.Sprintf(`{"id":%q}`, recipient),
		"messaging_type": "RESPONSE",
		"message":        fmt.Sprintf(`{"attachment":{"type":%q,"payload":{"is_reusable":false}}}`, kind),
	}
	file := meta.File{Field: "filedata", Name: name, ContentType: contentType, Content: content}
	var out sendResult
	err := m.graph.PostMultipart(ctx, m.config.AccessToken, m.config.PageID+"/messages", fields, file, &out)
	if err == nil || !meta.IsOutsideWindow(err) || !m.graph.App().HumanAgent {
		return out.MessageID, err
	}
	fields["messaging_type"] = "MESSAGE_TAG"
	fields["tag"] = "HUMAN_AGENT"
	err = m.graph.PostMultipart(ctx, m.config.AccessToken, m.config.PageID+"/messages", fields, file, &out)
	return out.MessageID, err
}

// profileName looks up the name of the person behind a scoped ID, caching the
// result. A failure is not fatal: the contact is stored with a generic name.
func (m *Messenger) profileName(ctx context.Context, senderID string) string {
	if v, ok := m.profiles.Load(senderID); ok {
		return v.(string)
	}

	var (
		name string
		err  error
	)
	if m.channel == ChannelInstagram {
		var p struct {
			Name     string `json:"name"`
			Username string `json:"username"`
		}
		err = m.graph.Get(ctx, m.config.AccessToken, url.PathEscape(senderID), url.Values{"fields": {"name,username"}}, &p)
		name = strings.TrimSpace(p.Name)
		if name == "" && p.Username != "" {
			name = "@" + p.Username
		}
	} else {
		var p struct {
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		}
		err = m.graph.Get(ctx, m.config.AccessToken, url.PathEscape(senderID), url.Values{"fields": {"first_name,last_name"}}, &p)
		name = strings.TrimSpace(p.FirstName + " " + p.LastName)
	}
	if err != nil {
		m.lo.Warn("error fetching contact profile", "channel", m.channel, "inbox_id", m.id, "sender_id", senderID, "error", err)
		return ""
	}

	m.pmu.Lock()
	if m.nProfile < maxProfileCache {
		m.profiles.Store(senderID, name)
		m.nProfile++
	}
	m.pmu.Unlock()
	return name
}

// attachmentKind is the Send API's attachment type for a file.
func attachmentKind(contentType string) string {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.HasPrefix(contentType, "video/"):
		return "video"
	case strings.HasPrefix(contentType, "audio/"):
		return "audio"
	}
	return "file"
}

// isPublicURL reports whether u is an absolute web address Meta can fetch.
func isPublicURL(u string) bool {
	parsed, err := url.Parse(u)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != ""
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
