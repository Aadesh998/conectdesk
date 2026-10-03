// Package meta holds what the WhatsApp, Messenger and Instagram inboxes share:
// the platform's Meta app, a Graph API client, the OAuth steps that connect a
// workspace's accounts, and the signed webhook every event arrives on.
//
// Every workspace connects through the one Meta app configured for the
// installation ([meta] in config), the way Gmail and Outlook inboxes connect
// through the platform's Google and Microsoft apps. Meta calls that app's single
// webhook URL for every connected account, and the event is routed to the
// workspace by the account it is addressed to.
package meta

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DefaultGraphVersion is the Graph API version used when config sets none.
	// Meta supports each version for about two years; bump it before it expires.
	DefaultGraphVersion = "v24.0"

	graphHost = "https://graph.facebook.com"

	// requestTimeout bounds one Graph API call.
	requestTimeout = 30 * time.Second

	// mediaTimeout is longer because a video can be tens of MB.
	mediaTimeout = 2 * time.Minute

	// maxResponseBytes caps a Graph API JSON response.
	maxResponseBytes = 4 << 20
)

// App is the installation's Meta app, which every workspace connects its
// WhatsApp numbers, Facebook Pages and Instagram accounts through.
type App struct {
	AppID     string
	AppSecret string

	// GraphVersion is the Graph API version, e.g. "v24.0".
	GraphVersion string

	// WebhookVerifyToken is the string entered as the "Verify token" when the
	// webhook is set up in the App Dashboard; Meta echoes it back to prove the
	// subscription request is ours.
	WebhookVerifyToken string

	// WhatsAppConfigID is the Facebook Login for Business configuration that
	// runs WhatsApp Embedded Signup. Without it WhatsApp can't be connected.
	WhatsAppConfigID string

	// LoginConfigID is an optional Facebook Login for Business configuration for
	// connecting Pages and Instagram accounts. Without it the dialog asks for
	// the permissions by scope instead.
	LoginConfigID string

	// HumanAgent retries a Messenger or Instagram reply that missed the 24-hour
	// window with the HUMAN_AGENT tag, which allows replies for 7 days. Only set
	// it once App Review has granted the human_agent permission.
	HumanAgent bool
}

// Configured reports whether the app has the credentials every channel needs.
func (a App) Configured() bool {
	return a.AppID != "" && a.AppSecret != ""
}

// Products lists the channels this app can connect.
func (a App) Products() []string {
	if !a.Configured() {
		return []string{}
	}
	out := []string{ProductMessenger, ProductInstagram}
	if a.WhatsAppConfigID != "" {
		out = append([]string{ProductWhatsApp}, out...)
	}
	return out
}

func (a App) version() string {
	if a.GraphVersion != "" {
		return a.GraphVersion
	}
	return DefaultGraphVersion
}

// Products a workspace can connect. They match the inbox channel names.
const (
	ProductWhatsApp  = "whatsapp"
	ProductMessenger = "messenger"
	ProductInstagram = "instagram"
)

