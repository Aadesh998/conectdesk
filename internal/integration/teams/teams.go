// Package teams talks to Microsoft Teams for the Teams bot, through the Bot
// Framework: posting conversation updates to a channel as threads, reading
// agents' card submissions and mentions, and checking Microsoft signed them.
package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultLoginBase is where the bot gets its tokens.
	DefaultLoginBase = "https://login.microsoftonline.com"
	// botScope is the audience of the tokens the bot calls the Bot Framework with.
	botScope = "https://api.botframework.com/.default"
	// multiTenant is the token authority of a multi-tenant bot registration.
	multiTenant = "botframework.com"
)

// App is the platform's Teams bot, an Azure Bot registration set in [teams] in config.
type App struct {
	// AppID and AppPassword are the bot's Microsoft app ID and client secret.
	AppID       string
	AppPassword string
	// AppTenantID is the Microsoft 365 tenant of a single-tenant bot; empty for
	// a multi-tenant one.
	AppTenantID string
	// Name is what the bot is called in Teams, which people @mention.
	Name string
}

// Configured reports whether the bot is set up enough to use.
func (a App) Configured() bool {
	return a.AppID != "" && a.AppPassword != ""
}

// Client calls the Bot Framework for the bot.
type Client struct {
	app       App
	http      *http.Client
	loginBase string
	// allowServiceURL reports whether a service URL is Microsoft's; tests
	// point it at a local server.
	allowServiceURL func(string) bool

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// New returns a client for the bot. loginBase is DefaultLoginBase outside tests.
func New(app App, httpClient *http.Client, loginBase string) *Client {
	if loginBase == "" {
		loginBase = DefaultLoginBase
	}
	return &Client{app: app, http: httpClient, loginBase: strings.TrimRight(loginBase, "/"), allowServiceURL: ValidServiceURL}
}

// App returns the bot the client is for.
func (c *Client) App() App { return c.app }

// AllowServiceURLs replaces the check that a service URL is Microsoft's, for tests.
func (c *Client) AllowServiceURLs(fn func(string) bool) { c.allowServiceURL = fn }

// ValidServiceURL reports whether a service URL is one of the Bot Framework's
// addresses for Teams. Requests name the URL to answer on, so answering any
// other would let a forged request point the bot's token somewhere else.
func ValidServiceURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "smba.trafficmanager.net" ||
		strings.HasSuffix(host, ".botframework.com") ||
		strings.HasSuffix(host, ".teams.microsoft.us")
}

// ErrServiceURL is returned for a service URL that isn't the Bot Framework's.
var ErrServiceURL = errors.New("not a bot framework service url")

// Account is a person or the bot in a conversation.
type Account struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	AADObjectID string `json:"aadObjectId,omitempty"`
}

// ConversationRef names a conversation: a channel, or a thread in one.
type ConversationRef struct {
	ID               string `json:"id"`
	TenantID         string `json:"tenantId,omitempty"`
	ConversationType string `json:"conversationType,omitempty"`
}

// ChannelData is what Teams adds to an activity about the team and channel.
type ChannelData struct {
	EventType string `json:"eventType,omitempty"`
	Team      *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"team,omitempty"`
	Channel *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"channel,omitempty"`
	Tenant *struct {
		ID string `json:"id"`
	} `json:"tenant,omitempty"`
}

// Activity is a message or event sent to or by the bot.
type Activity struct {
	Type         string           `json:"type"`
	ID           string           `json:"id,omitempty"`
	ServiceURL   string           `json:"serviceUrl,omitempty"`
	ChannelID    string           `json:"channelId,omitempty"`
	From         Account          `json:"from,omitempty"`
	Recipient    Account          `json:"recipient,omitempty"`
	Conversation ConversationRef  `json:"conversation,omitempty"`
	Text         string           `json:"text,omitempty"`
	TextFormat   string           `json:"textFormat,omitempty"`
	Value        json.RawMessage  `json:"value,omitempty"`
	ReplyToID    string           `json:"replyToId,omitempty"`
	ChannelData  *ChannelData     `json:"channelData,omitempty"`
	MembersAdded []Account        `json:"membersAdded,omitempty"`
	Attachments  []map[string]any `json:"attachments,omitempty"`
}

