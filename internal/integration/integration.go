// Package integration connects Slack channels and Microsoft Teams channels to
// a workspace through the platform's Slack app and Teams bot. It posts each
// conversation's updates to a thread of its own, and keeps the thread a
// conversation posts to, so agents' replies there reach the conversation.
package integration

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/conectdesk/conectdesk/internal/chatmsg"
	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/integration/slack"
	"github.com/conectdesk/conectdesk/internal/integration/teams"
	wmodels "github.com/conectdesk/conectdesk/internal/webhook/models"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
)

// Providers.
const (
	ProviderSlack = "slack"
	ProviderTeams = "teams"
)

// DefaultEvents are what a newly connected channel posts: new conversations,
// the messages in them, and who picks them up and closes them.
var DefaultEvents = []string{
	string(wmodels.EventConversationCreated),
	string(wmodels.EventMessageCreated),
	string(wmodels.EventConversationAssigned),
	string(wmodels.EventConversationStatusChanged),
}

// Events a channel can post.
var Events = []string{
	string(wmodels.EventConversationCreated),
	string(wmodels.EventConversationStatusChanged),
	string(wmodels.EventConversationTagsChanged),
	string(wmodels.EventConversationAssigned),
	string(wmodels.EventConversationUnassigned),
	string(wmodels.EventMessageCreated),
}

var (
	// ErrChannelTaken is returned for a channel connected to another workspace.
	ErrChannelTaken = errors.New("channel connected to another workspace")
	// ErrNotFound is returned for a connection that doesn't exist.
	ErrNotFound = errors.New("integration not found")
)

// Integration is a Slack or Teams channel connected to a workspace.
type Integration struct {
	ID               int            `db:"id" json:"id"`
	TenantID         int            `db:"tenant_id" json:"-"`
	CreatedAt        time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time      `db:"updated_at" json:"updated_at"`
	Provider         string         `db:"provider" json:"provider"`
	ExternalTeamID   string         `db:"external_team_id" json:"external_team_id"`
	ExternalTeamName string         `db:"external_team_name" json:"external_team_name"`
	ChannelID        string         `db:"channel_id" json:"channel_id"`
	ChannelName      string         `db:"channel_name" json:"channel_name"`
	ServiceURL       string         `db:"service_url" json:"-"`
	AccessToken      string         `db:"access_token" json:"-"`
	BotUserID        string         `db:"bot_user_id" json:"-"`
	Events           pq.StringArray `db:"events" json:"events"`
	IsActive         bool           `db:"is_active" json:"is_active"`
	ConnectedBy      null.Int       `db:"connected_by" json:"connected_by"`
}

type queries struct {
	GetIntegrations         *sqlx.Stmt `query:"get-integrations"`
	GetIntegration          *sqlx.Stmt `query:"get-integration"`
	GetIntegrationByChannel *sqlx.Stmt `query:"get-integration-by-channel"`
	GetIntegrationsForEvent *sqlx.Stmt `query:"get-integrations-for-event"`
	UpsertIntegration       *sqlx.Stmt `query:"upsert-integration"`
	UpdateIntegration       *sqlx.Stmt `query:"update-integration"`
	UpdateIntegrationToken  *sqlx.Stmt `query:"update-integration-token"`
	DeactivateTeam          *sqlx.Stmt `query:"deactivate-team"`
	DeleteIntegration       *sqlx.Stmt `query:"delete-integration"`
	GetThread               *sqlx.Stmt `query:"get-thread"`
	InsertThread            *sqlx.Stmt `query:"insert-thread"`
	GetThreadConversation   *sqlx.Stmt `query:"get-thread-conversation"`
}

// Opts configures the Manager.
type Opts struct {
	DB            *sqlx.DB
	Lo            *logf.Logger
	I18n          *i18ncat.Catalog
	EncryptionKey string
	// Workers post updates; each conversation's go through one of them, in order.
	Workers   int
	QueueSize int
	// RootURL returns the workspace's address, for links back to conversations.
	RootURL func(tenantID int) string
	Slack   *slack.Client
	Teams   *teams.Client
}

// task is an event to post, already encoded, since its payload may change after.
type task struct {
	tenantID int
	event    wmodels.WebhookEvent
	payload  []byte
}

