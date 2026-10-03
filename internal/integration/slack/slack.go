// Package slack talks to Slack for the Slack app: installing it into a
// workspace, posting conversation updates to a channel, and reading the
// button clicks, modals and thread replies agents send back.
package slack

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
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultAPIBase is where Slack's Web API is served.
	DefaultAPIBase = "https://slack.com/api"
	// authorizeURL is Slack's install page.
	authorizeURL = "https://slack.com/oauth/v2/authorize"

	// maxSignatureAge is how old a signed request may be, so a captured one
	// can't be replayed later.
	maxSignatureAge = 5 * time.Minute
)

// Scopes the bot is installed with: pick a channel on install, post and react
// there, join public channels, read thread replies, and look up agents' emails.
var Scopes = []string{
	"incoming-webhook",
	"chat:write",
	"channels:join",
	"channels:history",
	"groups:history",
	"reactions:write",
	"users:read",
	"users:read.email",
}

// App is the platform's Slack app, set in [slack] in config.
type App struct {
	ClientID      string
	ClientSecret  string
	SigningSecret string
}

// Configured reports whether the app is set up enough to install.
func (a App) Configured() bool {
	return a.ClientID != "" && a.ClientSecret != "" && a.SigningSecret != ""
}

// Client calls Slack's Web API for the app.
type Client struct {
	app     App
	http    *http.Client
	apiBase string
}

// New returns a client for the app. apiBase is DefaultAPIBase outside tests.
func New(app App, httpClient *http.Client, apiBase string) *Client {
	if apiBase == "" {
		apiBase = DefaultAPIBase
	}
	return &Client{app: app, http: httpClient, apiBase: strings.TrimRight(apiBase, "/")}
}

// App returns the app the client is for.
func (c *Client) App() App { return c.app }

// Error is an error Slack answered with, such as "channel_not_found".
type Error struct {
	Method string
	Code   string
}

func (e *Error) Error() string { return "slack " + e.Method + ": " + e.Code }

// IsCode reports whether err is Slack answering with the given error code.
func IsCode(err error, code string) bool {
	var se *Error
	return errors.As(err, &se) && se.Code == code
}

// AuthorizeURL is the Slack page that installs the app, where the admin picks
// the channel to post to.
func (c *Client) AuthorizeURL(redirectURI, state string) string {
	q := url.Values{}
	q.Set("client_id", c.app.ClientID)
	q.Set("scope", strings.Join(Scopes, ","))
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	return authorizeURL + "?" + q.Encode()
}

