// Package telegram implements a Telegram inbox on the Telegram Bot API.
//
// A workspace creates a bot with @BotFather and connects it with the bot's
// token. ConectDesk registers a webhook for the inbox, authenticated with a
// secret header, and people who write to the bot become conversations. Bots
// can't message someone who hasn't started them, so conversations are always
// started by the contact.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/zerodha/logf"
)

const (
	ChannelTelegram = "telegram"

	// ExternalIDPrefix namespaces Telegram contacts in users.external_user_id.
	// The rest is the private chat's ID, which is the person's user ID.
	ExternalIDPrefix = "telegram:"

	// DefaultMaxMediaSizeMB caps inbound media; bots can download 20 MB at most.
	DefaultMaxMediaSizeMB = 20

	// maxText and maxCaption are Telegram's limits, in characters.
	maxText    = 4096
	maxCaption = 1024

	apiBase         = "https://api.telegram.org"
	requestTimeout  = 30 * time.Second
	mediaTimeout    = 2 * time.Minute
	maxResponseBody = 4 << 20
)

// Config is the Telegram inbox configuration, stored as the inbox's JSON config.
type Config struct {
	// AccountID is the bot's ID, as a string: a bot can be connected to one
	// workspace only, which the index on account_id enforces.
	AccountID string `json:"account_id"`
	// AccessToken is the bot token from @BotFather, encrypted at rest.
	AccessToken string `json:"access_token"`
	// BotID and Username identify the bot, e.g. 123456 and acme_support_bot.
	BotID    int64  `json:"bot_id"`
	Username string `json:"username"`
	// MaxMediaSizeMB caps the size of an inbound file we will download.
	MaxMediaSizeMB int `json:"max_media_size_mb"`

	// AllowGroups opens a conversation per group chat the bot is in. Off by
	// default: a busy group would flood the helpdesk.
	AllowGroups bool `json:"allow_groups"`

	// Business is the Telegram Business account the bot answers for, set when
	// the business links the bot in Telegram's Business settings.
	Business *BusinessLink `json:"business,omitempty"`
}

// BusinessLink is a Telegram Business account connected to the bot. Replies
// to its customers go out as the business, through the connection.
type BusinessLink struct {
	ConnectionID string `json:"connection_id"`
	UserID       int64  `json:"user_id"`
	Name         string `json:"name"`
	Username     string `json:"username"`
	CanReply     bool   `json:"can_reply"`
}

// Button is a button under a reply: a quick reply the customer taps to answer
// with its label, or a link when URL is set.
type Button struct {
	Text string `json:"text"`
	URL  string `json:"url,omitempty"`
}

// MetaButtonsKey is the outgoing message meta key reply buttons travel under.
const MetaButtonsKey = "telegram_buttons"

// BusinessChatPrefix marks the chat ID of a customer who wrote to the linked
// business account rather than to the bot, so the reply goes through the
// business connection.
const BusinessChatPrefix = "biz:"

// callbackPrefix starts a quick reply's callback data.
const callbackPrefix = "r:"

// Telegram is a Telegram bot inbox.
type Telegram struct {
	id       int
	tenantID int
	name     string
	from     string
	config   Config
	api      *Client
	lo       *logf.Logger
}

// Opts holds the options required for the inbox.
type Opts struct {
	ID       int
	TenantID int
	Name     string
	From     string
	Config   Config
	Lo       *logf.Logger
}

// New returns a new Telegram inbox.
func New(opts Opts) (*Telegram, error) {
	cfg := opts.Config
	if cfg.AccessToken == "" {
		return nil, fmt.Errorf("telegram inbox %d has no bot token", opts.ID)
	}
	if cfg.MaxMediaSizeMB <= 0 {
		cfg.MaxMediaSizeMB = DefaultMaxMediaSizeMB
	}
	return &Telegram{
		id:       opts.ID,
		tenantID: opts.TenantID,
		name:     opts.Name,
		from:     opts.From,
		config:   cfg,
		api:      NewClient(cfg.AccessToken),
		lo:       opts.Lo,
	}, nil
}