// Manager stores connections and posts events to them.
type Manager struct {
	q             queries
	lo            *logf.Logger
	i18n          *i18ncat.Catalog
	encryptionKey string
	rootURL       func(int) string
	slack         *slack.Client
	teams         *teams.Client

	queues   []chan task
	wg       sync.WaitGroup
	closedMu sync.RWMutex
	closed   bool

	// notes are internal notes agents wrote in a thread, which are already in
	// the channel and aren't posted there again.
	notesMu sync.Mutex
	notes   map[string]time.Time
}

// New returns a Manager.
func New(o Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, o.DB, efs); err != nil {
		return nil, err
	}
	if o.Workers < 1 {
		o.Workers = 1
	}
	m := &Manager{
		q: q, lo: o.Lo, i18n: o.I18n, encryptionKey: o.EncryptionKey, rootURL: o.RootURL,
		slack: o.Slack, teams: o.Teams, notes: make(map[string]time.Time),
	}
	for range o.Workers {
		m.queues = append(m.queues, make(chan task, max(o.QueueSize/o.Workers, 1)))
	}
	return m, nil
}

// Slack and Teams return the provider clients.
func (m *Manager) Slack() *slack.Client { return m.slack }
func (m *Manager) Teams() *teams.Client { return m.teams }

// Translator is the workspace's language pack.
func (m *Manager) Translator(tenantID int) chatmsg.Translator {
	return m.i18n.For(tenantID)
}

// RootURL is the workspace's address.
func (m *Manager) RootURL(tenantID int) string {
	if m.rootURL == nil {
		return ""
	}
	return m.rootURL(tenantID)
}

// GetAll returns the workspace's connections.
func (m *Manager) GetAll(tenantID int) ([]Integration, error) {
	out := []Integration{}
	if err := m.q.GetIntegrations.Select(&out, tenantID); err != nil {
		return nil, err
	}
	return out, nil
}

// Get returns one of the workspace's connections, its token decrypted.
func (m *Manager) Get(tenantID, id int) (Integration, error) {
	var in Integration
	if err := m.q.GetIntegration.Get(&in, tenantID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return in, ErrNotFound
		}
		return in, err
	}
	return m.decrypt(in)
}

// FindByChannel returns the connection for a channel, whatever workspace it
// belongs to, as requests from Slack and Teams name the channel.
func (m *Manager) FindByChannel(provider, teamID, channelID string) (Integration, error) {
	var in Integration
	if err := m.q.GetIntegrationByChannel.Get(&in, provider, teamID, channelID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return in, ErrNotFound
		}
		return in, err
	}
	return m.decrypt(in)
}

// Connect is a channel being connected.
type Connect struct {
	Provider    string
	TeamID      string
	TeamName    string
	ChannelID   string
	ChannelName string
	ServiceURL  string
	AccessToken string
	BotUserID   string
	ConnectedBy int
}

// Connect connects a channel to the workspace with the default events, or
// refreshes it if it is already connected. A channel connected to another
// workspace returns ErrChannelTaken.
func (m *Manager) Connect(tenantID int, c Connect) (Integration, error) {
	token := c.AccessToken
	if token != "" {
		enc, err := crypto.Encrypt(token, m.encryptionKey)
		if err != nil {
			return Integration{}, err
		}
		token = enc
	}
	connectedBy := null.NewInt(c.ConnectedBy, c.ConnectedBy > 0)
	var in Integration
	err := m.q.UpsertIntegration.Get(&in, tenantID, c.Provider, c.TeamID, chatmsg.Truncate(c.TeamName, 200), c.ChannelID,
		chatmsg.Truncate(c.ChannelName, 200), c.ServiceURL, token, c.BotUserID, pq.Array(DefaultEvents), connectedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return in, ErrChannelTaken
	}
	if err != nil {
		return in, err
	}
	// A new Slack install replaces the token for the workspace's other channels too.
	if c.Provider == ProviderSlack && token != "" {
		if _, err := m.q.UpdateIntegrationToken.Exec(c.TeamID, tenantID, token, c.BotUserID); err != nil {
			m.lo.Error("error refreshing slack token for other channels", "team_id", c.TeamID, "error", err)
		}
	}
	return m.decrypt(in)
}

// Update changes what a connection posts and whether it is on.
func (m *Manager) Update(tenantID, id int, events []string, active bool) (Integration, error) {
	for _, e := range events {
		if !validEvent(e) {
			return Integration{}, fmt.Errorf("unknown event %q", e)
		}
	}
	var in Integration
	if err := m.q.UpdateIntegration.Get(&in, tenantID, id, pq.Array(events), active); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return in, ErrNotFound
		}
		return in, err
	}
	return in, nil
}

