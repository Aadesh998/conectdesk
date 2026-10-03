// Package automation automatically evaluates and applies rules to conversations based on events like new conversations, updates, and time triggers,
// and performs some actions if they are true.
package automation

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/conectdesk/conectdesk/internal/automation/models"
	cmodels "github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/fairshare"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
	// MaxQueueSize is the maximum size of the task queue.
	MaxQueueSize = 10000
)

// TaskType represents the type of conversation task.
type TaskType string

const (
	NewConversation    TaskType = "new"
	UpdateConversation TaskType = "update"
	TimeTrigger        TaskType = "time-trigger"
)

// ConversationTask represents a unit of work for processing conversations.
type ConversationTask struct {
	taskType       TaskType
	eventType      string
	conversation   cmodels.Conversation
	previousValues map[string]string
}

type Engine struct {
	rules             []models.Rule
	rulesMu           sync.RWMutex
	q                 queries
	lo                *logf.Logger
	i18n              *i18ncat.Catalog
	conversationStore conversationStore
	// systemUserID returns the ID of a tenant's system user.
	systemUserID func(tenantID int) (int, error)
	taskQueue    chan ConversationTask
	closed       bool
	closedMu     sync.RWMutex
	wg           sync.WaitGroup

	suppressed   map[string]int
	suppressedMu sync.Mutex
	share        *fairshare.Limiter
}

type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18ncat.Catalog
	// MaxQueuedPerTenant caps one tenant's conversations waiting for rule
	// evaluation; 0 means no cap.
	MaxQueuedPerTenant int
}

type conversationStore interface {
	ApplyAction(action models.RuleAction, conversation cmodels.Conversation, user umodels.User) error
	GetConversation(tenantID, id int, uuid, refNum string) (cmodels.Conversation, error)
	GetConversationsCreatedAfter(time.Time) ([]cmodels.Conversation, error)
}

type queries struct {
	GetAll                  *sqlx.Stmt `query:"get-all"`
	GetRule                 *sqlx.Stmt `query:"get-rule"`
	InsertRule              *sqlx.Stmt `query:"insert-rule"`
	UpdateRule              *sqlx.Stmt `query:"update-rule"`
	DeleteRule              *sqlx.Stmt `query:"delete-rule"`
	ToggleRule              *sqlx.Stmt `query:"toggle-rule"`
	GetEnabledRules         *sqlx.Stmt `query:"get-enabled-rules"`
	UpdateRuleWeight        *sqlx.Stmt `query:"update-rule-weight"`
	UpdateRuleExecutionMode *sqlx.Stmt `query:"update-rule-execution-mode"`
}

// New initializes a new Engine.
func New(opt Opts) (*Engine, error) {
	var (
		q queries
		e = &Engine{
			lo:        opt.Lo,
			i18n:      opt.I18n,
			taskQueue: make(chan ConversationTask, MaxQueueSize),
			share:     fairshare.New(opt.MaxQueuedPerTenant),
		}
	)
	if err := dbutil.ScanSQLFile("queries.sql", &q, opt.DB, efs); err != nil {
		return nil, err
	}
	e.q = q
	e.rules = e.queryRules()
	return e, nil
}

// SetConversationStore sets conversations store.
func (e *Engine) SetConversationStore(store conversationStore) {
	e.conversationStore = store
}

// SetSystemUserResolver sets the lookup of each tenant's system user, which
// identifies events raised by automation actions.
func (e *Engine) SetSystemUserResolver(fn func(tenantID int) (int, error)) {
	e.systemUserID = fn
}

// isSystemUser reports whether userID is the tenant's system user.
func (e *Engine) isSystemUser(tenantID, userID int) bool {
	if e.systemUserID == nil {
		return false
	}
	id, err := e.systemUserID(tenantID)
	return err == nil && id == userID
}

// ReloadRules reloads automation rules from DB.
func (e *Engine) ReloadRules() {
	e.rulesMu.Lock()
	defer e.rulesMu.Unlock()
	e.lo.Debug("reloading automation engine rules")
	e.rules = e.queryRules()
}

