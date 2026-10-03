package conversation

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/conectdesk/conectdesk/internal/attachment"
	amodels "github.com/conectdesk/conectdesk/internal/automation/models"
	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/image"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/livechat"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/voice"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/whatsapp"
	mmodels "github.com/conectdesk/conectdesk/internal/media/models"
	"github.com/conectdesk/conectdesk/internal/sla"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	wmodels "github.com/conectdesk/conectdesk/internal/webhook/models"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
)

const (
	maxMessagesPerPage = 500
	// Only allow visitor-to-contact upgrade within this window after the last continuity email.
	upgradeWindowTTL = 7 * 24 * time.Hour
)

// Matches <img ... src="URL"> and captures the URL for downstream parsing.
var imgSrcPattern = regexp.MustCompile(`(?i)<img\b[^>]*?\bsrc=["']([^"']*)["']`)

// fromNameVars is the template context for an inbox's from-name template.
type fromNameVars struct {
	Agent fromNameAgent
	Inbox fromNameInbox
}

type fromNameAgent struct{ FirstName, LastName, FullName string }

type fromNameInbox struct{ Name string }

// Run starts a pool of worker goroutines to handle message dispatching via inbox's channel and processes incoming messages. It scans for
// pending outgoing messages at the specified read interval and pushes them to the outgoing queue to be sent.
func (m *Manager) Run(ctx context.Context, incomingQWorkers, outgoingQWorkers, scanInterval time.Duration) {
	dbScanner := time.NewTicker(scanInterval)
	defer dbScanner.Stop()

	for range outgoingQWorkers {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.MessageSenderWorker(ctx)
		}()
	}
	for range incomingQWorkers {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.IncomingMessageWorker(ctx)
		}()
	}

	// Scan pending outgoing messages and send them.
	for {
		select {
		case <-ctx.Done():
			return
		case <-dbScanner.C:
			// Keep only about two messages per worker queued, so a message
			// never waits behind a long backlog; the query takes tenants in turn.
			batch := 2*int(outgoingQWorkers) - len(m.outgoingMessageQueue)
			if batch <= 0 {
				continue
			}
			var (
				pendingMessages = []models.Message{}
				messageIDs      = m.getOutgoingProcessingMessageIDs()
			)

			// Get pending outgoing messages and skip the currently processing message ids.
			if err := m.q.GetOutgoingPendingMessages.Select(&pendingMessages, pq.Array(messageIDs), batch); err != nil {
				m.lo.Error("error fetching pending messages from db", "error", err)
				continue
			}

			// Prepare and push the message to the outgoing queue.
			for _, message := range pendingMessages {
				// Put the message ID in the processing map.
				m.outgoingProcessingMessages.Store(message.ID, message.ID)

				// Push the message to the outgoing message queue.
				m.outgoingMessageQueue <- message
			}
		}
	}
}

// Close signals the Manager to stop processing messages, closes channels,
// and waits for all worker goroutines to finish processing.
func (m *Manager) Close() {
	m.closedMu.Lock()
	defer m.closedMu.Unlock()
	m.closed = true
	close(m.outgoingMessageQueue)
	close(m.incomingMessageQueue)
	m.wg.Wait()
}

// IncomingMessageWorker processes incoming messages from the incoming message queue.
func (m *Manager) IncomingMessageWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-m.incomingMessageQueue:
			if !ok {
				return
			}
			if _, err := m.ProcessIncomingMessage(msg); err != nil {
				m.lo.Error("error processing incoming msg", "error", err)
			}
			m.incomingShare.Release(msg.TenantID)
		}
	}
}

// MessageSenderWorker sends outgoing pending messages.
func (m *Manager) MessageSenderWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-m.outgoingMessageQueue:
			if !ok {
				return
			}
			m.sendOutgoingMessage(message)
		}
	}
}

// sendOutgoingMessage sends an outgoing message.
func (m *Manager) sendOutgoingMessage(message models.Message) {
	tenantID := message.TenantID
	defer m.outgoingProcessingMessages.Delete(message.ID)

	// Helper function to handle errors
	handleError := func(err error, errorMsg string) bool {
		if err != nil {
			m.lo.Error(errorMsg, "error", err, "message_id", message.ID)
			m.UpdateMessageStatus(tenantID, message.UUID, models.MessageStatusFailed)
			return true
		}
		return false
	}

	// Get inbox
	inb, err := m.inboxStore.Get(message.InboxID)
	if handleError(err, "error fetching inbox") {
		return
	}

	// Render content in template
	if err := m.RenderMessageInTemplate(inb.Channel(), &message); err != nil {
		handleError(err, "error rendering content in template")
		return
	}

	// Attach attachments to the message
	if err := m.attachAttachmentsToMessage(&message); err != nil {
		handleError(err, "error attaching attachments to message")
		return
	}

	// Convert to OutboundMessage for transport
	outbound := message.ToOutbound()

	if inb.Channel() == inbox.ChannelEmail {
		outbound.From = m.emailFromAddress(inb, message)

		// Set "In-Reply-To" and "References" headers for email threading.
		outbound.References, outbound.InReplyTo = m.BuildEmailThreadingHeaders(tenantID, message.ConversationID, outbound.SourceID)
	}

	// Send message. Chat channels report back what they sent, so later
	// receipts can be matched to this message; parts that went out before a
	// failure are recorded too, since they still get receipts.
	if ts, ok := inb.(inbox.TrackedSender); ok {
		var (
			recipient   string
			externalIDs []string
		)
		recipient, externalIDs, err = ts.SendTracked(outbound)
		m.recordDeliveries(tenantID, message.ID, message.InboxID, recipient, externalIDs)
	} else {
		err = inb.Send(outbound)
	}
	if err != nil && err != livechat.ErrClientNotConnected {
		handleError(err, "error sending message")
		return
	}

	// Update status as sent.
	m.UpdateMessageStatus(tenantID, message.UUID, models.MessageStatusSent)

	// Skip system user replies since we only update timestamps and SLA for human replies.
	systemUser, err := m.userStore.GetSystemUser(tenantID)
	if err != nil {
		m.lo.Error("error fetching system user", "error", err)
		return
	}
	if message.SenderID != systemUser.ID {
		conversation, err := m.GetConversation(tenantID, message.ConversationID, "", "")
		if err != nil {
			m.lo.Error("error fetching conversation", "conversation_id", message.ConversationID, "error", err)
			return
		}

		now := time.Now()
		nowStr := now.Format(time.RFC3339)
		wsData := map[string]any{"last_reply_at": nowStr, "waiting_since": nil}

		var isFirstReply bool
		if err := m.q.UpdateConversationReplyTimestamps.QueryRow(tenantID, message.ConversationID, now).Scan(&isFirstReply); err != nil {
			m.lo.Error("error updating conversation reply timestamps", "error", err)
		} else if isFirstReply {
			wsData["first_reply_at"] = nowStr
		}

		// Mark latest SLA event for next response as met.
		metAt, err := m.slaStore.SetLatestSLAEventMetAt(tenantID, conversation.AppliedSLAID.Int, sla.MetricNextResponse)
		if err != nil && !errors.Is(err, sla.ErrLatestSLAEventNotFound) {
			m.lo.Error("error setting next response SLA event `met_at`", "conversation_id", conversation.ID, "metric", sla.MetricNextResponse, "applied_sla_id", conversation.AppliedSLAID.Int, "error", err)
		} else if !metAt.IsZero() {
			wsData["next_response_met_at"] = metAt.Format(time.RFC3339)
		}

		m.BroadcastConversationUpdate(message.ConversationUUID, wsData)

		if message.ShouldEvaluateAutomation(systemUser.ID) {
			m.automation.EvaluateConversationUpdateRulesByID(tenantID, message.ConversationID, "", amodels.EventConversationMessageOutgoing, umodels.User{ID: message.SenderID})
		}
	}
}