// Outgoing is a message the bot posts.
type Outgoing struct {
	Type        string           `json:"type"`
	Text        string           `json:"text,omitempty"`
	TextFormat  string           `json:"textFormat,omitempty"`
	Attachments []map[string]any `json:"attachments,omitempty"`
}

// TeamName and ChannelName are the names Teams sent with the activity, if any.
func (a Activity) TeamName() string {
	if a.ChannelData != nil && a.ChannelData.Team != nil {
		return a.ChannelData.Team.Name
	}
	return ""
}

// ChannelRef is the channel the activity happened in, and the Microsoft 365
// tenant it belongs to.
func (a Activity) ChannelRef() (channelID, tenantID string) {
	if a.ChannelData != nil {
		if a.ChannelData.Channel != nil {
			channelID = a.ChannelData.Channel.ID
		}
		if a.ChannelData.Tenant != nil {
			tenantID = a.ChannelData.Tenant.ID
		}
	}
	if channelID == "" {
		channelID = ChannelOf(a.Conversation.ID)
	}
	if tenantID == "" {
		tenantID = a.Conversation.TenantID
	}
	return channelID, tenantID
}

// ChannelName is the channel's name, which Teams leaves out for General.
func (a Activity) ChannelName() string {
	if a.ChannelData != nil && a.ChannelData.Channel != nil {
		return a.ChannelData.Channel.Name
	}
	return ""
}

// ChannelOf is the channel a thread's conversation ID is in: the part before ";messageid=".
func ChannelOf(conversationID string) string {
	if i := strings.Index(conversationID, ";"); i >= 0 {
		return conversationID[:i]
	}
	return conversationID
}

// CreateThread starts a thread in a channel with the activity as its first
// message, and returns the thread's conversation ID, which replies go to.
func (c *Client) CreateThread(ctx context.Context, serviceURL, channelID string, act Outgoing) (string, error) {
	var out struct {
		ID         string `json:"id"`
		ActivityID string `json:"activityId"`
	}
	body := map[string]any{
		"isGroup":     true,
		"channelData": map[string]any{"channel": map[string]any{"id": channelID}},
		"activity":    act,
	}
	if err := c.call(ctx, http.MethodPost, serviceURL, "v3/conversations", body, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("teams: no conversation id for the new thread")
	}
	if !strings.Contains(out.ID, ";messageid=") && out.ActivityID != "" {
		out.ID = channelID + ";messageid=" + out.ActivityID
	}
	return out.ID, nil
}

// Send posts the activity to a conversation, such as a thread.
func (c *Client) Send(ctx context.Context, serviceURL, conversationID string, act Outgoing) error {
	return c.call(ctx, http.MethodPost, serviceURL, "v3/conversations/"+escape(conversationID)+"/activities", act, &struct{}{})
}

// Member is a person in a team, with the email that ties them to an agent.
type Member struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Email             string `json:"email"`
	UserPrincipalName string `json:"userPrincipalName"`
}

// GetMember looks up a person in the channel's team.
func (c *Client) GetMember(ctx context.Context, serviceURL, conversationID, userID string) (Member, error) {
	var m Member
	path := "v3/conversations/" + escape(ChannelOf(conversationID)) + "/members/" + escape(userID)
	err := c.call(ctx, http.MethodGet, serviceURL, path, nil, &m)
	return m, err
}

func (c *Client) call(ctx context.Context, method, serviceURL, path string, body, out any) error {
	if !c.allowServiceURL(serviceURL) {
		return ErrServiceURL
	}
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(serviceURL, "/")+"/"+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("teams %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("teams %s %s answered %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

// accessToken returns the bot's token for the Bot Framework, fetched again
// shortly before it expires.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}
	authority := c.app.AppTenantID
	if authority == "" {
		authority = multiTenant
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.app.AppID},
		"client_secret": {c.app.AppPassword},
		"scope":         {botScope},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.loginBase+"/"+url.PathEscape(authority)+"/oauth2/v2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("teams token: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return "", fmt.Errorf("teams token answered %d: %w", resp.StatusCode, err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("teams token: %s %s", out.Error, out.Description)
	}
	c.token = out.AccessToken
	c.tokenExp = time.Now().Add(time.Duration(out.ExpiresIn)*time.Second - 5*time.Minute)
	return c.token, nil
}

// escape escapes a conversation or user ID for a URL path the way the Bot
// Framework SDKs do, since the IDs hold ':', ';', '@' and '='.
func escape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}
