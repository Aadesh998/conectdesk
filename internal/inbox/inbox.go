// Package inbox provides functionality to manage inboxes in the system.
package inbox

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	imodels "github.com/conectdesk/conectdesk/internal/inbox/models"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

const (
	ChannelEmail     = "email"
	ChannelLiveChat  = "livechat"
	ChannelWhatsApp  = "whatsapp"
	ChannelMessenger = "messenger"
	ChannelInstagram = "instagram"
	ChannelTelegram  = "telegram"
	ChannelVoice     = "voice"
)

// IsMetaChannel reports whether a channel is connected through the platform's
// Meta app: WhatsApp, Messenger or Instagram.
func IsMetaChannel(channel string) bool {
	return channel == ChannelWhatsApp || channel == ChannelMessenger || channel == ChannelInstagram
}

// metaEditableKeys are the only config keys an admin may change on a Meta
// inbox. The rest (account IDs, the access token) come from Meta when the
// account is connected and are kept as stored.
var metaEditableKeys = []string{"max_media_size_mb", "mark_seen", "allow_groups", "ai_assistant_id"}

// metaSecretKeys are the Meta inbox config keys encrypted at rest.
var metaSecretKeys = []string{"access_token", "two_step_pin", "app_secret", "webhook_verify_token"}

var (
	// Embedded filesystem
	//go:embed queries.sql
	efs embed.FS

	// ErrInboxNotFound is returned when an inbox is not found.
	ErrInboxNotFound = errors.New("inbox not found")
)

type initFn func(imodels.Inbox, MessageStore, UserStore) (Inbox, error)

// Closer provides a function for closing an inbox.
type Closer interface {
	Close() error
}

// Identifier provides a method for obtaining a unique identifier for the inbox.
type Identifier interface {
	Identifier() int
}

// MessageHandler defines methods for handling message operations.
type MessageHandler interface {
	Receive(context.Context) error
	Send(models.OutboundMessage) error
}

// Inbox combines the operations of an inbox including its lifecycle, identification, and message handling.
type Inbox interface {
	Closer
	Identifier
	MessageHandler
	Name() string
	FromAddress() string
	FromNameTemplate() string
	ReplyToAddress() string
	Channel() string
}

// MessageStore defines methods for storing and processing messages.
type MessageStore interface {
	MessageExists(int, string) (bool, error)
	EnqueueIncoming(models.IncomingMessage) error
}

// UserStore defines methods for fetching user information.
type UserStore interface {
	GetAgent(tenantID, id int, email string) (umodels.User, error)
	IsEmailBlocked(tenantID int, email string) (bool, error)
}

// Opts contains the options for initializing the inbox manager.
type Opts struct {
	QueueSize   int
	Concurrency int
}

// receiverState tracks a *single*s inbox receiver goroutine.
type receiverState struct {
	cancel context.CancelFunc
	done   chan struct{} // closed when the goroutine exits
}

type Manager struct {
	mu      sync.RWMutex
	queries queries
	inboxes map[int]Inbox
	// tenants maps each registered inbox ID to the tenant owning it.
	tenants       map[int]int
	lo            *logf.Logger
	i18n          *i18ncat.Catalog
	receivers     map[int]receiverState
	msgStore      MessageStore
	usrStore      UserStore
	wg            sync.WaitGroup
	encryptionKey string
}

// Prepared queries.
type queries struct {
	GetInbox       *sqlx.Stmt `query:"get-inbox"`
	GetInboxByUUID *sqlx.Stmt `query:"get-inbox-by-uuid"`
	GetActive      *sqlx.Stmt `query:"get-active-inboxes"`
	GetAll         *sqlx.Stmt `query:"get-all-inboxes"`
	Update         *sqlx.Stmt `query:"update"`
	Toggle         *sqlx.Stmt `query:"toggle"`
	DisableOthers  *sqlx.Stmt `query:"disable-other-channels"`
	SoftDelete     *sqlx.Stmt `query:"soft-delete"`
	InsertInbox    *sqlx.Stmt `query:"insert-inbox"`
	UpdateConfig   *sqlx.Stmt `query:"update-config"`

	GetInboxByAccount     *sqlx.Stmt `query:"get-inbox-by-account"`
	GetInboxesConnectedBy *sqlx.Stmt `query:"get-inboxes-connected-by"`
	DisconnectInbox       *sqlx.Stmt `query:"disconnect-inbox"`
	ReconnectInbox        *sqlx.Stmt `query:"reconnect-inbox"`
}

