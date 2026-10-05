package aiagent

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/ai"
	aimodels "github.com/conectdesk/conectdesk/internal/ai/models"
	"github.com/conectdesk/conectdesk/internal/aiagent/models"
	cmodels "github.com/conectdesk/conectdesk/internal/conversation/models"
)

// Follow-ups: when a customer stops replying to an assistant, it nudges them,
// at most once per entry of its follow-up hours, each counted from its
// previous message. Jev decides whether the conversation is actually waiting
// on the customer, so a finished or declined chat is left alone. Inside
// WhatsApp's 24-hour window the LLM writes the follow-up; past it only an
// approved template can go, and Jev picks the one that fits from the
// assistant's list, or none.

const (
	// followupBatch caps the conversations one tick looks at.
	followupBatch = 200
	// followupRunTimeout bounds handling one conversation, LLM included.
	followupRunTimeout = 90 * time.Second
	// followupLockTTL keeps a second tick, or a second instance of the app,
	// off a conversation being followed up.
	followupLockTTL = 30 * time.Minute
	// followupDeclinedTTL remembers that Jev said not to follow up after a
	// message, so it isn't asked again every tick. Candidates are at most 8
	// days old.
	followupDeclinedTTL = 8 * 24 * time.Hour
)

const followupNote = `The customer hasn't replied for about %d hours since the last message in this conversation. Write one short, friendly follow-up that helps them take the next step. Don't repeat your previous message, don't apologise for following up, and don't call any tool except the knowledge base search. Reply with only the message text.`

type followupCandidate struct {
	TenantID       int       `db:"tenant_id"`
	ConversationID int       `db:"conversation_id"`
	AssistantID    int       `db:"assistant_id"`
	LastMessageAt  time.Time `db:"last_message_at"`
	FollowupsSent  int       `db:"followups_sent"`
}

// RunFollowups looks for quiet conversations to follow up every interval
// until ctx is done.
func (m *Manager) RunFollowups(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.followupTick(ctx)
		}
	}
}

func (m *Manager) followupTick(ctx context.Context) {
	var cands []followupCandidate
	if err := m.q.GetFollowupCandidates.Select(&cands, 1, followupBatch); err != nil {
		m.lo.Error("error fetching follow-up candidates", "error", err)
		return
	}
	for _, c := range cands {
		if ctx.Err() != nil {
			return
		}
		m.followUpWithRecover(ctx, c)
	}
}

func (m *Manager) followUpWithRecover(ctx context.Context, c followupCandidate) {
	defer func() {
		if r := recover(); r != nil {
			m.lo.Error("recovered from panic in ai follow-up", "conversation_id", c.ConversationID, "panic", r)
		}
	}()
	c2, cancel := context.WithTimeout(ctx, followupRunTimeout)
	defer cancel()
	m.followUp(c2, c)
}

// followupDue reports whether the next follow-up is due: there is one left,
// its delay since the last message has passed.
func followupDue(hours []int64, sent int, lastMessageAt, now time.Time) bool {
	if sent < 0 || sent >= len(hours) {
		return false
	}
	return !now.Before(lastMessageAt.Add(time.Duration(hours[sent]) * time.Hour))
}

