// Package dailyreport sends a workspace's owner the day's numbers on WhatsApp
// at a set time: enquiries, calls and missed calls, call-backs still waiting,
// and admissions. Each day's report is claimed in the database, so it goes out
// once however many instances run.
package dailyreport

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

//go:embed queries.sql
var efs embed.FS

// MaxRecipients caps who gets the report.
const MaxRecipients = 10

var timePattern = regexp.MustCompile(`^(?:[01]\d|2[0-3]):[0-5]\d$`)

// Settings are a workspace's daily report settings.
type Settings struct {
	Enabled bool `json:"daily_report.enabled"`
	// Time is when it goes out, HH:MM in the workspace's time zone.
	Time          string `json:"daily_report.time"`
	WhatsAppInbox int    `json:"daily_report.whatsapp_inbox_id"`
	TemplateName  string `json:"daily_report.template_name"`
	TemplateLang  string `json:"daily_report.template_language"`
	// Recipients are WhatsApp numbers in international form.
	Recipients []string `json:"daily_report.recipients"`
}

// Validate checks the settings of a report that is on.
func (s Settings) Validate() error {
	if !timePattern.MatchString(s.Time) {
		return errors.New("time")
	}
	if !s.Enabled {
		return nil
	}
	if s.WhatsAppInbox <= 0 || strings.TrimSpace(s.TemplateName) == "" || strings.TrimSpace(s.TemplateLang) == "" {
		return errors.New("template")
	}
	if len(s.Recipients) == 0 || len(s.Recipients) > MaxRecipients {
		return errors.New("recipients")
	}
	return nil
}

// Summary is a workspace's day.
type Summary struct {
	Date             string `db:"-" json:"date"`
	Enquiries        int    `db:"enquiries" json:"enquiries"`
	NewLeads         int    `db:"new_leads" json:"new_leads"`
	CallsIn          int    `db:"calls_in" json:"calls_in"`
	MissedCalls      int    `db:"missed_calls" json:"missed_calls"`
	CalledBack       int    `db:"called_back" json:"called_back"`
	PendingCallBacks int    `db:"pending_call_backs" json:"pending_call_backs"`
	OverdueCallBacks int    `db:"overdue_call_backs" json:"overdue_call_backs"`
	Admissions       int    `db:"admissions" json:"admissions"`
}

// Run is a report sent, or tried.
type Run struct {
	ReportDate string      `db:"report_date" json:"report_date"`
	CreatedAt  time.Time   `db:"created_at" json:"created_at"`
	Status     string      `db:"status" json:"status"`
	Error      null.String `db:"error" json:"error"`
}

// SettingStore reads the workspace's settings.
type SettingStore interface {
	GetByPrefix(tenantID int, prefix string) (types.JSONText, error)
	GetAppTimezone(tenantID int) string
}

// Sender sends a workspace's report.
type Sender func(tenantID int, s Settings, sum Summary) error

// Manager builds and schedules the reports.
type Manager struct {
	q        queries
	settings SettingStore
	// wonStages returns the stage keys that count as admissions.
	wonStages func(tenantID int) []string
	lo        *logf.Logger
}

// Opts holds the Manager's dependencies.
type Opts struct {
	DB        *sqlx.DB
	Settings  SettingStore
	WonStages func(tenantID int) []string
	Lo        *logf.Logger
}

type queries struct {
	Summary        *sqlx.Stmt `query:"summary"`
	EnabledTenants *sqlx.Stmt `query:"enabled-tenants"`
	ClaimRun       *sqlx.Stmt `query:"claim-run"`
	FinishRun      *sqlx.Stmt `query:"finish-run"`
	LastRuns       *sqlx.Stmt `query:"last-runs"`
}

// New returns a Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, settings: opts.Settings, wonStages: opts.WonStages, lo: opts.Lo}, nil
}