// Run starts the Engine with a worker pool to evaluate rules based on events.
func (e *Engine) Run(ctx context.Context, workerCount int) {
	// Spawn worker pool.
	for i := 0; i < workerCount; i++ {
		e.wg.Add(1)
		go e.worker(ctx)
	}

	// Hourly ticker for timed triggers.
	ticker := time.NewTicker(1 * time.Hour)
	defer func() {
		ticker.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.lo.Info("queuing time triggers")
			e.taskQueue <- ConversationTask{taskType: TimeTrigger}
		}
	}
}

// worker processes tasks from the taskQueue until it's closed or context is done.
func (e *Engine) worker(ctx context.Context) {
	defer e.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-e.taskQueue:
			if !ok {
				return
			}
			switch task.taskType {
			case NewConversation:
				e.handleNewConversation(task.conversation)
				e.share.Release(task.conversation.TenantID)
			case UpdateConversation:
				e.handleUpdateConversation(task.conversation, task.eventType, task.previousValues)
				e.share.Release(task.conversation.TenantID)
			case TimeTrigger:
				e.handleTimeTrigger()
			}
		}
	}
}

// Close signals the Engine to stop accepting any more messages and waits for all workers to finish.
func (e *Engine) Close() {
	e.closedMu.Lock()
	defer e.closedMu.Unlock()
	if e.closed {
		return
	}
	e.closed = true
	close(e.taskQueue)
	// Wait for all workers.
	e.wg.Wait()
}

