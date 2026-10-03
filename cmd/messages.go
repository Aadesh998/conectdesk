package main

import (
	"strings"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	authzModels "github.com/conectdesk/conectdesk/internal/authz/models"
	cmodels "github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/telegram"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/whatsapp"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

type messageReq struct {
	Attachments []int                  `json:"attachments"`
	Message     string                 `json:"message"`
	Private     bool                   `json:"private"`
	To          []string               `json:"to"`
	CC          []string               `json:"cc"`
	BCC         []string               `json:"bcc"`
	SenderType  string                 `json:"sender_type"`
	Mentions    []cmodels.MentionInput `json:"mentions"`
	EchoID      string                 `json:"echo_id"`
	// WhatsAppTemplate sends an approved template instead of Message, the only
	// way to reach a WhatsApp contact more than 24 hours after they last wrote.
	WhatsAppTemplate *whatsapp.TemplateRequest `json:"whatsapp_template"`
	// TelegramButtons are quick replies or links shown under a Telegram reply.
	TelegramButtons []telegram.Button `json:"telegram_buttons"`
}

// handleGetMessages returns messages for a conversation.
func handleGetMessages(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		uuid    = r.RequestCtx.UserValue("uuid").(string)
		auser   = r.RequestCtx.UserValue("user").(amodels.User)
		total   = 0
		private *bool
	)
	page, pageSize := getPagination(r)

	// Parse optional private filter (null = no filter)
	if r.RequestCtx.QueryArgs().Has("private") {
		p := r.RequestCtx.QueryArgs().GetBool("private")
		private = &p
	}

	// Parse repeated type params: ?type=incoming&type=outgoing
	var msgTypes []string
	for _, v := range r.RequestCtx.QueryArgs().PeekMulti("type") {
		msgTypes = append(msgTypes, string(v))
	}

	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Check permission
	_, err = enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	messages, pageSize, err := app.conversation.GetConversationMessages(tenantID(r), uuid, page, pageSize, private, msgTypes)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	rootURL := app.rootURL(tenantID(r))
	for i := range messages {
		total = messages[i].Total
		app.conversation.SignAttachmentURLs(tenantID(r), messages[i].Attachments)
		resolveQuotedCIDs(app, tenantID(r), &messages[i])
		resolveAttachmentCIDs(&messages[i], rootURL)
	}

	// Process CSAT status for all messages (will only affect CSAT messages)
	app.conversation.ProcessCSATStatus(tenantID(r), messages)

	// Strip CSAT UUID from agent sessions to prevent self-rating.
	if r.RequestCtx.UserValue("auth_method") != authMethodAPIKey {
		for i := range messages {
			if messages[i].HasCSAT() {
				messages[i].StripCSATUUID()
			}
		}
	}

	return r.SendEnvelope(envelope.PageResults{
		Total:      total,
		Results:    messages,
		Page:       page,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
	})
}