// Error is an error returned by the Graph API.
type Error struct {
	Status    int    `json:"-"`
	Message   string `json:"message"`
	Type      string `json:"type"`
	Code      int    `json:"code"`
	Subcode   int    `json:"error_subcode"`
	UserTitle string `json:"error_user_title"`
	UserMsg   string `json:"error_user_msg"`
	TraceID   string `json:"fbtrace_id"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("graph api: %s (code %d, subcode %d, status %d, trace %s)", e.Message, e.Code, e.Subcode, e.Status, e.TraceID)
}

// IsOutsideWindow reports whether a Messenger or Instagram send was refused
// because the person last wrote more than 24 hours ago.
func IsOutsideWindow(err error) bool {
	var ge *Error
	if !errors.As(err, &ge) {
		return false
	}
	// "This message is sent outside of allowed window": 2018278 on Messenger,
	// 2534022 on Instagram.
	return ge.Code == 10 && (ge.Subcode == 2018278 || ge.Subcode == 2534022)
}

// IsTokenInvalid reports whether the access token has expired or been revoked,
// which only reconnecting the account fixes.
func IsTokenInvalid(err error) bool {
	var ge *Error
	return errors.As(err, &ge) && ge.Code == 190
}

// Client calls the Graph API as the platform's app.
type Client struct {
	app  App
	http *http.Client
	// base is the Graph API root. Tests point it at a fake server.
	base string
}

// NewClient returns a Graph API client for app.
func NewClient(app App) *Client {
	return &Client{
		app:  app,
		http: &http.Client{Timeout: requestTimeout},
		base: graphHost,
	}
}

// App returns the app the client calls as.
func (c *Client) App() App {
	return c.app
}

// SetBaseURL points the client at another Graph API root, for tests.
func (c *Client) SetBaseURL(u string) {
	c.base = strings.TrimRight(u, "/")
}

// AppSecretProof is the HMAC of an access token with the app secret. Sending it
// with every call lets the app require it ("Require App Secret" in the App
// Dashboard), so a leaked token is useless without the secret.
func (c *Client) AppSecretProof(token string) string {
	mac := hmac.New(sha256.New, []byte(c.app.AppSecret))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *Client) endpoint(path string, token string, q url.Values) string {
	if q == nil {
		q = url.Values{}
	}
	if token != "" {
		q.Set("appsecret_proof", c.AppSecretProof(token))
	}
	u := c.base + "/" + c.app.version() + "/" + strings.TrimLeft(path, "/")
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// Get calls GET path with the given token and decodes the JSON response into out.
func (c *Client) Get(ctx context.Context, token, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(path, token, q), nil)
	if err != nil {
		return err
	}
	return c.do(req, token, out)
}

// Delete calls DELETE path.
func (c *Client) Delete(ctx context.Context, token, path string) error {
	return c.DeleteQuery(ctx, token, path, nil)
}

// DeleteQuery calls DELETE path with query parameters.
func (c *Client) DeleteQuery(ctx context.Context, token, path string, q url.Values) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.endpoint(path, token, q), nil)
	if err != nil {
		return err
	}
	return c.do(req, token, nil)
}

// PostJSON calls POST path with a JSON body.
func (c *Client) PostJSON(ctx context.Context, token, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(path, token, nil), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, token, out)
}

// PostForm calls POST path with form values, for the endpoints that take query
// style parameters, such as subscribed_apps.
func (c *Client) PostForm(ctx context.Context, token, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(path, token, nil), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, token, out)
}

// File is a file uploaded in a multipart request.
type File struct {
	Field       string
	Name        string
	ContentType string
	Content     []byte
}

// PostMultipart calls POST path with form fields and one file.
func (c *Client) PostMultipart(ctx context.Context, token, path string, fields map[string]string, file File, out any) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return err
		}
	}

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, file.Field, escapeQuotes(file.Name)))
	h.Set("Content-Type", file.ContentType)
	part, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	if _, err := part.Write(file.Content); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, mediaTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(path, token, nil), &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return c.do(req, token, out)
}

func (c *Client) do(req *http.Request, token string, out any) error {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return err
	}

	if resp.StatusCode >= 300 {
		var env struct {
			Error *Error `json:"error"`
		}
		if json.Unmarshal(body, &env) == nil && env.Error != nil {
			env.Error.Status = resp.StatusCode
			return env.Error
		}
		return &Error{Status: resp.StatusCode, Message: strings.TrimSpace(string(body))}
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decoding graph api response: %w", err)
	}
	return nil
}

// Download fetches a media file from one of Meta's CDNs, refusing anything
// larger than maxBytes. The WhatsApp media URL needs the access token; the
// Messenger and Instagram ones are signed and take no token, so pass "".
func (c *Client) Download(ctx context.Context, rawURL, token string, maxBytes int64) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("parsing media url: %w", err)
	}
	// The URL comes from a signed webhook or the Graph API, but pin it to Meta's
	// hosts anyway so a crafted one can't point the server at internal addresses.
	if u.Scheme != "https" || !IsMetaMediaHost(u.Hostname()) {
		return nil, "", fmt.Errorf("refusing media url on host %q", u.Hostname())
	}

	ctx, cancel := context.WithTimeout(ctx, mediaTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("media download returned status %d", resp.StatusCode)
	}
	if maxBytes > 0 && resp.ContentLength > maxBytes {
		return nil, "", fmt.Errorf("media is %d bytes, over the %d byte limit", resp.ContentLength, maxBytes)
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if maxBytes > 0 && int64(len(content)) > maxBytes {
		return nil, "", fmt.Errorf("media is over the %d byte limit", maxBytes)
	}
	return content, resp.Header.Get("Content-Type"), nil
}

// metaMediaHosts are the domains Meta serves message media from.
var metaMediaHosts = []string{
	"fbcdn.net",
	"fbsbx.com",
	"cdninstagram.com",
	"facebook.com",
	"whatsapp.net",
}

// IsMetaMediaHost reports whether host is one of Meta's media domains or a
// subdomain of one.
func IsMetaMediaHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, h := range metaMediaHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// SplitText breaks a message body into chunks of at most max bytes, preferring
// to break on a newline, then a space, so a split doesn't land mid-sentence.
// Returns nil for empty input.
func SplitText(s string, max int) []string {
	if s == "" {
		return nil
	}
	if len(s) <= max {
		return []string{s}
	}

	var out []string
	for len(s) > max {
		// A break right at the limit still fits, since the break itself is
		// trimmed off.
		window := s[:max+1]
		cut := strings.LastIndex(window, "\n")
		if cut <= 0 {
			cut = strings.LastIndex(window, " ")
		}
		if cut <= 0 {
			// No break to use, so cut at the limit. Back up to a rune boundary
			// first, or a split would land inside a multi-byte character.
			cut = max
			for cut > 0 && !utf8.RuneStart(s[cut]) {
				cut--
			}
			if cut == 0 {
				cut = max
			}
		}
		out = append(out, strings.TrimSpace(s[:cut]))
		s = strings.TrimSpace(s[cut:])
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

// MediaKind gives a human label for a MIME type, used as placeholder text when
// a media download fails.
func MediaKind(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.HasPrefix(mimeType, "audio/"):
		return "voice message"
	case mimeType == "":
		return "attachment"
	default:
		return "document"
	}
}

// SplitName splits a display name into first and last name: everything after
// the first space is the surname. fallback is used for an empty name.
func SplitName(name, fallback string) (string, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return fallback, ""
	}
	first, last, ok := strings.Cut(name, " ")
	if !ok {
		return first, ""
	}
	return first, strings.TrimSpace(last)
}

func escapeQuotes(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// UploadHandle uploads a file with the Resumable Upload API and returns its
// handle, which a message template's media header needs as its sample. The
// upload runs as the app, with the business's token.
func (c *Client) UploadHandle(ctx context.Context, token, name, contentType string, content []byte) (string, error) {
	var session struct {
		ID string `json:"id"`
	}
	q := url.Values{
		"file_name":   {name},
		"file_length": {fmt.Sprint(len(content))},
		"file_type":   {contentType},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(c.app.AppID+"/uploads", token, q), nil)
	if err != nil {
		return "", err
	}
	if err := c.do(req, token, &session); err != nil {
		return "", fmt.Errorf("starting upload: %w", err)
	}
	if session.ID == "" {
		return "", fmt.Errorf("starting upload: no session id")
	}

	ctx, cancel := context.WithTimeout(ctx, mediaTimeout)
	defer cancel()
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/"+c.app.version()+"/"+session.ID, bytes.NewReader(content))
	if err != nil {
		return "", err
	}
	// This endpoint takes the token as "OAuth", not "Bearer".
	req.Header.Set("file_offset", "0")
	req.Header.Set("Authorization", "OAuth "+token)
	var out struct {
		H string `json:"h"`
	}
	if err := c.do(req, "", &out); err != nil {
		return "", fmt.Errorf("uploading file: %w", err)
	}
	if out.H == "" {
		return "", fmt.Errorf("uploading file: no handle")
	}
	return out.H, nil
}
