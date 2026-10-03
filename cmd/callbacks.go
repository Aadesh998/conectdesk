package main

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/authz"
	authzmodels "github.com/conectdesk/conectdesk/internal/authz/models"
	cbmodels "github.com/conectdesk/conectdesk/internal/callback/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/voice"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/whatsapp"
	notifier "github.com/conectdesk/conectdesk/internal/notification"
	nmodels "github.com/conectdesk/conectdesk/internal/notification/models"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	wsmodels "github.com/conectdesk/conectdesk/internal/ws/models"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
)

// missedCallWhatsAppEvery is how often a caller may be sent the missed-call
// WhatsApp message at most, however many calls they miss.
const missedCallWhatsAppEvery = 24 * time.Hour

// onCallEnded turns how a call ended into call-back work, by plain rules:
//   - an inbound call nobody answered opens a call-back for the caller, or adds
//     to their open one, and a new one tells the agents and messages the
//     caller on WhatsApp;
//   - a call with the contact that connected, either way, closes it;
//   - an agent's call back that didn't connect counts as an attempt.
//
// agentID is the agent who took an inbound call, or 0.
func onCallEnded(app *App, v *voice.Voice, tid int, ev voice.CallEvent, agentID int) {
	cm, found, err := app.conversation.GetCall(tid, callSourceID(ev.SID))
	if err != nil || !found {
		return
	}
	conv, err := app.conversation.GetConversation(tid, cm.ConversationID, "", "")
	if err != nil {
		app.lo.Error("error fetching conversation of a call", "call_sid", ev.SID, "error", err)
		return
	}
	if ev.Direction == voice.DirectionOutbound {
		if call, ok := cm.Meta[voice.MetaCallKey].(map[string]any); ok {
			if id, ok := call["agent_id"].(float64); ok {
				agentID = int(id)
			}
		}
	}

	switch {
	case ev.Direction == voice.DirectionInbound && isMissed(ev.Status):
		cfg := v.Config().MissedCalls
		if cfg.NoCallBacks {
			return
		}
		number := voice.E164(ev.From)
		if len(number) < 9 {
			// A withheld number can't be called back.
			return
		}
		cb, created, err := app.callback.RecordMissed(tid, cbmodels.Missed{
			ContactID:      conv.ContactID,
			ConversationID: conv.ID,
			InboxID:        v.Identifier(),
			Phone:          number,
			CallSID:        ev.SID,
			Voicemail:      ev.Status == voice.StatusVoicemail,
			DueIn:          cfg.Due(),
		})
		if err != nil {
			return
		}
		callBacksChanged(app, tid)
		if created {
			app.lo.Info("call-back opened for a missed call", "call_back_id", cb.ID, "call_sid", ev.SID)
			notifyCallBack(app, tid, cb, false)
			go sendMissedCallWhatsApp(app, tid, cfg, cb)
		}

	case ev.Status == voice.StatusCompleted:
		reason := cbmodels.ReasonCalledBack
		if ev.Direction == voice.DirectionInbound {
			reason = cbmodels.ReasonAnswered
		}
		if id, _ := app.callback.CloseForContact(tid, conv.ContactID, reason, agentID); id > 0 {
			app.lo.Info("call-back closed by a connected call", "call_back_id", id, "call_sid", ev.SID, "reason", reason)
			callBacksChanged(app, tid)
		}

	case ev.Direction == voice.DirectionOutbound:
		if id, _ := app.callback.AddAttempt(tid, conv.ContactID); id > 0 {
			callBacksChanged(app, tid)
		}
	}
}

// isMissed reports whether an inbound call's outcome means nobody spoke to the
// caller: unanswered, every agent busy, or only a voicemail left.
func isMissed(status string) bool {
	return status == voice.StatusNoAnswer || status == voice.StatusBusy || status == voice.StatusVoicemail
}

// callBacksChanged tells the workspace's agents to fetch their call-backs
// again; each only gets the ones they may see.
func callBacksChanged(app *App, tid int) {
	app.conversation.BroadcastToTenant(tid, wsmodels.MessageTypeCallBacksChanged, map[string]any{})
}