// New returns a new inbox manager.
func New(lo *logf.Logger, db *sqlx.DB, i18n *i18ncat.Catalog, encryptionKey string) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, db, efs); err != nil {
		return nil, err
	}

	m := &Manager{
		lo:            lo,
		inboxes:       make(map[int]Inbox),
		tenants:       make(map[int]int),
		receivers:     make(map[int]receiverState),
		queries:       q,
		i18n:          i18n,
		encryptionKey: encryptionKey,
	}
	return m, nil
}

// SetMessageStore sets the message store for the manager.
func (m *Manager) SetMessageStore(store MessageStore) {
	m.msgStore = store
}

// SetUserStore sets the user store for the manager.
func (m *Manager) SetUserStore(store UserStore) {
	m.usrStore = store
}

// TypingNotifier is an inbox that can show the customer an agent is typing.
type TypingNotifier interface {
	SendTyping(recipient string)
}

// TrackedSender is an inbox whose sends return the platform's IDs for what
// went out, so the delivery and read receipts it reports later can be shown
// on the message. A reply can go as several platform messages.
type TrackedSender interface {
	SendTracked(models.OutboundMessage) (recipient string, externalIDs []string, err error)
}

// AccountInbox is an inbox tied to one account on an external platform, which
// that platform's webhooks are addressed to.
type AccountInbox interface {
	Inbox
	AccountID() string
	TenantID() int
}

// FindByAccount returns the running inbox of a channel for an account, across
// every tenant, for routing a webhook that names only the account.
func (m *Manager) FindByAccount(channel, accountID string) (AccountInbox, bool) {
	if accountID == "" {
		return nil, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, i := range m.inboxes {
		if i.Channel() != channel {
			continue
		}
		if a, ok := i.(AccountInbox); ok && a.AccountID() == accountID {
			return a, true
		}
	}
	return nil, false
}

// Register registers the inbox with the manager.
func (m *Manager) Register(i Inbox) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inboxes[i.Identifier()] = i
}

// Get retrieves the initialized inbox instance with the specified ID from memory.
func (m *Manager) Get(id int) (Inbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	i, ok := m.inboxes[id]
	if !ok {
		return nil, ErrInboxNotFound
	}
	return i, nil
}

