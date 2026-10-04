package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/conversation"
	cmodels "github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/voice"
	imodels "github.com/conectdesk/conectdesk/internal/inbox/models"
	"github.com/conectdesk/conectdesk/internal/user"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	wsmodels "github.com/conectdesk/conectdesk/internal/ws/models"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
)

// voiceTimeout bounds a call to Exotel made while an agent waits.
const voiceTimeout = 20 * time.Second

// voiceWebhookURL is one of a voice inbox's webhook addresses, on the
// workspace's own address, with the inbox secret as its token: Exotel doesn't
// sign its requests.
func voiceWebhookURL(app *App, record imodels.Inbox, kind string) string {
	return fmt.Sprintf("%s/webhook/voice/%s/%s?token=%s",
		strings.TrimRight(tenantConsts(app, record.TenantID).AppBaseURL, "/"), record.UUID, kind, url.QueryEscape(record.Secret.String))
}

// runningVoiceInbox returns a tenant's running voice inbox.
func runningVoiceInbox(app *App, tid, inboxID int) (*voice.Voice, imodels.Inbox, error) {
	record, err := app.inbox.GetDBRecord(tid, inboxID)
	if err != nil {
		return nil, record, err
	}
	if record.Channel != inbox.ChannelVoice {
		return nil, record, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("validation.notFoundInbox"), nil)
	}
	inb, err := app.inbox.Get(record.ID)
	if err != nil {
		return nil, record, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("status.disabledInbox"), nil)
	}
	v, ok := inb.(*voice.Voice)
	if !ok {
		return nil, record, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil)
	}
	return v, record, nil
}

// handleGetVoiceSetup returns the addresses to enter in Exotel for a voice
// inbox. They carry the inbox secret, so they come from an admin-only route.
func handleGetVoiceSetup(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	record, err := app.inbox.GetDBRecord(tid, id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if record.Channel != inbox.ChannelVoice {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("validation.notFoundInbox"), nil))
	}
	return r.SendEnvelope(map[string]string{
		"incoming":  voiceWebhookURL(app, record, "incoming"),
		"dial":      voiceWebhookURL(app, record, "dial"),
		"completed": voiceWebhookURL(app, record, "completed"),
		"voicemail": voiceWebhookURL(app, record, "voicemail"),
	})
}

// handleTestVoice checks a voice inbox's Exotel credentials.
func handleTestVoice(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	v, _, err := runningVoiceInbox(app, tid, id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	c, cancel := context.WithTimeout(app.ctx, voiceTimeout)
	defer cancel()
	if err := v.Exotel().Check(c); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tid).Ts("voice.checkFailed", "error", err.Error()), nil))
	}
	return r.SendEnvelope(true)
}

// startCallRequest picks the voice inbox a call goes out on.
type startCallRequest struct {
	InboxID int `json:"inbox_id"`
}