// GetAllRules retrieves all rules of a specific type.
func (e *Engine) GetAllRules(tenantID int, typ []byte) ([]models.RuleRecord, error) {
	var rules = make([]models.RuleRecord, 0)
	if err := e.q.GetAll.Select(&rules, tenantID, typ); err != nil {
		e.lo.Error("error fetching rules", "error", err)
		return rules, envelope.NewError(envelope.GeneralError, e.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return rules, nil
}

// GetRule retrieves a rule by ID.
func (e *Engine) GetRule(tenantID, id int) (models.RuleRecord, error) {
	var rule models.RuleRecord
	if err := e.q.GetRule.Get(&rule, tenantID, id); err != nil {
		if err == sql.ErrNoRows {
			return rule, envelope.NewError(envelope.InputError, e.i18n.For(tenantID).T("validation.notFoundRule"), nil)
		}
		e.lo.Error("error fetching rule", "error", err)
		return rule, envelope.NewError(envelope.GeneralError, e.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return rule, nil
}

// ToggleRule toggles the active status of a rule by ID.
func (e *Engine) ToggleRule(tenantID, id int) (models.RuleRecord, error) {
	var result models.RuleRecord
	if err := e.q.ToggleRule.Get(&result, tenantID, id); err != nil {
		e.lo.Error("error toggling rule", "error", err)
		return models.RuleRecord{}, envelope.NewError(envelope.GeneralError, e.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	// Reload rules.
	e.ReloadRules()
	return result, nil
}

// UpdateRule updates an existing rule.
func (e *Engine) UpdateRule(tenantID, id int, rule models.RuleRecord) (models.RuleRecord, error) {
	if rule.Events == nil {
		rule.Events = pq.StringArray{}
	}
	var result models.RuleRecord
	if err := e.q.UpdateRule.Get(&result, tenantID, id, rule.Name, rule.Description, rule.Type, rule.Events, rule.Rules, rule.Enabled); err != nil {
		e.lo.Error("error updating rule", "error", err)
		return models.RuleRecord{}, envelope.NewError(envelope.GeneralError, e.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	// Reload rules.
	e.ReloadRules()
	return result, nil
}

// CreateRule creates a new rule.
func (e *Engine) CreateRule(tenantID int, rule models.RuleRecord) (models.RuleRecord, error) {
	if rule.Events == nil {
		rule.Events = pq.StringArray{}
	}
	var result models.RuleRecord
	if err := e.q.InsertRule.Get(&result, tenantID, rule.Name, rule.Description, rule.Type, rule.Events, rule.Rules); err != nil {
		e.lo.Error("error creating rule", "error", err)
		return models.RuleRecord{}, envelope.NewError(envelope.GeneralError, e.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	// Reload rules.
	e.ReloadRules()
	return result, nil
}

// DeleteRule deletes a rule by ID.
func (e *Engine) DeleteRule(tenantID, id int) error {
	if _, err := e.q.DeleteRule.Exec(tenantID, id); err != nil {
		e.lo.Error("error deleting rule", "error", err)
		return envelope.NewError(envelope.GeneralError, e.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	// Reload rules.
	e.ReloadRules()
	return nil
}

// UpdateRuleWeights updates the weights of the automation rules.
func (e *Engine) UpdateRuleWeights(tenantID int, weights map[int]int) error {
	for id, weight := range weights {
		if _, err := e.q.UpdateRuleWeight.Exec(tenantID, id, weight); err != nil {
			e.lo.Error("error updating rule weight", "error", err)
			return envelope.NewError(envelope.GeneralError, e.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
	}
	// Reload rules.
	e.ReloadRules()
	return nil
}

// UpdateRuleExecutionMode updates the execution mode for a type of rule.
func (e *Engine) UpdateRuleExecutionMode(tenantID int, ruleType, mode string) error {
	if _, err := e.q.UpdateRuleExecutionMode.Exec(tenantID, ruleType, mode); err != nil {
		e.lo.Error("error updating rule execution mode", "error", err)
		return envelope.NewError(envelope.GeneralError, e.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	// Reload rules.
	e.ReloadRules()
	return nil
}

// EvaluateNewConversationRules enqueues a new conversation for rule evaluation.
func (e *Engine) EvaluateNewConversationRules(conversation cmodels.Conversation) {
	e.closedMu.RLock()
	defer e.closedMu.RUnlock()
	if e.closed {
		return
	}
	e.enqueue(ConversationTask{
		taskType:     NewConversation,
		conversation: conversation,
	})
}

// EvaluateConversationUpdateRules enqueues a conversation for rule evaluation. previousValues carries pre-change field values for filters like previous_status.
func (e *Engine) EvaluateConversationUpdateRules(conversation cmodels.Conversation, eventType string, previousValues map[string]string, actor umodels.User) {
	if eventType == "" {
		e.lo.Error("error evaluating conversation update rules: eventType is empty")
		return
	}
	// Drop only the engine's own in-flight events; a concurrent agent update on a suppressed conversation must still evaluate.
	if e.isSystemUser(conversation.TenantID, actor.ID) && e.isSuppressed(conversation.UUID) {
		e.lo.Debug("automation suppressed for conversation, skipping update rule evaluation", "uuid", conversation.UUID, "event_type", eventType)
		return
	}
	e.closedMu.RLock()
	defer e.closedMu.RUnlock()
	if e.closed {
		return
	}
	e.enqueue(ConversationTask{
		taskType:       UpdateConversation,
		eventType:      eventType,
		conversation:   conversation,
		previousValues: previousValues,
	})
}

// enqueue queues a conversation's rule evaluation, dropping it when the queue
// or the tenant's share of it is full. The caller holds closedMu.
func (e *Engine) enqueue(task ConversationTask) {
	tenantID := task.conversation.TenantID
	if !e.share.Acquire(tenantID) {
		e.lo.Warn("tenant's share of the automation queue is full, dropping rule evaluation",
			"tenant_id", tenantID, "uuid", task.conversation.UUID, "task", task.taskType)
		return
	}
	select {
	case e.taskQueue <- task:
	default:
		e.share.Release(tenantID)
		e.lo.Warn("automation queue is full, dropping rule evaluation", "tenant_id", tenantID, "uuid", task.conversation.UUID)
	}
}

// EvaluateConversationUpdateRulesByID fetches conversation by ID and enqueues for rule evaluation; it has no pre-change snapshot, so previous_* filters never match.
func (e *Engine) EvaluateConversationUpdateRulesByID(tenantID, conversationID int, conversationUUID, eventType string, actor umodels.User) {
	conversation, err := e.conversationStore.GetConversation(tenantID, conversationID, conversationUUID, "")
	if err != nil {
		e.lo.Error("error fetching conversation", "conversation_id", conversationID, "error", err)
		return
	}
	e.EvaluateConversationUpdateRules(conversation, eventType, nil, actor)
}

// handleNewConversation handles new conversation events.
func (e *Engine) handleNewConversation(conversation cmodels.Conversation) {
	e.lo.Debug("handling new conversation for automation rule evaluation", "uuid", conversation.UUID)
	rules := e.filterRulesByType(conversation.TenantID, models.RuleTypeNewConversation, "")
	if len(rules) == 0 {
		e.lo.Info("no rules to evaluate for new conversation rule evaluation", "uuid", conversation.UUID)
		return
	}
	e.evalConversationRules(rules, conversation, nil)
}

// handleUpdateConversation handles update conversation events with specific eventType.
func (e *Engine) handleUpdateConversation(conversation cmodels.Conversation, eventType string, previousValues map[string]string) {
	e.lo.Debug("handling update conversation for automation rule evaluation", "uuid", conversation.UUID, "event_type", eventType)
	rules := e.filterRulesByType(conversation.TenantID, models.RuleTypeConversationUpdate, eventType)
	if len(rules) == 0 {
		e.lo.Info("no rules to evaluate for conversation update", "uuid", conversation.UUID, "event_type", eventType)
		return
	}
	e.evalConversationRules(rules, conversation, previousValues)
}

// handleTimeTrigger handles time trigger events.
func (e *Engine) handleTimeTrigger() {
	e.lo.Info("running time trigger evaluation for automation rules")
	thirtyDaysAgo := time.Now().Add(-30 * 24 * time.Hour)
	conversations, err := e.conversationStore.GetConversationsCreatedAfter(thirtyDaysAgo)
	if err != nil {
		e.lo.Error("error fetching conversations for time trigger", "error", err)
		return
	}
	e.lo.Info("fetched conversations for evaluating time triggers", "conversations_count", len(conversations))
	for _, c := range conversations {
		// Each conversation is evaluated against its own tenant's rules.
		rules := e.filterRulesByType(c.TenantID, models.RuleTypeTimeTrigger, "")
		if len(rules) == 0 {
			continue
		}
		// Fetch entire conversation.
		conversation, err := e.conversationStore.GetConversation(c.TenantID, 0, c.UUID, "")
		if err != nil {
			e.lo.Error("error fetching conversation for time trigger", "uuid", c.UUID, "error", err)
			continue
		}
		e.evalConversationRules(rules, conversation, nil)
	}
}

// suppress marks a conversation as having automation actions in flight.
func (e *Engine) suppress(conversationUUID string) {
	e.suppressedMu.Lock()
	defer e.suppressedMu.Unlock()
	if e.suppressed == nil {
		e.suppressed = map[string]int{}
	}
	e.suppressed[conversationUUID]++
}

// unsuppress releases one in-flight claim on a conversation.
func (e *Engine) unsuppress(conversationUUID string) {
	e.suppressedMu.Lock()
	defer e.suppressedMu.Unlock()
	if e.suppressed[conversationUUID] <= 1 {
		delete(e.suppressed, conversationUUID)
		return
	}
	e.suppressed[conversationUUID]--
}

func (e *Engine) isSuppressed(conversationUUID string) bool {
	e.suppressedMu.Lock()
	defer e.suppressedMu.Unlock()
	return e.suppressed[conversationUUID] > 0
}

// queryRules fetches automation rules from the database.
// Rules of every tenant are loaded, each tagged with its tenant.
func (e *Engine) queryRules() []models.Rule {
	var (
		rules         []models.RuleRecord
		filteredRules []models.Rule
	)
	err := e.q.GetEnabledRules.Select(&rules)
	if err != nil {
		e.lo.Error("error fetching automation rules", "error", err)
		return filteredRules
	}

	for _, rule := range rules {
		var rulesBatch []models.Rule
		if err := json.Unmarshal([]byte(rule.Rules), &rulesBatch); err != nil {
			e.lo.Error("error unmarshalling rule JSON", "error", err)
			continue
		}
		// Set values from DB.
		for i := range rulesBatch {
			rulesBatch[i].TenantID = rule.TenantID
			rulesBatch[i].Type = rule.Type
			rulesBatch[i].Events = rule.Events
			rulesBatch[i].ExecutionMode = rule.ExecutionMode
		}
		filteredRules = append(filteredRules, rulesBatch...)
	}
	return filteredRules
}

// filterRulesByType filters the tenant's rules by type and event.
func (e *Engine) filterRulesByType(tenantID int, ruleType, eventType string) []models.Rule {
	e.rulesMu.RLock()
	defer e.rulesMu.RUnlock()

	var filteredRules []models.Rule
	for _, rule := range e.rules {
		if rule.TenantID == tenantID && rule.Type == ruleType && (eventType == "" || slices.Contains(rule.Events, eventType)) {
			filteredRules = append(filteredRules, rule)
		}
	}
	return filteredRules
}
