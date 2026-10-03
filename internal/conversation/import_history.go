package conversation

import (
	"fmt"
	"time"

	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/volatiletech/null/v9"
)

// HistoryMessage is one message of a chat imported from a channel's history.
type HistoryMessage struct {
	models.IncomingMessage
	// Outgoing is a message the business sent, stored as sent by the system user.
	Outgoing bool
	SentAt   time.Time
}

// ImportChat stores one chat's history, oldest message first, as a new
// conversation in the inbox, and returns how many messages it stored.
//
// Nothing runs on the messages: no automations, AI replies, SLAs or
// notifications, since none of it is happening now. Messages already here,
// e.g. ones that arrived through the webhook, are skipped by their source ID,
// and a chat with nothing new creates nothing. The history goes into its own
// conversation, resolved when resolve is set, so it never changes the latest
// message of one an agent is working on.
func (m *Manager) ImportChat(tenantID, inboxID int, contact models.IncomingContact, history []HistoryMessage, resolve bool) (int, error) {
	var fresh []HistoryMessage
	for _, h := range history {
		if h.SourceID.String == "" {
			continue
		}
		convID, err := m.messageExistsBySourceID(tenantID, []string{h.SourceID.String})
		if err != nil && err != errConversationNotFound {
			return 0, err
		}
		if convID == 0 {
			fresh = append(fresh, h)
		}
	}
	if len(fresh) == 0 {
		return 0, nil
	}

	user := umodels.User{
		FirstName:      contact.FirstName,
		LastName:       contact.LastName,
		PhoneNumber:    contact.PhoneNumber,
		ExternalUserID: contact.ExternalUserID,
		Type:           umodels.UserTypeContact,
	}
	if err := m.userStore.ResolveContact(tenantID, &user, umodels.ContactSync); err != nil {
		return 0, fmt.Errorf("resolving contact: %w", err)
	}
	systemUser, err := m.userStore.GetSystemUser(tenantID)
	if err != nil {
		return 0, fmt.Errorf("fetching system user: %w", err)
	}

	first := fresh[0]
	convID, convUUID, err := m.CreateConversation(tenantID, user.ID, inboxID,
		stringutil.HTML2Text(first.Content), first.SentAt, first.Subject,
		false, map[string]any{"imported": true}, nil, 0, 0)
	if err != nil || convID == 0 {
		return 0, fmt.Errorf("creating conversation: %w", err)
	}

	imported := 0
	for _, h := range fresh {
		senderID := user.ID
		if h.Outgoing {
			senderID = systemUser.ID
		}
		msg := h.ToMessage(senderID, convID, convUUID)
		if h.Outgoing {
			msg.Type = models.MessageOutgoing
			msg.SenderType = models.SenderTypeAgent
			msg.Status = models.MessageStatusSent
		}
		if !h.SentAt.IsZero() {
			msg.SentAt = null.TimeFrom(h.SentAt)
		}
		if err := m.uploadMessageAttachments(&msg); err != nil {
			m.lo.Error("error uploading imported message attachments", "source_id", h.SourceID.String, "error", err)
			msg.Attachments = nil
			msg.Media = nil
		}
		if err := m.InsertMessage(tenantID, &msg); err != nil {
			m.lo.Error("error inserting imported message", "source_id", h.SourceID.String, "error", err)
			continue
		}
		imported++
	}

	if imported == 0 {
		if err := m.DeleteConversation(tenantID, convUUID); err != nil {
			m.lo.Error("error deleting empty imported conversation", "uuid", convUUID, "error", err)
		}
		return 0, nil
	}
	if _, err := m.q.FinishImportedConversation.Exec(tenantID, convID, resolve); err != nil {
		m.lo.Error("error finishing imported conversation", "uuid", convUUID, "error", err)
	}
	return imported, nil
}