// handleStartCall calls a conversation's contact: Exotel rings the agent, on
// their own phone or in their browser, then connects the customer. The call is logged in the
// conversation and updated as it goes.
func handleStartCall(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		cuuid = r.RequestCtx.UserValue("uuid").(string)
		req   startCallRequest
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	agent, err := app.user.GetAgentCachedOrLoad(tid, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conv, err := enforceConversationAccess(app, tid, cuuid, agent)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	customerPhone := voice.ContactE164(conv.Contact.PhoneNumberCountryCode.String, conv.Contact.PhoneNumber.String)
	msg, _, err := placeCall(app, tid, agent, cuuid, customerPhone, req.InboxID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(msg)
}

// agentLeg is where Exotel rings an agent for a call: their browser, when
// they take calls there, else their own phone. noPhoneKey is the message for
// an agent who can take calls on neither.
func agentLeg(app *App, tid int, agent umodels.User, noPhoneKey string) (string, error) {
	acc, found, err := app.softphone.Get(tid, agent.ID)
	if err != nil {
		return "", envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil)
	}
	if found && acc.RingBrowser {
		return voice.SIPLeg(acc.Username), nil
	}
	if p := voice.E164(agent.PhoneNumber.String); p != "" {
		return p, nil
	}
	return "", envelope.NewError(envelope.InputError, app.i18n.For(tid).T(noPhoneKey), nil)
}

// placeCall rings the agent, on their phone or in their browser, then
// connects the customer from a voice inbox's ExoPhone, and logs the call in
// the conversation.
func placeCall(app *App, tid int, agent umodels.User, cuuid, customerPhone string, inboxID int) (cmodels.Message, string, error) {
	v, record, err := runningVoiceInbox(app, tid, inboxID)
	if err != nil {
		return cmodels.Message{}, "", err
	}
	leg, err := agentLeg(app, tid, agent, "voice.agentNoPhone")
	if err != nil {
		return cmodels.Message{}, "", err
	}
	if len(customerPhone) < 9 {
		return cmodels.Message{}, "", envelope.NewError(envelope.InputError, app.i18n.For(tid).T("voice.contactNoPhone"), nil)
	}

	c, cancel := context.WithTimeout(app.ctx, voiceTimeout)
	defer cancel()
	sid, err := v.Call(c, leg, customerPhone, voiceWebhookURL(app, record, "status"), cuuid)
	if err != nil {
		app.lo.Error("error starting exotel call", "conversation_uuid", cuuid, "error", err)
		return cmodels.Message{}, "", envelope.NewError(envelope.GeneralError, app.i18n.For(tid).Ts("voice.callFailed", "error", err.Error()), nil)
	}

	call := voice.Call{
		SID: sid, Provider: voice.ProviderExotel, Direction: voice.DirectionOutbound, Status: voice.StatusRinging,
		From: v.Config().CallerID, To: customerPhone, AgentID: agent.ID, AgentName: agent.FullName(),
		StartedAt: time.Now().UTC(),
	}
	msg, err := app.conversation.RecordOutgoingCall(tid, cuuid, agent.ID, callSourceID(sid),
		voice.StatusLabel(call)+" "+customerPhone, map[string]any{voice.MetaCallKey: call, "voice_inbox_id": v.Identifier()})
	if err != nil {
		app.lo.Error("error logging outgoing call", "call_sid", sid, "error", err)
	}
	app.lo.Info("outgoing call started", "conversation_uuid", cuuid, "call_sid", sid, "agent_id", agent.ID)
	return msg, sid, nil
}

func callSourceID(sid string) string { return "exotel:" + sid }

// voiceWebhookInbox authenticates a webhook request and returns its inbox.
func voiceWebhookInbox(r *fastglue.Request) (*voice.Voice, imodels.Inbox, bool) {
	app := r.Context.(*App)
	id := r.RequestCtx.UserValue("uuid").(string)
	if _, err := uuid.Parse(id); err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		return nil, imodels.Inbox{}, false
	}
	record, err := app.inbox.GetDBRecord(tenantID(r), id)
	if err != nil || record.Channel != inbox.ChannelVoice {
		r.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		return nil, record, false
	}
	token := string(r.RequestCtx.QueryArgs().Peek("token"))
	if record.Secret.String == "" || subtle.ConstantTimeCompare([]byte(token), []byte(record.Secret.String)) != 1 {
		app.lo.Warn("rejected voice webhook with a bad token", "inbox_uuid", id)
		r.RequestCtx.SetStatusCode(fasthttp.StatusUnauthorized)
		return nil, record, false
	}
	inb, err := app.inbox.Get(record.ID)
	if err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusServiceUnavailable)
		return nil, record, false
	}
	v, ok := inb.(*voice.Voice)
	if !ok {
		r.RequestCtx.SetStatusCode(fasthttp.StatusServiceUnavailable)
		return nil, record, false
	}
	return v, record, true
}

// passthruQuery returns an Exotel Passthru request's parameters: a GET's
// query, or a POST's form.
func passthruQuery(r *fastglue.Request) url.Values {
	q := url.Values{}
	r.RequestCtx.QueryArgs().VisitAll(func(k, v []byte) { q.Add(string(k), string(v)) })
	r.RequestCtx.PostArgs().VisitAll(func(k, v []byte) { q.Add(string(k), string(v)) })
	return q
}