func (t *Telegram) Identifier() int               { return t.id }
func (t *Telegram) Channel() string               { return ChannelTelegram }
func (t *Telegram) Name() string                  { return t.name }
func (t *Telegram) FromAddress() string           { return t.from }
func (t *Telegram) ReplyToAddress() string        { return "" }
func (t *Telegram) FromNameTemplate() string      { return "" }
func (t *Telegram) Receive(context.Context) error { return nil }
func (t *Telegram) Close() error                  { return nil }

// Config returns the inbox's resolved configuration.
func (t *Telegram) Config() Config { return t.config }

// API returns the Bot API client for this inbox's bot.
func (t *Telegram) API() *Client { return t.api }

// Send delivers an outgoing message. The recipient's chat ID travels in the
// message's `to`, set by the conversation manager from the contact.
//
// Text goes as plain text: agents write in a rich editor, and Telegram's own
// markup would need every special character escaped.
func (t *Telegram) Send(message models.OutboundMessage) error {
	target := ""
	for _, to := range message.To {
		if to = strings.TrimSpace(to); to != "" {
			target = to
			break
		}
	}
	chatID, business, err := t.resolveChat(target)
	if err != nil {
		return fmt.Errorf("%w on message %s", err, message.UUID)
	}

	ctx := context.Background()
	base := map[string]any{"chat_id": chatID}
	if business != "" {
		base["business_connection_id"] = business
	}

	// Agents write in a rich editor. Telegram's HTML keeps their formatting;
	// a reply too long for one message goes as plain text in parts, since a
	// split could land inside a tag.
	formatted := FormatHTML(message.Content)
	plain := strings.TrimSpace(stringutil.HTML2Text(message.Content))
	markup := replyMarkup(buttonsFromMeta(message.Meta))

	// The first file carries a short reply as its caption, and the buttons.
	caption, captionHTML := "", false
	if len(message.Attachments) > 0 {
		switch {
		case len([]rune(formatted)) <= maxCaption:
			caption, captionHTML = formatted, true
			plain, formatted = "", ""
		case len([]rune(plain)) <= maxCaption:
			caption, plain, formatted = plain, "", ""
		}
	}

	var chunks []map[string]any
	switch {
	case formatted != "" && len([]rune(formatted)) <= maxText:
		chunks = append(chunks, map[string]any{"text": formatted, "parse_mode": "HTML"})
	default:
		for _, c := range meta.SplitText(plain, maxText) {
			chunks = append(chunks, map[string]any{"text": c})
		}
	}
	for i, c := range chunks {
		for k, v := range base {
			c[k] = v
		}
		// Buttons sit under the last part, unless a file follows to carry them.
		if i == len(chunks)-1 && markup != nil && len(message.Attachments) == 0 {
			c["reply_markup"] = markup
		}
		if err := t.api.Call(ctx, "sendMessage", c, nil); err != nil {
			return fmt.Errorf("sending telegram text: %w", err)
		}
	}

	for i, att := range message.Attachments {
		if len(att.Content) == 0 {
			continue
		}
		method, field := uploadMethod(att.ContentType)
		fields := map[string]string{"chat_id": chatID}
		if business != "" {
			fields["business_connection_id"] = business
		}
		if i == 0 && caption != "" {
			fields["caption"] = caption
			if captionHTML {
				fields["parse_mode"] = "HTML"
			}
		}
		if i == len(message.Attachments)-1 && markup != nil {
			b, _ := json.Marshal(markup)
			fields["reply_markup"] = string(b)
		}
		if err := t.api.Upload(ctx, method, fields, field, att.Name, att.ContentType, att.Content, nil); err != nil {
			return fmt.Errorf("sending telegram file %q: %w", att.Name, err)
		}
	}
	t.lo.Info("telegram message sent", "inbox_id", t.id, "message_uuid", message.UUID, "attachments", len(message.Attachments))
	return nil
}

