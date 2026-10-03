package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/ai"
	cmodels "github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/report"
	rmodels "github.com/conectdesk/conectdesk/internal/report/models"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/jmoiron/sqlx/types"
)

const (
	// maxSLAToolDays bounds how far back an SLA tool looks, as the dashboard does.
	maxSLAToolDays = 365

	// defaultSLAListLimit and maxSLAListLimit bound how many conversations an SLA list tool returns.
	defaultSLAListLimit = 10
	maxSLAListLimit     = 25

	// defaultSLARiskMinutes and maxSLARiskMinutes bound how far ahead list_sla_at_risk looks.
	defaultSLARiskMinutes = 60
	maxSLARiskMinutes     = 7 * 24 * 60
)

var (
	slaDaysParam = `"days": {
		"type": "integer",
		"description": "How many days back to look, 0 for today only. Leave out to use the dashboard's range."
	}`
	slaMetricParam = `"metric": {
		"type": "string",
		"enum": ["first_response", "next_response", "resolution"],
		"description": "Only this SLA metric. Leave out for all three."
	}`

	slaDaysToolParams = types.JSONText(`{
		"type": "object",
		"properties": {` + slaDaysParam + `}
	}`)

	slaBreakdownToolParams = types.JSONText(`{
		"type": "object",
		"properties": {
			"group_by": {
				"type": "string",
				"enum": ["team", "agent", "policy", "inbox", "priority"],
				"description": "What to group the SLA results by."
			},
			` + slaMetricParam + `,
			` + slaDaysParam + `
		},
		"required": ["group_by"]
	}`)

	slaBreachesToolParams = types.JSONText(`{
		"type": "object",
		"properties": {
			` + slaMetricParam + `,
			"team": {
				"type": "string",
				"description": "Only conversations assigned to the team with this exact name."
			},
			"agent": {
				"type": "string",
				"description": "Only conversations assigned to an agent whose name contains this."
			},
			"limit": {
				"type": "integer",
				"description": "How many to list, at most 25. Defaults to 10."
			},
			` + slaDaysParam + `
		}
	}`)

	slaAtRiskToolParams = types.JSONText(`{
		"type": "object",
		"properties": {
			"within_minutes": {
				"type": "integer",
				"description": "List deadlines due within this many minutes from now. Defaults to 60."
			},
			"limit": {
				"type": "integer",
				"description": "How many to list, at most 25. Defaults to 10."
			}
		}
	}`)

	slaTrendToolParams = types.JSONText(`{
		"type": "object",
		"properties": {
			"interval": {
				"type": "string",
				"enum": ["day", "week"],
				"description": "Group by day or by week. Defaults to day."
			},
			` + slaDaysParam + `
		}
	}`)
)

// slaToolBase carries what every SLA tool needs: the asking agent and the dashboard's range.
type slaToolBase struct {
	app  *App
	user umodels.User
	days int
}

type slaOverviewTool struct{ slaToolBase }
type slaBreakdownTool struct{ slaToolBase }
type slaBreachesTool struct{ slaToolBase }
type slaAtRiskTool struct{ slaToolBase }
type slaTrendTool struct{ slaToolBase }
type slaPoliciesTool struct{ slaToolBase }

// slaCopilotTools builds the read-only tools attached to the SLA dashboard copilot; days is the dashboard's range.
func slaCopilotTools(app *App, user umodels.User, days int) []ai.Tool {
	base := slaToolBase{app: app, user: user, days: days}
	return []ai.Tool{
		&slaOverviewTool{base},
		&slaBreakdownTool{base},
		&slaBreachesTool{base},
		&slaAtRiskTool{base},
		&slaTrendTool{base},
		&slaPoliciesTool{base},
		&fetchConversationTool{app: app, user: user},
	}
}

func (t *slaOverviewTool) Name() string { return "get_sla_overview" }

func (t *slaOverviewTool) Description() string {
	return "Get the SLA totals the dashboard shows: met, breached and compliance for first response, next response and resolution, with average times."
}

func (t *slaOverviewTool) Parameters() types.JSONText { return slaDaysToolParams }