// handleVoiceIncoming is the first Passthru of an inbound call's flow: the
// caller becomes a conversation in the voice inbox, or joins theirs, with the
// call logged as ringing. Answering 200 lets the flow go on to ring agents.
func handleVoiceIncoming(r *fastglue.Request) error {
	app := r.Context.(*App)
	v, record, ok := voiceWebhookInbox(r)
	if !ok {
		return nil
	}
	ev, err := voice.ParsePassthru(passthruQuery(r))
	if err != nil {
		app.lo.Error("error reading exotel passthru", "inbox_id", record.ID, "error", err)
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	}

	caller := voice.E164(ev.From)
	call := voice.Call{
		SID: ev.SID, Provider: voice.ProviderExotel, Direction: voice.DirectionInbound, Status: voice.StatusRinging,
		From: caller, To: v.Config().CallerID, StartedAt: time.Now().UTC(),
	}
	// The digit pressed in the flow's menu routes the call to a team.
	if opt, ok := v.Config().MenuChoice(ev.Pressed); ok {
		call.TeamID, call.Menu = opt.TeamID, opt.Label
	}
	metaJSON, _ := json.Marshal(map[string]any{voice.MetaCallKey: call})
	digits := voice.Digits(ev.From)
	in := cmodels.IncomingMessage{
		TenantID:    record.TenantID,
		Channel:     inbox.ChannelVoice,
		InboxID:     record.ID,
		SourceID:    null.StringFrom(callSourceID(ev.SID)),
		Content:     voice.StatusLabel(call) + " " + caller,
		ContentType: cmodels.ContentTypeText,
		Meta:        metaJSON,
		Contact: cmodels.IncomingContact{
			FirstName:      caller,
			PhoneNumber:    null.NewString(caller, caller != ""),
			ExternalUserID: null.NewString(voice.ExternalIDPrefix+digits, digits != ""),
		},
	}
	if digits == "" {
		// A withheld number: log the call against an anonymous caller.
		in.Contact.FirstName = "Unknown caller"
		in.Contact.ExternalUserID = null.StringFrom(voice.ExternalIDPrefix + "unknown")
	}
	// Processed here, not queued, so the call's end finds its message.
	msg, err := app.conversation.ProcessIncomingMessage(in)
	if err != nil {
		app.lo.Error("error logging incoming call", "inbox_id", record.ID, "call_sid", ev.SID, "error", err)
	}
	if call.TeamID > 0 && msg.ConversationUUID != "" {
		if sys, err := app.user.GetSystemUser(record.TenantID); err == nil {
			if err := app.conversation.UpdateConversationTeamAssignee(record.TenantID, msg.ConversationUUID, call.TeamID, sys); err != nil {
				app.lo.Error("error assigning call to team", "call_sid", ev.SID, "team_id", call.TeamID, "error", err)
			}
		}
	}

	// Pop the call up for the agents about to be rung.
	ids := []int{}
	for _, a := range agentsToRing(app, record.TenantID, v.Config(), call.TeamID) {
		ids = append(ids, a.ID)
	}
	app.conversation.BroadcastToAgents(record.TenantID, ids, wsmodels.MessageTypeIncomingCall, map[string]any{
		"call_sid":          ev.SID,
		"from":              caller,
		"contact_name":      msg.Author.FirstName,
		"conversation_uuid": msg.ConversationUUID,
		"inbox_name":        record.Name,
		"menu":              call.Menu,
	})

	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	r.RequestCtx.SetBodyString("OK")
	return nil
}

