package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/conectdesk/conectdesk/internal/ai"
	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/conversation"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/voice"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const (
	// callAIKey is the message meta key a call's transcript and summary are
	// kept under.
	callAIKey = "call_ai"
	// callAITimeout bounds transcribing and summarising one call.
	callAITimeout = 5 * time.Minute
	// leadFillQuiet is how long a chat has to go quiet before its lead
	// details are read, so a burst of messages is read once.
	leadFillQuiet = 90 * time.Second
	// maxCallAIJobs caps the calls being transcribed at once.
	maxCallAIJobs = 3
)

// aiFeatures are the workspace's switches for the AI features that cost a
// provider request each time they run.
type aiFeatures struct {
	// CallSummaries transcribes and summarises every recorded call.
	CallSummaries bool `json:"ai_features.call_summaries"`
	// LeadExtraction fills contacts' empty fields from their chats and calls.
	LeadExtraction bool `json:"ai_features.lead_extraction"`
}

func getAIFeatures(app *App, tid int) aiFeatures {
	var f aiFeatures
	if b, err := app.setting.GetByPrefix(tid, "ai_features."); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &f)
	}
	return f
}

// callAI is a call's transcript and summary, on its message's meta.
type callAI struct {
	// Status is working, done or failed.
	Status     string    `json:"status"`
	Summary    string    `json:"summary,omitempty"`
	Transcript string    `json:"transcript,omitempty"`
	Error      string    `json:"error,omitempty"`
	At         time.Time `json:"at"`
	// Filled are the contact fields the call filled in.
	Filled map[string]any `json:"filled,omitempty"`
}

// callAIJobs limits concurrent call transcriptions.
var callAIJobs = make(chan struct{}, maxCallAIJobs)

// summarizeCall transcribes a call's recording, summarises it for the team on
// the call's card, and, with lead extraction on, fills the caller's empty
// contact fields from what was said.
func summarizeCall(app *App, tid int, sourceID string, rec *conversation.Recording) {
	cm, found, err := app.conversation.GetCall(tid, sourceID)
	if err != nil || !found {
		return
	}
	conv, err := app.conversation.GetConversation(tid, cm.ConversationID, "", "")
	if err != nil {
		return
	}
	runCallAI(app, tid, cm.MessageUUID, conv.ContactID, rec.Name, rec.ContentType, rec.Content)
}

func runCallAI(app *App, tid int, messageUUID string, contactID int, name, contentType string, audio []byte) {
	callAIJobs <- struct{}{}
	defer func() { <-callAIJobs }()

	set := func(c callAI) {
		c.At = time.Now().UTC()
		_ = app.conversation.SetMessageMeta(tid, messageUUID, callAIKey, c)
	}
	set(callAI{Status: "working"})

	ctx, cancel := context.WithTimeout(app.ctx, callAITimeout)
	defer cancel()
	text, err := app.ai.Transcribe(tid, ctx, name, contentType, audio)
	if err != nil {
		app.lo.Error("error transcribing call", "message_uuid", messageUUID, "error", err)
		set(callAI{Status: "failed", Error: callBackErrText(err)})
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		set(callAI{Status: "done", Summary: "- " + app.i18n.For(tid).T("callAI.noSpeech")})
		return
	}
	summary, err := app.ai.SummarizeCall(tid, ctx, text, workspaceLanguage(app, tid))
	if err != nil {
		app.lo.Error("error summarising call", "message_uuid", messageUUID, "error", err)
		set(callAI{Status: "failed", Transcript: text, Error: callBackErrText(err)})
		return
	}
	result := callAI{Status: "done", Summary: summary, Transcript: text}
	if contactID > 0 && getAIFeatures(app, tid).LeadExtraction {
		if filled, err := fillLeadFields(app, ctx, tid, contactID, "Phone call transcript:\n"+text); err == nil && len(filled) > 0 {
			result.Filled = filled
		}
	}
	set(result)
}

// workspaceLanguage is the workspace's language by name, for AI output.
func workspaceLanguage(app *App, tid int) string {
	name := app.i18n.For(tid).T("_.name")
	if i := strings.Index(name, " ("); i > 0 {
		name = name[:i]
	}
	return name
}

