package main

import (
	"fmt"
	"strconv"
	"strings"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/lead"
	lmodels "github.com/conectdesk/conectdesk/internal/lead/models"
	wsmodels "github.com/conectdesk/conectdesk/internal/ws/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// onNewContactConversation puts a contact who just got in touch on the first
// lead stage, when the workspace has that on and they aren't on it yet.
func onNewContactConversation(app *App) func(tenantID, contactID int) {
	return func(tid, contactID int) {
		stage, err := app.lead.EnsureLead(tid, contactID)
		if err != nil || stage == "" {
			return
		}
		leadStageChanged(app, tid, contactID, stage)
	}
}

// leadStageChanged tells the workspace's agents a contact moved, for the
// board and contact panels to follow.
func leadStageChanged(app *App, tid, contactID int, stage string) {
	app.conversation.BroadcastToTenant(tid, wsmodels.MessageTypeLeadStageChanged, map[string]any{
		"contact_id": contactID,
		"stage":      stage,
	})
}

// handleGetLeadStages returns the workspace's lead stages, for every agent.
func handleGetLeadStages(r *fastglue.Request) error {
	app := r.Context.(*App)
	stages, err := app.lead.Stages(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if stages == nil {
		stages = []lmodels.Stage{}
	}
	return r.SendEnvelope(stages)
}

// handleGetLeadSettings returns the lead settings and how many contacts each
// stage has, for the admin page.
func handleGetLeadSettings(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
	)
	s, err := app.lead.Settings(tid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	counts, err := app.lead.CountByStage(tid)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(map[string]any{"stages": s.Stages, "auto_new": s.AutoNew, "counts": counts})
}

// leadSettingsRequest is the lead settings as the admin page sends them. A
// new stage comes without a key; one is made from its name.
type leadSettingsRequest struct {
	Stages  []lmodels.Stage `json:"stages"`
	AutoNew bool            `json:"auto_new"`
}

// handleUpdateLeadSettings saves the lead stages.
func handleUpdateLeadSettings(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
		req leadSettingsRequest
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	used := map[string]bool{}
	for _, st := range req.Stages {
		if st.Key != "" {
			used[st.Key] = true
		}
	}
	for i := range req.Stages {
		if req.Stages[i].Key != "" {
			continue
		}
		base := lead.KeyFor(req.Stages[i].Label)
		if base == "" {
			base = "stage"
		}
		key := base
		for n := 2; used[key]; n++ {
			key = fmt.Sprintf("%s_%d", base, n)
		}
		req.Stages[i].Key, used[key] = key, true
	}
	if err := app.lead.UpdateSettings(tid, lmodels.Settings{Stages: req.Stages, AutoNew: req.AutoNew}); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return handleGetLeadSettings(r)
}

// handleGetLeadBoard returns the leads board: each stage's newest-moved
// contacts, filtered by ?q=.
func handleGetLeadBoard(r *fastglue.Request) error {
	var (
		app  = r.Context.(*App)
		args = r.RequestCtx.QueryArgs()
	)
	perStage, _ := strconv.Atoi(string(args.Peek("per_stage")))
	cols, err := app.lead.Board(tenantID(r), perStage, strings.TrimSpace(string(args.Peek("q"))))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(cols)
}

// handleGetContactLead returns a contact's stage and how it got there.
func handleGetContactLead(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	cl, err := app.lead.ContactLead(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(cl)
}

// handleSetContactLead moves a contact to another stage, or off the pipeline
// with an empty stage.
func handleSetContactLead(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		req   struct {
			Stage string `json:"stage"`
		}
	)
	if err := r.Decode(&req, "json"); err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	moved, err := app.lead.SetStage(tid, id, req.Stage, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if moved {
		leadStageChanged(app, tid, id, req.Stage)
	}
	cl, err := app.lead.ContactLead(tid, id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(cl)
}