func (t *slaOverviewTool) Execute(ctx context.Context, args string) (string, error) {
	var in struct {
		Days *int `json:"days"`
	}
	if err := parseToolArgs(args, &in); err != nil {
		return "", err
	}
	days := t.resolveDays(in.Days)
	raw, err := t.app.report.GetOverviewSLA(t.user.TenantID, days)
	if err != nil {
		return "", err
	}
	var s rmodels.OverviewSLA
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "SLA overview for %s:\n", periodLabel(days))
	writeOverviewLine(&b, "First response", s.FirstResponseMetCount, s.FirstResponseBreachedCount, s.FirstResponseCompliancePercent, s.AvgFirstResponseTimeSec)
	writeOverviewLine(&b, "Next response", s.NextResponseMetCount, s.NextResponseBreachedCount, s.NextResponseCompliancePercent, s.AvgNextResponseTimeSec)
	writeOverviewLine(&b, "Resolution", s.ResolutionMetCount, s.ResolutionBreachedCount, s.ResolutionCompliancePercent, s.AvgResolutionTimeSec)
	return b.String(), nil
}

func writeOverviewLine(b *strings.Builder, label string, met, breached int, compliance, avgSec float64) {
	fmt.Fprintf(b, "- %s: %d met, %d breached, %.1f%% compliance", label, met, breached, compliance)
	if met > 0 {
		fmt.Fprintf(b, ", average time when met %s", formatToolDuration(avgSec))
	}
	b.WriteString("\n")
}

func (t *slaBreakdownTool) Name() string { return "get_sla_breakdown" }

func (t *slaBreakdownTool) Description() string {
	return "Break the SLA results down by team, agent, policy, inbox or priority, to see which ones breach the most. Team and agent are the conversation's current assignment."
}

func (t *slaBreakdownTool) Parameters() types.JSONText { return slaBreakdownToolParams }