// agentsToRing returns who an inbound call rings: the online agents with a
// phone number or browser calling, only the menu's team's when the caller
// chose one.
func agentsToRing(app *App, tid int, cfg voice.Config, teamID int) []user.PhoneAgent {
	if !cfg.RingAgents {
		return nil
	}
	agents, err := app.user.AgentsWithPhone(tid)
	if err != nil {
		return nil
	}
	var inTeam map[int]bool
	if teamID > 0 {
		members, err := app.team.GetMembers(tid, teamID)
		if err != nil {
			app.lo.Error("error fetching team members for a call", "team_id", teamID, "error", err)
			return nil
		}
		inTeam = map[int]bool{}
		for _, m := range members {
			inTeam[m.ID] = true
		}
	}
	var out []user.PhoneAgent
	for _, a := range agents {
		if a.Availability != "online" || (inTeam != nil && !inTeam[a.ID]) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// handleVoiceDial answers the Connect applet's "dial whom" URL with the
// numbers to ring: the online agents who can take calls, each on their phone
// or in their browser, else the fallback list.
func handleVoiceDial(r *fastglue.Request) error {
	app := r.Context.(*App)
	v, record, ok := voiceWebhookInbox(r)
	if !ok {
		return nil
	}
	cfg := v.Config()
	// The team the menu routed the call to, from the call's log.
	teamID := 0
	if sid := string(r.RequestCtx.QueryArgs().Peek("CallSid")); sid != "" {
		if c, found, err := app.conversation.GetCall(record.TenantID, callSourceID(sid)); err == nil && found {
			if call, ok := c.Meta[voice.MetaCallKey].(map[string]any); ok {
				if id, ok := call["team_id"].(float64); ok {
					teamID = int(id)
				}
			}
		}
	}
	var numbers []string
	for _, a := range agentsToRing(app, record.TenantID, cfg, teamID) {
		if a.SIPUsername != "" {
			numbers = append(numbers, voice.SIPLeg(a.SIPUsername))
		} else if n := voice.E164(a.PhoneNumber); n != "" {
			numbers = append(numbers, n)
		}
	}
	if len(numbers) == 0 {
		for _, n := range cfg.FallbackNumbers {
			if n = voice.E164(n); n != "" {
				numbers = append(numbers, n)
			}
		}
	}

	if string(r.RequestCtx.QueryArgs().Peek("format")) == "text" {
		r.RequestCtx.SetContentType("text/plain; charset=utf-8")
		r.RequestCtx.SetBodyString(strings.Join(numbers, ","))
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"fetch_after_attempt":   false,
		"destination":           map[string]any{"numbers": numbers},
		"outgoing_phone_number": cfg.CallerID,
		"record":                cfg.Record,
		"recording_channels":    "dual",
		"max_ringing_duration":  ringSeconds(cfg),
	})
	r.RequestCtx.SetContentType("application/json")
	r.RequestCtx.SetBody(body)
	return nil
}

// handleVoiceCompleted is the Passthru after the Connect applet: it records
// how an inbound call went, who answered, and its recording.
func handleVoiceCompleted(r *fastglue.Request) error {
	app := r.Context.(*App)
	v, record, ok := voiceWebhookInbox(r)
	if !ok {
		return nil
	}
	ev, err := voice.ParsePassthru(passthruQuery(r))
	if err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	}
	// A Connect applet that ended without a status left the call unanswered.
	if ev.Status == voice.StatusRinging {
		ev.Status = voice.StatusNoAnswer
	}
	finishCall(app, v, record.TenantID, ev)
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	r.RequestCtx.SetBodyString("OK")
	return nil
}

// handleVoiceVoicemail is the Passthru after the flow's Voicemail applet, on
// the Connect applet's "nobody answered" branch: the message the caller left
// is attached to the call, or the call stays a missed one if they left none.
func handleVoiceVoicemail(r *fastglue.Request) error {
	app := r.Context.(*App)
	v, record, ok := voiceWebhookInbox(r)
	if !ok {
		return nil
	}
	ev, err := voice.ParsePassthru(passthruQuery(r))
	if err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	}
	ev.Status, ev.Duration, ev.AnsweredBy = voice.StatusNoAnswer, 0, ""
	if ev.RecordingURL != "" {
		ev.Status = voice.StatusVoicemail
	}
	finishCall(app, v, record.TenantID, ev)
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	r.RequestCtx.SetBodyString("OK")
	return nil
}

