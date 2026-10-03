package conversation

import (
	"encoding/json"
	"time"

	"github.com/conectdesk/conectdesk/internal/conversation/models"
)

// Delivery states of a message sent on a chat channel, in the order a message
// moves through them. Failed is final.
const (
	DeliverySent      = "sent"
	DeliveryDelivered = "delivered"
	DeliveryRead      = "read"
	DeliveryFailed    = "failed"
)

var deliveryRank = map[string]int{DeliverySent: 1, DeliveryDelivered: 2, DeliveryRead: 3, DeliveryFailed: 4}

// Delivery is a message's combined delivery state, kept on its meta under
// "delivery" for the message bubble.
type Delivery struct {
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// recordDeliveries remembers the platform IDs a send returned for a message,
// so the receipts that name them can be applied to it.
func (m *Manager) recordDeliveries(tenantID, messageID, inboxID int, recipient string, externalIDs []string) {
	for _, id := range externalIDs {
		if _, err := m.q.InsertMessageDelivery.Exec(tenantID, messageID, inboxID, recipient, id); err != nil {
			m.lo.Error("error recording message delivery", "message_id", messageID, "external_id", id, "error", err)
		}
	}
	if len(externalIDs) > 0 {
		m.refreshDelivery(tenantID, messageID)
	}
}

// ApplyDelivery applies a receipt that names the platform message it is for:
// a WhatsApp status, a Messenger delivery, an Instagram read. A receipt for a
// message we didn't send from here, or one older than what is already known,
// changes nothing.
func (m *Manager) ApplyDelivery(tenantID, inboxID int, externalID, status, errText string) {
	if _, ok := deliveryRank[status]; !ok || externalID == "" {
		return
	}
	var ids []int
	if err := m.q.ApplyMessageDelivery.Select(&ids, tenantID, inboxID, externalID, status, errText); err != nil {
		m.lo.Error("error applying message delivery", "inbox_id", inboxID, "external_id", externalID, "error", err)
		return
	}
	for _, id := range ids {
		m.refreshDelivery(tenantID, id)
	}
}

// ApplyReadWatermark applies a Messenger read receipt, which names no message
// but a time: everything sent to the person up to it has been read.
func (m *Manager) ApplyReadWatermark(tenantID, inboxID int, recipient string, watermark time.Time) {
	if recipient == "" || watermark.IsZero() {
		return
	}
	var ids []int
	if err := m.q.ApplyMessageReadWatermark.Select(&ids, tenantID, inboxID, recipient, watermark); err != nil {
		m.lo.Error("error applying read watermark", "inbox_id", inboxID, "error", err)
		return
	}
	seen := map[int]bool{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			m.refreshDelivery(tenantID, id)
		}
	}
}

// combineDeliveries reduces the states of a message's parts to one: failed if
// any part failed, otherwise the least advanced part, since the message has
// only been read once all of it has.
func combineDeliveries(parts []struct {
	Status string `db:"status"`
	Error  string `db:"error"`
}) (string, string) {
	status, errText := "", ""
	for _, p := range parts {
		if p.Status == DeliveryFailed {
			return DeliveryFailed, p.Error
		}
		if status == "" || deliveryRank[p.Status] < deliveryRank[status] {
			status = p.Status
		}
	}
	return status, errText
}

// refreshDelivery recomputes a message's delivery state from its parts, stores
// it on the message and pushes it to the agents viewing the conversation. A
// failure also marks the message failed, which offers the agent a retry.
func (m *Manager) refreshDelivery(tenantID, messageID int) {
	var parts []struct {
		Status string `db:"status"`
		Error  string `db:"error"`
	}
	if err := m.q.GetMessageDeliveries.Select(&parts, tenantID, messageID); err != nil {
		m.lo.Error("error fetching message deliveries", "message_id", messageID, "error", err)
		return
	}
	status, errText := combineDeliveries(parts)
	if status == "" {
		return
	}

	delivery, err := json.Marshal(Delivery{Status: status, Error: errText, UpdatedAt: time.Now().UTC()})
	if err != nil {
		return
	}
	var row struct {
		UUID             string          `db:"uuid"`
		Status           string          `db:"status"`
		Meta             json.RawMessage `db:"meta"`
		ConversationUUID string          `db:"conversation_uuid"`
	}
	if err := m.q.SetMessageDelivery.Get(&row, tenantID, messageID, delivery); err != nil {
		m.lo.Error("error storing message delivery", "message_id", messageID, "error", err)
		return
	}

	if status == DeliveryFailed && row.Status != models.MessageStatusFailed {
		// Broadcasts the status change and fires the message.updated webhook.
		m.UpdateMessageStatus(tenantID, row.UUID, models.MessageStatusFailed)
	}
	m.BroadcastMessageUpdate(row.ConversationUUID, row.UUID, map[string]any{"meta": row.Meta})
}
