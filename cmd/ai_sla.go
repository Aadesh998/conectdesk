package main

import (
	"strings"

	"github.com/conectdesk/conectdesk/internal/ai"
	aimodels "github.com/conectdesk/conectdesk/internal/ai/models"
	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/zerodha/fastglue"
)

type slaCopilotReq struct {
	Message string `json:"message"`
	// Days is the dashboard's range, 0 for today.
	Days int `json:"days"`
}

// handleSLACopilot answers an agent's question about the SLA dashboard.
func handleSLACopilot(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req slaCopilotReq
	)
	if err := r.Decode(&req, "json"); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil))
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" || req.Days < 0 || req.Days > maxSLAToolDays {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil))
	}
	auser := r.RequestCtx.UserValue("user").(amodels.User)
	user, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	saved, err := app.ai.GetReportCopilotMessages(tenantID(r), auser.ID, ai.ReportScopeSLA, maxCopilotHistoryMessages)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	history := make([]aimodels.ChatMessage, 0, len(saved)+1)
	for _, m := range saved {
		history = append(history, aimodels.ChatMessage{Role: m.Role, Content: m.Content})
	}
	history = append(history, aimodels.ChatMessage{Role: aimodels.RoleUser, Content: req.Message})

	resp, err := app.ai.SLACopilot(tenantID(r), r.RequestCtx, req.Days, history, slaCopilotTools(app, user, req.Days))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if strings.TrimSpace(resp) == "" {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("ai.emptyResponse"), nil))
	}
	// Persist the exchange only after a successful reply, so a failed or empty call leaves no orphaned turn.
	if err := app.ai.SaveReportCopilotMessage(tenantID(r), auser.ID, ai.ReportScopeSLA, aimodels.RoleUser, req.Message); err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := app.ai.SaveReportCopilotMessage(tenantID(r), auser.ID, ai.ReportScopeSLA, aimodels.RoleAssistant, resp); err != nil {
		app.lo.Error("error saving sla copilot reply", "error", err)
	}
	return r.SendEnvelope(stringutil.Markdown2HTML(resp))
}

// handleGetSLACopilotMessages returns the requesting agent's SLA dashboard copilot chat.
func handleGetSLACopilotMessages(r *fastglue.Request) error {
	app := r.Context.(*App)
	auser := r.RequestCtx.UserValue("user").(amodels.User)
	msgs, err := app.ai.GetReportCopilotMessages(tenantID(r), auser.ID, ai.ReportScopeSLA, maxCopilotHistoryMessages)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	// Assistant turns are stored as markdown; the panel renders HTML.
	for i := range msgs {
		if msgs[i].Role == aimodels.RoleAssistant {
			msgs[i].Content = stringutil.Markdown2HTML(msgs[i].Content)
		}
	}
	return r.SendEnvelope(msgs)
}

// handleClearSLACopilotMessages clears the requesting agent's SLA dashboard copilot chat.
func handleClearSLACopilotMessages(r *fastglue.Request) error {
	app := r.Context.(*App)
	auser := r.RequestCtx.UserValue("user").(amodels.User)
	if err := app.ai.ClearReportCopilotMessages(tenantID(r), auser.ID, ai.ReportScopeSLA); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}
