package main

import (
	"encoding/json"
	"fmt"
	"mime"
	"slices"
	"strconv"
	"strings"
	"time"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	authzModels "github.com/conectdesk/conectdesk/internal/authz/models"
	"github.com/conectdesk/conectdesk/internal/automation/models"
	cmodels "github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/voice"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/whatsapp"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	vmodels "github.com/conectdesk/conectdesk/internal/view/models"
	wmodels "github.com/conectdesk/conectdesk/internal/webhook/models"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
)

type assigneeChangeReq struct {
	AssigneeID int `json:"assignee_id"`
}

type teamAssigneeChangeReq struct {
	AssigneeID int `json:"assignee_id"`
}

type priorityUpdateReq struct {
	Priority string `json:"priority"`
}

type statusUpdateReq struct {
	Status       string `json:"status"`
	SnoozedUntil string `json:"snoozed_until,omitempty"`
}

type tagsUpdateReq struct {
	Tags   []string `json:"tags"`
	Action string   `json:"action,omitempty"`
}

type createConversationRequest struct {
	InboxID          int            `json:"inbox_id"`
	AssignedAgentID  int            `json:"agent_id"`
	AssignedTeamID   int            `json:"team_id"`
	Email            string         `json:"contact_email"`
	PhoneNumber      string         `json:"contact_phone_number"`
	FirstName        string         `json:"first_name"`
	LastName         string         `json:"last_name"`
	ExternalUserID   string         `json:"external_user_id"`
	ReuseContact     bool           `json:"reuse_contact"`
	Subject          string         `json:"subject"`
	Content          string         `json:"content"`
	Attachments      []int          `json:"attachments"`
	Initiator        string         `json:"initiator"` // "contact" | "agent"
	CustomAttributes map[string]any `json:"custom_attributes"`
	// WhatsAppTemplate opens a WhatsApp conversation, which only a template can.
	WhatsAppTemplate *whatsapp.TemplateRequest `json:"whatsapp_template"`
}

