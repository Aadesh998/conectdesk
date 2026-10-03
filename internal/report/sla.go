package report

import (
	"time"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/volatiletech/null/v9"
)

// SLA metrics, as stored in sla_metric.
const (
	SLAMetricFirstResponse = "first_response"
	SLAMetricNextResponse  = "next_response"
	SLAMetricResolution    = "resolution"
)

// SLA breakdown groupings.
var SLAGroupings = []string{"team", "agent", "policy", "inbox", "priority"}

// SLA trend intervals.
var SLAIntervals = []string{"day", "week"}

// SLAGroupStats is one group's results for one metric.
type SLAGroupStats struct {
	GroupName     string  `db:"group_name" json:"group_name"`
	Metric        string  `db:"metric" json:"metric"`
	Met           int     `db:"met" json:"met"`
	Breached      int     `db:"breached" json:"breached"`
	Pending       int     `db:"pending" json:"pending"`
	AvgMetSeconds float64 `db:"avg_met_seconds" json:"avg_met_seconds"`
}

// SLAConversation is one SLA metric on a conversation, breached or coming due.
type SLAConversation struct {
	Total           int       `db:"total" json:"-"`
	ReferenceNumber string    `db:"reference_number" json:"reference_number"`
	Subject         string    `db:"subject" json:"subject"`
	Metric          string    `db:"metric" json:"metric"`
	DeadlineAt      null.Time `db:"deadline_at" json:"deadline_at"`
	BreachedAt      null.Time `db:"breached_at" json:"breached_at"`
	MetAt           null.Time `db:"met_at" json:"met_at"`
	Status          string    `db:"status" json:"status"`
	TeamName        string    `db:"team_name" json:"team_name"`
	AgentName       string    `db:"agent_name" json:"agent_name"`
	PolicyName      string    `db:"policy_name" json:"policy_name"`
	AssignedUserID  null.Int  `db:"assigned_user_id" json:"-"`
	AssignedTeamID  null.Int  `db:"assigned_team_id" json:"-"`
}

// SLATrendPoint is one period's met and breached count for one metric.
type SLATrendPoint struct {
	Period   time.Time `db:"period" json:"period"`
	Metric   string    `db:"metric" json:"metric"`
	Met      int       `db:"met" json:"met"`
	Breached int       `db:"breached" json:"breached"`
}

// SLAPolicySummary is an SLA policy's targets.
type SLAPolicySummary struct {
	Name              string `db:"name" json:"name"`
	Description       string `db:"description" json:"description"`
	FirstResponseTime string `db:"first_response_time" json:"first_response_time"`
	NextResponseTime  string `db:"next_response_time" json:"next_response_time"`
	ResolutionTime    string `db:"resolution_time" json:"resolution_time"`
}

// GetSLABreakdown returns the SLA results of the last days (0 for today) grouped by groupBy, for one metric or all ("").
func (m *Manager) GetSLABreakdown(tenantID, days int, groupBy, metric string) ([]SLAGroupStats, error) {
	out := []SLAGroupStats{}
	if err := m.db.Select(&out, m.q.GetSLABreakdown, tenantID, days, groupBy, metric); err != nil {
		m.lo.Error("error fetching sla breakdown", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// GetSLABreaches returns up to limit SLA breaches of the last days (0 for today), newest first, and how many match in all; blank filters keep all.
func (m *Manager) GetSLABreaches(tenantID, days int, metric, team, agent string, limit int) ([]SLAConversation, int, error) {
	out := []SLAConversation{}
	if err := m.db.Select(&out, m.q.GetSLABreaches, tenantID, days, metric, team, agent, limit); err != nil {
		m.lo.Error("error fetching sla breaches", "error", err)
		return nil, 0, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return out, totalOf(out), nil
}

// GetSLAAtRisk returns up to limit open SLA deadlines due within the next minutes or already past, soonest first, and how many there are in all.
func (m *Manager) GetSLAAtRisk(tenantID, minutes, limit int) ([]SLAConversation, int, error) {
	out := []SLAConversation{}
	if err := m.db.Select(&out, m.q.GetSLAAtRisk, tenantID, minutes, limit); err != nil {
		m.lo.Error("error fetching sla deadlines at risk", "error", err)
		return nil, 0, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return out, totalOf(out), nil
}

// GetSLATrend returns met and breached counts of the last days (0 for today) per interval ("day" or "week").
func (m *Manager) GetSLATrend(tenantID, days int, interval string) ([]SLATrendPoint, error) {
	out := []SLATrendPoint{}
	if err := m.db.Select(&out, m.q.GetSLATrend, tenantID, days, interval); err != nil {
		m.lo.Error("error fetching sla trend", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

// GetSLAPolicies returns the workspace's SLA policies and their targets.
func (m *Manager) GetSLAPolicies(tenantID int) ([]SLAPolicySummary, error) {
	out := []SLAPolicySummary{}
	if err := m.db.Select(&out, m.q.GetSLAPolicies, tenantID); err != nil {
		m.lo.Error("error fetching sla policies", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return out, nil
}

func totalOf(rows []SLAConversation) int {
	if len(rows) == 0 {
		return 0
	}
	return rows[0].Total
}