// Delete disconnects a channel.
func (m *Manager) Delete(tenantID, id int) error {
	_, err := m.q.DeleteIntegration.Exec(tenantID, id)
	return err
}

// DeactivateTeam turns off every channel of a Slack workspace that removed the
// app, or whose token stopped working.
func (m *Manager) DeactivateTeam(provider, teamID string) {
	if _, err := m.q.DeactivateTeam.Exec(provider, teamID); err != nil {
		m.lo.Error("error deactivating integrations", "provider", provider, "team_id", teamID, "error", err)
	}
}

// ThreadConversation returns the conversation a thread of the connection
// belongs to, 0 if none.
func (m *Manager) ThreadConversation(in Integration, threadRef string) (int, error) {
	var id int
	err := m.q.GetThreadConversation.Get(&id, in.TenantID, in.ID, threadRef)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// Thread returns the thread a conversation posts to in the connection's
// channel, "" if none yet.
func (m *Manager) Thread(in Integration, conversationID int) (string, error) {
	var ref string
	err := m.q.GetThread.Get(&ref, in.TenantID, in.ID, conversationID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return ref, err
}

// ExpectNote marks the next internal note by the agent in the conversation as
// written in the channel, so it isn't posted back to the thread it came from.
func (m *Manager) ExpectNote(integrationID int, conversationUUID string, senderID int) {
	m.notesMu.Lock()
	defer m.notesMu.Unlock()
	now := time.Now()
	for k, exp := range m.notes {
		if now.After(exp) {
			delete(m.notes, k)
		}
	}
	m.notes[noteKey(integrationID, conversationUUID, senderID)] = now.Add(time.Minute)
}

// ForgetNote undoes ExpectNote when the note couldn't be added.
func (m *Manager) ForgetNote(integrationID int, conversationUUID string, senderID int) {
	m.notesMu.Lock()
	delete(m.notes, noteKey(integrationID, conversationUUID, senderID))
	m.notesMu.Unlock()
}

func (m *Manager) takeNote(integrationID int, conversationUUID string, senderID int) bool {
	m.notesMu.Lock()
	defer m.notesMu.Unlock()
	k := noteKey(integrationID, conversationUUID, senderID)
	exp, ok := m.notes[k]
	delete(m.notes, k)
	return ok && time.Now().Before(exp)
}

func noteKey(integrationID int, conversationUUID string, senderID int) string {
	return fmt.Sprintf("%d:%s:%d", integrationID, conversationUUID, senderID)
}

// HandleEvent queues a conversation event for the workspace's connected
// channels. It never blocks: when the queue is full the event is dropped.
func (m *Manager) HandleEvent(tenantID int, event wmodels.WebhookEvent, data any) {
	if event == wmodels.EventWebhookTest || !validEvent(string(event)) {
		return
	}
	payload, err := json.Marshal(data)
	if err != nil {
		m.lo.Error("error encoding event for chat integrations", "event", event, "error", err)
		return
	}

	m.closedMu.RLock()
	defer m.closedMu.RUnlock()
	if m.closed {
		return
	}
	// One conversation's events go to one worker, so they post in order and its
	// thread is started once.
	q := m.queues[conversationID(payload)%len(m.queues)]
	select {
	case q <- task{tenantID: tenantID, event: event, payload: payload}:
	default:
		m.lo.Warn("chat integration queue is full, dropping event", "tenant_id", tenantID, "event", event)
	}
}

// Run starts the workers.
func (m *Manager) Run(ctx context.Context) {
	for _, q := range m.queues {
		m.wg.Add(1)
		go func(q chan task) {
			defer m.wg.Done()
			for t := range q {
				m.process(ctx, t)
			}
		}(q)
	}
}

// Close stops taking events and waits for the queued ones to post.
func (m *Manager) Close() {
	m.closedMu.Lock()
	if m.closed {
		m.closedMu.Unlock()
		return
	}
	m.closed = true
	for _, q := range m.queues {
		close(q)
	}
	m.closedMu.Unlock()
	m.wg.Wait()
}

// process posts an event to each of the workspace's channels subscribed to it.
func (m *Manager) process(ctx context.Context, t task) {
	if chatmsg.Skips(t.event, t.payload) {
		return
	}
	var ins []Integration
	if err := m.q.GetIntegrationsForEvent.Select(&ins, t.tenantID, string(t.event)); err != nil {
		m.lo.Error("error fetching chat integrations for event", "tenant_id", t.tenantID, "event", t.event, "error", err)
		return
	}
	if len(ins) == 0 {
		return
	}

	var msg chatmsg.MessageInfo
	if t.event == wmodels.EventMessageCreated {
		msg, _ = chatmsg.ParseMessage(t.payload)
	}
	post, err := chatmsg.Build(m.Translator(t.tenantID), t.event, t.payload, m.RootURL(t.tenantID))
	if err != nil {
		m.lo.Error("error building chat post", "event", t.event, "error", err)
		return
	}
	if post.ConversationID == 0 {
		return
	}

	for _, in := range ins {
		if msg.Private && m.takeNote(in.ID, post.ConversationUUID, msg.SenderID) {
			continue
		}
		in, err := m.decrypt(in)
		if err != nil {
			continue
		}
		c, cancel := context.WithTimeout(ctx, 20*time.Second)
		if err := m.Post(c, in, post); err != nil {
			m.lo.Error("error posting to chat integration", "integration_id", in.ID, "provider", in.Provider, "event", t.event, "error", err)
		}
		cancel()
	}
}

// Post posts to a channel: the conversation's first post starts its thread,
// with the actions agents take on it, and later ones reply in the thread.
func (m *Manager) Post(ctx context.Context, in Integration, post chatmsg.Post) error {
	ref, err := m.Thread(in, post.ConversationID)
	if err != nil {
		return err
	}
	t := m.Translator(in.TenantID)

	switch in.Provider {
	case ProviderSlack:
		if m.slack == nil {
			return errors.New("slack app not configured")
		}
		msg := slack.Message{Channel: in.ChannelID, Text: post.Title}
		if ref == "" {
			msg.Blocks = slack.RootBlocks(t, post)
		} else {
			msg.Blocks = chatmsg.SlackBlocks(post)
			msg.ThreadTS = ref
		}
		ts, err := m.slack.PostMessage(ctx, in.AccessToken, msg)
		if err != nil {
			if slack.IsCode(err, "token_revoked") || slack.IsCode(err, "invalid_auth") || slack.IsCode(err, "account_inactive") {
				m.lo.Warn("slack app removed from workspace, turning its channels off", "team_id", in.ExternalTeamID)
				m.DeactivateTeam(ProviderSlack, in.ExternalTeamID)
			}
			return err
		}
		if ref == "" {
			return m.saveThread(in, post.ConversationID, ts)
		}
		return nil

	case ProviderTeams:
		if m.teams == nil {
			return errors.New("teams bot not configured")
		}
		if ref == "" {
			id, err := m.teams.CreateThread(ctx, in.ServiceURL, in.ChannelID, teams.CardMessage(teams.RootCard(t, post)))
			if err != nil {
				return err
			}
			return m.saveThread(in, post.ConversationID, id)
		}
		return m.teams.Send(ctx, in.ServiceURL, ref, teams.CardMessage(teams.UpdateCard(post)))
	}
	return fmt.Errorf("unknown provider %q", in.Provider)
}

func (m *Manager) saveThread(in Integration, conversationID int, ref string) error {
	_, err := m.q.InsertThread.Exec(in.TenantID, in.ID, conversationID, ref)
	return err
}

func (m *Manager) decrypt(in Integration) (Integration, error) {
	if in.AccessToken == "" {
		return in, nil
	}
	tok, err := crypto.Decrypt(in.AccessToken, m.encryptionKey)
	if err != nil {
		m.lo.Error("error decrypting integration token", "integration_id", in.ID, "error", err)
		return in, err
	}
	in.AccessToken = tok
	return in, nil
}

func validEvent(e string) bool {
	for _, v := range Events {
		if v == e {
			return true
		}
	}
	return false
}

// conversationID reads the conversation an event is about, for picking its worker.
func conversationID(payload []byte) int {
	var p struct {
		ID             int `json:"id"`
		ConversationID int `json:"conversation_id"`
		Conversation   *struct {
			ID int `json:"id"`
		} `json:"conversation"`
	}
	json.Unmarshal(payload, &p)
	switch {
	case p.Conversation != nil:
		return max(p.Conversation.ID, 0)
	case p.ConversationID > 0:
		return p.ConversationID
	}
	return max(p.ID, 0)
}