// fillLeadFields reads a transcript for the contact's custom fields that are
// still empty, and saves what it states. Fields an agent filled are never
// changed. It returns the fields it filled.
func fillLeadFields(app *App, ctx context.Context, tid, contactID int, transcript string) (map[string]any, error) {
	defs, err := app.customAttribute.GetAll(tid, "contact")
	if err != nil || len(defs) == 0 {
		return nil, err
	}
	current, err := app.user.GetCustomAttributes(tid, contactID)
	if err != nil {
		return nil, err
	}
	var fields []ai.LeadField
	for _, d := range defs {
		if v, ok := current[d.Key]; ok && v != nil && v != "" {
			continue
		}
		fields = append(fields, ai.LeadField{Key: d.Key, Name: d.Name, Description: d.Description, DataType: d.DataType, Values: d.Values})
	}
	if len(fields) == 0 {
		return map[string]any{}, nil
	}
	found, err := app.ai.ExtractLeadFields(tid, ctx, transcript, fields)
	if err != nil || len(found) == 0 {
		return found, err
	}
	merged, err := app.user.FillEmptyCustomAttributes(tid, contactID, found)
	if err != nil {
		return nil, err
	}
	// Only what was written counts: an agent may have filled a field meanwhile.
	filled := map[string]any{}
	for k, v := range found {
		if fmt.Sprint(merged[k]) == fmt.Sprint(v) {
			filled[k] = v
		}
	}
	app.conversation.BroadcastContactUpdate(tid, contactID, map[string]any{"custom_attributes": merged})
	app.lo.Info("filled contact fields from a conversation", "contact_id", contactID, "fields", len(filled))
	return filled, nil
}

// leadFills waits for each chat to go quiet before reading its lead details.
var leadFills = struct {
	sync.Mutex
	timers map[string]*time.Timer
}{timers: map[string]*time.Timer{}}

// onIncomingForLeads schedules reading a chat's lead details once it goes
// quiet, when the workspace has lead extraction on. Phone conversations are
// read from their calls' transcripts instead.
func onIncomingForLeads(app *App) func(tid int, conversationUUID string) {
	return func(tid int, uuid string) {
		if !getAIFeatures(app, tid).LeadExtraction {
			return
		}
		key := strconv.Itoa(tid) + ":" + uuid
		leadFills.Lock()
		defer leadFills.Unlock()
		if t, ok := leadFills.timers[key]; ok {
			t.Stop()
		}
		leadFills.timers[key] = time.AfterFunc(leadFillQuiet, func() {
			leadFills.Lock()
			delete(leadFills.timers, key)
			leadFills.Unlock()

			conv, err := app.conversation.GetConversation(tid, 0, uuid, "")
			if err != nil || conv.InboxChannel == voice.ChannelVoice {
				return
			}
			ctx, cancel := context.WithTimeout(app.ctx, time.Minute)
			defer cancel()
			if _, err := fillLeadFields(app, ctx, tid, conv.ContactID, conversationTranscript(app, tid, uuid)); err != nil {
				app.lo.Error("error filling lead fields from a chat", "conversation_uuid", uuid, "error", err)
			}
		})
	}
}

// handleGetAIFeatures returns the workspace's AI feature switches.
func handleGetAIFeatures(r *fastglue.Request) error {
	return r.SendEnvelope(getAIFeatures(r.Context.(*App), tenantID(r)))
}

// handleUpdateAIFeatures saves the workspace's AI feature switches.
func handleUpdateAIFeatures(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
		f   aiFeatures
	)
	if err := r.Decode(&f, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if err := app.setting.Update(tid, f); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(f)
}

// handleSummarizeCall transcribes and summarises a recorded call on request,
// e.g. one recorded before call summaries were on. It runs in the background
// and the call's card updates when it's done.
func handleSummarizeCall(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		cuuid = r.RequestCtx.UserValue("uuid").(string)
		muuid = r.RequestCtx.UserValue("muuid").(string)
	)
	agent, err := app.user.GetAgentCachedOrLoad(tid, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conv, err := enforceConversationAccess(app, tid, cuuid, agent)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	msg, err := app.conversation.GetMessage(tid, muuid)
	if err != nil || msg.ConversationUUID != conv.UUID {
		return sendErrorEnvelope(r, envelope.NewError(envelope.NotFoundError, app.i18n.For(tid).T("globals.messages.notFound"), nil))
	}
	for _, att := range msg.Attachments {
		if !strings.HasPrefix(att.ContentType, "audio/") {
			continue
		}
		blob, err := app.media.GetBlob(att.UUID)
		if err != nil {
			app.lo.Error("error reading call recording", "message_uuid", muuid, "error", err)
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
		}
		go runCallAI(app, tid, muuid, conv.ContactID, att.Name, att.ContentType, blob)
		return r.SendEnvelope(map[string]string{"status": "working"})
	}
	return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("callAI.noRecording"), nil))
}

// handleFillContactWithAI fills a contact's empty fields from a conversation
// of theirs, on request.
func handleFillContactWithAI(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		req   struct {
			ConversationUUID string `json:"conversation_uuid"`
		}
	)
	if err := r.Decode(&req, "json"); err != nil || req.ConversationUUID == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	agent, err := app.user.GetAgentCachedOrLoad(tid, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conv, err := enforceConversationAccess(app, tid, req.ConversationUUID, agent)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if conv.ContactID != id {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("globals.messages.badRequest"), nil))
	}
	transcript := conversationTranscript(app, tid, conv.UUID)
	ctx, cancel := context.WithTimeout(app.ctx, time.Minute)
	defer cancel()
	filled, err := fillLeadFields(app, ctx, tid, id, transcript)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if filled == nil {
		filled = map[string]any{}
	}
	return r.SendEnvelope(filled)
}