// handleGetMessage fetches a single from DB using the uuid.
func handleGetMessage(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		cuuid = r.RequestCtx.UserValue("cuuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Check permission
	_, err = enforceConversationAccess(app, tenantID(r), cuuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	message, err := app.conversation.GetMessage(tenantID(r), uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if message.ConversationUUID != cuuid {
		return sendErrorEnvelope(r, envelope.NewError(envelope.PermissionError, "Permission denied", nil))
	}

	// Process CSAT status for the message (will only affect CSAT messages)
	messages := []cmodels.Message{message}
	app.conversation.ProcessCSATStatus(tenantID(r), messages)
	message = messages[0]

	// Strip CSAT UUID from agent sessions to prevent self-rating.
	if r.RequestCtx.UserValue("auth_method") != authMethodAPIKey && message.HasCSAT() {
		message.StripCSATUUID()
	}

	rootURL := app.rootURL(tenantID(r))
	app.conversation.SignAttachmentURLs(tenantID(r), message.Attachments)
	resolveQuotedCIDs(app, tenantID(r), &message)
	resolveAttachmentCIDs(&message, rootURL)

	return r.SendEnvelope(message)
}

// handleRetryMessage changes message status to `pending`, so it's enqueued for sending.
func handleRetryMessage(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		cuuid = r.RequestCtx.UserValue("cuuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)

	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Check permission
	_, err = enforceConversationAccess(app, tenantID(r), cuuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Only outgoing agent messages that have failed can be retried.
	msg, err := app.conversation.GetMessage(tenantID(r), uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if msg.SenderType != cmodels.SenderTypeAgent || msg.Status != cmodels.MessageStatusFailed || msg.SenderID != user.ID || msg.ConversationUUID != cuuid {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	if err = app.conversation.MarkMessageAsPending(tenantID(r), uuid); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleSendMessage sends a message in a conversation.
func handleSendMessage(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		cuuid = r.RequestCtx.UserValue("cuuid").(string)
		req   = messageReq{}
	)

	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Check access to conversation.
	conv, err := enforceConversationAccess(app, tenantID(r), cuuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error unmarshalling message request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	// Make sure the inbox is enabled.
	inbox, err := app.inbox.GetDBRecord(tenantID(r), conv.InboxID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if !inbox.Enabled {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("status.disabledInbox"), nil, envelope.InputError)
	}

	if req.SenderType != umodels.UserTypeAgent && req.SenderType != umodels.UserTypeContact {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Contacts cannot send private messages
	if req.SenderType == umodels.UserTypeContact && req.Private {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.badRequest"), nil, envelope.InputError)
	}

	// Check if user has permission to send messages as contact
	if req.SenderType == umodels.UserTypeContact {
		parts := strings.Split(authzModels.PermMessagesWriteAsContact, ":")
		if len(parts) != 2 {
			app.lo.Error("error parsing permission string", "permission", authzModels.PermMessagesWriteAsContact)
			return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
		}
		ok, err := app.authz.Enforce(user, parts[0], parts[1])
		if err != nil {
			app.lo.Error("error checking permission", "error", err)
			return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
		}
		if !ok {
			return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.For(tenantID(r)).T("status.deniedPermission"), nil, envelope.PermissionError)
		}
	}

	// Get media for all attachments, skip any already associated with a model.
	media, err := getUnassociatedMedia(app, tenantID(r), req.Attachments)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	rootURL := app.rootURL(tenantID(r))

	// Create contact message.
	if req.SenderType == umodels.UserTypeContact {
		message, err := app.conversation.CreateContactMessage(tenantID(r), media, int(conv.ContactID), cuuid, req.Message, cmodels.ContentTypeHTML, false)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		resolveQuotedCIDs(app, tenantID(r), &message)
		resolveAttachmentCIDs(&message, rootURL)
		return r.SendEnvelope(message)
	}

	// Send private note.
	if req.Private {
		message, err := app.conversation.SendPrivateNote(tenantID(r), media, user.ID, cuuid, req.Message, req.Mentions)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		resolveAttachmentCIDs(&message, rootURL)
		return r.SendEnvelope(message)
	}

	// Queue outgoing reply.
	meta := map[string]any{}
	if req.EchoID != "" {
		meta["echo_id"] = req.EchoID
	}

	if len(req.TelegramButtons) > 0 {
		if inbox.Channel != telegram.ChannelTelegram {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.badRequest"), nil, envelope.InputError)
		}
		for i := range req.TelegramButtons {
			req.TelegramButtons[i].Text = strings.TrimSpace(req.TelegramButtons[i].Text)
			req.TelegramButtons[i].URL = strings.TrimSpace(req.TelegramButtons[i].URL)
		}
		if err := telegram.ValidateButtons(req.TelegramButtons); err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("telegram.invalidButtons", "error", err.Error()), nil, envelope.InputError)
		}
		meta[telegram.MetaButtonsKey] = req.TelegramButtons
	}

	// WhatsApp only takes a free-form reply within 24 hours of the contact's
	// last message; after that, only a template. Checked here so the agent is
	// told at once, rather than Meta reporting the message failed later.
	if inbox.Channel == whatsapp.ChannelWhatsApp {
		if req.WhatsAppTemplate != nil {
			content, tmplMeta, err := prepareWhatsAppTemplate(app, tenantID(r), conv.InboxID, *req.WhatsAppTemplate, media)
			if err != nil {
				return sendErrorEnvelope(r, err)
			}
			req.Message = content
			for k, v := range tmplMeta {
				meta[k] = v
			}
		} else {
			window, err := whatsAppWindow(app, tenantID(r), conv.ContactID, conv.InboxID)
			if err != nil {
				return sendErrorEnvelope(r, err)
			}
			if !window.Open {
				return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("whatsapp.windowClosed"), nil, envelope.InputError)
			}
		}
	}
	message, err := app.conversation.QueueReply(tenantID(r), media, conv.InboxID, user.ID, conv.ContactID, cuuid, req.Message, req.To, req.CC, req.BCC, meta)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	markAssignmentNotificationRead(app, tenantID(r), conv, user)
	resolveQuotedCIDs(app, tenantID(r), &message)
	resolveAttachmentCIDs(&message, rootURL)
	return r.SendEnvelope(message)
}

// resolveAttachmentCIDs replaces inline image cid: references in email message content
// with actual attachment URLs and resolves relative /uploads/ paths to absolute URLs.
func resolveAttachmentCIDs(msg *cmodels.Message, rootURL string) {
	for _, att := range msg.Attachments {
		if att.ContentID != "" && att.URL != "" {
			msg.Content = strings.ReplaceAll(msg.Content, "cid:"+att.ContentID, att.URL)
		}
	}
	if rootURL != "" {
		msg.Content = strings.ReplaceAll(msg.Content, `src="/uploads/`, `src="`+rootURL+`/uploads/`)
		msg.Content = strings.ReplaceAll(msg.Content, `src='/uploads/`, `src='`+rootURL+`/uploads/`)
	}
}

// resolveQuotedCIDs replaces cid: refs to media on other messages with signed URLs.
func resolveQuotedCIDs(app *App, tenantID int, msg *cmodels.Message) {
	refs, err := app.conversation.GetInlineMediaRefs(msg)
	if err != nil {
		app.lo.Error("error fetching inline media refs", "conversation_uuid", msg.ConversationUUID, "error", err)
		return
	}
	for _, ref := range refs {
		url := app.media.GetURL(tenantID, ref.UUID, ref.ContentType, ref.Filename)
		msg.Content = strings.ReplaceAll(msg.Content, "cid:"+ref.ContentID, url)
	}
}
