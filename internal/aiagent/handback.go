package aiagent

import (
	"context"
	"strconv"
	"time"
)

// Handing conversations back to the AI. On an inbox with an AI assistant
// set, the assistant is the default: it takes every unassigned conversation,
// a person can take any of them over, and when that person leaves a customer
// waiting, the assistant takes the conversation back so the customer still
// gets an answer. Conversations the assistant itself handed to people stay
// with people for a while: it handed them off for a reason.

const (
	// handoffHoldHours is how long after an assistant hands a conversation
	// off that it stays with people.
	handoffHoldHours = 24
	// offlineWait is how long a customer waits on someone who isn't online
	// before the assistant takes over.
	offlineWait = time.Minute
	// returnBatch caps the conversations one check hands back.
	returnBatch = 200
)

type returnCandidate struct {
	TenantID         int       `db:"tenant_id"`
	ConversationID   int       `db:"conversation_id"`
	ConversationUUID string    `db:"conversation_uuid"`
	AssistantID      int       `db:"assistant_id"`
	AgentID          int       `db:"agent_id"`
	AgentStatus      string    `db:"agent_status"`
	WaitingSince     time.Time `db:"waiting_since"`
}

// RecentlyHandedOff reports whether an assistant handed the conversation to
// people recently, in which case it shouldn't be given back to the AI.
func (m *Manager) RecentlyHandedOff(tenantID, conversationID int) bool {
	var exists bool
	if err := m.q.RecentHandoffExists.Get(&exists, tenantID, conversationID, handoffHoldHours); err != nil {
		m.lo.Error("error checking recent ai handoff", "conversation_id", conversationID, "error", err)
		// Unsure: leave it with people.
		return true
	}
	return exists
}

// RunHandBacks checks every interval for customers left waiting by a person
// on an AI inbox, and hands those conversations back to the inbox's
// assistant after wait, or after a minute when the person isn't online.
func (m *Manager) RunHandBacks(ctx context.Context, interval, wait time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.handBackTick(ctx, wait)
		}
	}
}

func (m *Manager) handBackTick(ctx context.Context, wait time.Duration) {
	var cands []returnCandidate
	if err := m.q.GetReturnCandidates.Select(&cands, int(wait.Minutes()), int(offlineWait.Minutes()), handoffHoldHours, returnBatch); err != nil {
		m.lo.Error("error fetching conversations to hand back to ai", "error", err)
		return
	}
	for _, c := range cands {
		if ctx.Err() != nil {
			return
		}
		m.handBack(c, wait)
	}
}

// handBack assigns a waiting conversation to its inbox's assistant, which
// then answers the customer, and leaves the person a note saying why.
func (m *Manager) handBack(c returnCandidate, wait time.Duration) {
	a, err := m.GetAssistant(c.TenantID, c.AssistantID)
	if err != nil || !a.Enabled {
		return
	}
	sys, err := m.user.GetSystemUser(c.TenantID)
	if err != nil {
		m.lo.Error("error fetching system user to hand back to ai", "tenant_id", c.TenantID, "error", err)
		return
	}
	t := m.i18n.For(c.TenantID)
	note := t.Ts("ai.agent.handedBack", "name", a.Name, "minutes", strconv.Itoa(int(wait.Minutes())))
	if c.AgentStatus != "online" {
		note = t.Ts("ai.agent.handedBackOffline", "name", a.Name)
	}
	if err := m.convo.UpdateConversationUserAssignee(c.TenantID, c.ConversationUUID, a.UserID, sys); err != nil {
		m.lo.Error("error handing conversation back to ai", "conversation_uuid", c.ConversationUUID, "error", err)
		return
	}
	if _, err := m.convo.SendPrivateNote(c.TenantID, nil, a.UserID, c.ConversationUUID, note, nil); err != nil {
		m.lo.Error("error posting hand-back note", "conversation_uuid", c.ConversationUUID, "error", err)
	}
	m.lo.Info("conversation handed back to ai", "conversation_uuid", c.ConversationUUID, "assistant_id", a.ID,
		"agent_id", c.AgentID, "agent_status", c.AgentStatus, "waited", time.Since(c.WaitingSince).Round(time.Second))
}