func ringSeconds(cfg voice.Config) int {
	if cfg.RingSeconds >= 10 && cfg.RingSeconds <= 120 {
		return cfg.RingSeconds
	}
	return 30
}

// transferCallRequest names the agent a customer is handed to.
type transferCallRequest struct {
	AgentID int `json:"agent_id"`
	InboxID int `json:"inbox_id"`
}

// handleTransferCall hands a customer over to another agent. Exotel can't
// move a call that's already connected, so the conversation is assigned to
// the other agent and a new call is placed from them, on their phone or in
// their browser, to the customer, logged as transferred.
func handleTransferCall(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		cuuid = r.RequestCtx.UserValue("uuid").(string)
		req   transferCallRequest
	)
	if err := r.Decode(&req, "json"); err != nil || req.AgentID == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	actor, err := app.user.GetAgentCachedOrLoad(tid, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conv, err := enforceConversationAccess(app, tid, cuuid, actor)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	target, err := app.user.GetAgentCachedOrLoad(tid, req.AgentID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	targetLeg, err := agentLeg(app, tid, target, "voice.transferNoPhone")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	customerPhone := voice.ContactE164(conv.Contact.PhoneNumberCountryCode.String, conv.Contact.PhoneNumber.String)
	if len(customerPhone) < 9 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("voice.contactNoPhone"), nil, envelope.InputError)
	}
	v, record, err := runningVoiceInbox(app, tid, req.InboxID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err := app.conversation.UpdateConversationUserAssignee(tid, cuuid, target.ID, actor); err != nil {
		return sendErrorEnvelope(r, err)
	}
	c, cancel := context.WithTimeout(app.ctx, voiceTimeout)
	defer cancel()
	sid, err := v.Call(c, targetLeg, customerPhone, voiceWebhookURL(app, record, "status"), cuuid)
	if err != nil {
		app.lo.Error("error placing transfer call", "conversation_uuid", cuuid, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadGateway, app.i18n.For(tid).Ts("voice.callFailed", "error", err.Error()), nil, envelope.GeneralError)
	}
	call := voice.Call{
		SID: sid, Provider: voice.ProviderExotel, Direction: voice.DirectionOutbound, Status: voice.StatusRinging,
		From: v.Config().CallerID, To: customerPhone, AgentID: target.ID, AgentName: target.FullName(),
		TransferredFrom: actor.FullName(), StartedAt: time.Now().UTC(),
	}
	msg, err := app.conversation.RecordOutgoingCall(tid, cuuid, target.ID, callSourceID(sid),
		fmt.Sprintf("Transferred by %s: %s %s", actor.FullName(), voice.StatusLabel(call), customerPhone),
		map[string]any{voice.MetaCallKey: call, "voice_inbox_id": v.Identifier()})
	if err != nil {
		app.lo.Error("error logging transfer call", "call_sid", sid, "error", err)
	}
	app.lo.Info("call transferred", "conversation_uuid", cuuid, "from_agent", actor.ID, "to_agent", target.ID, "call_sid", sid)
	return r.SendEnvelope(msg)
}

