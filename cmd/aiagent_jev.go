package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/aiagent"
	amodels "github.com/conectdesk/conectdesk/internal/aiagent/models"
	cmodels "github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/voice"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/whatsapp"
	lmodels "github.com/conectdesk/conectdesk/internal/lead/models"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/conectdesk/conectdesk/internal/systemone"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// aiFollowupInterval is how often the AI agent looks for quiet conversations
// to follow up: ai_agent.followup_interval, 5 minutes by default.
func aiFollowupInterval() time.Duration {
	if d := ko.Duration("ai_agent.followup_interval"); d >= time.Minute {
		return d
	}
	return 5 * time.Minute
}

// aiHandBackAfter is how long a customer waits on a person before an AI
// inbox's assistant takes the conversation back: ai_agent.hand_back_after,
// 5 minutes by default.
func aiHandBackAfter() time.Duration {
	if d := ko.Duration("ai_agent.hand_back_after"); d >= time.Minute {
		return d
	}
	return 5 * time.Minute
}

// onIncomingMessage runs after each customer message: an unassigned
// conversation on an AI inbox goes to its assistant, then lead details are
// read from the chat.
func onIncomingMessage(app *App) func(tid int, conversationUUID string) {
	leads := onIncomingForLeads(app)
	return func(tid int, uuid string) {
		if conv, err := app.conversation.GetConversation(tid, 0, uuid, ""); err == nil {
			assignToInboxAssistant(app, tid, conv)
		}
		leads(tid, uuid)
	}
}

// aiAgentHooks gives the AI agent what its Jev layer and follow-ups need from
// the rest of the app: WhatsApp's reply window and templates, the lead
// pipeline, and business hours.
func aiAgentHooks(app *App) aiagent.Hooks {
	return aiagent.Hooks{
		ReplyWindowOpen: func(tid, contactID, inboxID int) bool {
			record, err := app.inbox.GetDBRecord(tid, inboxID)
			if err != nil {
				return false
			}
			switch record.Channel {
			case inbox.ChannelWhatsApp, inbox.ChannelMessenger, inbox.ChannelInstagram:
				w, err := whatsAppWindow(app, tid, contactID, inboxID)
				return err == nil && w.Open
			}
			return true
		},
		TemplateText: func(tid, inboxID int, ref amodels.TemplateRef) (string, bool) {
			wa, err := runningWhatsAppInbox(app, tid, inboxID)
			if err != nil {
				return "", false
			}
			c, cancel := context.WithTimeout(app.ctx, whatsAppGraphTimeout)
			defer cancel()
			t, err := wa.FindTemplate(c, ref.Name, ref.Language)
			if err != nil {
				return "", false
			}
			// The agent fills in body values only, as for missed-call messages.
			s := whatsapp.Summarize(t)
			if !s.Supported || s.HeaderMedia != "" || len(s.HeaderParams) > 0 || len(s.ButtonParams) > 0 {
				return "", false
			}
			return s.Body, true
		},
		SendTemplate: func(conv cmodels.Conversation, senderID int, ref amodels.TemplateRef, meta map[string]any) error {
			tid := conv.TenantID
			name := conv.Contact.FirstName
			if looksLikeNumber(name) {
				name = app.i18n.For(tid).T("callBack.templateNameFallback")
			}
			phone := voice.ContactE164(conv.Contact.PhoneNumberCountryCode.String, conv.Contact.PhoneNumber.String)
			vref := voice.TemplateRef{Name: ref.Name, Language: ref.Language, Body: ref.Body}
			content, tmeta, err := prepareWhatsAppTemplate(app, tid, conv.InboxID, whatsapp.TemplateRequest{
				Name:     ref.Name,
				Language: ref.Language,
				Body:     vref.Values(name, phone, tenantConsts(app, tid).SiteName),
			}, nil)
			if err != nil {
				return err
			}
			for k, v := range meta {
				tmeta[k] = v
			}
			_, err = app.conversation.QueueReply(tid, nil, conv.InboxID, senderID, conv.ContactID, conv.UUID, content, nil, nil, nil, tmeta)
			return err
		},
		LeadStage: func(tid, contactID int) ([]lmodels.Stage, string, error) {
			stages, err := app.lead.Stages(tid)
			if err != nil {
				return nil, "", err
			}
			cl, err := app.lead.ContactLead(tid, contactID)
			if err != nil {
				return nil, "", err
			}
			return stages, cl.Stage.String, nil
		},
		SetLeadStage: func(tid, contactID int, stage string, actorID int) error {
			changed, err := app.lead.SetStage(tid, contactID, stage, actorID)
			if err == nil && changed {
				leadStageChanged(app, tid, contactID, stage)
			}
			return err
		},
		BusinessOpen: func(tid int, t time.Time) bool {
			return businessOpen(app, tid, t)
		},
	}
}