// notifyCallBack tells the agents a call-back belongs to about it: the phone
// conversation's agent, else its team, else the agents who take calls. An
// overdue one also goes to the agents who see reports.
func notifyCallBack(app *App, tid int, cb cbmodels.CallBack, overdue bool) {
	var recipients []int
	switch {
	case cb.AssignedUserID.Valid:
		recipients = []int{int(cb.AssignedUserID.Int)}
	case cb.AssignedTeamID.Valid:
		if members, err := app.team.GetMembers(tid, int(cb.AssignedTeamID.Int)); err == nil {
			for _, m := range members {
				recipients = append(recipients, m.ID)
			}
		}
	default:
		if agents, err := app.user.AgentsWithPhone(tid); err == nil {
			for _, a := range agents {
				recipients = append(recipients, a.ID)
			}
		}
	}
	if overdue {
		if ids, err := app.callback.ManagerIDs(tid); err == nil {
			recipients = append(recipients, ids...)
		}
	}
	slices.Sort(recipients)
	recipients = slices.Compact(recipients)
	if len(recipients) == 0 {
		return
	}

	t := app.i18n.For(tid)
	name := cb.ContactName()
	if name == "" {
		name = cb.Phone
	}
	typ, title := nmodels.NotificationTypeCallBack, t.Ts("callBack.notifyTitle", "name", name)
	if overdue {
		typ, title = nmodels.NotificationTypeCallBackOverdue, t.Ts("callBack.notifyOverdueTitle", "name", name)
	}
	body := t.Ts("callBack.notifyBody", "phone", cb.Phone, "count", strconv.Itoa(cb.MissedCalls))
	if cb.Voicemail {
		body += " · " + t.T("callBack.voicemailLeft")
	}
	meta, _ := json.Marshal(map[string]any{"call_back_id": cb.ID, "phone": cb.Phone})
	app.notifDispatcher.Send(notifier.Notification{
		TenantID:         tid,
		Type:             typ,
		RecipientIDs:     recipients,
		Title:            title,
		Body:             null.StringFrom(body),
		ConversationID:   null.IntFrom(cb.ConversationID),
		ConversationUUID: cb.ConversationUUID,
		Meta:             meta,
	})
}

// onCallBackOverdue is the overdue checker's action: the agents it belongs to
// and the managers hear about it once.
func onCallBackOverdue(app *App) func(int, cbmodels.CallBack) {
	return func(tid int, cb cbmodels.CallBack) {
		notifyCallBack(app, tid, cb, true)
		callBacksChanged(app, tid)
	}
}

// sendMissedCallWhatsApp messages a caller nobody answered on WhatsApp, from
// the phone line's WhatsApp inbox and with its approved template, or its
// after-hours one outside business hours. The message goes out through the
// outgoing queue like any reply, in the contact's WhatsApp conversation, so
// their answer lands next to it. A caller is messaged once a day at most.
func sendMissedCallWhatsApp(app *App, tid int, cfg voice.MissedCallConfig, cb cbmodels.CallBack) {
	if cfg.WhatsAppInboxID == 0 || cfg.Template.Name == "" {
		return
	}
	if sent, err := app.callback.WhatsAppSentSince(tid, cb.ContactID, time.Now().Add(-missedCallWhatsAppEvery)); err != nil || sent {
		return
	}
	ref := cfg.Template
	if cfg.AfterHoursTemplate.Name != "" && !businessOpen(app, tid, time.Now()) {
		ref = cfg.AfterHoursTemplate
	}

	fail := func(err error) {
		app.lo.Error("error sending missed-call whatsapp message", "call_back_id", cb.ID, "error", err)
		app.callback.SetWhatsApp(tid, cb.ID, 0, err.Error())
		callBacksChanged(app, tid)
	}

	name := cb.ContactFirstName
	if looksLikeNumber(name) {
		name = app.i18n.For(tid).T("callBack.templateNameFallback")
	}
	req := whatsapp.TemplateRequest{
		Name:     ref.Name,
		Language: ref.Language,
		Body:     ref.Values(name, cb.Phone, tenantConsts(app, tid).SiteName),
	}
	content, meta, err := prepareWhatsAppTemplate(app, tid, cfg.WhatsAppInboxID, req, nil)
	if err != nil {
		fail(err)
		return
	}

	convUUID, err := app.conversation.LatestContactConversation(tid, cb.ContactID, cfg.WhatsAppInboxID)
	if err != nil {
		fail(err)
		return
	}
	if convUUID == "" {
		if _, convUUID, err = app.conversation.CreateConversation(tid, cb.ContactID, cfg.WhatsAppInboxID, "", time.Now(),
			"WhatsApp: "+cb.Phone, true, nil, map[string]any{}, 0, 0); err != nil {
			fail(err)
			return
		}
	}
	sys, err := app.user.GetSystemUser(tid)
	if err != nil {
		fail(err)
		return
	}
	msg, err := app.conversation.QueueReply(tid, nil, cfg.WhatsAppInboxID, sys.ID, cb.ContactID, convUUID, content,
		[]string{whatsapp.WAIDFromPhone(cb.Phone)}, nil, nil, meta)
	if err != nil {
		fail(err)
		return
	}
	app.callback.SetWhatsApp(tid, cb.ID, msg.ID, "")
	callBacksChanged(app, tid)
}