// resolveChat turns a recipient into the chat ID to send to and, for a
// customer of the linked business account, the connection to send through.
func (t *Telegram) resolveChat(target string) (chatID, business string, err error) {
	if id, ok := strings.CutPrefix(target, BusinessChatPrefix); ok {
		if t.config.Business == nil || t.config.Business.ConnectionID == "" {
			return "", "", fmt.Errorf("the telegram business account is no longer connected")
		}
		if !t.config.Business.CanReply {
			return "", "", fmt.Errorf("the telegram business account doesn't let the bot reply")
		}
		target, business = id, t.config.Business.ConnectionID
	}
	if _, err := strconv.ParseInt(target, 10, 64); err != nil {
		return "", "", fmt.Errorf("invalid telegram chat %q", target)
	}
	return target, business, nil
}

// SendTyping shows "typing…" in a chat while an agent writes. Telegram shows
// it for about five seconds, so callers repeat it at most that often.
func (t *Telegram) SendTyping(recipient string) {
	chatID, business, err := t.resolveChat(recipient)
	if err != nil {
		return
	}
	params := map[string]any{"chat_id": chatID, "action": "typing"}
	if business != "" {
		params["business_connection_id"] = business
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := t.api.Call(ctx, "sendChatAction", params, nil); err != nil {
		t.lo.Debug("error sending telegram typing action", "inbox_id", t.id, "error", err)
	}
}

// buttonsFromMeta reads the reply buttons an agent added to a message.
func buttonsFromMeta(raw json.RawMessage) []Button {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil || len(m[MetaButtonsKey]) == 0 {
		return nil
	}
	var buttons []Button
	if json.Unmarshal(m[MetaButtonsKey], &buttons) != nil {
		return nil
	}
	return buttons
}

// replyMarkup lays buttons out one per row. A quick reply's callback data is
// its label, which comes back when the customer taps it.
func replyMarkup(buttons []Button) map[string]any {
	if len(buttons) == 0 {
		return nil
	}
	rows := make([][]map[string]string, 0, len(buttons))
	for _, b := range buttons {
		if b.URL != "" {
			rows = append(rows, []map[string]string{{"text": b.Text, "url": b.URL}})
		} else {
			rows = append(rows, []map[string]string{{"text": b.Text, "callback_data": callbackPrefix + b.Text}})
		}
	}
	return map[string]any{"inline_keyboard": rows}
}

// ValidateButtons checks the buttons an agent adds to a reply: up to 10, a
// label of 1 to 60 bytes (callback data is limited to 64), and web links.
func ValidateButtons(buttons []Button) error {
	if len(buttons) > 10 {
		return fmt.Errorf("a reply can have 10 buttons at most")
	}
	for i, b := range buttons {
		text := strings.TrimSpace(b.Text)
		if text == "" || len(text) > 60 {
			return fmt.Errorf("button %d's label must be 1 to 60 characters", i+1)
		}
		if b.URL != "" && !strings.HasPrefix(b.URL, "https://") && !strings.HasPrefix(b.URL, "http://") {
			return fmt.Errorf("button %d's link must start with https://", i+1)
		}
	}
	return nil
}

// uploadMethod picks the Bot API method a file is sent with. Only the formats
// Telegram plays inline go as photos, videos and audio; the rest as documents.
func uploadMethod(contentType string) (method, field string) {
	switch contentType {
	case "image/jpeg", "image/png", "image/webp":
		return "sendPhoto", "photo"
	case "video/mp4":
		return "sendVideo", "video"
	case "audio/mpeg", "audio/mp4", "audio/m4a":
		return "sendAudio", "audio"
	case "audio/ogg":
		return "sendVoice", "voice"
	}
	return "sendDocument", "document"
}

// Client calls the Bot API for one bot.
type Client struct {
	token string
	http  *http.Client
	base  string
}

// NewClient returns a Bot API client for a bot token.
func NewClient(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: requestTimeout}, base: apiBase}
}

// SetBaseURL points the client at another Bot API server, for tests.
func (c *Client) SetBaseURL(u string) { c.base = strings.TrimRight(u, "/") }