// assignToInboxAssistant hands an unassigned conversation to the AI
// assistant its inbox is set to, so the assistant answers the customer.
// It runs on every customer message, so a reopened or never-assigned
// conversation goes to the AI too. A conversation someone owns is left
// alone, and so is one the assistant recently handed to people.
func assignToInboxAssistant(app *App, tid int, conv cmodels.Conversation) {
	if conv.AssignedUserID.Valid {
		return
	}
	record, err := app.inbox.GetDBRecord(tid, conv.InboxID)
	if err != nil || !inbox.IsMetaChannel(record.Channel) && record.Channel != inbox.ChannelTelegram {
		return
	}
	var cfg struct {
		AIAssistantID int `json:"ai_assistant_id"`
	}
	if json.Unmarshal(record.Config, &cfg) != nil || cfg.AIAssistantID == 0 {
		return
	}
	a, err := app.aiAgent.GetAssistant(tid, cfg.AIAssistantID)
	if err != nil || !a.Enabled {
		return
	}
	if app.aiAgent.RecentlyHandedOff(tid, conv.ID) {
		return
	}
	sys, err := app.user.GetSystemUser(tid)
	if err != nil {
		return
	}
	if err := app.conversation.UpdateConversationUserAssignee(tid, conv.UUID, a.UserID, sys); err != nil {
		app.lo.Error("error assigning new conversation to the inbox's ai assistant", "conversation_uuid", conv.UUID, "assistant_id", a.ID, "error", err)
	}
}

// jevSettings are the workspace's TypeSafe System One (Jev) settings. The API
// key is stored encrypted and never sent back in full.
type jevSettings struct {
	Enabled bool   `json:"jev.enabled"`
	APIKey  string `json:"jev.api_key"`
	Model   string `json:"jev.model"`
}

func getJevSettings(app *App, tid int) jevSettings {
	var s jevSettings
	if b, err := app.setting.GetByPrefix(tid, "jev."); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &s)
	}
	if s.Model == "" {
		s.Model = systemone.DefaultModel
	}
	return s
}

// jevSettingsView is the settings as the admin page sees them.
type jevSettingsView struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
	HasKey  bool   `json:"has_key"`
}

func (s jevSettings) view() jevSettingsView {
	v := jevSettingsView{Enabled: s.Enabled, Model: s.Model, HasKey: s.APIKey != ""}
	if v.HasKey {
		v.APIKey = strings.Repeat(stringutil.PasswordDummy, 10)
	}
	return v
}

// handleGetJevSettings returns the workspace's Jev settings, key masked.
func handleGetJevSettings(r *fastglue.Request) error {
	return r.SendEnvelope(getJevSettings(r.Context.(*App), tenantID(r)).view())
}

// handleUpdateJevSettings saves the Jev settings. A blank or masked key keeps
// the stored one.
func handleUpdateJevSettings(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
		req jevSettingsView
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	cur := getJevSettings(app, tid)
	next := jevSettings{Enabled: req.Enabled, Model: strings.TrimSpace(req.Model), APIKey: cur.APIKey}
	if key := strings.TrimSpace(req.APIKey); key != "" && !strings.Contains(key, stringutil.PasswordDummy) {
		next.APIKey = key
	}
	if next.Model == "" || len(next.Model) > 64 || strings.ContainsAny(next.Model, " \t\r\n") {
		next.Model = systemone.DefaultModel
	}
	if next.Enabled && next.APIKey == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("ai.jev.keyRequired"), nil, envelope.InputError)
	}
	if err := app.setting.Update(tid, next); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(next.view())
}

// handleTestJevSettings checks the stored key, or one being entered, by
// asking Jev one question.
func handleTestJevSettings(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
		req jevSettingsView
	)
	_ = r.Decode(&req, "json")
	s := getJevSettings(app, tid)
	if key := strings.TrimSpace(req.APIKey); key != "" && !strings.Contains(key, stringutil.PasswordDummy) {
		s.APIKey = key
	}
	if m := strings.TrimSpace(req.Model); m != "" {
		s.Model = m
	}
	if s.APIKey == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("ai.jev.keyRequired"), nil, envelope.InputError)
	}
	c, cancel := context.WithTimeout(app.ctx, 15*time.Second)
	defer cancel()
	start := time.Now()
	res, err := systemone.New(s.APIKey, s.Model).Evaluate(c, "Customer: hi, what are your fees?", map[string]systemone.Question{
		"question": systemone.Noul("Does the customer ask a question?"),
	})
	if err != nil {
		app.lo.Warn("jev test call failed", "tenant_id", tid, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).Ts("ai.jev.testFailed", "error", err.Error()), nil, envelope.InputError)
	}
	return r.SendEnvelope(map[string]any{
		"model":      res.Model,
		"latency_ms": time.Since(start).Milliseconds(),
	})
}

// takeOverFromAssistant assigns a conversation an AI assistant is handling to
// the agent replying in it. An assistant's own replies, and conversations
// assigned to a person or nobody, are left alone.
func takeOverFromAssistant(app *App, tid int, conv cmodels.Conversation, agent umodels.User) {
	if !conv.AssignedUserID.Valid || agent.ID == 0 || app.aiAgent.IsAssistantUser(agent.ID) {
		return
	}
	if !app.aiAgent.IsAssistantUser(int(conv.AssignedUserID.Int)) {
		return
	}
	if err := app.conversation.UpdateConversationUserAssignee(tid, conv.UUID, agent.ID, agent); err != nil {
		app.lo.Error("error taking a conversation over from its ai assistant", "conversation_uuid", conv.UUID, "agent_id", agent.ID, "error", err)
		return
	}
	app.lo.Info("agent took a conversation over from its ai assistant", "conversation_uuid", conv.UUID, "agent_id", agent.ID)
}