func (t *slaBreakdownTool) Execute(ctx context.Context, args string) (string, error) {
	var in struct {
		GroupBy string `json:"group_by"`
		Metric  string `json:"metric"`
		Days    *int   `json:"days"`
	}
	if err := parseToolArgs(args, &in); err != nil {
		return "", err
	}
	if !slices.Contains(report.SLAGroupings, in.GroupBy) {
		return "group_by must be one of: " + strings.Join(report.SLAGroupings, ", ") + ".", nil
	}
	metric, ok := slaMetricArg(in.Metric)
	if !ok {
		return invalidSLAMetric, nil
	}
	days := t.resolveDays(in.Days)
	rows, err := t.app.report.GetSLABreakdown(t.user.TenantID, days, in.GroupBy, metric)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return fmt.Sprintf("No SLA results for %s.", periodLabel(days)), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "SLA results by %s for %s, most breaches first:\n", in.GroupBy, periodLabel(days))
	for _, r := range rows {
		fmt.Fprintf(&b, "- %s | %s: %d met, %d breached, %d pending, %s compliance",
			r.GroupName, slaMetricLabel(r.Metric), r.Met, r.Breached, r.Pending, compliance(r.Met, r.Breached))
		if r.Met > 0 {
			fmt.Fprintf(&b, ", average time when met %s", formatToolDuration(r.AvgMetSeconds))
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func (t *slaBreachesTool) Name() string { return "list_sla_breaches" }

func (t *slaBreachesTool) Description() string {
	return "List conversations that breached an SLA, newest breach first, with their reference numbers, team, agent and policy. Can filter by metric, team or agent."
}

func (t *slaBreachesTool) Parameters() types.JSONText { return slaBreachesToolParams }

func (t *slaBreachesTool) Execute(ctx context.Context, args string) (string, error) {
	var in struct {
		Metric string `json:"metric"`
		Team   string `json:"team"`
		Agent  string `json:"agent"`
		Limit  int    `json:"limit"`
		Days   *int   `json:"days"`
	}
	if err := parseToolArgs(args, &in); err != nil {
		return "", err
	}
	metric, ok := slaMetricArg(in.Metric)
	if !ok {
		return invalidSLAMetric, nil
	}
	days := t.resolveDays(in.Days)
	rows, total, err := t.app.report.GetSLABreaches(t.user.TenantID, days, metric, strings.TrimSpace(in.Team), strings.TrimSpace(in.Agent), slaListLimit(in.Limit))
	if err != nil {
		return "", err
	}
	if total == 0 {
		return fmt.Sprintf("No SLA breaches match for %s.", periodLabel(days)), nil
	}
	visible := filterAccessibleSLAConversations(t.app, t.user, rows)
	var b strings.Builder
	b.WriteString(untrustedConversationData)
	fmt.Fprintf(&b, "%d SLA breaches match for %s; showing %d:\n", total, periodLabel(days), len(visible))
	for _, r := range visible {
		fmt.Fprintf(&b, "- #%s | %s breached %s", r.ReferenceNumber, slaMetricLabel(r.Metric), formatToolTime(r.BreachedAt.Time))
		if r.DeadlineAt.Valid {
			fmt.Fprintf(&b, " (deadline %s)", formatToolTime(r.DeadlineAt.Time))
		}
		if r.MetAt.Valid {
			fmt.Fprintf(&b, ", done late %s", formatToolTime(r.MetAt.Time))
		}
		writeSLAConversationDetails(&b, r)
	}
	writeHiddenNote(&b, len(rows)-len(visible))
	return b.String(), nil
}

func (t *slaAtRiskTool) Name() string { return "list_sla_at_risk" }

func (t *slaAtRiskTool) Description() string {
	return "List open conversations whose SLA deadline is coming up soon or has just passed without being marked, soonest first. Use for questions like what is about to breach."
}

func (t *slaAtRiskTool) Parameters() types.JSONText { return slaAtRiskToolParams }

func (t *slaAtRiskTool) Execute(ctx context.Context, args string) (string, error) {
	var in struct {
		WithinMinutes int `json:"within_minutes"`
		Limit         int `json:"limit"`
	}
	if err := parseToolArgs(args, &in); err != nil {
		return "", err
	}
	minutes := in.WithinMinutes
	if minutes <= 0 {
		minutes = defaultSLARiskMinutes
	}
	minutes = min(minutes, maxSLARiskMinutes)
	rows, total, err := t.app.report.GetSLAAtRisk(t.user.TenantID, minutes, slaListLimit(in.Limit))
	if err != nil {
		return "", err
	}
	window := formatToolDuration(float64(minutes * 60))
	if total == 0 {
		return fmt.Sprintf("No open SLA deadlines are due within the next %s.", window), nil
	}
	visible := filterAccessibleSLAConversations(t.app, t.user, rows)
	now := time.Now()
	var b strings.Builder
	b.WriteString(untrustedConversationData)
	fmt.Fprintf(&b, "%d open SLA deadlines are due within the next %s or already past; showing %d, soonest first:\n", total, window, len(visible))
	for _, r := range visible {
		left := r.DeadlineAt.Time.Sub(now)
		due := "due in " + formatToolDuration(left.Seconds())
		if left < 0 {
			due = "overdue by " + formatToolDuration(-left.Seconds())
		}
		fmt.Fprintf(&b, "- #%s | %s %s (deadline %s)", r.ReferenceNumber, slaMetricLabel(r.Metric), due, formatToolTime(r.DeadlineAt.Time))
		writeSLAConversationDetails(&b, r)
	}
	writeHiddenNote(&b, len(rows)-len(visible))
	return b.String(), nil
}

func (t *slaTrendTool) Name() string { return "get_sla_trend" }

func (t *slaTrendTool) Description() string {
	return "Get met, breached and compliance per day or per week, to see whether SLA performance is getting better or worse."
}

func (t *slaTrendTool) Parameters() types.JSONText { return slaTrendToolParams }

func (t *slaTrendTool) Execute(ctx context.Context, args string) (string, error) {
	var in struct {
		Interval string `json:"interval"`
		Days     *int   `json:"days"`
	}
	if err := parseToolArgs(args, &in); err != nil {
		return "", err
	}
	interval := in.Interval
	if interval == "" {
		interval = "day"
	}
	if !slices.Contains(report.SLAIntervals, interval) {
		return "interval must be day or week.", nil
	}
	days := t.resolveDays(in.Days)
	rows, err := t.app.report.GetSLATrend(t.user.TenantID, days, interval)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return fmt.Sprintf("No SLA results for %s.", periodLabel(days)), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "SLA results per %s for %s (a week is labelled by its Monday):\n", interval, periodLabel(days))
	for _, r := range rows {
		fmt.Fprintf(&b, "- %s | %s: %d met, %d breached, %s compliance\n",
			r.Period.Format("2006-01-02"), slaMetricLabel(r.Metric), r.Met, r.Breached, compliance(r.Met, r.Breached))
	}
	return b.String(), nil
}

func (t *slaPoliciesTool) Name() string { return "list_sla_policies" }

func (t *slaPoliciesTool) Description() string {
	return "List the workspace's SLA policies and their targets for first response, next response and resolution."
}

func (t *slaPoliciesTool) Parameters() types.JSONText { return emptyObjectToolParams }

func (t *slaPoliciesTool) Execute(ctx context.Context, args string) (string, error) {
	policies, err := t.app.report.GetSLAPolicies(t.user.TenantID)
	if err != nil {
		return "", err
	}
	if len(policies) == 0 {
		return "This workspace has no SLA policies.", nil
	}
	var b strings.Builder
	b.WriteString("SLA policies (targets as configured, e.g. 30m, 4h, 2d):\n")
	for _, p := range policies {
		fmt.Fprintf(&b, "- %s: first response %s", p.Name, p.FirstResponseTime)
		if p.NextResponseTime != "" {
			fmt.Fprintf(&b, ", next response %s", p.NextResponseTime)
		}
		fmt.Fprintf(&b, ", resolution %s", p.ResolutionTime)
		if d := strings.TrimSpace(p.Description); d != "" {
			fmt.Fprintf(&b, " | %s", d)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// resolveDays returns the days the model asked for, bounded like the dashboard's range, or the dashboard's own range when it asked for none.
func (b slaToolBase) resolveDays(days *int) int {
	if days == nil {
		return b.days
	}
	return min(max(*days, 0), maxSLAToolDays)
}

const invalidSLAMetric = "metric must be first_response, next_response or resolution."

// slaMetricArg checks a metric argument; blank means all metrics.
func slaMetricArg(metric string) (string, bool) {
	switch metric {
	case "", report.SLAMetricFirstResponse, report.SLAMetricNextResponse, report.SLAMetricResolution:
		return metric, true
	}
	return "", false
}

func slaMetricLabel(metric string) string {
	switch metric {
	case report.SLAMetricFirstResponse:
		return "first response"
	case report.SLAMetricNextResponse:
		return "next response"
	case report.SLAMetricResolution:
		return "resolution"
	}
	return metric
}

func slaListLimit(limit int) int {
	if limit <= 0 {
		return defaultSLAListLimit
	}
	return min(limit, maxSLAListLimit)
}

func periodLabel(days int) string {
	if days == 0 {
		return "today"
	}
	if days == 1 {
		return "the last 1 day"
	}
	return fmt.Sprintf("the last %d days", days)
}

// compliance renders met / (met + breached) as the dashboard does, or "no" when nothing finished.
func compliance(met, breached int) string {
	if met+breached == 0 {
		return "no"
	}
	return fmt.Sprintf("%.1f%%", math.Round(float64(met)/float64(met+breached)*1000)/10)
}

func writeSLAConversationDetails(b *strings.Builder, r report.SLAConversation) {
	if s := strings.TrimSpace(r.Subject); s != "" {
		fmt.Fprintf(b, " | Subject: %s", s)
	}
	if r.Status != "" {
		fmt.Fprintf(b, " | Status: %s", r.Status)
	}
	team := r.TeamName
	if team == "" {
		team = "none"
	}
	agent := r.AgentName
	if agent == "" {
		agent = "unassigned"
	}
	fmt.Fprintf(b, " | Team: %s | Agent: %s", team, agent)
	if r.PolicyName != "" {
		fmt.Fprintf(b, " | Policy: %s", r.PolicyName)
	}
	b.WriteString("\n")
}

func writeHiddenNote(b *strings.Builder, hidden int) {
	if hidden > 0 {
		fmt.Fprintf(b, "%d more in this list are in conversations this agent cannot open, so they are left out.\n", hidden)
	}
}

// filterAccessibleSLAConversations drops conversations the agent cannot open, using the same enforcer the UI uses.
func filterAccessibleSLAConversations(app *App, user umodels.User, rows []report.SLAConversation) []report.SLAConversation {
	out := make([]report.SLAConversation, 0, len(rows))
	for _, r := range rows {
		allowed, err := app.authz.EnforceConversationAccess(user, cmodels.Conversation{
			AssignedUserID: r.AssignedUserID,
			AssignedTeamID: r.AssignedTeamID,
		})
		if err != nil || !allowed {
			continue
		}
		out = append(out, r)
	}
	return out
}

// parseToolArgs decodes a tool's JSON arguments; blank arguments leave in untouched.
func parseToolArgs(args string, in any) error {
	if strings.TrimSpace(args) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(args), in); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func formatToolTime(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04")
}

// formatToolDuration renders seconds as the two largest units, such as 2h 15m or 1d 3h.
func formatToolDuration(sec float64) string {
	s := int(math.Round(sec))
	if s < 60 {
		return fmt.Sprintf("%ds", max(s, 0))
	}
	d, h, m := s/86400, (s%86400)/3600, (s%3600)/60
	switch {
	case d > 0 && h > 0:
		return fmt.Sprintf("%dd %dh", d, h)
	case d > 0:
		return fmt.Sprintf("%dd", d)
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", m)
}