// handleGetPhoneAgents lists the agents who can take calls, for transfer.
func handleGetPhoneAgents(r *fastglue.Request) error {
	app := r.Context.(*App)
	agents, err := app.user.AgentsWithPhone(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	out := make([]map[string]any, 0, len(agents))
	for _, a := range agents {
		out = append(out, map[string]any{
			"id": a.ID, "name": strings.TrimSpace(a.FirstName + " " + a.LastName), "availability": a.Availability,
		})
	}
	return r.SendEnvelope(out)
}

// handleVoiceStatus receives the status callback of a click-to-call.
func handleVoiceStatus(r *fastglue.Request) error {
	app := r.Context.(*App)
	v, record, ok := voiceWebhookInbox(r)
	if !ok {
		return nil
	}
	ev, err := voice.ParseStatusCallback(r.RequestCtx.PostBody(), string(r.RequestCtx.Request.Header.ContentType()))
	if err != nil {
		app.lo.Error("error reading exotel status callback", "inbox_id", record.ID, "error", err)
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	}
	finishCall(app, v, record.TenantID, ev)
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	return nil
}

// answeredBy reports whether the leg Exotel says answered a call is the
// agent's: their phone number, or their browser's SIP user.
func answeredBy(a user.PhoneAgent, leg string) bool {
	if a.SIPUsername != "" && voice.SIPUser(leg) == strings.ToLower(a.SIPUsername) {
		return true
	}
	d := voice.Digits(leg)
	return d != "" && voice.Digits(a.PhoneNumber) == d
}

// finishCall updates a call's message with how it ended, and attaches its
// recording when there is one.
func finishCall(app *App, v *voice.Voice, tid int, ev voice.CallEvent) {
	// Who picked up an inbound call, matched on the number, or the browser's
	// SIP user, that answered.
	var agentID int
	var agentName string
	if ev.AnsweredBy != "" {
		if agents, err := app.user.AgentsWithPhone(tid); err == nil {
			for _, a := range agents {
				if answeredBy(a, ev.AnsweredBy) {
					agentID, agentName = a.ID, strings.TrimSpace(a.FirstName+" "+a.LastName)
					break
				}
			}
		}
	}

	// Before the recording's download, which can take a while: the call-back
	// and the caller's WhatsApp message shouldn't wait for it.
	onCallEnded(app, v, tid, ev, agentID)

	var rec *conversation.Recording
	recErr := ""
	if ev.RecordingURL != "" {
		c, cancel := context.WithTimeout(app.ctx, 2*time.Minute)
		content, ct, err := v.Exotel().DownloadRecording(c, ev.RecordingURL, int64(v.Config().MaxRecordingMB)*1024*1024)
		cancel()
		if err != nil {
			app.lo.Error("error downloading call recording", "call_sid", ev.SID, "error", err)
			recErr = err.Error()
		} else {
			name := path.Base(strings.SplitN(ev.RecordingURL, "?", 2)[0])
			if !strings.Contains(name, ".") {
				name = ev.SID + ".mp3"
			}
			rec = &conversation.Recording{Name: name, ContentType: ct, Content: content}
		}
	}

	found, err := app.conversation.UpdateCall(tid, callSourceID(ev.SID), func(meta map[string]any) string {
		var call voice.Call
		if raw, err := json.Marshal(meta[voice.MetaCallKey]); err == nil {
			_ = json.Unmarshal(raw, &call)
		}
		call.Status, call.EndedAt = ev.Status, time.Now().UTC()
		if ev.Duration > 0 {
			call.Duration = ev.Duration
		}
		if agentID > 0 {
			call.AgentID, call.AgentName = agentID, agentName
		}
		if recErr != "" {
			call.RecordingError = recErr
		}
		meta[voice.MetaCallKey] = call
		label := voice.StatusLabel(call)
		if call.Direction == voice.DirectionInbound && call.AgentName != "" && call.Status == voice.StatusCompleted {
			label += " · answered by " + call.AgentName
		}
		return label
	}, rec)
	if err != nil {
		app.lo.Error("error updating call", "call_sid", ev.SID, "error", err)
	} else if !found {
		app.lo.Warn("status for an unknown call", "call_sid", ev.SID)
	} else if rec != nil && getAIFeatures(app, tid).CallSummaries {
		go summarizeCall(app, tid, callSourceID(ev.SID), rec)
	}

	// The incoming call stopped ringing: close its pop-up.
	if ev.Direction == voice.DirectionInbound {
		if agents, err := app.user.AgentsWithPhone(tid); err == nil {
			ids := make([]int, 0, len(agents))
			for _, a := range agents {
				ids = append(ids, a.ID)
			}
			app.conversation.BroadcastToAgents(tid, ids, wsmodels.MessageTypeIncomingCallEnded, map[string]any{"call_sid": ev.SID})
		}
	}
}