// Install is what Slack returns once the app is installed.
type Install struct {
	AccessToken string `json:"access_token"`
	BotUserID   string `json:"bot_user_id"`
	Team        struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"team"`
	IncomingWebhook struct {
		Channel   string `json:"channel"`
		ChannelID string `json:"channel_id"`
	} `json:"incoming_webhook"`
}

// ExchangeCode trades the install's code for the bot token and the channel picked.
func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI string) (Install, error) {
	var out struct {
		response
		Install
	}
	form := url.Values{"client_id": {c.app.ClientID}, "client_secret": {c.app.ClientSecret}, "code": {code}, "redirect_uri": {redirectURI}}
	if err := c.callForm(ctx, "oauth.v2.access", "", form, &out); err != nil {
		return Install{}, err
	}
	if out.AccessToken == "" || out.Team.ID == "" || out.IncomingWebhook.ChannelID == "" {
		return Install{}, errors.New("slack oauth.v2.access: no token, team or channel in the answer")
	}
	return out.Install, nil
}

// JoinChannel adds the bot to a public channel, so it can post there and see
// thread replies. A private channel answers method_not_supported_for_channel_type;
// someone in it has to invite the bot.
func (c *Client) JoinChannel(ctx context.Context, token, channel string) error {
	return c.callJSON(ctx, "conversations.join", token, map[string]any{"channel": channel}, &response{})
}

// Message is a message the bot posts.
type Message struct {
	Channel string `json:"channel"`
	// Text is the fallback shown in notifications.
	Text   string           `json:"text"`
	Blocks []map[string]any `json:"blocks,omitempty"`
	// ThreadTS posts the message in the thread of that message.
	ThreadTS    string `json:"thread_ts,omitempty"`
	UnfurlLinks bool   `json:"unfurl_links"`
	UnfurlMedia bool   `json:"unfurl_media"`
}

// PostMessage posts a message and returns its ts, which identifies it.
func (c *Client) PostMessage(ctx context.Context, token string, msg Message) (string, error) {
	var out struct {
		response
		TS string `json:"ts"`
	}
	if err := c.callJSON(ctx, "chat.postMessage", token, msg, &out); err != nil {
		return "", err
	}
	return out.TS, nil
}

// PostEphemeral shows a message in the channel only to one person.
func (c *Client) PostEphemeral(ctx context.Context, token, channel, user, threadTS, text string) error {
	body := map[string]any{"channel": channel, "user": user, "text": text}
	if threadTS != "" {
		body["thread_ts"] = threadTS
	}
	return c.callJSON(ctx, "chat.postEphemeral", token, body, &response{})
}

// AddReaction reacts to a message with an emoji, such as white_check_mark.
func (c *Client) AddReaction(ctx context.Context, token, channel, ts, name string) error {
	err := c.callJSON(ctx, "reactions.add", token, map[string]any{"channel": channel, "timestamp": ts, "name": name}, &response{})
	if IsCode(err, "already_reacted") {
		return nil
	}
	return err
}

// OpenView opens a modal for the person who clicked, within three seconds of the click.
func (c *Client) OpenView(ctx context.Context, token, triggerID string, view map[string]any) error {
	return c.callJSON(ctx, "views.open", token, map[string]any{"trigger_id": triggerID, "view": view}, &response{})
}

// User is the part of a Slack user the app needs.
type User struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
	IsBot   bool   `json:"is_bot"`
	Profile struct {
		Email    string `json:"email"`
		RealName string `json:"real_name"`
	} `json:"profile"`
}

// UserInfo looks up a user, whose email ties them to an agent.
func (c *Client) UserInfo(ctx context.Context, token, userID string) (User, error) {
	var out struct {
		response
		User User `json:"user"`
	}
	if err := c.callForm(ctx, "users.info", token, url.Values{"user": {userID}}, &out); err != nil {
		return User{}, err
	}
	return out.User, nil
}

// Respond posts to an interaction's response_url, which answers the person who
// clicked without needing the channel.
func (c *Client) Respond(ctx context.Context, responseURL string, body map[string]any) error {
	u, err := url.Parse(responseURL)
	if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), "slack.com") {
		return fmt.Errorf("not a slack response url: %q", responseURL)
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, responseURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack response url answered %d", resp.StatusCode)
	}
	return nil
}

// response is the envelope every Web API answer shares.
type response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func (r *response) err(method string) error {
	if r.OK {
		return nil
	}
	code := r.Error
	if code == "" {
		code = "unknown_error"
	}
	return &Error{Method: method, Code: code}
}

type okChecker interface{ err(method string) error }

func (c *Client) callJSON(ctx context.Context, method, token string, body any, out okChecker) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/"+method, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	return c.do(req, method, token, out)
}

func (c *Client) callForm(ctx context.Context, method, token string, form url.Values, out okChecker) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/"+method, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, method, token, out)
}

func (c *Client) do(req *http.Request, method, token string, out okChecker) error {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("slack %s: %w", method, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("slack %s: %w", method, err)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return &Error{Method: method, Code: "ratelimited"}
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("slack %s answered %d: %w", method, resp.StatusCode, err)
	}
	return out.err(method)
}

// VerifySignature checks that a request came from Slack: its body is signed
// with the app's signing secret, and it isn't older than a few minutes.
func VerifySignature(signingSecret string, body []byte, timestamp, signature string, now time.Time) bool {
	if signingSecret == "" || timestamp == "" || !strings.HasPrefix(signature, "v0=") {
		return false
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if age := now.Sub(time.Unix(ts, 0)); age > maxSignatureAge || age < -maxSignatureAge {
		return false
	}
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte("v0:" + timestamp + ":"))
	mac.Write(body)
	want := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(signature))
}
