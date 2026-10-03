// Package webhook handles the management of webhooks and webhook deliveries.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/conectdesk/conectdesk/internal/chatmsg"
	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/fairshare"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/ssrf"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/conectdesk/conectdesk/internal/version"
	"github.com/conectdesk/conectdesk/internal/webhook/models"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
)

// Manager handles webhook-related operations.
type Manager struct {
	q             queries
	lo            *logf.Logger
	i18n          *i18ncat.Catalog
	db            *sqlx.DB
	deliveryQueue chan DeliveryTask
	httpClient    *http.Client
	workers       int
	closed        bool
	closedMu      sync.RWMutex
	wg            sync.WaitGroup
	encryptionKey string
	share         *fairshare.Limiter
	// rootURL returns the address a tenant is served on, for links in chat posts.
	rootURL func(tenantID int) string
	// sink also receives every event, for the Slack and Teams apps; nil for none.
	sink EventSink
}

// EventSink receives conversation events besides the webhooks. HandleEvent
// must not block.
type EventSink interface {
	HandleEvent(tenantID int, event models.WebhookEvent, data any)
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB            *sqlx.DB
	Lo            *logf.Logger
	I18n          *i18ncat.Catalog
	Workers       int
	QueueSize     int
	Timeout       time.Duration
	EncryptionKey string
	DialControl   ssrf.Control
	// MaxQueuedPerTenant caps one tenant's deliveries waiting in the queue,
	// so a tenant flooding it has its own deliveries dropped; 0 means no cap.
	MaxQueuedPerTenant int
	// RootURL returns the address a tenant is served on, for links in Slack and Teams posts.
	RootURL func(tenantID int) string
}

// DeliveryTask represents a webhook delivery task
type DeliveryTask struct {
	// TenantID is the tenant the event happened in; only its webhooks receive it.
	TenantID int
	Event    models.WebhookEvent
	Payload  any
	// Zero means fan out to all subscribers of Event.
	WebhookID int
}

// queries contains prepared SQL queries.
type queries struct {
	GetWebhooksCompact *sqlx.Stmt `query:"get-webhooks-compact"`
	GetAllWebhooks     *sqlx.Stmt `query:"get-all-webhooks"`
	GetWebhook         *sqlx.Stmt `query:"get-webhook"`
	GetWebhookSecret   *sqlx.Stmt `query:"get-webhook-secret"`
	GetActiveWebhooks  *sqlx.Stmt `query:"get-active-webhooks"`
	GetWebhooksByEvent *sqlx.Stmt `query:"get-webhooks-by-event"`
	InsertWebhook      *sqlx.Stmt `query:"insert-webhook"`
	UpdateWebhook      *sqlx.Stmt `query:"update-webhook"`
	DeleteWebhook      *sqlx.Stmt `query:"delete-webhook"`
	ToggleWebhook      *sqlx.Stmt `query:"toggle-webhook"`
}

// New creates and returns a new instance of the Manager.
func New(opts Opts) (*Manager, error) {
	var q queries

	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}

	transport := ssrf.NewTransport(opts.DialControl, 3*time.Second)
	transport.TLSHandshakeTimeout = 3 * time.Second
	transport.ResponseHeaderTimeout = 3 * time.Second

	return &Manager{
		q:             q,
		lo:            opts.Lo,
		i18n:          opts.I18n,
		db:            opts.DB,
		deliveryQueue: make(chan DeliveryTask, opts.QueueSize),
		httpClient: &http.Client{
			Timeout:   opts.Timeout,
			Transport: transport,
		},
		workers:       opts.Workers,
		encryptionKey: opts.EncryptionKey,
		share:         fairshare.New(opts.MaxQueuedPerTenant),
		rootURL:       opts.RootURL,
	}, nil
}