// handleGetAllConversations retrieves all conversations.
func handleGetAllConversations(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		user    = r.RequestCtx.UserValue("user").(amodels.User)
		order   = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		filters = string(r.RequestCtx.QueryArgs().Peek("filters"))
		total   = 0
	)
	page, pageSize := getPagination(r)

	conversations, err := app.conversation.GetAllConversationsList(tenantID(r), user.ID, order, orderBy, filters, page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if len(conversations) > 0 {
		total = conversations[0].Total
	}

	return r.SendEnvelope(envelope.PageResults{
		Results:    conversations,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetAssignedConversations retrieves conversations assigned to the current user.
func handleGetAssignedConversations(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		user    = r.RequestCtx.UserValue("user").(amodels.User)
		order   = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		filters = string(r.RequestCtx.QueryArgs().Peek("filters"))
		total   = 0
	)
	page, pageSize := getPagination(r)
	conversations, err := app.conversation.GetAssignedConversationsList(tenantID(r), user.ID, user.ID, order, orderBy, filters, page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if len(conversations) > 0 {
		total = conversations[0].Total
	}

	return r.SendEnvelope(envelope.PageResults{
		Results:    conversations,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetUnassignedConversations retrieves unassigned conversations.
func handleGetUnassignedConversations(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		user    = r.RequestCtx.UserValue("user").(amodels.User)
		order   = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		filters = string(r.RequestCtx.QueryArgs().Peek("filters"))
		total   = 0
	)
	page, pageSize := getPagination(r)

	conversations, err := app.conversation.GetUnassignedConversationsList(tenantID(r), user.ID, order, orderBy, filters, page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if len(conversations) > 0 {
		total = conversations[0].Total
	}

	return r.SendEnvelope(envelope.PageResults{
		Results:    conversations,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetMentionedConversations retrieves conversations where the current user is mentioned.
func handleGetMentionedConversations(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		user    = r.RequestCtx.UserValue("user").(amodels.User)
		order   = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		filters = string(r.RequestCtx.QueryArgs().Peek("filters"))
		total   = 0
	)
	page, pageSize := getPagination(r)

	conversations, err := app.conversation.GetMentionedConversationsList(tenantID(r), user.ID, order, orderBy, filters, page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if len(conversations) > 0 {
		total = conversations[0].Total
	}

	return r.SendEnvelope(envelope.PageResults{
		Results:    conversations,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetViewConversations retrieves conversations for a view.
func handleGetViewConversations(r *fastglue.Request) error {
	var (
		app       = r.Context.(*App)
		auser     = r.RequestCtx.UserValue("user").(amodels.User)
		viewID, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		order     = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy   = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		total     = 0
	)
	page, pageSize := getPagination(r)
	if viewID < 1 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Check if user has access to the view.
	view, err := app.view.Get(tenantID(r), viewID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	hasAccess := false
	switch view.Visibility {
	case vmodels.VisibilityUser:
		hasAccess = view.UserID != nil && *view.UserID == auser.ID
	case vmodels.VisibilityAll:
		hasAccess = true
	case vmodels.VisibilityTeam:
		if view.TeamID != nil {
			hasAccess = slices.Contains(user.Teams.IDs(), *view.TeamID)
		}
	}

	if !hasAccess {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.For(tenantID(r)).T("conversation.viewPermissionDenied"), nil, envelope.PermissionError)
	}

	// Prepare lists user has access to based on user permissions, internally this prepares the SQL query.
	lists := []string{}
	hasTeamAll := slices.Contains(user.Permissions, authzModels.PermConversationsReadTeamAll)
	for _, perm := range user.Permissions {
		if perm == authzModels.PermConversationsReadAll {
			// No further lists required as user has access to all conversations.
			lists = []string{cmodels.AllConversations}
			break
		}
		if perm == authzModels.PermConversationsReadUnassigned {
			lists = append(lists, cmodels.UnassignedConversations)
		}
		if perm == authzModels.PermConversationsReadAssigned {
			lists = append(lists, cmodels.AssignedConversations)
		}
		// Skip TeamUnassignedConversations if user has TeamAllConversations (superset).
		if perm == authzModels.PermConversationsReadTeamInbox && !hasTeamAll {
			lists = append(lists, cmodels.TeamUnassignedConversations)
		}
		if perm == authzModels.PermConversationsReadTeamAll {
			lists = append(lists, cmodels.TeamAllConversations)
		}
	}

	// No lists found, user doesn't have access to any conversations.
	if len(lists) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.For(tenantID(r)).T("status.deniedPermission"), nil, envelope.PermissionError)
	}

	conversations, err := app.conversation.GetViewConversationsList(tenantID(r), user.ID, user.ID, user.Teams.IDs(), lists, order, orderBy, string(view.Filters), page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if len(conversations) > 0 {
		total = conversations[0].Total
	}

	return r.SendEnvelope(envelope.PageResults{
		Results:    conversations,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetTeamUnassignedConversations returns conversations assigned to a team but not to any user.
func handleGetTeamUnassignedConversations(r *fastglue.Request) error {
	var (
		app       = r.Context.(*App)
		auser     = r.RequestCtx.UserValue("user").(amodels.User)
		teamIDStr = r.RequestCtx.UserValue("id").(string)
		order     = string(r.RequestCtx.QueryArgs().Peek("order"))
		orderBy   = string(r.RequestCtx.QueryArgs().Peek("order_by"))
		filters   = string(r.RequestCtx.QueryArgs().Peek("filters"))
		total     = 0
	)
	page, pageSize := getPagination(r)
	teamID, _ := strconv.Atoi(teamIDStr)
	if teamID < 1 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Check if user belongs to the team.
	exists, err := app.team.UserBelongsToTeam(tenantID(r), teamID, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if !exists {
		return sendErrorEnvelope(r, envelope.NewError(envelope.PermissionError, app.i18n.For(tenantID(r)).T("conversation.notMemberOfTeam"), nil))
	}

	conversations, err := app.conversation.GetTeamUnassignedConversationsList(tenantID(r), auser.ID, teamID, order, orderBy, filters, page, pageSize)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if len(conversations) > 0 {
		total = conversations[0].Total
	}

	return r.SendEnvelope(envelope.PageResults{
		Results:    conversations,
		Total:      total,
		PerPage:    pageSize,
		TotalPages: (total + pageSize - 1) / pageSize,
		Page:       page,
	})
}

// handleGetConversation retrieves a single conversation by it's UUID.
func handleGetConversation(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)

	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	conv, err := enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	prev, _ := app.conversation.GetContactPreviousConversations(tenantID(r), conv.ContactID, 10)
	conv.PreviousConversations = filterCurrentPreviousConv(prev, conv.UUID)
	return r.SendEnvelope(conv)
}

// handleDownloadConversationTranscript sends the conversation transcript as a text file download.
func handleDownloadConversationTranscript(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)

	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	conversation, err := enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	private := false
	messages, err := app.conversation.GetAllConversationMessages(tenantID(r), uuid, &private, []string{cmodels.MessageIncoming, cmodels.MessageOutgoing}, 0)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	transcript := app.conversation.BuildTranscript(*conversation, messages, time.Now())
	safeRef := stringutil.SanitizeFilename(conversation.ReferenceNumber)
	filename := fmt.Sprintf("transcript-%s.txt", safeRef)
	r.RequestCtx.Response.Header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	r.RequestCtx.Response.Header.Set("X-Content-Type-Options", "nosniff")
	r.RequestCtx.SetContentType("text/plain; charset=utf-8")
	r.RequestCtx.SetBody(transcript)
	return nil
}

// handleGetContactPageVisits returns the recent page visits for the contact of a conversation.
func handleGetContactPageVisits(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)

	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	conv, err := enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	pages := getPageVisitsFromRedis(app, conv.ContactID)
	return r.SendEnvelope(pages)
}

// handleUpdateConversationAssigneeLastSeen updates the current user's last seen timestamp for a conversation.
func handleUpdateConversationAssigneeLastSeen(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err = app.conversation.UpdateUserLastSeen(tenantID(r), uuid, auser.ID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleMarkConversationAsUnread marks a conversation as unread for the current user.
func handleMarkConversationAsUnread(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err = app.conversation.MarkAsUnread(tenantID(r), uuid, auser.ID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleGetConversationParticipants retrieves participants of a conversation.
func handleGetConversationParticipants(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	p, err := app.conversation.GetConversationParticipants(tenantID(r), uuid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(p)
}

// handleUpdateUserAssignee updates the user assigned to a conversation.
func handleUpdateUserAssignee(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		req   = assigneeChangeReq{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error decoding assignee change request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	conversation, err := enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Already assigned?
	if conversation.AssignedUserID.Int == req.AssigneeID {
		return r.SendEnvelope(true)
	}

	if err := app.conversation.UpdateConversationUserAssignee(tenantID(r), uuid, req.AssigneeID, user); err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(true)
}

// handleUpdateTeamAssignee updates the team assigned to a conversation.
func handleUpdateTeamAssignee(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		req   = teamAssigneeChangeReq{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error decoding team assignee change request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	assigneeID := req.AssigneeID

	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	_, err = app.team.Get(tenantID(r), assigneeID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	conversation, err := enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Already assigned?
	if conversation.AssignedTeamID.Int == assigneeID {
		return r.SendEnvelope(true)
	}
	if err := app.conversation.UpdateConversationTeamAssignee(tenantID(r), uuid, assigneeID, user); err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(true)
}

// handleUpdateConversationPriority updates the priority of a conversation.
func handleUpdateConversationPriority(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		req   = priorityUpdateReq{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error decoding priority update request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	priority := req.Priority
	if priority == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.empty", "name", "`priority`"), nil, envelope.InputError)
	}

	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := app.conversation.UpdateConversationPriority(tenantID(r), uuid, 0 /**priority_id**/, priority, user); err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(true)
}

// handleUpdateConversationStatus updates the status of a conversation.
func handleUpdateConversationStatus(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		req   = statusUpdateReq{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error decoding status update request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	status := req.Status
	snoozedUntil := req.SnoozedUntil

	// Validate inputs
	if status == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.empty", "name", "`status`"), nil, envelope.InputError)
	}
	if snoozedUntil == "" && status == cmodels.StatusSnoozed {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.empty", "name", "`snoozed_until`"), nil, envelope.InputError)
	}
	if status == cmodels.StatusSnoozed {
		_, err := time.ParseDuration(snoozedUntil)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.badRequest"), nil, envelope.InputError)
		}
	}

	// Enforce conversation access.
	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conversation, err := enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Update conversation status.
	if err := app.conversation.UpdateConversationStatus(tenantID(r), uuid, 0 /**status_id**/, status, snoozedUntil, user); err != nil {
		return sendErrorEnvelope(r, err)
	}
	markAssignmentNotificationRead(app, tenantID(r), conversation, user)
	return r.SendEnvelope(true)
}

// handleUpdateConversationtags updates conversation tags.
func handleUpdateConversationtags(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		req   = tagsUpdateReq{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error decoding tags update request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	tagNames := req.Tags

	// Default to set tags if action is not provided (backwards compatibility).
	action := models.ActionSetTags
	switch req.Action {
	case models.ActionAddTags, models.ActionRemoveTags, models.ActionSetTags:
		action = req.Action
	case "":
	default:
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err := app.conversation.SetConversationTags(tenantID(r), uuid, action, tagNames, user); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleUpdateConversationCustomAttributes updates custom attributes of a conversation.
func handleUpdateConversationCustomAttributes(r *fastglue.Request) error {
	var (
		app        = r.Context.(*App)
		attributes = map[string]any{}
		auser      = r.RequestCtx.UserValue("user").(amodels.User)
		uuid       = r.RequestCtx.UserValue("uuid").(string)
	)
	if err := r.Decode(&attributes, ""); err != nil {
		app.lo.Error("error unmarshalling custom attributes JSON", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	// Enforce conversation access.
	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Update custom attributes.
	if err := app.conversation.UpdateConversationCustomAttributes(tenantID(r), uuid, attributes); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleUpdateContactCustomAttributes updates custom attributes of a contact.
func handleUpdateContactCustomAttributes(r *fastglue.Request) error {
	var (
		app        = r.Context.(*App)
		attributes = map[string]any{}
		auser      = r.RequestCtx.UserValue("user").(amodels.User)
		uuid       = r.RequestCtx.UserValue("uuid").(string)
	)
	if err := r.Decode(&attributes, ""); err != nil {
		app.lo.Error("error unmarshalling custom attributes JSON", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	// Enforce conversation access.
	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conversation, err := enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := app.user.SaveCustomAttributes(tenantID(r), conversation.ContactID, attributes, false); err != nil {
		return sendErrorEnvelope(r, err)
	}
	// Broadcast update.
	app.conversation.BroadcastContactUpdate(tenantID(r), conversation.ContactID, map[string]any{"custom_attributes": attributes})
	return r.SendEnvelope(true)
}

// enforceConversationAccess fetches the conversation and checks if the user has access to it.
func enforceConversationAccess(app *App, tenantID int, uuid string, user umodels.User) (*cmodels.Conversation, error) {
	conversation, err := app.conversation.GetConversation(tenantID, 0, uuid, "")
	if err != nil {
		return nil, err
	}
	allowed, err := app.authz.EnforceConversationAccess(user, conversation)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, envelope.NewError(envelope.PermissionError, "Permission denied", nil)
	}
	return &conversation, nil
}

// handleRemoveUserAssignee removes the user assigned to a conversation.
func handleRemoveUserAssignee(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err = app.conversation.RemoveConversationAssignee(tenantID(r), uuid, "user", user); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleRemoveTeamAssignee removes the team assigned to a conversation.
func handleRemoveTeamAssignee(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	_, err = enforceConversationAccess(app, tenantID(r), uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err = app.conversation.RemoveConversationAssignee(tenantID(r), uuid, "team", user); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// filterCurrentPreviousConv removes the current conversation from the list of previous conversations.
func filterCurrentPreviousConv(convs []cmodels.PreviousConversation, uuid string) []cmodels.PreviousConversation {
	for i, c := range convs {
		if c.UUID == uuid {
			return append(convs[:i], convs[i+1:]...)
		}
	}
	return []cmodels.PreviousConversation{}
}

// handleCreateConversation creates a new conversation and sends a message to it.
func handleCreateConversation(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		req   = createConversationRequest{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		app.lo.Error("error decoding create conversation request", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)

	channel, err := createConversationChannel(req, app, tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err := validateCreateConversationRequest(req, channel, app, tenantID(r)); err != nil {
		return sendErrorEnvelope(r, err)
	}

	email := req.Email
	to := []string{email}
	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	contact := umodels.User{
		Email:            null.NewString(email, email != ""),
		FirstName:        req.FirstName,
		LastName:         req.LastName,
		ExternalUserID:   null.NewString(req.ExternalUserID, req.ExternalUserID != ""),
		CustomAttributes: json.RawMessage(`{}`),
	}

	// A WhatsApp conversation opens with a template, prepared before anything
	// is created so a bad value doesn't leave an empty conversation behind.
	replyMeta := map[string]any{}
	if channel == inbox.ChannelWhatsApp {
		headerMedia, err := getUnassociatedMedia(app, tenantID(r), req.Attachments)
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
		}
		content, tmplMeta, err := prepareWhatsAppTemplate(app, tenantID(r), req.InboxID, *req.WhatsAppTemplate, headerMedia)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		req.Content, replyMeta = content, tmplMeta
		if strings.TrimSpace(req.Subject) == "" {
			req.Subject = "WhatsApp: " + req.PhoneNumber
		}

		waID := whatsapp.WAIDFromPhone(req.PhoneNumber)
		to = []string{waID}
		contact.PhoneNumber = null.StringFrom(waID)
		if req.ExternalUserID == "" {
			contact.ExternalUserID = null.StringFrom(whatsapp.ExternalIDPrefix + waID)
		}
	}

	// A phone conversation opens with a call, so the agent needs a number of
	// their own before anything is created. The contact is keyed on their
	// number, as an incoming call from them would be.
	if channel == inbox.ChannelVoice {
		if voice.E164(user.PhoneNumber.String) == "" {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("voice.agentNoPhone"), nil, envelope.InputError)
		}
		phone := voice.E164(req.PhoneNumber)
		contact.PhoneNumber = null.StringFrom(phone)
		if req.ExternalUserID == "" {
			contact.ExternalUserID = null.StringFrom(voice.ExternalIDPrefix + voice.Digits(phone))
		}
		if strings.TrimSpace(req.Subject) == "" {
			req.Subject = "Call: " + phone
		}
	}

	canWriteContacts, err := app.authz.Enforce(user, "contacts", "write")
	if err != nil {
		app.lo.Error("error checking permission", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	policy := umodels.ContactReuse
	if canWriteContacts && !req.ReuseContact {
		policy = umodels.ContactSync
	}
	if err := app.user.ResolveContact(tenantID(r), &contact, policy); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	if channel == inbox.ChannelEmail && policy == umodels.ContactReuse && contact.Email.String != "" {
		to = []string{contact.Email.String}
	}

	// Create conversation first.
	conversationID, conversationUUID, err := app.conversation.CreateConversation(tenantID(r),
		contact.ID,
		req.InboxID,
		"",         /** last_message **/
		time.Now(), /** last_message_at **/
		req.Subject,
		true, /** append reference number to subject? **/
		nil,
		req.CustomAttributes,
		0, 0,
	)
	if err != nil {
		app.lo.Error("error creating conversation", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}

	// Get media for the attachment ids, skip any already associated with a model.
	media, err := getUnassociatedMedia(app, tenantID(r), req.Attachments)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Assign team first, it clears any assigned agent.
	if req.AssignedTeamID > 0 {
		app.conversation.UpdateConversationTeamAssignee(tenantID(r), conversationUUID, req.AssignedTeamID, user)
	}
	if req.AssignedAgentID > 0 {
		app.conversation.UpdateConversationUserAssignee(tenantID(r), conversationUUID, req.AssignedAgentID, user)
	}

	// Send initial message based on the initiator of conversation.
	switch {
	case channel == inbox.ChannelVoice:
		// Call the customer from the agent, on their phone or in their browser;
		// the call is the first message. A call that can't be placed leaves
		// nothing behind.
		if _, _, err := placeCall(app, tenantID(r), user, conversationUUID, voice.E164(req.PhoneNumber), req.InboxID); err != nil {
			if err := app.conversation.DeleteConversation(tenantID(r), conversationUUID); err != nil {
				app.lo.Error("error deleting conversation", "error", err)
			}
			return sendErrorEnvelope(r, err)
		}
		if c, err := app.conversation.GetConversation(tenantID(r), 0, conversationUUID, ""); err == nil {
			app.webhook.TriggerEvent(tenantID(r), wmodels.EventConversationCreated, c)
		}
	case req.Initiator == umodels.UserTypeAgent:
		// Queue reply.
		if _, err := app.conversation.QueueReply(tenantID(r), media, req.InboxID, auser.ID /**sender_id**/, contact.ID, conversationUUID, req.Content, to, nil /**cc**/, nil /**bcc**/, replyMeta); err != nil {
			// Delete the conversation if msg queue fails.
			if err := app.conversation.DeleteConversation(tenantID(r), conversationUUID); err != nil {
				app.lo.Error("error deleting conversation", "error", err)
			}
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.errorSendingMessage"), nil))
		}
		// Trigger webhook for agent-initiated conversation, for contact intitiated the incoming message hooks handle it.
		if c, err := app.conversation.GetConversation(tenantID(r), 0, conversationUUID, ""); err == nil {
			app.webhook.TriggerEvent(tenantID(r), wmodels.EventConversationCreated, c)
		}
	case req.Initiator == umodels.UserTypeContact:
		// Create contact message.
		if _, err := app.conversation.CreateContactMessage(tenantID(r), media, contact.ID, conversationUUID, req.Content, cmodels.ContentTypeHTML, true); err != nil {
			// Delete the conversation if message creation fails.
			if err := app.conversation.DeleteConversation(tenantID(r), conversationUUID); err != nil {
				app.lo.Error("error deleting conversation", "error", err)
			}
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.errorSendingMessage"), nil))
		}
	default:
		// Guard anyway.
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	conversation, _ := app.conversation.GetConversation(tenantID(r), conversationID, "", "")
	return r.SendEnvelope(conversation)
}

func createConversationChannel(req createConversationRequest, app *App, tenantID int) (string, error) {
	if req.InboxID <= 0 {
		return "", envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.required", "name", "`inbox_id`"), nil)
	}

	record, err := app.inbox.GetDBRecord(tenantID, req.InboxID)
	if err != nil {
		return "", err
	}
	if !record.Enabled {
		return "", envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.disabled"), nil)
	}
	// Email and WhatsApp can start a conversation, WhatsApp only with an
	// approved template, and a phone inbox with a call. Messenger and
	// Instagram only allow a business to message someone who wrote first.
	if record.Channel != inbox.ChannelEmail && record.Channel != inbox.ChannelWhatsApp && record.Channel != inbox.ChannelVoice {
		return "", envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return record.Channel, nil
}

func validateCreateConversationRequest(req createConversationRequest, channel string, app *App, tenantID int) error {
	if req.FirstName == "" {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.required", "name", "`first_name`"), nil)
	}
	if req.Initiator != umodels.UserTypeContact && req.Initiator != umodels.UserTypeAgent {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	switch channel {
	case inbox.ChannelWhatsApp:
		if !whatsapp.IsValidWAID(whatsapp.WAIDFromPhone(req.PhoneNumber)) {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("whatsapp.invalidPhoneNumber"), nil)
		}
		// The business writes first, so there is no contact message to record.
		if req.Initiator != umodels.UserTypeAgent {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
		if req.WhatsAppTemplate == nil {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("whatsapp.templateRequired"), nil)
		}
	case inbox.ChannelVoice:
		if len(voice.E164(req.PhoneNumber)) < 9 {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("voice.contactNoPhone"), nil)
		}
		// The conversation opens with the agent calling the customer.
		if req.Initiator != umodels.UserTypeAgent {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
	default:
		if req.Content == "" {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.required", "name", "`content`"), nil)
		}
		if req.Email == "" {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.required", "name", "`contact_email`"), nil)
		}
		if !stringutil.ValidEmail(req.Email) {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalidEmail"), nil)
		}
	}

	// Validate custom attribute keys. Skip unknown keys.
	if len(req.CustomAttributes) > 0 {
		attrs, err := app.customAttribute.GetAll(tenantID, "conversation")
		if err != nil {
			return err
		}
		validKeys := make(map[string]struct{}, len(attrs))
		for _, a := range attrs {
			validKeys[a.Key] = struct{}{}
		}
		for key := range req.CustomAttributes {
			if _, ok := validKeys[key]; !ok {
				delete(req.CustomAttributes, key)
			}
		}
	}

	return nil
}