// BuildTemplateData builds the common template data map for rendering message content variables.
func (m *Manager) BuildTemplateData(tenantID int, conversationUUID string, senderID int) (map[string]any, error) {
	conversation, err := m.GetConversation(tenantID, 0, conversationUUID, "")
	if err != nil {
		return nil, fmt.Errorf("fetching conversation: %w", err)
	}

	sender, err := m.userStore.GetAgent(tenantID, senderID, "")
	if err != nil {
		return nil, fmt.Errorf("fetching message sender user: %w", err)
	}

	data := map[string]any{
		"Conversation": map[string]any{
			"ReferenceNumber": conversation.ReferenceNumber,
			"Subject":         conversation.Subject.String,
			"Priority":        conversation.Priority.String,
			"UUID":            conversation.UUID,
		},
		"Contact": map[string]any{
			"FirstName": conversation.Contact.FirstName,
			"LastName":  conversation.Contact.LastName,
			"FullName":  conversation.Contact.FullName(),
			"Email":     conversation.Contact.Email.String,
		},
		"Recipient": map[string]any{
			"FirstName": conversation.Contact.FirstName,
			"LastName":  conversation.Contact.LastName,
			"FullName":  conversation.Contact.FullName(),
			"Email":     conversation.Contact.Email.String,
		},
		"Author": map[string]any{
			"FirstName": sender.FirstName,
			"LastName":  sender.LastName,
			"FullName":  sender.FullName(),
			"Email":     sender.Email.String,
		},
		// Lets templates disclose AI-composed replies, e.g. {{ if .IsAIComposed }}Composed by AI{{ end }}.
		"IsAIComposed": sender.Type == umodels.UserTypeAIAssistant,
	}

	// For automated replies set author fields to empty strings as the recipients will see name as System.
	if sender.IsSystemUser() {
		data["Author"] = map[string]any{
			"FirstName": "",
			"LastName":  "",
			"FullName":  "",
			"Email":     "",
		}
	}

	return data, nil
}

// RenderMessageInTemplate renders message content in the email base template for sending.
func (m *Manager) RenderMessageInTemplate(channel string, message *models.Message) error {
	switch channel {
	case inbox.ChannelEmail:
		data, err := m.BuildTemplateData(message.TenantID, message.ConversationUUID, message.SenderID)
		if err != nil {
			return err
		}

		// Expose message meta flags to the template.
		var (
			isContinuity bool
			meta         map[string]any
		)
		if len(message.Meta) > 0 && json.Unmarshal(message.Meta, &meta) == nil {
			isContinuity, _ = meta["continuity_email"].(bool)
		}
		data["IsContinuityEmail"] = isContinuity

		message.Content, err = m.template.RenderEmailWithTemplate(message.TenantID, data, message.Content)
		if err != nil {
			m.lo.Error("could not render email content using template", "id", message.ID, "error", err)
			return fmt.Errorf("could not render email content using template: %w", err)
		}
	case inbox.ChannelLiveChat, inbox.ChannelWhatsApp, inbox.ChannelMessenger, inbox.ChannelInstagram, inbox.ChannelTelegram, inbox.ChannelVoice:
		// Only email renders messages through the email template.
		return nil
	default:
		m.lo.Warn("unknown message channel", "channel", channel)
		return fmt.Errorf("unknown message channel: %s", channel)
	}
	return nil
}