// businessOpen reports whether the workspace's default business hours are open
// at t. Without business hours set, it's always open.
func businessOpen(app *App, tid int, t time.Time) bool {
	raw, err := app.setting.Get(tid, "app.business_hours_id")
	if err != nil {
		return true
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		var n int
		if json.Unmarshal(raw, &n) != nil {
			return true
		}
		s = strconv.Itoa(n)
	}
	id, _ := strconv.Atoi(s)
	if id <= 0 {
		return true
	}
	bh, err := app.businessHours.Get(tid, id)
	if err != nil {
		return true
	}
	return bh.IsOpen(t, app.setting.GetAppTimezone(tid))
}

// looksLikeNumber reports whether a name is only a phone number, as an
// unknown caller's is.
func looksLikeNumber(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || strings.Trim(s, "+-() .0123456789") == ""
}

// callBackScope is which call-backs an agent may see: the same as which phone
// conversations they may read.
func callBackScope(u umodels.User) cbmodels.Scope {
	has := func(p string) bool { return slices.Contains(u.Permissions, p) }
	s := cbmodels.Scope{UserID: u.ID, TeamIDs: u.Teams.IDs()}
	if !has(authzmodels.PermConversationsRead) {
		return s
	}
	s.All = has(authzmodels.PermConversationsReadAll)
	s.Assigned = has(authzmodels.PermConversationsReadAssigned)
	s.TeamAll = has(authzmodels.PermConversationsReadTeamAll)
	s.TeamInbox = has(authzmodels.PermConversationsReadTeamInbox)
	s.Unassigned = has(authzmodels.PermConversationsReadUnassigned)
	if s.TeamIDs == nil {
		s.TeamIDs = []int{}
	}
	return s
}

// handleGetCallBacks lists the call-backs the agent may see: waiting ones,
// most overdue first, or done ones with ?status=done. ?mine=true keeps the
// agent's own.
func handleGetCallBacks(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		args  = r.RequestCtx.QueryArgs()
	)
	agent, err := app.user.GetAgentCachedOrLoad(tid, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	page, _ := strconv.Atoi(string(args.Peek("page")))
	perPage, _ := strconv.Atoi(string(args.Peek("per_page")))
	list, total, err := app.callback.List(tid, callBackScope(agent), cbmodels.ListOpts{
		Status:   string(args.Peek("status")),
		MineOnly: string(args.Peek("mine")) == "true",
		Page:     page,
		PerPage:  perPage,
	})
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if perPage <= 0 {
		perPage = len(list)
	}
	return r.SendEnvelope(envelope.PageResults{Results: list, Total: total, PerPage: perPage, Page: max(page, 1)})
}

// handleGetCallBackCounts returns how many call-backs the agent may see are
// waiting and overdue, for the badge on the navigation.
func handleGetCallBackCounts(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	agent, err := app.user.GetAgentCachedOrLoad(tid, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	c, err := app.callback.Counts(tid, callBackScope(agent))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(c)
}

// handleMarkCallBackDone closes a call-back by hand, e.g. when the caller was
// reached another way.
func handleMarkCallBackDone(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	id, err := strconv.ParseInt(r.RequestCtx.UserValue("id").(string), 10, 64)
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	agent, err := app.user.GetAgentCachedOrLoad(tid, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	cb, err := app.callback.Get(tid, id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if !authz.CanReadAssignment(agent, cb.AssignedUserID, cb.AssignedTeamID) {
		return sendErrorEnvelope(r, envelope.NewError(envelope.PermissionError, app.i18n.For(tid).T("status.deniedPermission"), nil))
	}
	if err := app.callback.MarkDone(tid, id, agent.ID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	callBacksChanged(app, tid)
	return r.SendEnvelope(true)
}