// Error is an error the Bot API returned.
type Error struct {
	Code        int    `json:"error_code"`
	Description string `json:"description"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("telegram: %s (%d)", e.Description, e.Code)
}

// Call calls a Bot API method with a JSON body and decodes its result into out.
func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/bot"+c.token+"/"+method, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

// Upload calls a Bot API method with a file, as multipart.
func (c *Client) Upload(ctx context.Context, method string, fields map[string]string, fileField, name, contentType string, content []byte, out any) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return err
		}
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fileField, strings.ReplaceAll(name, `"`, "")))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	if _, err := part.Write(content); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, mediaTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/bot"+c.token+"/"+method, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		// The URL holds the token; never let it reach a log line.
		return fmt.Errorf("telegram request failed: %s", strings.ReplaceAll(err.Error(), c.token, "<token>"))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return err
	}
	var env struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("telegram: status %d", resp.StatusCode)
	}
	if !env.OK {
		return &env.Error
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

// Bot is the bot a token belongs to.
type Bot struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
	IsBot     bool   `json:"is_bot"`
}

// GetMe checks a token and returns its bot.
func (c *Client) GetMe(ctx context.Context) (Bot, error) {
	var b Bot
	err := c.Call(ctx, "getMe", map[string]any{}, &b)
	return b, err
}

// SetWebhook points the bot's updates at url. Telegram sends secret in the
// X-Telegram-Bot-Api-Secret-Token header of every update, which is how the
// webhook knows a request came from Telegram.
func (c *Client) SetWebhook(ctx context.Context, url, secret string) error {
	return c.Call(ctx, "setWebhook", map[string]any{
		"url":             url,
		"secret_token":    secret,
		"allowed_updates": []string{"message", "callback_query", "business_connection", "business_message"},
		"max_connections": 20,
	}, nil)
}

// AnswerCallback acknowledges a button tap, which stops the button's spinner
// on the customer's side.
func (c *Client) AnswerCallback(ctx context.Context, id string) error {
	return c.Call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id}, nil)
}

// DeleteWebhook stops the bot's updates.
func (c *Client) DeleteWebhook(ctx context.Context) error {
	return c.Call(ctx, "deleteWebhook", map[string]any{}, nil)
}

// WebhookInfo is the state of the bot's webhook, for the settings page.
type WebhookInfo struct {
	URL                string `json:"url"`
	PendingUpdateCount int    `json:"pending_update_count"`
	LastErrorDate      int64  `json:"last_error_date"`
	LastErrorMessage   string `json:"last_error_message"`
}

// GetWebhookInfo returns the bot's webhook state.
func (c *Client) GetWebhookInfo(ctx context.Context) (WebhookInfo, error) {
	var w WebhookInfo
	err := c.Call(ctx, "getWebhookInfo", map[string]any{}, &w)
	return w, err
}

// Download fetches a file the bot received, by its file ID.
func (c *Client) Download(ctx context.Context, fileID string, maxBytes int64) ([]byte, string, error) {
	var f struct {
		FilePath string `json:"file_path"`
		FileSize int64  `json:"file_size"`
	}
	if err := c.Call(ctx, "getFile", map[string]any{"file_id": fileID}, &f); err != nil {
		return nil, "", err
	}
	if f.FilePath == "" {
		return nil, "", fmt.Errorf("telegram gave no path for the file")
	}
	if maxBytes > 0 && f.FileSize > maxBytes {
		return nil, "", fmt.Errorf("file is %d bytes, over the %d byte limit", f.FileSize, maxBytes)
	}

	ctx, cancel := context.WithTimeout(ctx, mediaTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/file/bot"+c.token+"/"+f.FilePath, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("telegram download failed: %s", strings.ReplaceAll(err.Error(), c.token, "<token>"))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("telegram download returned status %d", resp.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if maxBytes > 0 && int64(len(content)) > maxBytes {
		return nil, "", fmt.Errorf("file is over the %d byte limit", maxBytes)
	}
	return content, f.FilePath, nil
}

var _ inbox.Inbox = (*Telegram)(nil)