func (m *Manager) followUp(ctx context.Context, c followupCandidate) {
	a, err := m.GetAssistant(c.TenantID, c.AssistantID)
	if err != nil || !a.Enabled || !a.FollowupsEnabled {
		return
	}
	now := time.Now()
	if !followupDue(a.FollowupHours, c.FollowupsSent, c.LastMessageAt, now) {
		return
	}
	if m.hooks.BusinessOpen != nil && !m.hooks.BusinessOpen(c.TenantID, now) {
		return
	}
	// The reply worker has this conversation: the customer just wrote.
	m.mu.Lock()
	busy := m.inflight[c.ConversationID]
	m.mu.Unlock()
	if busy {
		return
	}
	// One attempt per message waited on, across ticks and app instances.
	lockKey := fmt.Sprintf("ai_followup:%d:%d:%d", c.TenantID, c.ConversationID, c.LastMessageAt.UnixNano())
	if ok, err := m.redis.SetNX(ctx, lockKey, 1, followupLockTTL).Result(); err != nil || !ok {
		return
	}
	declined := func() { m.redis.Expire(context.Background(), lockKey, followupDeclinedTTL) }

	conv, err := m.convo.GetConversation(c.TenantID, c.ConversationID, "", "")
	if err != nil || !conv.AssignedUserID.Valid || int(conv.AssignedUserID.Int) != a.UserID || nonActionableCategories[conv.StatusCategory.String] {
		return
	}
	private := false
	msgs, _, err := m.convo.GetConversationMessages(c.TenantID, conv.UUID, 1, m.maxHistoryMessages, &private, []string{cmodels.MessageIncoming, cmodels.MessageOutgoing})
	if err != nil {
		m.lo.Error("error fetching messages for follow-up", "conversation_uuid", conv.UUID, "error", err)
		return
	}
	slices.Reverse(msgs)

	jev := m.jevClient(c.TenantID)
	windowOpen := m.hooks.ReplyWindowOpen == nil || m.hooks.ReplyWindowOpen(c.TenantID, conv.ContactID, conv.InboxID)

	// Past the window only a template reaches the customer, and only Jev can
	// pick one.
	var refs []models.TemplateRef
	var texts []string
	if !windowOpen {
		if jev == nil || m.hooks.SendTemplate == nil || m.hooks.TemplateText == nil {
			declined()
			return
		}
		for _, ref := range a.FollowupTemplates {
			if text, ok := m.hooks.TemplateText(c.TenantID, conv.InboxID, ref); ok {
				refs, texts = append(refs, ref), append(texts, text)
			}
		}
		if len(refs) == 0 {
			declined()
			return
		}
	}

	ref := -1
	if jev != nil {
		d, err := m.decideFollowup(ctx, jev, m.transcript(msgs, conv.ContactID), now.Sub(c.LastMessageAt).Hours(), c.FollowupsSent, texts, a.JevConfidence)
		if err != nil {
			// Unsolicited messages need a yes; try again on a later tick.
			m.lo.Warn("jev follow-up decision unavailable", "conversation_uuid", conv.UUID, "error", err)
			m.redis.Del(context.Background(), lockKey)
			return
		}
		if !d.FollowUp || (!windowOpen && d.Template < 0) {
			m.lo.Debug("jev decided against a follow-up", "conversation_uuid", conv.UUID, "follow_up", d.FollowUp, "template", d.Template)
			declined()
			return
		}
		ref = d.Template
	}

	meta := map[string]any{"ai_followup": true}
	if windowOpen {
		text, err := m.writeFollowup(ctx, conv, a, msgs, now.Sub(c.LastMessageAt))
		if err != nil || text == "" {
			if err != nil {
				m.lo.Error("error writing follow-up", "conversation_uuid", conv.UUID, "error", err)
			}
			return
		}
		if jev != nil && a.JevGuard {
			g, err := m.guard(ctx, jev, a, lastCustomerText(m, msgs, conv.ContactID), text, nil, a.JevConfidence)
			if err != nil || g.Blocked() {
				if g.Blocked() {
					m.recordEvent(c.TenantID, a.ID, conv.ID, "block")
					if _, err := m.convo.SendPrivateNote(c.TenantID, nil, a.UserID, conv.UUID, m.blockedNote(c.TenantID, g, text), nil); err != nil {
						m.lo.Error("error posting blocked follow-up note", "conversation_uuid", conv.UUID, "error", err)
					}
				}
				declined()
				return
			}
		}
		if err := m.postReply(conv, a, text, meta); err != nil {
			return
		}
	} else {
		if err := m.hooks.SendTemplate(conv, a.UserID, refs[ref], map[string]any{"ai_followup": true, "ai_assistant_id": a.ID}); err != nil {
			m.lo.Error("error sending follow-up template", "conversation_uuid", conv.UUID, "template", refs[ref].Name, "error", err)
			declined()
			return
		}
	}
	m.recordEvent(c.TenantID, a.ID, conv.ID, "followup")
	m.lo.Info("ai assistant followed up", "conversation_uuid", conv.UUID, "followup", c.FollowupsSent+1, "template", !windowOpen)
}

// writeFollowup has the LLM write a follow-up in the assistant's voice.
func (m *Manager) writeFollowup(ctx context.Context, conv cmodels.Conversation, a models.Assistant, msgs []cmodels.Message, quiet time.Duration) (string, error) {
	systemPrompt := buildSystemPrompt(a, m.now(conv.TenantID)) + "\n\n" + promptSection("followup_task", fmt.Sprintf(followupNote, int(quiet.Hours())))
	contactLines := contactFieldLines(conv.Contact)
	history := m.buildHistory(conv.TenantID, msgs, conv.ContactID)
	if block := customerContextBlock(conv, contactLines); block != "" {
		history = append([]aimodels.ChatMessage{{Role: aimodels.RoleUser, Content: block}}, history...)
	}
	tctx := ai.ToolContext{
		ContactID:        conv.Contact.ID,
		ContactType:      conv.Contact.Type,
		ConversationUUID: conv.UUID,
		InboxID:          conv.InboxID,
		ContactEmail:     func() string { return conv.Contact.Email.String },
		Verified:         func() bool { return false },
	}
	tools := []ai.Tool{&searchKnowledgeTool{m: m, tenantID: conv.TenantID}}
	answer, err := m.ai.RunAgentWithTools(conv.TenantID, ctx, systemPrompt, history, m.maxSteps, tctx, nil, false, false, tools)
	if err != nil {
		return "", err
	}
	main, confirm := splitConfirmation(strings.TrimSpace(answer))
	return strings.TrimSpace(main + "\n\n" + confirm), nil
}

func lastCustomerText(m *Manager, msgs []cmodels.Message, contactID int) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Type == cmodels.MessageIncoming && msgs[i].SenderID == contactID {
			return m.messageText(msgs[i])
		}
	}
	return ""
}