// Settings returns a workspace's report settings.
func (m *Manager) Settings(tenantID int) (Settings, error) {
	s := Settings{Time: "20:00", TemplateLang: "en"}
	b, err := m.settings.GetByPrefix(tenantID, "daily_report.")
	if err != nil {
		return s, err
	}
	if len(b) > 0 && string(b) != "null" {
		if err := json.Unmarshal(b, &s); err != nil {
			return s, err
		}
	}
	if s.Recipients == nil {
		s.Recipients = []string{}
	}
	return s, nil
}

// Location is a workspace's time zone.
func (m *Manager) Location(tenantID int) *time.Location {
	if loc, err := time.LoadLocation(m.settings.GetAppTimezone(tenantID)); err == nil {
		return loc
	}
	return time.UTC
}

// Summarize returns the numbers for the day of at, in the workspace's time
// zone, up to at.
func (m *Manager) Summarize(tenantID int, at time.Time) (Summary, error) {
	local := at.In(m.Location(tenantID))
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	var won []string
	if m.wonStages != nil {
		won = m.wonStages(tenantID)
	}
	var s Summary
	if err := m.q.Summary.Get(&s, tenantID, start, at, pq.Array(won)); err != nil {
		m.lo.Error("error building daily summary", "error", err)
		return s, err
	}
	s.Date = local.Format("2 Jan 2006")
	return s, nil
}

// LastRuns returns the last week's reports.
func (m *Manager) LastRuns(tenantID int) ([]Run, error) {
	runs := []Run{}
	err := m.q.LastRuns.Select(&runs, tenantID)
	return runs, err
}

// Run sends each workspace's report once a day once its time has come,
// checking every interval.
func (m *Manager) Run(ctx context.Context, interval time.Duration, send Sender) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			m.sendDue(now, send)
		}
	}
}

func (m *Manager) sendDue(now time.Time, send Sender) {
	var tenants []int
	if err := m.q.EnabledTenants.Select(&tenants); err != nil {
		m.lo.Error("error listing workspaces with a daily report", "error", err)
		return
	}
	for _, tid := range tenants {
		s, err := m.Settings(tid)
		if err != nil || !s.Enabled || s.Validate() != nil {
			continue
		}
		local := now.In(m.Location(tid))
		if !Due(s.Time, local) {
			continue
		}
		runID, ok := m.claim(tid, local)
		if !ok {
			continue
		}
		sum, err := m.Summarize(tid, now)
		if err == nil {
			err = send(tid, s, sum)
		}
		m.finish(tid, runID, err)
	}
}

// Due reports whether a report set for at (HH:MM) is due at local: it's that
// time or later on local's day.
func Due(at string, local time.Time) bool {
	return local.Format("15:04") >= at
}

func (m *Manager) claim(tenantID int, local time.Time) (int64, bool) {
	var id int64
	if err := m.q.ClaimRun.Get(&id, tenantID, local.Format(time.DateOnly)); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			m.lo.Error("error claiming daily report", "tenant_id", tenantID, "error", err)
		}
		return 0, false
	}
	return id, true
}

func (m *Manager) finish(tenantID int, runID int64, sendErr error) {
	status, msg := "sent", null.String{}
	if sendErr != nil {
		status, msg = "failed", null.StringFrom(sendErr.Error())
		m.lo.Error("error sending daily report", "tenant_id", tenantID, "error", sendErr)
	}
	if _, err := m.q.FinishRun.Exec(tenantID, runID, status, msg); err != nil {
		m.lo.Error("error recording daily report", "tenant_id", tenantID, "error", err)
	}
}

// Values are a summary as the report template's body values, in order: the
// date, enquiries, calls in, missed calls, call-backs still waiting and
// admissions. A template with fewer values takes the first ones.
func (s Summary) Values(n int) []string {
	all := []string{
		s.Date,
		itoa(s.Enquiries),
		itoa(s.CallsIn),
		itoa(s.MissedCalls),
		itoa(s.PendingCallBacks),
		itoa(s.Admissions),
	}
	if n < len(all) {
		return all[:n]
	}
	return all
}

func itoa(n int) string { return strconv.Itoa(n) }
