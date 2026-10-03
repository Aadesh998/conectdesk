package conversation

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"html"

	"github.com/conectdesk/conectdesk/internal/conversation/models"
	mmodels "github.com/conectdesk/conectdesk/internal/media/models"
	wsmodels "github.com/conectdesk/conectdesk/internal/ws/models"
	"github.com/volatiletech/null/v9"
)

// Recording is a call recording to attach to the call's message.
type Recording struct {
	Name        string
	ContentType string
	Content     []byte
}

// RecordOutgoingCall logs a call an agent placed as a note in the
// conversation, which updates as the call rings, connects and ends. It is
// private: a call record isn't a message to the customer.
func (m *Manager) RecordOutgoingCall(tenantID int, conversationUUID string, agentID int, sourceID, content string, meta map[string]any) (models.Message, error) {
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return models.Message{}, err
	}
	msg := models.Message{
		ConversationUUID: conversationUUID,
		SenderID:         agentID,
		Type:             models.MessageOutgoing,
		SenderType:       models.SenderTypeAgent,
		Status:           models.MessageStatusSent,
		Private:          true,
		Content:          callHTML(content),
		ContentType:      models.ContentTypeHTML,
		SourceID:         null.StringFrom(sourceID),
		Meta:             metaJSON,
	}
	if err := m.InsertMessage(tenantID, &msg); err != nil {
		return models.Message{}, err
	}
	return msg, nil
}

// UpdateCall rewrites the message of a call, found by its source ID, as the
// provider reports what happened: update changes the call's details on the
// meta and returns the new text. A recording is attached to the message.
// It returns false if no message has the source ID.
func (m *Manager) UpdateCall(tenantID int, sourceID string, update func(meta map[string]any) string, rec *Recording) (bool, error) {
	var row struct {
		ID               int             `db:"id"`
		UUID             string          `db:"uuid"`
		Meta             json.RawMessage `db:"meta"`
		ConversationUUID string          `db:"conversation_uuid"`
		ConversationID   int             `db:"conversation_id"`
	}
	if err := m.q.GetMessageBySourceID.Get(&row, tenantID, sourceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	meta := map[string]any{}
	if len(row.Meta) > 0 {
		_ = json.Unmarshal(row.Meta, &meta)
	}
	text := update(meta)
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return true, err
	}
	if _, err := m.q.UpdateMessageContentMeta.Exec(tenantID, row.ID, callHTML(text), text, metaJSON); err != nil {
		return true, err
	}

	if rec != nil && len(rec.Content) > 0 {
		if _, err := m.mediaStore.UploadAndInsert(tenantID, rec.Name, rec.ContentType, "",
			null.StringFrom(mmodels.ModelMessages), null.IntFrom(row.ID),
			bytes.NewReader(rec.Content), len(rec.Content), null.StringFrom("attachment"), []byte("{}"), false); err != nil {
			m.lo.Error("error storing call recording", "message_id", row.ID, "error", err)
		}
	}

	// Push the whole message, recording included, to agents viewing it.
	if msg, err := m.GetMessage(tenantID, row.UUID); err == nil {
		m.SignAttachmentURLs(tenantID, msg.Attachments)
		m.BroadcastMessageUpdate(row.ConversationUUID, row.UUID, map[string]any{
			"meta":        msg.Meta,
			"content":     msg.Content,
			"attachments": msg.Attachments,
		})
	}
	return true, nil
}

// callHTML is a call's text as a message's content.
func callHTML(text string) string {
	return "<p>" + html.EscapeString(text) + "</p>"
}

// CallMessage is where a call is logged: its message's meta and conversation.
type CallMessage struct {
	MessageUUID      string
	Meta             map[string]any
	ConversationUUID string
	ConversationID   int
}

// GetCall returns the message a call is logged on, by its source ID.
func (m *Manager) GetCall(tenantID int, sourceID string) (CallMessage, bool, error) {
	var row struct {
		ID               int             `db:"id"`
		UUID             string          `db:"uuid"`
		Meta             json.RawMessage `db:"meta"`
		ConversationUUID string          `db:"conversation_uuid"`
		ConversationID   int             `db:"conversation_id"`
	}
	if err := m.q.GetMessageBySourceID.Get(&row, tenantID, sourceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CallMessage{}, false, nil
		}
		return CallMessage{}, false, err
	}
	meta := map[string]any{}
	_ = json.Unmarshal(row.Meta, &meta)
	return CallMessage{MessageUUID: row.UUID, Meta: meta, ConversationUUID: row.ConversationUUID, ConversationID: row.ConversationID}, true, nil
}

// BroadcastToAgents pushes a websocket event to the given agents, e.g. the
// pop-up for an incoming call to the agents being rung.
func (m *Manager) BroadcastToAgents(tenantID int, userIDs []int, msgType string, data any) {
	if len(userIDs) == 0 {
		return
	}
	m.broadcastToUsers(tenantID, userIDs, wsmodels.Message{Type: msgType, Data: data})
}

// SetMessageMeta sets one key of a message's meta and pushes the message's
// new meta to the agents viewing it.
func (m *Manager) SetMessageMeta(tenantID int, messageUUID, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var row struct {
		Meta             json.RawMessage `db:"meta"`
		ConversationUUID string          `db:"conversation_uuid"`
	}
	if err := m.q.SetMessageMetaKey.Get(&row, tenantID, messageUUID, key, b); err != nil {
		m.lo.Error("error setting message meta", "message_uuid", messageUUID, "key", key, "error", err)
		return err
	}
	m.BroadcastMessageUpdate(row.ConversationUUID, messageUUID, map[string]any{"meta": row.Meta})
	return nil
}

// BroadcastToTenant pushes a websocket event to every agent of a tenant, e.g.
// that the call-backs changed, for each to fetch what they may see.
func (m *Manager) BroadcastToTenant(tenantID int, msgType string, data any) {
	m.broadcastToUsers(tenantID, nil, wsmodels.Message{Type: msgType, Data: data})
}

// LatestContactConversation returns the UUID of a contact's newest
// conversation in an inbox that isn't resolved or closed, or "" if none.
func (m *Manager) LatestContactConversation(tenantID, contactID, inboxID int) (string, error) {
	var uuid string
	if err := m.q.GetLatestContactInboxConversation.Get(&uuid, tenantID, contactID, inboxID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		m.lo.Error("error fetching contact's conversation", "contact_id", contactID, "inbox_id", inboxID, "error", err)
		return "", err
	}
	return uuid, nil
}