// GetDBRecord returns the inbox record from the DB by numeric ID or UUID.
// If the identifier contains a dash, it's treated as a UUID; otherwise as a numeric ID.
func (m *Manager) GetDBRecord(tenantID int, identifier any) (imodels.Inbox, error) {
	var inbox imodels.Inbox

	// If it's a string with dashes, look up by UUID; otherwise by numeric ID.
	str := fmt.Sprintf("%v", identifier)
	if strings.Contains(str, "-") {
		if err := m.queries.GetInboxByUUID.Get(&inbox, tenantID, str); err != nil {
			if err == sql.ErrNoRows {
				return inbox, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("validation.notFoundInbox"), nil)
			}
			m.lo.Error("error fetching inbox", "error", err)
			return inbox, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
	} else {
		id, err := strconv.Atoi(str)
		if err != nil {
			return inbox, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("validation.notFoundInbox"), nil)
		}
		if err := m.queries.GetInbox.Get(&inbox, tenantID, id); err != nil {
			if err == sql.ErrNoRows {
				return inbox, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("validation.notFoundInbox"), nil)
			}
			m.lo.Error("error fetching inbox", "error", err)
			return inbox, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
	}

	decryptedConfig, err := m.decryptInboxConfig(inbox.Config)
	if err != nil {
		m.lo.Error("error decrypting inbox config", "identifier", identifier, "error", err)
		return imodels.Inbox{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	inbox.Config = decryptedConfig

	m.decryptInboxSecret(&inbox)

	return inbox, nil
}

// GetAllActive returns the enabled inboxes of every tenant, each carrying its
// TenantID. It is for background workers that serve all tenants.
func (m *Manager) GetAllActive() ([]imodels.Inbox, error) {
	return m.getActive()
}

// GetAll returns all inboxes from the DB.
func (m *Manager) GetAll(tenantID int) ([]imodels.Inbox, error) {
	var inboxes = make([]imodels.Inbox, 0)
	if err := m.queries.GetAll.Select(&inboxes, tenantID); err != nil {
		m.lo.Error("error fetching inboxes", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Decrypt sensitive fields in each inbox config
	for i := range inboxes {
		decryptedConfig, err := m.decryptInboxConfig(inboxes[i].Config)
		if err != nil {
			m.lo.Error("error decrypting inbox config", "id", inboxes[i].ID, "error", err)
			return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
		inboxes[i].Config = decryptedConfig

		// Decrypt secret field
		m.decryptInboxSecret(&inboxes[i])
	}

	return inboxes, nil
}

// Create creates an inbox in the DB.
func (m *Manager) Create(tenantID int, inbox imodels.Inbox) (imodels.Inbox, error) {
	// Live chat uses the secret to sign widget identity tokens, and Telegram to
	// authenticate its webhook. Both get one generated if unset.
	if inbox.Channel == ChannelLiveChat || inbox.Channel == ChannelTelegram || inbox.Channel == ChannelVoice {
		secret := inbox.Secret.String
		if secret == "" {
			generated, err := stringutil.RandomAlphanumeric(32)
			if err != nil {
				return imodels.Inbox{}, fmt.Errorf("generating inbox secret: %w", err)
			}
			secret = generated
		}
		encryptedSecret, err := crypto.Encrypt(secret, m.encryptionKey)
		if err != nil {
			return imodels.Inbox{}, fmt.Errorf("encrypting inbox secret: %w", err)
		}
		inbox.Secret = null.StringFrom(encryptedSecret)
	}

	// Encrypt sensitive fields before saving
	encryptedConfig, err := m.encryptInboxConfig(inbox.Config)
	if err != nil {
		m.lo.Error("error encrypting inbox config", "error", err)
		return imodels.Inbox{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	var createdInbox imodels.Inbox
	if err := m.queries.InsertInbox.Get(&createdInbox, tenantID, inbox.Channel, encryptedConfig, inbox.Name, inbox.From, inbox.Enabled, inbox.CSATEnabled, inbox.PromptTagsOnReply, inbox.Secret, inbox.LinkedEmailInboxID, inbox.FromNameTemplate); err != nil {
		m.lo.Error("error creating inbox", "error", err)
		return imodels.Inbox{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Decrypt before returning
	decryptedConfig, err := m.decryptInboxConfig(createdInbox.Config)
	if err != nil {
		m.lo.Error("error decrypting inbox config after creation", "error", err)
	} else {
		createdInbox.Config = decryptedConfig
	}

	// Decrypt secret field
	m.decryptInboxSecret(&createdInbox)

	return createdInbox, nil
}

// InitInboxes initializes and registers active inboxes with the manager.
func (m *Manager) InitInboxes(initFn initFn) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inboxRecords, err := m.getActive()
	if err != nil {
		m.lo.Error("error fetching active inboxes", "error", err)
		return fmt.Errorf("fetching active inboxes: %v", err)
	}

	for _, inboxRecord := range inboxRecords {
		inbox, err := initFn(inboxRecord, m.msgStore, m.usrStore)
		if err != nil {
			m.lo.Error("error initializing inbox",
				"name", inboxRecord.Name,
				"channel", inboxRecord.Channel,
				"error", err)
			continue
		}
		m.inboxes[inbox.Identifier()] = inbox
		m.tenants[inbox.Identifier()] = inboxRecord.TenantID
	}
	return nil
}

// ReloadInbox reloads a single inbox of the tenant by ID. It stops the old
// receiver, fetches the current state from DB, and re-initializes if active.
// An inbox running for another tenant is left alone.
func (m *Manager) ReloadInbox(ctx context.Context, tenantID, id int, initFn initFn) error {
	m.mu.RLock()
	owner, running := m.tenants[id]
	m.mu.RUnlock()
	if running && owner != tenantID {
		return ErrInboxNotFound
	}

	// Stop old receiver and close old inbox.
	m.stopInbox(id)

	// Fetch current inbox state from DB.
	record, err := m.GetDBRecord(tenantID, id)
	if err != nil {
		// Not found (e.g. deleted) - already removed above.
		return nil
	}

	// Only re-init if enabled.
	if !record.Enabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	inbox, err := initFn(record, m.msgStore, m.usrStore)
	if err != nil {
		return fmt.Errorf("initializing inbox %s: %w", record.Name, err)
	}
	m.inboxes[inbox.Identifier()] = inbox
	m.tenants[inbox.Identifier()] = record.TenantID
	m.startReceiver(ctx, inbox)
	return nil
}

// Update updates an inbox in the DB.
func (m *Manager) Update(tenantID, id int, inbox imodels.Inbox) (imodels.Inbox, error) {
	current, err := m.GetDBRecord(tenantID, id)
	if err != nil {
		return imodels.Inbox{}, err
	}

	// Preserve existing passwords if update has empty password
	switch current.Channel {
	case "email":
		var currentCfg struct {
			AuthType             string            `json:"auth_type"`
			OAuth                map[string]string `json:"oauth"`
			IMAP                 []map[string]any  `json:"imap"`
			SMTP                 []map[string]any  `json:"smtp"`
			ReplyTo              string            `json:"reply_to"`
			EnablePlusAddressing bool              `json:"enable_plus_addressing"`
		}
		var updateCfg struct {
			AuthType             string            `json:"auth_type"`
			OAuth                map[string]string `json:"oauth"`
			IMAP                 []map[string]any  `json:"imap"`
			SMTP                 []map[string]any  `json:"smtp"`
			ReplyTo              string            `json:"reply_to"`
			EnablePlusAddressing bool              `json:"enable_plus_addressing"`
		}

		if err := json.Unmarshal(current.Config, &currentCfg); err != nil {
			m.lo.Error("error unmarshalling current config", "id", id, "error", err)
			return imodels.Inbox{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
		if len(inbox.Config) == 0 {
			return imodels.Inbox{}, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).Ts("globals.messages.empty", "name", "{globals.terms.config}"), nil)
		}
		if err := json.Unmarshal(inbox.Config, &updateCfg); err != nil {
			m.lo.Error("error unmarshalling update config", "id", id, "error", err)
			return imodels.Inbox{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}

		if len(updateCfg.IMAP) == 0 {
			return imodels.Inbox{}, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("inbox.emptyIMAP"), nil)
		}

		if len(updateCfg.SMTP) == 0 {
			return imodels.Inbox{}, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("inbox.emptySMTP"), nil)
		}

		// Preserve existing IMAP passwords if update has empty password
		for i := range updateCfg.IMAP {
			if updateCfg.IMAP[i]["password"] == "" && i < len(currentCfg.IMAP) {
				updateCfg.IMAP[i]["password"] = currentCfg.IMAP[i]["password"]
			}
		}

		// Preserve existing SMTP passwords if update has empty password
		for i := range updateCfg.SMTP {
			if updateCfg.SMTP[i]["password"] == "" && i < len(currentCfg.SMTP) {
				updateCfg.SMTP[i]["password"] = currentCfg.SMTP[i]["password"]
			}
		}

		// Preserve existing OAuth fields if update has empty
		if currentCfg.OAuth != nil {
			if updateCfg.OAuth == nil {
				updateCfg.OAuth = make(map[string]string)
			}
			for k, v := range currentCfg.OAuth {
				if updateCfg.OAuth[k] == "" {
					updateCfg.OAuth[k] = v
				}
			}
		}

		updatedConfig, err := json.Marshal(updateCfg)
		if err != nil {
			m.lo.Error("error marshalling updated config", "id", id, "error", err)
			return imodels.Inbox{}, err
		}
		inbox.Config = updatedConfig
	case ChannelLiveChat:
		if inbox.Secret.Valid && strings.Contains(inbox.Secret.String, stringutil.PasswordDummy) {
			inbox.Secret = current.Secret
		}
	case ChannelVoice:
		// The admin edits a voice inbox's settings, all but the API token when
		// it comes back blank or masked.
		inbox.Channel = current.Channel
		inbox.Secret = current.Secret
		updatedConfig, err := preserveSecretKeys(current.Config, inbox.Config, "access_token")
		if err != nil {
			m.lo.Error("error merging inbox config", "id", id, "error", err)
			return imodels.Inbox{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
		inbox.Config = updatedConfig
	case ChannelWhatsApp, ChannelMessenger, ChannelInstagram, ChannelTelegram:
		inbox.Channel = current.Channel
		inbox.From = current.From
		inbox.Secret = current.Secret
		updatedConfig, err := mergeMetaConfig(current.Config, inbox.Config)
		if err != nil {
			m.lo.Error("error merging inbox config", "id", id, "channel", current.Channel, "error", err)
			return imodels.Inbox{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
		inbox.Config = updatedConfig
	}

	// Encrypt sensitive fields before updating
	encryptedConfig, err := m.encryptInboxConfig(inbox.Config)
	if err != nil {
		m.lo.Error("error encrypting inbox config", "error", err)
		return imodels.Inbox{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	if inbox.Secret.Valid && inbox.Secret.String != "" {
		encryptedSecret, err := crypto.Encrypt(inbox.Secret.String, m.encryptionKey)
		if err != nil {
			m.lo.Error("error encrypting inbox secret", "id", id, "error", err)
			return imodels.Inbox{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
		inbox.Secret = null.StringFrom(encryptedSecret)
	}

	// Update the inbox in the DB.
	var updatedInbox imodels.Inbox
	if err := m.queries.Update.Get(&updatedInbox, tenantID, id, inbox.Channel, encryptedConfig, inbox.Name, inbox.From, inbox.CSATEnabled, inbox.PromptTagsOnReply, inbox.Enabled, inbox.Secret, inbox.LinkedEmailInboxID, inbox.FromNameTemplate); err != nil {
		m.lo.Error("error updating inbox", "error", err)
		return imodels.Inbox{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Decrypt before returning
	decryptedConfig, err := m.decryptInboxConfig(updatedInbox.Config)
	if err != nil {
		m.lo.Error("error decrypting inbox config after update", "error", err)
	} else {
		updatedInbox.Config = decryptedConfig
	}

	// Decrypt secret field
	m.decryptInboxSecret(&updatedInbox)

	return updatedInbox, nil
}

// Toggle toggles the status of an inbox in the DB.
func (m *Manager) Toggle(tenantID, id int) (imodels.Inbox, error) {
	var updatedInbox imodels.Inbox
	if err := m.queries.Toggle.Get(&updatedInbox, tenantID, id); err != nil {
		m.lo.Error("error toggling inbox", "error", err)
		return imodels.Inbox{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return updatedInbox, nil
}

// DisableOtherChannels turns off the tenant's enabled inboxes whose channel
// isn't in channels, and returns their IDs.
func (m *Manager) DisableOtherChannels(tenantID int, channels []string) ([]int, error) {
	var ids []int
	if err := m.queries.DisableOthers.Select(&ids, tenantID, pq.StringArray(channels)); err != nil {
		m.lo.Error("error disabling inboxes of other channels", "error", err)
		return nil, err
	}
	return ids, nil
}

// SoftDelete soft deletes an inbox in the DB.
func (m *Manager) SoftDelete(tenantID, id int) error {
	if _, err := m.queries.SoftDelete.Exec(tenantID, id); err != nil {
		m.lo.Error("error deleting inbox", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// AccountOwner returns the tenant and inbox an account of a channel is
// connected to, in any tenant. found is false if it isn't connected anywhere.
func (m *Manager) AccountOwner(channel, accountID string) (tenantID, inboxID int, found bool, err error) {
	var row struct {
		ID       int `db:"id"`
		TenantID int `db:"tenant_id"`
	}
	if err := m.queries.GetInboxByAccount.Get(&row, channel, accountID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, false, nil
		}
		m.lo.Error("error fetching inbox by account", "channel", channel, "error", err)
		return 0, 0, false, err
	}
	return row.TenantID, row.ID, true, nil
}

// ConnectedBy returns the Meta inboxes a Meta user connected, in every tenant,
// with their configs decrypted.
func (m *Manager) ConnectedBy(metaUserID string) ([]imodels.Inbox, error) {
	var inboxes []imodels.Inbox
	if metaUserID == "" {
		return inboxes, nil
	}
	if err := m.queries.GetInboxesConnectedBy.Select(&inboxes, metaUserID); err != nil {
		m.lo.Error("error fetching inboxes by connecting user", "error", err)
		return nil, err
	}
	for i := range inboxes {
		cfg, err := m.decryptInboxConfig(inboxes[i].Config)
		if err != nil {
			return nil, err
		}
		inboxes[i].Config = cfg
	}
	return inboxes, nil
}

// Disconnect disables a Meta inbox and deletes its access token, for when the
// person who connected it removes the app or asks Meta to delete their data.
// Conversations are kept: they belong to the workspace.
func (m *Manager) Disconnect(tenantID, id int) error {
	record, err := m.GetDBRecord(tenantID, id)
	if err != nil {
		return err
	}
	cfg := map[string]any{}
	if len(record.Config) > 0 {
		if err := json.Unmarshal(record.Config, &cfg); err != nil {
			return fmt.Errorf("unmarshalling inbox config: %w", err)
		}
	}
	delete(cfg, "access_token")
	cfg["disconnected"] = true
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	encrypted, err := m.encryptInboxConfig(b)
	if err != nil {
		return err
	}
	if _, err := m.queries.DisconnectInbox.Exec(tenantID, id, encrypted); err != nil {
		m.lo.Error("error disconnecting inbox", "id", id, "error", err)
		return err
	}
	m.stopInbox(id)
	return nil
}

// Reconnect replaces a Meta inbox's config with a freshly connected one and
// enables it, for an account connected again after its token was revoked or
// the person reconnected it. config is plaintext.
func (m *Manager) Reconnect(tenantID, id int, config json.RawMessage) error {
	encrypted, err := m.encryptInboxConfig(config)
	if err != nil {
		return err
	}
	if _, err := m.queries.ReconnectInbox.Exec(tenantID, id, encrypted); err != nil {
		m.lo.Error("error reconnecting inbox", "id", id, "error", err)
		return err
	}
	return nil
}

// Running returns the running inboxes of a channel, across every tenant.
func (m *Manager) Running(channel string) []Inbox {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Inbox
	for _, i := range m.inboxes {
		if i.Channel() == channel {
			out = append(out, i)
		}
	}
	return out
}

// UpdateConfig updates only the config field of an inbox in the DB.
func (m *Manager) UpdateConfig(tenantID, id int, config json.RawMessage) error {
	// Encrypt fields before updating
	encryptedConfig, err := m.encryptInboxConfig(config)
	if err != nil {
		m.lo.Error("error encrypting inbox config", "id", id, "error", err)
		return fmt.Errorf("encrypting inbox config: %w", err)
	}

	if _, err := m.queries.UpdateConfig.Exec(tenantID, id, encryptedConfig); err != nil {
		m.lo.Error("error updating inbox config", "id", id, "error", err)
		return fmt.Errorf("updating inbox config: %w", err)
	}
	return nil
}

// stopInbox cancels the receiver for a single inbox, waits for its goroutine
// to exit, then closes the inbox. Caller must NOT hold m.mu.
func (m *Manager) stopInbox(id int) {
	m.mu.Lock()
	rs, hasReceiver := m.receivers[id]
	if hasReceiver {
		rs.cancel()
		delete(m.receivers, id)
	}
	delete(m.tenants, id)
	m.mu.Unlock()

	// Wait outside lock so the receiver goroutine can finish.
	if hasReceiver {
		<-rs.done
	}

	m.mu.Lock()
	if inb, ok := m.inboxes[id]; ok {
		inb.Close()
		delete(m.inboxes, id)
	}
	m.mu.Unlock()
}

// startReceiver starts a receiver goroutine for the given inbox.
// Caller must hold m.mu.
func (m *Manager) startReceiver(ctx context.Context, inb Inbox) {
	done := make(chan struct{})
	receiverCtx, cancel := context.WithCancel(ctx)
	m.receivers[inb.Identifier()] = receiverState{cancel: cancel, done: done}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer close(done)
		if err := inb.Receive(receiverCtx); err != nil {
			m.lo.Error("error starting inbox receiver", "error", err)
		}
	}()
}

// Start starts the receiver for each inbox.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inb := range m.inboxes {
		m.startReceiver(ctx, inb)
	}
	return nil
}

// Close closes all inboxes.
func (m *Manager) Close() {
	m.mu.Lock()

	// Cancel all receivers.
	for _, rs := range m.receivers {
		rs.cancel()
	}

	// Close all inboxes.
	for _, inb := range m.inboxes {
		inb.Close()
	}
	m.mu.Unlock()

	// Wait for all receiver goroutines to finish.
	m.wg.Wait()
}

// getActive returns all active inboxes from the DB.
func (m *Manager) getActive() ([]imodels.Inbox, error) {
	var inboxes []imodels.Inbox
	if err := m.queries.GetActive.Select(&inboxes); err != nil {
		m.lo.Error("fetching active inboxes", "error", err)
		return nil, err
	}

	// Decrypt sensitive fields in each inbox config
	for i := range inboxes {
		decryptedConfig, err := m.decryptInboxConfig(inboxes[i].Config)
		if err != nil {
			m.lo.Error("error decrypting inbox config", "id", inboxes[i].ID, "error", err)
			return nil, fmt.Errorf("decrypting inbox config for ID %d: %w", inboxes[i].ID, err)
		}
		inboxes[i].Config = decryptedConfig

		// Decrypt secret field
		m.decryptInboxSecret(&inboxes[i])
	}

	return inboxes, nil
}

// encryptInboxConfig encrypts sensitive fields in the inbox config JSON.
func (m *Manager) encryptInboxConfig(config json.RawMessage) (json.RawMessage, error) {
	if len(config) == 0 {
		return config, nil
	}

	var cfg map[string]any
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	// Encrypt SMTP passwords
	if smtpSlice, ok := cfg["smtp"].([]any); ok {
		for i, smtpItem := range smtpSlice {
			if smtpMap, ok := smtpItem.(map[string]any); ok {
				if password, ok := smtpMap["password"].(string); ok && password != "" {
					encrypted, err := crypto.Encrypt(password, m.encryptionKey)
					if err != nil {
						return nil, fmt.Errorf("encrypting SMTP password at index %d: %w", i, err)
					}
					smtpMap["password"] = encrypted
				}
			}
		}
	}

	// Encrypt IMAP passwords
	if imapSlice, ok := cfg["imap"].([]any); ok {
		for i, imapItem := range imapSlice {
			if imapMap, ok := imapItem.(map[string]any); ok {
				if password, ok := imapMap["password"].(string); ok && password != "" {
					encrypted, err := crypto.Encrypt(password, m.encryptionKey)
					if err != nil {
						return nil, fmt.Errorf("encrypting IMAP password at index %d: %w", i, err)
					}
					imapMap["password"] = encrypted
				}
			}
		}
	}

	// Encrypt OAuth fields if present
	if oauthMap, ok := cfg["oauth"].(map[string]any); ok {
		fields := []string{"client_secret", "access_token", "refresh_token"}
		for _, fieldName := range fields {
			if fieldValue, ok := oauthMap[fieldName].(string); ok && fieldValue != "" {
				encrypted, err := crypto.Encrypt(fieldValue, m.encryptionKey)
				if err != nil {
					return nil, fmt.Errorf("encrypting OAuth %s: %w", fieldName, err)
				}
				oauthMap[fieldName] = encrypted
			}
		}
	}

	// Encrypt the access token, and WhatsApp's two-step verification PIN, on
	// WhatsApp, Messenger and Instagram inboxes.
	for _, key := range metaSecretKeys {
		if v, ok := cfg[key].(string); ok && v != "" {
			encrypted, err := crypto.Encrypt(v, m.encryptionKey)
			if err != nil {
				return nil, fmt.Errorf("encrypting %s: %w", key, err)
			}
			cfg[key] = encrypted
		}
	}

	encrypted, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshalling encrypted config: %w", err)
	}

	return encrypted, nil
}

// Decrypt failures clear the field so the app stays usable across encryption_key rotation.
func (m *Manager) decryptInboxConfig(config json.RawMessage) (json.RawMessage, error) {
	if len(config) == 0 {
		return config, nil
	}

	var cfg map[string]any
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	if smtpSlice, ok := cfg["smtp"].([]any); ok {
		for i, smtpItem := range smtpSlice {
			if smtpMap, ok := smtpItem.(map[string]any); ok {
				if password, ok := smtpMap["password"].(string); ok && password != "" {
					decrypted, err := crypto.Decrypt(password, m.encryptionKey)
					if err != nil {
						m.lo.Error("error decrypting SMTP password, clearing field", "index", i, "error", err)
						smtpMap["password"] = ""
						continue
					}
					smtpMap["password"] = decrypted
				}
			}
		}
	}

	if imapSlice, ok := cfg["imap"].([]any); ok {
		for i, imapItem := range imapSlice {
			if imapMap, ok := imapItem.(map[string]any); ok {
				if password, ok := imapMap["password"].(string); ok && password != "" {
					decrypted, err := crypto.Decrypt(password, m.encryptionKey)
					if err != nil {
						m.lo.Error("error decrypting IMAP password, clearing field", "index", i, "error", err)
						imapMap["password"] = ""
						continue
					}
					imapMap["password"] = decrypted
				}
			}
		}
	}

	if oauthMap, ok := cfg["oauth"].(map[string]any); ok {
		fields := []string{"client_secret", "access_token", "refresh_token"}
		for _, fieldName := range fields {
			if fieldValue, ok := oauthMap[fieldName].(string); ok && fieldValue != "" {
				decrypted, err := crypto.Decrypt(fieldValue, m.encryptionKey)
				if err != nil {
					m.lo.Error("error decrypting OAuth field, clearing field", "field", fieldName, "error", err)
					oauthMap[fieldName] = ""
					continue
				}
				oauthMap[fieldName] = decrypted
			}
		}
	}

	for _, key := range metaSecretKeys {
		if v, ok := cfg[key].(string); ok && v != "" {
			plain, err := crypto.Decrypt(v, m.encryptionKey)
			if err != nil {
				m.lo.Error("error decrypting inbox config field, clearing it", "field", key, "error", err)
				cfg[key] = ""
				continue
			}
			cfg[key] = plain
		}
	}

	decrypted, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshalling decrypted config: %w", err)
	}

	return decrypted, nil
}

// decryptInboxSecret decrypts the inbox secret field if present.
func (m *Manager) decryptInboxSecret(inbox *imodels.Inbox) {
	if inbox.Secret.Valid && inbox.Secret.String != "" {
		decrypted, err := crypto.Decrypt(inbox.Secret.String, m.encryptionKey)
		if err != nil {
			m.lo.Error("error decrypting inbox secret", "inbox_id", inbox.ID, "error", err)
			return
		}
		inbox.Secret = null.StringFrom(decrypted)
	}
}

// preserveSecretKeys keeps the stored values of secret config keys an update
// sends blank or still masked. Both configs are plaintext at this point.
func preserveSecretKeys(currentConfig, updateConfig json.RawMessage, keys ...string) (json.RawMessage, error) {
	update := map[string]any{}
	if len(updateConfig) > 0 {
		if err := json.Unmarshal(updateConfig, &update); err != nil {
			return nil, fmt.Errorf("unmarshalling update config: %w", err)
		}
	}
	current := map[string]any{}
	if len(currentConfig) > 0 {
		if err := json.Unmarshal(currentConfig, &current); err != nil {
			return nil, fmt.Errorf("unmarshalling current config: %w", err)
		}
	}
	for _, k := range keys {
		v, _ := update[k].(string)
		if v == "" || strings.Contains(v, stringutil.PasswordDummy) {
			update[k] = current[k]
		}
	}
	return json.Marshal(update)
}

// mergeMetaConfig applies an admin's update to a WhatsApp, Messenger or
// Instagram inbox config. Only the message handling options are taken from the
// update; the account and its token stay as Meta issued them, so the form can't
// point an inbox at another account or wipe its token. Both configs are
// plaintext at this point.
func mergeMetaConfig(currentConfig, updateConfig json.RawMessage) (json.RawMessage, error) {
	current := map[string]any{}
	if len(currentConfig) > 0 {
		if err := json.Unmarshal(currentConfig, &current); err != nil {
			return nil, fmt.Errorf("unmarshalling current config: %w", err)
		}
	}
	if len(updateConfig) > 0 {
		var update map[string]any
		if err := json.Unmarshal(updateConfig, &update); err != nil {
			return nil, fmt.Errorf("unmarshalling update config: %w", err)
		}
		for _, k := range metaEditableKeys {
			if v, ok := update[k]; ok {
				current[k] = v
			}
		}
	}
	merged, err := json.Marshal(current)
	if err != nil {
		return nil, fmt.Errorf("marshalling config: %w", err)
	}
	return merged, nil
}