// GetAllCompact retrieves all webhooks with only id and name.
func (m *Manager) GetAllCompact(tenantID int) ([]models.WebhookCompact, error) {
	var webhooks = make([]models.WebhookCompact, 0)
	if err := m.q.GetWebhooksCompact.Select(&webhooks, tenantID); err != nil {
		m.lo.Error("error fetching webhooks", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return webhooks, nil
}

// GetAll retrieves all webhooks.
func (m *Manager) GetAll(tenantID int) ([]models.Webhook, error) {
	var webhooks = make([]models.Webhook, 0)
	if err := m.q.GetAllWebhooks.Select(&webhooks, tenantID); err != nil {
		m.lo.Error("error fetching webhooks", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Decrypt secrets
	m.decryptWebhooks(webhooks)

	return webhooks, nil
}

// Get retrieves a webhook by ID.
func (m *Manager) Get(tenantID, id int) (models.Webhook, error) {
	var webhook models.Webhook
	if err := m.q.GetWebhook.Get(&webhook, tenantID, id); err != nil {
		if err == sql.ErrNoRows {
			return webhook, envelope.NewError(envelope.NotFoundError, m.i18n.For(tenantID).T("globals.messages.notFound"), nil)
		}
		m.lo.Error("error fetching webhook", "error", err)
		return webhook, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Decrypt secret
	if err := m.decryptWebhook(&webhook); err != nil {
		return webhook, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	return webhook, nil
}

// Create creates a new webhook.
func (m *Manager) Create(tenantID int, webhook models.Webhook) (models.Webhook, error) {
	var result models.Webhook

	// Encrypt secret before storing
	encryptedSecret, err := m.encryptSecret(webhook.Secret)
	if err != nil {
		return models.Webhook{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	storedURL, err := m.storedURL(webhook)
	if err != nil {
		return models.Webhook{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	if err := m.q.InsertWebhook.Get(&result, tenantID, webhook.Name, storedURL, pq.Array(webhook.Events), encryptedSecret, webhook.IsActive, kindOrCustom(webhook.Kind)); err != nil {
		if dbutil.IsUniqueViolationError(err) {
			return models.Webhook{}, envelope.NewError(envelope.ConflictError, m.i18n.For(tenantID).T("globals.messages.errorAlreadyExists"), nil)
		}
		m.lo.Error("error inserting webhook", "error", err)
		return models.Webhook{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Decrypt secret before returning (ignore errors as non-critical)
	if err := m.decryptWebhook(&result); err != nil {
		m.lo.Error("error decrypting webhook secret after creation", "webhook_id", result.ID, "error", err)
	}

	return result, nil
}

// Update updates a webhook by ID.
func (m *Manager) Update(tenantID, id int, webhook models.Webhook) (models.Webhook, error) {
	var result models.Webhook

	// Preserve the existing encrypted secret.
	encryptedSecret := webhook.Secret
	if strings.Contains(webhook.Secret, stringutil.PasswordDummy) {
		var existingSecret string
		if err := m.q.GetWebhookSecret.Get(&existingSecret, tenantID, id); err != nil {
			m.lo.Error("error fetching existing webhook secret", "id", id, "error", err)
			return models.Webhook{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
		encryptedSecret = existingSecret
	} else if !crypto.IsEncrypted(webhook.Secret) {
		// Encrypt new secret before storing
		var err error
		encryptedSecret, err = m.encryptSecret(webhook.Secret)
		if err != nil {
			return models.Webhook{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
	}

	storedURL, err := m.storedURL(webhook)
	if err != nil {
		return models.Webhook{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	if err := m.q.UpdateWebhook.Get(&result, tenantID, id, webhook.Name, storedURL, pq.Array(webhook.Events), encryptedSecret, webhook.IsActive, kindOrCustom(webhook.Kind)); err != nil {
		m.lo.Error("error updating webhook", "error", err)
		return models.Webhook{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Decrypt secret before returning (ignore errors as non-critical)
	if err := m.decryptWebhook(&result); err != nil {
		m.lo.Error("error decrypting webhook secret after update", "webhook_id", result.ID, "error", err)
	}

	return result, nil
}

// Delete deletes a webhook by ID.
func (m *Manager) Delete(tenantID, id int) error {
	if _, err := m.q.DeleteWebhook.Exec(tenantID, id); err != nil {
		m.lo.Error("error deleting webhook", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// Toggle toggles the active status of a webhook by ID.
func (m *Manager) Toggle(tenantID, id int) (models.Webhook, error) {
	var result models.Webhook
	if err := m.q.ToggleWebhook.Get(&result, tenantID, id); err != nil {
		m.lo.Error("error toggling webhook", "error", err)
		return models.Webhook{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if err := m.decryptWebhook(&result); err != nil {
		m.lo.Error("error decrypting webhook after toggle", "webhook_id", result.ID, "error", err)
	}
	return result, nil
}

// SendTestWebhook sends a test webhook to the specified webhook ID and reports
// whether the endpoint accepted it.
func (m *Manager) SendTestWebhook(tenantID, id int) error {
	webhook, err := m.Get(tenantID, id)
	if err != nil {
		return envelope.NewError(envelope.NotFoundError, m.i18n.For(tenantID).T("globals.messages.notFound"), nil)
	}

	if err := m.deliverSingleWebhook(webhook, DeliveryTask{
		TenantID: tenantID,
		Event:    models.EventWebhookTest,
		Payload: map[string]any{
			"id":   webhook.ID,
			"name": webhook.Name,
		},
	}); err != nil {
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).Ts("webhook.testFailed", "error", err.Error()), nil)
	}

	return nil
}

// SetEventSink sets where events go besides the webhooks. Call it before Run.
func (m *Manager) SetEventSink(sink EventSink) {
	m.sink = sink
}

// TriggerEvent triggers webhooks for a specific event with the provided data.
// Only webhooks of the tenant receive it.
func (m *Manager) TriggerEvent(tenantID int, event models.WebhookEvent, data any) {
	if m.sink != nil {
		m.sink.HandleEvent(tenantID, event, data)
	}

	m.closedMu.RLock()
	defer m.closedMu.RUnlock()
	if m.closed {
		return
	}

	m.enqueue(DeliveryTask{TenantID: tenantID, Event: event, Payload: data})
}

// TriggerWebhook enqueues a delivery of the given event to one specific webhook.
func (m *Manager) TriggerWebhook(tenantID, webhookID int, event models.WebhookEvent, data any) {
	// A non-positive ID would be treated as a fan out to every subscriber of the event.
	if webhookID <= 0 {
		m.lo.Warn("dropping targeted webhook delivery, webhook ID is not positive", "webhook_id", webhookID, "event", event)
		return
	}

	m.closedMu.RLock()
	defer m.closedMu.RUnlock()
	if m.closed {
		return
	}

	m.enqueue(DeliveryTask{TenantID: tenantID, Event: event, Payload: data, WebhookID: webhookID})
}

// enqueue queues a delivery, dropping it when the queue or the tenant's share
// of it is full. The caller holds closedMu.
func (m *Manager) enqueue(task DeliveryTask) {
	if !m.share.Acquire(task.TenantID) {
		m.lo.Warn("tenant's share of the webhook queue is full, dropping webhook delivery",
			"tenant_id", task.TenantID, "event", task.Event, "webhook_id", task.WebhookID)
		return
	}
	select {
	case m.deliveryQueue <- task:
	default:
		m.share.Release(task.TenantID)
		m.lo.Warn("webhook delivery queue is full, dropping webhook delivery",
			"tenant_id", task.TenantID, "event", task.Event, "webhook_id", task.WebhookID, "queue_size", len(m.deliveryQueue))
	}
}

// Run starts the webhook delivery worker pool.
func (m *Manager) Run(ctx context.Context) {
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.worker(ctx)
		}()
	}
}

// Close signals the manager to stop processing and waits for all workers to finish.
func (m *Manager) Close() {
	m.closedMu.Lock()
	defer m.closedMu.Unlock()
	if m.closed {
		return
	}
	m.closed = true
	close(m.deliveryQueue)
	m.wg.Wait()
}

// worker processes webhook delivery tasks from the queue.
func (m *Manager) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-m.deliveryQueue:
			if !ok {
				return
			}
			m.deliverWebhook(task)
			m.share.Release(task.TenantID)
		}
	}
}

// deliverWebhook delivers webhooks for an event by making HTTP requests.
func (m *Manager) deliverWebhook(task DeliveryTask) {
	if task.WebhookID > 0 {
		webhook, err := m.Get(task.TenantID, task.WebhookID)
		if err != nil {
			m.lo.Error("error fetching webhook for delivery", "webhook_id", task.WebhookID, "event", task.Event, "error", err)
			return
		}
		if !webhook.IsActive {
			m.lo.Debug("skipping delivery, webhook is inactive", "webhook_id", webhook.ID, "event", task.Event)
			return
		}
		m.deliverSingleWebhook(webhook, task)
		return
	}

	webhooks, err := m.getWebhooksByEvent(task.TenantID, string(task.Event))
	if err != nil {
		m.lo.Error("error fetching webhooks for event", "event", task.Event, "error", err)
		return
	}

	for _, webhook := range webhooks {
		m.deliverSingleWebhook(webhook, task)
	}
}

// deliverSingleWebhook delivers a webhook to a single endpoint. It returns an
// error when the request fails or the endpoint doesn't answer with a 2xx.
func (m *Manager) deliverSingleWebhook(webhook models.Webhook, task DeliveryTask) error {
	payloadBytes, err := m.requestBody(webhook, task)
	if err != nil {
		m.lo.Error("error marshaling webhook payload", "webhook_id", webhook.ID, "event", task.Event, "error", err)
		return err
	}
	if payloadBytes == nil {
		m.lo.Debug("skipping event for chat webhook", "webhook_id", webhook.ID, "kind", webhook.Kind, "event", task.Event)
		return nil
	}

	// A Slack or Teams URL is the credential to post to the channel, so logs get it masked.
	logURL := webhook.URL
	if IsChatKind(webhook.Kind) {
		logURL = MaskURL(webhook.URL)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", webhook.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		m.lo.Error("error creating webhook request", "webhook_id", webhook.ID, "url", logURL, "event", task.Event, "error", err)
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ConectDesk-Webhook/"+version.Version)

	// Add signature if secret is provided. Slack and Teams don't check one.
	if webhook.Secret != "" && !IsChatKind(webhook.Kind) {
		signature := m.generateSignature(payloadBytes, webhook.Secret)
		req.Header.Set("X-ConectDesk-Signature", signature)
	}

	m.lo.Debug("delivering webhook",
		"webhook_id", webhook.ID,
		"kind", webhook.Kind,
		"event", task.Event,
		"payload", string(payloadBytes),
	)

	// Make the request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		m.lo.Error("webhook delivery failed - HTTP request error",
			"webhook_id", webhook.ID,
			"url", logURL,
			"event", task.Event,
			"error", err)
		return err
	}
	defer resp.Body.Close()

	// Read response body
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		m.lo.Error("error reading webhook response", "webhook_id", webhook.ID, "error", err)
		responseBody = []byte(fmt.Sprintf("Error reading response: %v", err))
	}

	// Check if delivery was successful (2xx status codes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		m.lo.Error("webhook delivery failed",
			"webhook_id", webhook.ID,
			"event", task.Event,
			"url", logURL,
			"status_code", resp.StatusCode,
			"response", string(responseBody))
		return fmt.Errorf("endpoint answered %d %s", resp.StatusCode, chatmsg.Truncate(strings.TrimSpace(string(responseBody)), 200))
	}

	m.lo.Info("webhook delivered successfully",
		"webhook_id", webhook.ID,
		"event", task.Event,
		"status_code", resp.StatusCode)
	return nil
}

// requestBody builds what a webhook is sent for an event: the raw event for a
// custom webhook, a chat message for Slack and Teams. A nil body with no error
// means the event is not posted to this webhook.
func (m *Manager) requestBody(webhook models.Webhook, task DeliveryTask) ([]byte, error) {
	if !IsChatKind(webhook.Kind) {
		return json.Marshal(map[string]any{
			"event":     task.Event,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"payload":   task.Payload,
		})
	}

	raw, err := json.Marshal(task.Payload)
	if err != nil {
		return nil, err
	}
	if chatmsg.Skips(task.Event, raw) {
		return nil, nil
	}
	var rootURL string
	if m.rootURL != nil {
		rootURL = m.rootURL(task.TenantID)
	}
	return chatBody(m.translator(task.TenantID), webhook.Kind, task.Event, raw, rootURL)
}

// translator is the workspace's language pack, for the wording of chat posts.
func (m *Manager) translator(tenantID int) chatmsg.Translator {
	if m.i18n == nil {
		return keyTranslator{}
	}
	return m.i18n.For(tenantID)
}

// keyTranslator stands in for a language pack when there is none, in tests.
type keyTranslator struct{}

func (keyTranslator) T(key string) string { return key }

// storedURL is the URL as saved: encrypted for Slack and Teams, whose URL is
// itself the credential to post to the channel, and as typed for a custom one.
func (m *Manager) storedURL(webhook models.Webhook) (string, error) {
	if !IsChatKind(webhook.Kind) || crypto.IsEncrypted(webhook.URL) {
		return webhook.URL, nil
	}
	enc, err := crypto.Encrypt(webhook.URL, m.encryptionKey)
	if err != nil {
		m.lo.Error("error encrypting webhook url", "error", err)
		return "", err
	}
	return enc, nil
}

// kindOrCustom treats a webhook saved without a kind as a custom one.
func kindOrCustom(kind string) string {
	if kind == "" {
		return models.KindCustom
	}
	return kind
}

// generateSignature generates HMAC-SHA256 signature for webhook payload.
func (m *Manager) generateSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// getWebhooksByEvent retrieves active webhooks that are subscribed to a specific event.
func (m *Manager) getWebhooksByEvent(tenantID int, event string) ([]models.Webhook, error) {
	var webhooks = make([]models.Webhook, 0)
	if err := m.q.GetWebhooksByEvent.Select(&webhooks, tenantID, event); err != nil {
		return nil, err
	}

	// Decrypt secrets
	m.decryptWebhooks(webhooks)

	return webhooks, nil
}