// GetConversationMessages retrieves messages for a specific conversation.
func (m *Manager) GetConversationMessages(tenantID int, conversationUUID string, page, pageSize int, private *bool, msgTypes []string) ([]models.Message, int, error) {
	var (
		messages = make([]models.Message, 0)
		qArgs    []any
	)

	// Convert msgTypes slice to pq.StringArray for PostgreSQL
	var typesArg any
	if len(msgTypes) > 0 {
		typesArg = pq.StringArray(msgTypes)
	}

	qArgs = append(qArgs, tenantID, conversationUUID, private, typesArg)
	query, pageSize, qArgs, err := m.generateMessagesQuery(m.q.GetMessages, qArgs, page, pageSize)
	if err != nil {
		m.lo.Error("error generating messages query", "error", err)
		return messages, pageSize, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	tx, err := m.db.BeginTxx(context.Background(), &sql.TxOptions{
		ReadOnly: true,
	})
	defer tx.Rollback()
	if err != nil {
		m.lo.Error("error preparing get messages query", "error", err)
		return messages, pageSize, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	if err := tx.Select(&messages, query, qArgs...); err != nil {
		m.lo.Error("error fetching conversations", "error", err)
		return messages, pageSize, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	return messages, pageSize, nil
}

// GetAllConversationMessages returns the newest messages in a conversation in chronological order, capped at limit; a non-positive limit returns them all.
func (m *Manager) GetAllConversationMessages(tenantID int, conversationUUID string, private *bool, msgTypes []string, limit int) ([]models.Message, error) {
	var all []models.Message
	pageSize := maxMessagesPerPage
	if limit > 0 && limit < pageSize {
		pageSize = limit
	}
	for page := 1; ; page++ {
		messages, _, err := m.GetConversationMessages(tenantID, conversationUUID, page, pageSize, private, msgTypes)
		if err != nil {
			return nil, err
		}
		all = append(all, messages...)
		if len(messages) == 0 || len(all) >= messages[0].Total || (limit > 0 && len(all) >= limit) {
			break
		}
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	slices.Reverse(all)
	return all, nil
}

// GetMessage retrieves a message by UUID.
func (m *Manager) GetMessage(tenantID int, uuid string) (models.Message, error) {
	var message models.Message
	if err := m.q.GetMessage.Get(&message, tenantID, uuid); err != nil {
		m.lo.Error("error fetching message", "uuid", uuid, "error", err)
		return message, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Generate signed URLs for attachments.
	m.SignAttachmentURLs(tenantID, message.Attachments)

	return message, nil
}

// SignAttachmentURLs adds access URLs for the original image and its thumbnail.
func (m *Manager) SignAttachmentURLs(tenantID int, attachments attachment.Attachments) {
	for i := range attachments {
		attachments[i].URL = m.mediaStore.GetURL(tenantID, attachments[i].UUID, attachments[i].ContentType, attachments[i].Name)
		if strings.HasPrefix(attachments[i].ContentType, "image/") {
			attachments[i].ThumbnailURL = m.mediaStore.GetThumbnailURL(tenantID, attachments[i].UUID)
		}
	}
}

// UpdateMessageStatus updates the status of a message.
func (m *Manager) UpdateMessageStatus(tenantID int, messageUUID string, status string) error {
	if _, err := m.q.UpdateMessageStatus.Exec(tenantID, status, messageUUID); err != nil {
		m.lo.Error("error updating message status", "message_uuid", messageUUID, "error", err)
		return err
	}

	// Broadcast message status update to all conversation subscribers.
	conversationUUID, _ := m.getConversationUUIDFromMessageUUID(tenantID, messageUUID)
	m.BroadcastMessageUpdate(conversationUUID, messageUUID, map[string]any{"status": status})

	// Trigger webhook for message update.
	if message, err := m.GetMessage(tenantID, messageUUID); err != nil {
		m.lo.Error("error fetching message for webhook event", "uuid", messageUUID, "error", err)
	} else {
		m.webhookStore.TriggerEvent(tenantID, wmodels.EventMessageUpdated, message)
	}

	return nil
}

// MarkMessageAsPending updates message status to `Pending`, enqueuing it for sending.
func (m *Manager) MarkMessageAsPending(tenantID int, uuid string) error {
	if err := m.UpdateMessageStatus(tenantID, uuid, models.MessageStatusPending); err != nil {
		m.lo.Error("error marking message as pending", "uuid", uuid, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.errorSendingMessage"), nil)
	}
	return nil
}

// SendPrivateNote inserts a private message in a conversation.
func (m *Manager) SendPrivateNote(tenantID int, media []mmodels.Media, senderID int, conversationUUID, content string, mentions []models.MentionInput) (models.Message, error) {
	// Best-effort render template variables before saving.
	if data, err := m.BuildTemplateData(tenantID, conversationUUID, senderID); err == nil {
		content = m.template.RenderString(tenantID, data, content)
	}

	message := models.Message{
		ConversationUUID: conversationUUID,
		SenderID:         senderID,
		Type:             models.MessageOutgoing,
		SenderType:       models.SenderTypeAgent,
		Status:           models.MessageStatusSent,
		Content:          content,
		ContentType:      models.ContentTypeHTML,
		Private:          true,
		Media:            media,
	}
	if err := m.InsertMessage(tenantID, &message); err != nil {
		return models.Message{}, err
	}

	// Insert mentions if any.
	if len(mentions) > 0 {
		if err := m.InsertMentions(tenantID, message.ConversationID, message.ID, senderID, mentions); err != nil {
			m.lo.Error("error inserting mentions", "error", err)
		}
		go m.NotifyMention(conversationUUID, message, mentions, senderID)
	}

	return message, nil
}

// CreateContactMessage creates a contact message in a conversation.
func (m *Manager) CreateContactMessage(tenantID int, media []mmodels.Media, contactID int, conversationUUID, content, contentType string, isNewConversation bool) (models.Message, error) {
	message := models.Message{
		ConversationUUID: conversationUUID,
		SenderID:         contactID,
		Type:             models.MessageIncoming,
		SenderType:       models.SenderTypeContact,
		Status:           models.MessageStatusReceived,
		Content:          content,
		ContentType:      contentType,
		Private:          false,
		Media:            media,
	}
	if err := m.InsertMessage(tenantID, &message); err != nil {
		return models.Message{}, err
	}

	// Process post-message hooks (reopen, waiting since, automation, SLA).
	if err := m.ProcessIncomingMessageHooks(tenantID, conversationUUID, isNewConversation); err != nil {
		m.lo.Error("error processing incoming message hooks", "conversation_uuid", conversationUUID, "error", err)
	}

	return message, nil
}

// QueueReply queues a reply message in a conversation.
func (m *Manager) QueueReply(tenantID int, media []mmodels.Media, inboxID, senderID, contactID int, conversationUUID, content string, to, cc, bcc []string, metaMap map[string]interface{}) (models.Message, error) {
	var (
		message = models.Message{}
	)

	inboxRecord, err := m.inboxStore.GetDBRecord(tenantID, inboxID)
	if err != nil {
		m.lo.Error("error fetching inbox record", "inbox_id", inboxID, "error", err)
		return models.Message{}, err
	}

	if !inboxRecord.Enabled {
		return models.Message{}, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("status.disabledInbox"), nil)
	}

	var sourceID string
	switch inboxRecord.Channel {
	case inbox.ChannelEmail:
		// Add `to`, `cc`, and `bcc` recipients to meta map.
		to = stringutil.RemoveEmpty(to)
		cc = stringutil.RemoveEmpty(cc)
		bcc = stringutil.RemoveEmpty(bcc)
		if len(to) > 0 {
			metaMap["to"] = to
		}
		if len(cc) > 0 {
			metaMap["cc"] = cc
		}
		if len(bcc) > 0 {
			metaMap["bcc"] = bcc
		}
		if len(to) == 0 {
			return message, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).Ts("globals.messages.empty", "name", "`to`"), nil)
		}
		sourceID, err = stringutil.GenerateEmailMessageID(conversationUUID, inboxRecord.From)
		if err != nil {
			m.lo.Error("error generating source message id", "error", err)
			return models.Message{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
	case inbox.ChannelVoice:
		// A phone conversation is answered by calling back, not in writing.
		return message, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("voice.noWrittenReplies"), nil)
	case inbox.ChannelWhatsApp, inbox.ChannelMessenger, inbox.ChannelInstagram, inbox.ChannelTelegram:
		// These replies are addressed by the contact's ID on the channel, which is
		// derived from the contact rather than typed by the agent, so any `to`
		// from the request is ignored.
		recipient, err := m.chatRecipient(tenantID, contactID, inboxRecord.Channel)
		if err != nil {
			return message, err
		}
		metaMap["to"] = []string{recipient}
	}

	// Marshal meta.
	metaJSON, err := json.Marshal(metaMap)
	if err != nil {
		m.lo.Error("error marshalling message meta map to JSON", "error", err)
		return models.Message{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Best-effort render template variables before saving so agents see rendered content immediately.
	if data, err := m.BuildTemplateData(tenantID, conversationUUID, senderID); err == nil {
		content = m.template.RenderString(tenantID, data, content)
	}

	// Insert the message into the database
	message = models.Message{
		ConversationUUID:  conversationUUID,
		SenderID:          senderID,
		Type:              models.MessageOutgoing,
		SenderType:        models.SenderTypeAgent,
		Status:            models.MessageStatusPending,
		Content:           content,
		ContentType:       models.ContentTypeHTML,
		Private:           false,
		Media:             media,
		SourceID:          null.StringFrom(sourceID),
		MessageReceiverID: contactID,
		Meta:              metaJSON,
	}
	if err := m.InsertMessage(tenantID, &message); err != nil {
		return models.Message{}, err
	}
	return message, nil
}

// InsertMessage inserts a message and attaches the media to the message.
// The message is stored in, and stamped with, the tenant.
func (m *Manager) InsertMessage(tenantID int, message *models.Message) error {
	message.TenantID = tenantID
	if message.Private {
		message.Status = models.MessageStatusSent
	}
	if len(message.Meta) == 0 || string(message.Meta) == "null" {
		message.Meta = json.RawMessage(`{}`)
	}

	// Handle empty content type enum, default to text.
	if message.ContentType == "" {
		message.ContentType = models.ContentTypeText
	}

	inlineUUIDs := extractInlineImageUUIDs(message.Content)

	// Rewrite inline image URLs to cid:ldsk-<uuid>. The read API resolves them back to signed URLs.
	message.Content = rewriteInlineImagesToCID(message.Content)

	// Convert content to plain text for search.
	if message.ContentType == models.ContentTypeText {
		message.TextContent = message.Content
	} else {
		message.TextContent = stringutil.HTML2Text(message.Content)
	}

	tx, err := m.db.Beginx()
	if err != nil {
		m.lo.Error("error beginning message insert transaction", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	defer tx.Rollback()

	if err := tx.Stmtx(m.q.InsertMessage).Get(message, tenantID, message.Type, message.Status, message.ConversationID, message.ConversationUUID, message.Content, message.TextContent, message.SenderID, message.SenderType,
		message.Private, message.ContentType, message.SourceID, message.Meta, message.SentAt); err != nil {
		// A duplicate source ID is returned unwrapped so callers can recognise it:
		// an at-least-once webhook delivering the same message twice is a routine
		// event to skip, and an envelope error would hide the Postgres code.
		if dbutil.IsUniqueViolationError(err) {
			return err
		}
		m.lo.Error("error inserting message in db", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	if err := m.mediaStore.LinkMessageMediaTx(tx, message.TenantID, message.ID, message.Media, inlineUUIDs); err != nil {
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	if err := tx.Commit(); err != nil {
		m.lo.Error("error committing message insert transaction", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Add this user as a participant if not already present.
	m.addConversationParticipant(message.TenantID, message.SenderID, message.ConversationUUID)

	// Skip updating last_message and broadcasting for continuity emails.
	if !message.IsContinuityMessage() {
		// Hide CSAT message content as it contains a public link to the survey.
		lastMessage := message.TextContent
		if message.HasCSAT() {
			lastMessage = "Please rate your experience with us"
		}

		// HTML2Text drops <img> tags, so image-only messages have empty text. Fall back to a media-type preview.
		if strings.TrimSpace(lastMessage) == "" {
			switch {
			case len(message.Media) > 0:
				lastMessage = m.getMediaPreview(message.Media[0])
			case len(inlineUUIDs) > 0:
				lastMessage = m.i18n.For(tenantID).T("globals.terms.image")
			}
		}

		// Update conversation last message details (also conditionally updates last_interaction if not activity/private).
		m.UpdateConversationLastMessage(message.TenantID, message.ConversationID, message.ConversationUUID, lastMessage, message.SenderType, message.Type, message.Private, message.CreatedAt, message.SenderID)

		var convItem *models.ConversationListItem
		if item, err := m.GetConversationListItem(message.TenantID, message.ConversationUUID); err == nil {
			convItem = &item
		} else {
			m.lo.Error("error fetching conversation list item for broadcast", "uuid", message.ConversationUUID, "error", err)
		}
		m.BroadcastNewMessage(message, convItem, lastMessage)
	}

	// Refetch the message to get all fields populated (e.g., author, media URLs).
	refetchedMessage, err := m.GetMessage(message.TenantID, message.UUID)
	if err != nil {
		m.lo.Error("error fetching message after insert", "error", err)
	} else {
		*message = refetchedMessage
	}

	// Trigger webhook for new message created.
	m.webhookStore.TriggerEvent(message.TenantID, wmodels.EventMessageCreated, message)

	return nil
}

// RecordAssigneeUserChange records an activity for a user assignee change.
func (m *Manager) RecordAssigneeUserChange(tenantID int, conversationUUID string, assigneeID int, actor umodels.User) error {
	// Self assignment.
	if assigneeID == actor.ID {
		return m.InsertConversationActivity(tenantID, models.ActivitySelfAssign, conversationUUID, actor.FullName(), actor)
	}

	// Assignment to another user.
	assignee, err := m.userStore.GetAgent(tenantID, assigneeID, "")
	if err != nil {
		return err
	}
	return m.InsertConversationActivity(tenantID, models.ActivityAssignedUserChange, conversationUUID, assignee.FullName(), actor)
}

// RecordAssigneeTeamChange records an activity for a team assignee change.
func (m *Manager) RecordAssigneeTeamChange(tenantID int, conversationUUID string, teamID int, actor umodels.User) error {
	team, err := m.teamStore.Get(tenantID, teamID)
	if err != nil {
		return err
	}
	return m.InsertConversationActivity(tenantID, models.ActivityAssignedTeamChange, conversationUUID, team.Name, actor)
}

// RecordPriorityChange records an activity for a priority change.
func (m *Manager) RecordPriorityChange(tenantID int, priority, conversationUUID string, actor umodels.User) error {
	return m.InsertConversationActivity(tenantID, models.ActivityPriorityChange, conversationUUID, priority, actor)
}

// RecordStatusChange records an activity for a status change.
func (m *Manager) RecordStatusChange(tenantID int, status, conversationUUID string, actor umodels.User) error {
	return m.InsertConversationActivity(tenantID, models.ActivityStatusChange, conversationUUID, status, actor)
}

// RecordSLASet records an activity for an SLA set.
func (m *Manager) RecordSLASet(tenantID int, conversationUUID string, slaName string, actor umodels.User) error {
	return m.InsertConversationActivity(tenantID, models.ActivitySLASet, conversationUUID, slaName, actor)
}

// RecordTagAddition records an activity for a tag addition.
func (m *Manager) RecordTagAddition(tenantID int, conversationUUID string, tag string, actor umodels.User) error {
	return m.InsertConversationActivity(tenantID, models.ActivityTagAdded, conversationUUID, tag, actor)
}

// RecordTagRemoval records an activity for a tag removal.
func (m *Manager) RecordTagRemoval(tenantID int, conversationUUID string, tag string, actor umodels.User) error {
	return m.InsertConversationActivity(tenantID, models.ActivityTagRemoved, conversationUUID, tag, actor)
}

// InsertConversationActivity inserts an activity message.
func (m *Manager) InsertConversationActivity(tenantID int, activityType, conversationUUID, newValue string, actor umodels.User) error {
	content, err := m.getMessageActivityContent(activityType, newValue, actor.FullName())
	if err != nil {
		m.lo.Error("error could not generate activity content", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Store the activity type structurally so callers can filter activities without parsing i18n content.
	meta, _ := json.Marshal(map[string]string{"activity_type": activityType})

	message := models.Message{
		Type:             models.MessageActivity,
		Status:           models.MessageStatusSent,
		Content:          content,
		ContentType:      models.ContentTypeText,
		ConversationUUID: conversationUUID,
		Private:          true,
		SenderID:         actor.ID,
		SenderType:       models.SenderTypeAgent,
		Meta:             meta,
	}

	if err := m.InsertMessage(tenantID, &message); err != nil {
		m.lo.Error("error inserting activity message", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// getConversationUUIDFromMessageUUID returns conversation UUID from message UUID.
func (m *Manager) getConversationUUIDFromMessageUUID(tenantID int, uuid string) (string, error) {
	var conversationUUID string
	if err := m.q.GetConversationUUIDFromMessageUUID.Get(&conversationUUID, tenantID, uuid); err != nil {
		m.lo.Error("error fetching conversation uuid from message uuid", "uuid", uuid, "error", err)
		return conversationUUID, err
	}
	return conversationUUID, nil
}

// getMessageActivityContent generates activity content based on the activity type.
func (m *Manager) getMessageActivityContent(activityType, newValue, actorName string) (string, error) {
	var content = ""
	switch activityType {
	case models.ActivityAssignedUserChange:
		content = fmt.Sprintf("Assigned to %s by %s", newValue, actorName)
	case models.ActivityAssignedTeamChange:
		content = fmt.Sprintf("Assigned to %s team by %s", newValue, actorName)
	case models.ActivitySelfAssign:
		content = fmt.Sprintf("%s self-assigned this conversation", actorName)
	case models.ActivityPriorityChange:
		content = fmt.Sprintf("%s set priority to %s", actorName, newValue)
	case models.ActivityStatusChange:
		content = fmt.Sprintf("%s marked the conversation as %s", actorName, newValue)
	case models.ActivityTagAdded:
		content = fmt.Sprintf("%s added tag %s", actorName, newValue)
	case models.ActivityTagRemoved:
		content = fmt.Sprintf("%s removed tag %s", actorName, newValue)
	case models.ActivitySLASet:
		content = fmt.Sprintf("%s set %s SLA policy", actorName, newValue)
	case models.ActivityParticipantAdded:
		content = fmt.Sprintf("%s joined the conversation", newValue)
	default:
		return "", fmt.Errorf("invalid activity type %s", activityType)
	}
	return content, nil
}

// ProcessIncomingMessage handles the insertion of an incoming message and
// associated contact. It finds or creates the contact, checks for existing
// conversations, and creates a new conversation if necessary. It also
// inserts the message, uploads any attachments, and queues the conversation evaluation of automation rules.
func (m *Manager) ProcessIncomingMessage(in models.IncomingMessage) (models.Message, error) {
	// Return early if this message already exists (same source ID).
	dupConvID, err := m.messageExistsBySourceID(in.TenantID, []string{in.SourceID.String})
	if err != nil && err != errConversationNotFound {
		return models.Message{}, err
	}
	if dupConvID > 0 {
		return models.Message{}, nil
	}

	// Resolve sender and conversation from plus addressing.
	senderID, conversationID, conversationUUID, err := m.resolveSender(&in)
	if err != nil {
		return models.Message{}, err
	}

	// Find or create contact.
	if senderID == 0 {
		user := umodels.User{
			FirstName:      in.Contact.FirstName,
			LastName:       in.Contact.LastName,
			Email:          in.Contact.Email,
			PhoneNumber:    in.Contact.PhoneNumber,
			ExternalUserID: in.Contact.ExternalUserID,
			Type:           umodels.UserTypeContact,
		}
		if err := m.userStore.ResolveContact(in.TenantID, &user, umodels.ContactSync); err != nil {
			m.lo.Error("error creating contact for incoming message", "message_source_id", in.SourceID.String, "error", err)
			return models.Message{}, fmt.Errorf("creating contact: %w", err)
		}
		senderID = user.ID
		in.Contact.ID = senderID
	}

	// Match conversation if not already matched by plus-addressing.
	var isNewConversation bool
	if conversationID == 0 {
		conversationID, conversationUUID, isNewConversation, err = m.findOrCreateConversation(in)
		if err != nil {
			m.lo.Error("error finding or creating conversation for incoming message", "message_source_id", in.SourceID.String, "error", err)
			return models.Message{}, err
		}
	}

	// For existing conversations, override sender with the conversation's contact when emails match.
	// Guarded on a non-empty email: on channels without one (e.g. WhatsApp) every
	// contact would otherwise "match" and inherit the conversation's contact.
	if !isNewConversation && conversationID > 0 && in.Contact.Email.String != "" {
		conversation, convErr := m.GetConversation(in.TenantID, conversationID, "", "")
		if convErr == nil && strings.EqualFold(conversation.Contact.Email.String, in.Contact.Email.String) {
			senderID = conversation.ContactID
			in.Contact.ID = senderID
		}
	}

	// Convert to Message for attachment upload and insertion.
	msg := in.ToMessage(senderID, conversationID, conversationUUID)

	// Upload message attachments. On failure, delete the conversation if it was just created for this message.
	if upErr := m.uploadMessageAttachments(&msg); upErr != nil {
		m.lo.Error("error uploading message attachments", "message_source_id", in.SourceID.String, "error", upErr)
		if isNewConversation && conversationUUID != "" {
			m.lo.Info("deleting conversation as message attachment upload failed", "conversation_uuid", conversationUUID, "message_source_id", in.SourceID.String)
			if err := m.DeleteConversation(in.TenantID, conversationUUID); err != nil {
				return models.Message{}, fmt.Errorf("deleting conversation after message attachment upload failure: %w", err)
			}
		}
		return models.Message{}, fmt.Errorf("uploading message attachments: %w", upErr)
	}

	// Insert message. On failure, delete the conversation if it was just created for this message.
	if err = m.InsertMessage(in.TenantID, &msg); err != nil {
		// Webhook delivery is at-least-once: a provider may send the same event
		// twice, and two copies racing each other both pass the source ID check
		// above before either has been written. The unique index on source_id is
		// what actually settles it, so a duplicate here is an expected outcome,
		// not a failure to report.
		if dbutil.IsUniqueViolationError(err) {
			m.lo.Debug("skipping duplicate incoming message", "message_source_id", in.SourceID.String, "conversation_uuid", conversationUUID)
			if isNewConversation && conversationUUID != "" {
				if delErr := m.DeleteConversation(in.TenantID, conversationUUID); delErr != nil {
					m.lo.Error("error deleting conversation created for a duplicate message", "conversation_uuid", conversationUUID, "error", delErr)
				}
			}
			return models.Message{}, nil
		}
		m.lo.Error("error inserting incoming message", "message_source_id", in.SourceID.String, "conversation_uuid", conversationUUID, "is_new", isNewConversation, "error", err)
		if isNewConversation && conversationUUID != "" {
			if delErr := m.DeleteConversation(in.TenantID, conversationUUID); delErr != nil {
				return models.Message{}, fmt.Errorf("deleting conversation after message insert failure: %w", delErr)
			}
		}
		return models.Message{}, fmt.Errorf("inserting message: %w", err)
	}

	// When a customer replies to a continuity emailsync the message to their live chat widget via WebSocket.
	// No-op if the conversation's inbox isn't livechat.
	m.broadcastMessageToWidgetClients(&msg)

	// Process post-message hooks (automation rules, webhooks, SLA, etc.).
	if err := m.ProcessIncomingMessageHooks(in.TenantID, msg.ConversationUUID, isNewConversation); err != nil {
		m.lo.Error("error processing incoming message hooks", "conversation_uuid", msg.ConversationUUID, "error", err)
		return models.Message{}, fmt.Errorf("processing incoming message hooks: %w", err)
	}
	return msg, nil
}

// resolveSender resolves the sender for an incoming message via plus-addressing.
// Returns senderID, and optionally conversationID/UUID if matched.
// If sender is not resolved here, ProcessIncomingMessage handles it with conversation context.
func (m *Manager) resolveSender(in *models.IncomingMessage) (senderID, conversationID int, conversationUUID string, err error) {
	if in.ConversationUUIDFromReplyTo != "" {
		senderID, conversationID, conversationUUID, err = m.resolveByPlusAddress(in)
		if err != nil {
			return 0, 0, "", err
		}
		if senderID > 0 {
			in.Contact.ID = senderID
		}
	}
	return senderID, conversationID, conversationUUID, nil
}

// resolveByPlusAddress attempts to match a conversation via plus-addressed Reply-To
// (e.g., inbox+conv-{uuid}@domain). If the conversation contact is a visitor, it upgrades
// them to a contact (proving email ownership). Returns senderID > 0 if resolved.
func (m *Manager) resolveByPlusAddress(in *models.IncomingMessage) (senderID, conversationID int, conversationUUID string, err error) {
	conversation, err := m.GetConversation(in.TenantID, 0, in.ConversationUUIDFromReplyTo, "")
	if err != nil {
		// Not found return with no error.
		if envErr, ok := err.(envelope.Error); ok && envErr.ErrorType == envelope.NotFoundError {
			return 0, 0, "", nil
		}
		// Other errors.
		return 0, 0, "", fmt.Errorf("fetching conversation: %w", err)
	}

	m.lo.Debug("matched conversation by plus-addressed Reply-To", "conversation_uuid", conversation.UUID, "contact_email", in.Contact.Email.String)

	conversationID = conversation.ID
	conversationUUID = conversation.UUID
	senderID = conversation.Contact.ID

	// Already a contact - if same email, return as sender. If different email, let contact resolution find the actual sender.
	if conversation.Contact.Type == umodels.UserTypeContact {
		if !strings.EqualFold(conversation.Contact.Email.String, in.Contact.Email.String) {
			return 0, conversationID, conversationUUID, nil
		}
		return senderID, conversationID, conversationUUID, nil
	}

	// Visitor upgrade requires email match - proves email ownership.
	// If a different email replied, thread the message but don't upgrade.
	if !strings.EqualFold(conversation.Contact.Email.String, in.Contact.Email.String) {
		return 0, conversationID, conversationUUID, nil
	}

	user, contactErr := m.userStore.Get(in.TenantID, 0, in.Contact.Email.String, []string{umodels.UserTypeContact})
	if contactErr == nil {
		m.lo.Debug("a contact already exists with the same email as visitor; not upgrading visitor", "conversation_uuid", conversation.UUID, "contact_email", in.Contact.Email.String, "contact_user_id", user.ID)
		// A contact with this email already exists; don't upgrade visitor.
		// Let contact resolution find the correct sender ID.
		return 0, conversationID, conversationUUID, nil
	}

	if envErr, ok := contactErr.(envelope.Error); !ok || envErr.ErrorType != envelope.NotFoundError {
		return 0, 0, "", fmt.Errorf("fetching contact by email: %w", contactErr)
	}

	// Block upgrade if continuity email TTL expired.
	if !m.isVisitorUpgradeSafe(conversation) {
		return 0, conversationID, conversationUUID, nil
	}

	// Upgrade visitor as no contact exist with this email.
	if err := m.userStore.UpgradeVisitorToContact(in.TenantID, conversation.Contact.ID); err != nil {
		return 0, 0, "", fmt.Errorf("upgrading visitor to contact: %w", err)
	}

	m.lo.Debug("upgraded visitor to contact", "conversation_uuid", conversation.UUID, "contact_id", conversation.Contact.ID)

	// Notify conversation subscribers that the contact type has changed.
	m.BroadcastContactUpdate(in.TenantID, conversation.ContactID, map[string]any{"type": umodels.UserTypeContact})

	return senderID, conversationID, conversationUUID, nil
}

// isVisitorUpgradeSafe checks whether a visitor-to-contact upgrade should proceed.
// Blocks upgrade if the continuity email TTL has expired.
func (m *Manager) isVisitorUpgradeSafe(conversation models.Conversation) bool {
	if conversation.LastContinuityEmailSentAt.Valid {
		if time.Since(conversation.LastContinuityEmailSentAt.Time) > upgradeWindowTTL {
			m.lo.Info("visitor upgrade blocked: continuity email TTL expired",
				"conversation_uuid", conversation.UUID,
				"last_sent", conversation.LastContinuityEmailSentAt.Time,
				"age", time.Since(conversation.LastContinuityEmailSentAt.Time).String(),
				"max_ttl", upgradeWindowTTL.String())
			return false
		}
	}
	return true
}

// ProcessIncomingLiveChatMessage handles incoming live chat messages.
func (m *Manager) ProcessIncomingLiveChatMessage(msg models.Message) (models.Message, error) {
	// Upload message attachments.
	if err := m.uploadMessageAttachments(&msg); err != nil {
		return models.Message{}, fmt.Errorf("uploading message attachments: %w", err)
	}

	// Insert message.
	if err := m.InsertMessage(msg.TenantID, &msg); err != nil {
		return models.Message{}, err
	}

	// Advance contact_last_seen_at.
	if err := m.UpdateConversationContactLastSeen(msg.TenantID, msg.ConversationUUID); err != nil {
		m.lo.Error("error updating contact last seen after livechat message", "conversation_uuid", msg.ConversationUUID, "error", err)
	}

	// Process post-message hooks (automation rules, webhooks, SLA, etc.).
	// isNewConversation = false since conversation always exists for live chat.
	if err := m.ProcessIncomingMessageHooks(msg.TenantID, msg.ConversationUUID, false); err != nil {
		m.lo.Error("error processing incoming message hooks", "conversation_uuid", msg.ConversationUUID, "error", err)
	}

	return msg, nil
}

// MessageExists checks if a message with the given messageID exists.
func (m *Manager) MessageExists(tenantID int, messageID string) (bool, error) {
	_, err := m.messageExistsBySourceID(tenantID, []string{messageID})
	if err != nil {
		if errors.Is(err, errConversationNotFound) {
			return false, nil
		}
		m.lo.Error("error fetching message from db", "error", err)
		return false, err
	}
	return true, nil
}

// EnqueueIncoming enqueues an incoming message for inserting in db.
func (m *Manager) EnqueueIncoming(message models.IncomingMessage) error {
	m.closedMu.Lock()
	defer m.closedMu.Unlock()
	if m.closed {
		return errors.New("incoming message queue is closed")
	}

	// A tenant flooding the queue is refused before it fills it for everyone.
	if !m.incomingShare.Acquire(message.TenantID) {
		m.lo.Warn("tenant's share of the incoming message queue is full", "tenant_id", message.TenantID, "inbox_id", message.InboxID)
		return errors.New("incoming message queue is full for this tenant")
	}
	select {
	case m.incomingMessageQueue <- message:
		return nil
	default:
		m.incomingShare.Release(message.TenantID)
		m.lo.Warn("WARNING: incoming message queue is full")
		return errors.New("incoming message queue is full")
	}
}

// GetConversationByMessageID returns conversation by message id.
func (m *Manager) GetConversationByMessageID(tenantID, id int) (models.Conversation, error) {
	var conversation = models.Conversation{}
	if err := m.q.GetConversationByMessageID.Get(&conversation, tenantID, id); err != nil {
		if err == sql.ErrNoRows {
			return conversation, envelope.NewError(envelope.NotFoundError, m.i18n.For(tenantID).T("validation.notFoundConversation"), nil)
		}
		m.lo.Error("error fetching message from DB", "error", err)
		return conversation, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return conversation, nil
}

// generateMessagesQuery generates the SQL query for fetching messages in a conversation.
func (c *Manager) generateMessagesQuery(baseQuery string, qArgs []interface{}, page, pageSize int) (string, int, []interface{}, error) {
	if pageSize > maxMessagesPerPage {
		pageSize = maxMessagesPerPage
	}

	// Calculate the offset
	offset := (page - 1) * pageSize

	// Append LIMIT and OFFSET to query arguments
	qArgs = append(qArgs, pageSize, offset)

	// Include LIMIT and OFFSET in the SQL query
	sqlQuery := fmt.Sprintf(baseQuery, fmt.Sprintf("LIMIT $%d OFFSET $%d", len(qArgs)-1, len(qArgs)))
	return sqlQuery, pageSize, qArgs, nil
}

// extractInlineImageUUIDs returns unique media UUIDs from <img src="..."> URLs in order of first appearance, skipping the cid: form.
func extractInlineImageUUIDs(content string) []string {
	matches := imgSrcPattern.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		url := m[1]
		if strings.HasPrefix(url, "cid:") {
			continue
		}
		u := stringutil.ExtractUUID(url)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// extractInlineContentIDs returns unique content_ids referenced via <img src="cid:..."> in the body.
func extractInlineContentIDs(content string) []string {
	matches := imgSrcPattern.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		url := m[1]
		if !strings.HasPrefix(url, "cid:") {
			continue
		}
		cid := strings.TrimPrefix(url, "cid:")
		if cid == "" || seen[cid] {
			continue
		}
		seen[cid] = true
		out = append(out, cid)
	}
	return out
}

// rewriteInlineImagesToCID rewrites every <img src="...<uuid>..."> to <img src="cid:ldsk-<uuid>">. Already-cid form is left alone.
func rewriteInlineImagesToCID(content string) string {
	return imgSrcPattern.ReplaceAllStringFunc(content, func(match string) string {
		sub := imgSrcPattern.FindStringSubmatch(match)
		url := sub[1]
		if strings.HasPrefix(url, "cid:") {
			return match
		}
		u := stringutil.ExtractUUID(url)
		if u == "" {
			return match
		}
		return strings.Replace(match, url, "cid:"+inlineContentID(u), 1)
	})
}

// uploadMessageAttachments uploads all attachments for a message.
func (m *Manager) uploadMessageAttachments(message *models.Message) error {
	if len(message.Attachments) == 0 {
		return nil
	}

	for _, attachment := range message.Attachments {
		contentID := attachment.ContentID
		if contentID != "" {
			storedCID, exists, mediaUUID := m.findExistingMedia(message.TenantID, contentID, message.ConversationUUID)

			// Make body's cid match the stored content_id so the read path can find it.
			if storedCID != contentID {
				message.Content = strings.ReplaceAll(message.Content, fmt.Sprintf("cid:%s", contentID), fmt.Sprintf("cid:%s", storedCID))
			}

			if exists {
				m.lo.Debug("inline attachment exists, reusing", "content_id", storedCID, "media_uuid", mediaUUID)
				continue
			}
			contentID = storedCID
		}

		attachment.Name = stringutil.SanitizeFilename(attachment.Name)

		if len(attachment.Content) == 0 {
			m.lo.Warn("skipping empty attachment", "name", attachment.Name, "content_id", contentID, "content_type", attachment.ContentType, "disposition", attachment.Disposition, "message_source_id", message.SourceID.String, "conversation_uuid", message.ConversationUUID)
			continue
		}

		m.lo.Debug("uploading message attachment", "name", attachment.Name, "content_id", contentID, "size", attachment.Size, "content_type", attachment.ContentType, "disposition", attachment.Disposition)

		// Upload and insert entry in media table.
		attachReader := bytes.NewReader(attachment.Content)
		media, err := m.mediaStore.UploadAndInsert(message.TenantID,
			attachment.Name,
			attachment.ContentType,
			contentID,
			/** Linking media to message happens later **/
			null.String{}, /** modelType */
			null.Int{},    /** modelID **/
			attachReader,
			attachment.Size,
			null.StringFrom(attachment.Disposition),
			[]byte("{}"), /** meta **/
			true,         /** private **/
		)
		if err != nil {
			m.lo.Error("failed to upload attachment", "name", attachment.Name, "content_type", attachment.ContentType, "size", attachment.Size, "content_id", contentID, "disposition", attachment.Disposition, "conversation_uuid", message.ConversationUUID, "message_source_id", message.SourceID.String, "error", err)
			return fmt.Errorf("failed to upload media %s: %w", attachment.Name, err)
		}

		// If the attachment is an image, generate and upload a thumbnail. Log any errors and continue.
		attachmentExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(attachment.Name)), ".")
		if slices.Contains(image.Exts, attachmentExt) && image.IsImageByContent(bytes.NewReader(attachment.Content)) {
			if err := m.uploadThumbnailForMedia(media, attachment.Content); err != nil {
				m.lo.Error("error uploading thumbnail", "error", err)
			}
		}

		message.Media = append(message.Media, media)
	}
	return nil
}

// findOrCreateConversation finds or creates a conversation for the given incoming message.
func (m *Manager) findOrCreateConversation(in models.IncomingMessage) (int, string, bool, error) {
	var (
		conversationID   int
		conversationUUID string
		err              error
	)

	// Chat channels have no threading headers: a chat is one continuous thread,
	// so an inbound message joins the contact's most recent unresolved
	// conversation in this inbox and only starts a new one once that has been
	// resolved.
	if isChatChannel(in.Channel) {
		// Look up and create under one lock, or two messages arriving together
		// each find nothing and each open a conversation for the same chat.
		defer m.lockChatThread(in.Contact.ID, in.InboxID)()

		conversationID, err = m.latestUnresolvedConversation(in.TenantID, in.Contact.ID, in.InboxID)
		if err != nil && err != errConversationNotFound {
			return 0, "", false, err
		}
	} else {
		// Search for existing conversation using the in-reply-to and references.
		m.lo.Debug("searching conversation using in-reply-to and references", "in_reply_to", in.InReplyTo, "references", in.References)

		sourceIDs := append([]string{in.InReplyTo}, in.References...)
		conversationID, err = m.messageExistsBySourceID(in.TenantID, sourceIDs)
		if err != nil && err != errConversationNotFound {
			return 0, "", false, err
		}
	}

	// Conversation not found, create one.
	if conversationID == 0 {
		m.lo.Debug("no conversation found with in-reply-to and references, creating new conversation", "in_reply_to", in.InReplyTo, "references", in.References)
		lastMessage := stringutil.HTML2Text(in.Content)
		lastMessageAt := time.Now()
		conversationID, conversationUUID, err = m.CreateConversation(in.TenantID, in.Contact.ID,
			in.InboxID,
			lastMessage,
			lastMessageAt,
			in.Subject,
			false, /**append reference number to subject**/
			nil,   /** meta **/
			nil,   /** customer attributes **/
			0,     /** max conversation **/
			0,     /** rate limit window **/
		)
		if err != nil || conversationID == 0 {
			return 0, "", false, err
		}
		return conversationID, conversationUUID, true, nil
	}

	// Get UUID for the found conversation ID.
	conversationUUID, err = m.GetConversationUUID(in.TenantID, conversationID)
	if err != nil {
		return 0, "", false, err
	}
	return conversationID, conversationUUID, false, nil
}

// isChatChannel reports whether a channel threads as one continuous chat per
// contact rather than by reply headers.
func isChatChannel(channel string) bool {
	return channel == inbox.ChannelWhatsApp || channel == inbox.ChannelMessenger || channel == inbox.ChannelInstagram ||
		channel == inbox.ChannelTelegram || channel == inbox.ChannelVoice
}

// chatRecipient resolves a contact to the ID a WhatsApp, Messenger or
// Instagram reply is addressed to.
//
// The ID is stored on the contact's external_user_id, prefixed with the
// channel, when their first inbound message creates them, which keeps it
// stable even if someone edits the contact later. For WhatsApp a contact added
// by hand has their phone number to fall back on; Messenger and Instagram IDs
// are scoped to the Page, so there is nothing to fall back on there.
func (m *Manager) chatRecipient(tenantID, contactID int, channel string) (string, error) {
	// The type filter must be a non-nil slice: it reaches the query as an array,
	// and a nil one arrives as NULL, where `cardinality(NULL) = 0` is itself NULL
	// and the lookup matches nothing.
	contact, err := m.userStore.Get(tenantID, contactID, "", []string{umodels.UserTypeContact, umodels.UserTypeVisitor})
	if err != nil {
		m.lo.Error("error fetching contact for reply", "contact_id", contactID, "channel", channel, "error", err)
		return "", envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	if id, ok := strings.CutPrefix(contact.ExternalUserID.String, channel+":"); ok && id != "" {
		return id, nil
	}

	if channel == inbox.ChannelWhatsApp {
		// The contact form stores the country as an ISO code, e.g. "IN".
		if waID := whatsapp.WAIDFromPhone(voice.ContactE164(contact.PhoneNumberCountryCode.String, contact.PhoneNumber.String)); whatsapp.IsValidWAID(waID) {
			return waID, nil
		}
	}

	return "", envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("inbox.noChatRecipient"), nil)
}

// lockChatThread locks find-or-create for one contact in one inbox and
// returns the unlock function. Locks are kept rather than deleted on release:
// there is one per contact per chat inbox, and dropping them would risk
// releasing a lock another goroutine is about to take.
func (m *Manager) lockChatThread(contactID, inboxID int) func() {
	key := fmt.Sprintf("%d:%d", inboxID, contactID)
	value, _ := m.chatThreadLocks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// LastIncomingAt returns when a contact last wrote to an inbox, or the zero
// time if they never have.
func (m *Manager) LastIncomingAt(tenantID, contactID, inboxID int) (time.Time, error) {
	var at null.Time
	if err := m.q.GetLastIncomingAt.QueryRow(tenantID, contactID, inboxID).Scan(&at); err != nil {
		m.lo.Error("error fetching last incoming message time", "contact_id", contactID, "inbox_id", inboxID, "error", err)
		return time.Time{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return at.Time, nil
}

// latestUnresolvedConversation returns the contact's most recent conversation in
// an inbox that isn't in a resolved status, used to thread channels that carry no
// reply headers of their own.
func (m *Manager) latestUnresolvedConversation(tenantID, contactID, inboxID int) (int, error) {
	if contactID == 0 || inboxID == 0 {
		return 0, errConversationNotFound
	}
	var conversationID int
	if err := m.q.GetLatestUnresolvedConversation.QueryRow(tenantID, contactID, inboxID).Scan(&conversationID); err != nil {
		if err == sql.ErrNoRows {
			return 0, errConversationNotFound
		}
		m.lo.Error("error fetching latest unresolved conversation", "contact_id", contactID, "inbox_id", inboxID, "error", err)
		return 0, err
	}
	return conversationID, nil
}

// messageExistsBySourceID returns conversation ID if a message with any of the given source IDs exists.
func (m *Manager) messageExistsBySourceID(tenantID int, messageSourceIDs []string) (int, error) {
	messageSourceIDs = stringutil.RemoveEmpty(messageSourceIDs)
	if len(messageSourceIDs) == 0 {
		return 0, errConversationNotFound
	}
	var conversationID int
	if err := m.q.MessageExistsBySourceID.QueryRow(tenantID, pq.Array(messageSourceIDs)).Scan(&conversationID); err != nil {
		if err == sql.ErrNoRows {
			return conversationID, errConversationNotFound
		}
		m.lo.Error("error fetching msg from DB", "error", err)
		return conversationID, err
	}
	return conversationID, nil
}

// GetInlineMediaRefs returns media referenced via cid: in the body but linked to other messages (quoted history).
func (m *Manager) GetInlineMediaRefs(message *models.Message) ([]mmodels.Media, error) {
	cids := extractInlineContentIDs(message.Content)
	if len(cids) == 0 {
		return nil, nil
	}
	existing := make(map[string]bool, len(message.Attachments))
	for _, a := range message.Attachments {
		if a.ContentID != "" {
			existing[a.ContentID] = true
		}
	}
	missing := make([]string, 0, len(cids))
	for _, cid := range cids {
		if !existing[cid] {
			missing = append(missing, cid)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	return m.mediaStore.GetByContentIDs(message.TenantID, missing, message.ConversationUUID)
}

// fetchMessageAttachments fetches attachments (also inline images) for a single message ID.
func (m *Manager) fetchMessageAttachments(tenantID, messageID int) (attachment.Attachments, error) {
	var attachments attachment.Attachments

	// Get all media for this message.
	medias, err := m.mediaStore.GetByModel(tenantID, messageID, mmodels.ModelMessages)
	if err != nil {
		return attachments, fmt.Errorf("error fetching message attachments: %w", err)
	}

	// Fetch blobs for each media item.
	for _, media := range medias {
		blob, err := m.mediaStore.GetBlob(media.UUID)
		if err != nil {
			return attachments, fmt.Errorf("error fetching media blob: %w", err)
		}

		contentID := media.ContentID
		if contentID == "" {
			contentID = media.UUID
		}

		attachment := attachment.Attachment{
			Name:        media.Filename,
			UUID:        media.UUID,
			ContentType: media.ContentType,
			ContentID:   contentID,
			Content:     blob,
			Size:        media.Size,
			Header:      attachment.MakeHeader(media.ContentType, contentID, media.Filename, "base64", media.Disposition.String),
			URL:         m.mediaStore.GetURL(tenantID, media.UUID, media.ContentType, media.Filename),
			PublicURL:   m.mediaStore.GetSignedURL(tenantID, media.UUID),
		}
		if strings.HasPrefix(media.ContentType, "image/") {
			attachment.ThumbnailURL = m.mediaStore.GetThumbnailURL(tenantID, media.UUID)
		}
		attachments = append(attachments, attachment)
	}

	return attachments, nil
}

// attachAttachmentsToMessage attaches attachment blobs to message.
func (m *Manager) attachAttachmentsToMessage(message *models.Message) error {
	attachments, err := m.fetchMessageAttachments(message.TenantID, message.ID)
	if err != nil {
		m.lo.Error("error fetching message attachments", "error", err)
		return err
	}

	// Attach attachments.
	message.Attachments = attachments

	return nil
}

// getOutgoingProcessingMessageIDs returns the IDs of outgoing messages currently being processed.
func (m *Manager) getOutgoingProcessingMessageIDs() []int {
	var out = make([]int, 0)
	m.outgoingProcessingMessages.Range(func(key, _ any) bool {
		if k, ok := key.(int); ok {
			out = append(out, k)
		}
		return true
	})
	return out
}

// uploadThumbnailForMedia prepares and uploads a thumbnail for an image attachment.
func (m *Manager) uploadThumbnailForMedia(media mmodels.Media, content []byte) error {
	// Create a reader from the content
	file := bytes.NewReader(content)

	// Seek to the beginning of the file
	file.Seek(0, 0)

	// Create the thumbnail
	thumbFile, err := image.CreateThumb(image.DefThumbSize, file)
	if err != nil {
		return fmt.Errorf("error creating thumbnail: %w", err)
	}

	// Generate thumbnail name
	thumbName := fmt.Sprintf("thumb_%s", media.UUID)

	// Upload the thumbnail
	if _, _, err := m.mediaStore.Upload(thumbName, media.ContentType, thumbFile); err != nil {
		m.lo.Error("error uploading thumbnail", "error", err)
		return fmt.Errorf("error uploading thumbnail: %w", err)
	}
	return nil
}

// ProcessIncomingMessageHooks handles automation rules, webhooks, SLA events, and other post-processing
// for incoming messages. This allows other channels to insert messages first and then call this
// function to trigger the necessary hooks.
func (m *Manager) ProcessIncomingMessageHooks(tenantID int, conversationUUID string, isNewConversation bool) error {
	// Start waiting since clock, cleared when agent replies.
	now := time.Now()
	m.UpdateConversationWaitingSince(tenantID, conversationUUID, &now)

	if m.onIncomingMessage != nil {
		m.onIncomingMessage(tenantID, conversationUUID)
	}

	// Handle new conversation events.
	if isNewConversation {
		conversation, err := m.GetConversation(tenantID, 0, conversationUUID, "")
		if err == nil {
			m.webhookStore.TriggerEvent(tenantID, wmodels.EventConversationCreated, conversation)
			m.automation.EvaluateNewConversationRules(conversation)
			if m.onNewConversation != nil {
				m.onNewConversation(tenantID, conversation.ContactID)
			}
		}
		return nil
	}

	// Snapshot before reopening so previous_* filters see the pre-reopen state.
	var previousValues map[string]string
	if preReopen, err := m.GetConversation(tenantID, 0, conversationUUID, ""); err == nil {
		previousValues = amodels.PreviousValues(preReopen)
	}

	// Reopen conversation if it's not Open.
	systemUser, err := m.userStore.GetSystemUser(tenantID)
	if err != nil {
		m.lo.Error("error fetching system user", "error", err)
	} else {
		if err := m.ReOpenConversation(tenantID, conversationUUID, systemUser); err != nil {
			m.lo.Error("error reopening conversation", "error", err)
		}
	}

	// Create SLA event for next response if a SLA is applied and has next response time set, subsequent agent replies will mark this event as met.
	// This cycle continues for next response time SLA metric.
	conversation, err := m.GetConversation(tenantID, 0, conversationUUID, "")
	if err != nil {
		m.lo.Error("error fetching conversation for incoming message hooks", "conversation_uuid", conversationUUID, "error", err)
	} else {
		// Trigger automations on incoming message event.
		m.automation.EvaluateConversationUpdateRules(conversation, amodels.EventConversationMessageIncoming, previousValues, umodels.User{ID: conversation.ContactID})

		// If assigned to an AI assistant, let it respond to this inbound customer message.
		if m.aiAgent != nil && conversation.AssignedUserID.Valid {
			m.aiAgent.HandleConversationEvent(tenantID, conversation.ID, conversation.AssignedUserID.Int)
		}

		if conversation.SLAPolicyID.Int == 0 {
			m.lo.Info("no SLA policy applied to conversation, skipping next response SLA event creation")
			return nil
		}
		if deadline, err := m.slaStore.CreateNextResponseSLAEvent(tenantID, conversation.ID, conversation.AppliedSLAID.Int, conversation.SLAPolicyID.Int, conversation.AssignedTeamID.Int); err != nil && !errors.Is(err, sla.ErrUnmetSLAEventAlreadyExists) {
			m.lo.Error("error creating next response SLA event", "conversation_id", conversation.ID, "error", err)
		} else if !deadline.IsZero() {
			m.lo.Info("next response SLA event created for conversation", "conversation_id", conversation.ID, "deadline", deadline, "sla_policy_id", conversation.SLAPolicyID.Int)
			m.BroadcastConversationUpdate(conversationUUID, map[string]any{
				"next_response_deadline_at": deadline.Format(time.RFC3339),
				"next_response_met_at":      nil,
			})
		}
	}
	return nil
}

// broadcastMessageToWidgetClients sends a message to widget clients if the conversation belongs to a livechat inbox.
func (m *Manager) broadcastMessageToWidgetClients(message *models.Message) {
	conversation, err := m.GetConversation(message.TenantID, 0, message.ConversationUUID, "")
	if err != nil {
		return
	}

	inboxInstance, err := m.inboxStore.Get(conversation.InboxID)
	if err != nil {
		return
	}

	liveChatInbox, ok := inboxInstance.(*livechat.LiveChat)
	if !ok {
		return
	}

	m.SignAttachmentURLs(message.TenantID, message.Attachments)
	m.SignAvatarURL(message.TenantID, &message.Author.AvatarURL)
	liveChatInbox.BroadcastMessageToClients(message.ConversationUUID, conversation.ContactID, models.ChatMessage{
		UUID:             message.UUID,
		Status:           message.Status,
		ConversationUUID: message.ConversationUUID,
		CreatedAt:        message.CreatedAt,
		Content:          message.Content,
		TextContent:      message.TextContent,
		Author:           message.Author,
		Attachments:      message.Attachments,
		Meta:             message.Meta,
	})
}

// getMediaPreview returns a localized preview string based on attachment type.
func (m *Manager) getMediaPreview(media mmodels.Media) string {
	contentType := media.ContentType
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return m.i18n.For(media.TenantID).T("globals.terms.image")
	case strings.HasPrefix(contentType, "video/"):
		return m.i18n.For(media.TenantID).T("globals.terms.video")
	case strings.HasPrefix(contentType, "audio/"):
		return m.i18n.For(media.TenantID).T("globals.terms.audio")
	default:
		return m.i18n.For(media.TenantID).T("globals.terms.file")
	}
}

// inlineContentID lowercases the uuid to match the content_id the DB stamps from uuid::TEXT.
func inlineContentID(uuid string) string {
	return "ldsk-" + strings.ToLower(uuid)
}

// findExistingMedia resolves an inbound cid to its stored form: ldsk-* is left as-is, others are namespaced by conversation to avoid cross-conversation collisions.
func (m *Manager) findExistingMedia(tenantID int, rawContentID, conversationUUID string) (string, bool, string) {
	storedCID := rawContentID
	if !strings.HasPrefix(rawContentID, "ldsk-") {
		storedCID = conversationUUID + "_" + rawContentID
	}
	exists, mediaUUID, err := m.mediaStore.ContentIDExists(tenantID, storedCID, conversationUUID)
	if err != nil {
		m.lo.Error("error checking media existence by content ID", "content_id", storedCID, "error", err)
	}
	return storedCID, exists, mediaUUID
}

// emailFromAddress returns the From header, applying the inbox from-name template for agent senders
// Falls back to the inbox's default from address if the template is empty, the sender is not an agent, or any errors occur.
func (m *Manager) emailFromAddress(inb inbox.Inbox, message models.Message) string {
	from := inb.FromAddress()

	tpl := inb.FromNameTemplate()
	if tpl == "" || message.SenderType != models.SenderTypeAgent {
		return from
	}

	agent, err := m.userStore.GetAgentCachedOrLoad(message.TenantID, message.SenderID)
	if err != nil {
		m.lo.Error("error fetching agent for from name template", "error", err, "sender_id", message.SenderID)
		return from
	}
	if agent.IsSystemUser() {
		return from
	}

	addr, err := mail.ParseAddress(from)
	if err != nil {
		m.lo.Error("error parsing inbox from address for name template", "error", err, "from", from)
		return from
	}

	firstName := strings.TrimSpace(agent.FirstName)
	lastName := strings.TrimSpace(agent.LastName)
	data := fromNameVars{
		Agent: fromNameAgent{
			FirstName: firstName,
			LastName:  lastName,
			FullName:  strings.TrimSpace(firstName + " " + lastName),
		},
		Inbox: fromNameInbox{Name: inb.Name()},
	}

	t, err := template.New("from").Parse(tpl)
	if err != nil {
		m.lo.Error("error parsing from name template", "error", err, "template", tpl)
		return from
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		m.lo.Error("error executing from name template", "error", err, "template", tpl)
		return from
	}

	name := strings.TrimSpace(buf.String())
	if name == "" {
		return from
	}
	addr.Name = name
	return addr.String()
}
