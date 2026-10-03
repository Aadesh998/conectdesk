// Package report handles the management of reports.
package report

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/report/models"
	"github.com/jmoiron/sqlx"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
)

type Manager struct {
	q    queries
	lo   *logf.Logger
	i18n *i18ncat.Catalog
	db   *sqlx.DB
}

// Opts contains options for initializing the report Manager.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18ncat.Catalog
}

// queries contains prepared SQL queries.
type queries struct {
	GetOverviewCharts          string `query:"get-overview-charts"`
	GetOverviewCounts          string `query:"get-overview-counts"`
	GetOverviewSLA             string `query:"get-overview-sla-counts"`
	GetOverviewCSAT            string `query:"get-overview-csat"`
	GetOverviewMessageVolume   string `query:"get-overview-message-volume"`
	GetOverviewTagDistribution string `query:"get-overview-tag-distribution"`
	GetCallStats               string `query:"get-call-stats"`
	GetCallLog                 string `query:"get-call-log"`
	GetSLABreakdown            string `query:"get-sla-breakdown"`
	GetSLABreaches             string `query:"get-sla-breaches"`
	GetSLAAtRisk               string `query:"get-sla-at-risk"`
	GetSLATrend                string `query:"get-sla-trend"`
	GetSLAPolicies             string `query:"get-sla-policies"`
}

// New creates and returns a new instance of the Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{
		q:    q,
		lo:   opts.Lo,
		i18n: opts.I18n,
		db:   opts.DB,
	}, nil
}

// GetOverViewCounts returns overview counts
func (m *Manager) GetOverViewCounts(tenantID int) (json.RawMessage, error) {
	var counts = json.RawMessage{}
	tx, err := m.db.BeginTxx(context.Background(), &sql.TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		m.lo.Error("error starting db txn", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	defer tx.Rollback()

	if err := tx.Get(&counts, m.q.GetOverviewCounts, tenantID); err != nil {
		m.lo.Error("error fetching overview counts", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	if err := tx.Commit(); err != nil {
		m.lo.Error("error committing db txn", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	return counts, nil
}

// GetOverviewSLA returns overview SLA data
func (m *Manager) GetOverviewSLA(tenantID, days int) (json.RawMessage, error) {
	tx, err := m.db.BeginTxx(context.Background(), &sql.TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		m.lo.Error("error starting db txn", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	defer tx.Rollback()

	var result models.OverviewSLA
	// Format query with days parameter for both CTEs
	query := fmt.Sprintf(m.q.GetOverviewSLA, days, days, days, days)
	if err := tx.Get(&result, query, tenantID); err != nil {
		m.lo.Error("error fetching overview SLA data", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	if err := tx.Commit(); err != nil {
		m.lo.Error("error committing db txn", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	slaData, err := json.Marshal(result)
	if err != nil {
		m.lo.Error("error marshaling SLA data", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	return slaData, nil
}

// GetOverviewChart returns overview chart data
func (m *Manager) GetOverviewChart(tenantID, days int) (json.RawMessage, error) {
	var stats = json.RawMessage{}
	tx, err := m.db.BeginTxx(context.Background(), &sql.TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		m.lo.Error("error starting db txn", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	defer tx.Rollback()

	query := fmt.Sprintf(m.q.GetOverviewCharts, days, days, days, days)
	if err := tx.Get(&stats, query, tenantID); err != nil {
		m.lo.Error("error fetching overview charts", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return stats, nil
}

// GetCallStats returns phone call metrics for the last days (0 for today).
func (m *Manager) GetCallStats(tenantID, days int) (json.RawMessage, error) {
	var stats = json.RawMessage{}
	if err := m.db.Get(&stats, m.q.GetCallStats, tenantID, days); err != nil {
		m.lo.Error("error fetching call stats", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return stats, nil
}

// CallLogEntry is one call in the call log. RecordingURL is filled in by the
// caller, which signs it for the media store.
type CallLogEntry struct {
	Total                int             `db:"total" json:"-"`
	MessageUUID          string          `db:"message_uuid" json:"message_uuid"`
	CreatedAt            time.Time       `db:"created_at" json:"created_at"`
	Call                 json.RawMessage `db:"call" json:"call"`
	ConversationUUID     string          `db:"conversation_uuid" json:"conversation_uuid"`
	ReferenceNumber      string          `db:"reference_number" json:"reference_number"`
	ContactName          string          `db:"contact_name" json:"contact_name"`
	RecordingUUID        null.String     `db:"recording_uuid" json:"-"`
	RecordingName        null.String     `db:"recording_name" json:"recording_name"`
	RecordingContentType null.String     `db:"recording_content_type" json:"recording_content_type"`
	RecordingURL         string          `db:"-" json:"recording_url"`
}

// GetCallLog returns a page of the calls in the last days (0 for today),
// newest first, and how many there are. recorded keeps only calls with a
// recording.
func (m *Manager) GetCallLog(tenantID, days int, recorded bool, page, perPage int) ([]CallLogEntry, int, error) {
	out := []CallLogEntry{}
	if err := m.db.Select(&out, m.q.GetCallLog, tenantID, days, recorded, perPage, (page-1)*perPage); err != nil {
		m.lo.Error("error fetching call log", "error", err)
		return nil, 0, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}
	return out, total, nil
}

// GetOverviewCSAT returns CSAT metrics for the overview dashboard
func (m *Manager) GetOverviewCSAT(tenantID, days int) (json.RawMessage, error) {
	var stats = json.RawMessage{}
	tx, err := m.db.BeginTxx(context.Background(), &sql.TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		m.lo.Error("error starting db txn", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	defer tx.Rollback()

	query := fmt.Sprintf(m.q.GetOverviewCSAT, days, days)
	if err := tx.Get(&stats, query, tenantID); err != nil {
		m.lo.Error("error fetching overview CSAT", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return stats, nil
}

// GetOverviewMessageVolume returns message volume metrics for the overview dashboard
func (m *Manager) GetOverviewMessageVolume(tenantID, days int) (json.RawMessage, error) {
	var stats = json.RawMessage{}
	tx, err := m.db.BeginTxx(context.Background(), &sql.TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		m.lo.Error("error starting db txn", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	defer tx.Rollback()

	query := fmt.Sprintf(m.q.GetOverviewMessageVolume, days, days)
	if err := tx.Get(&stats, query, tenantID); err != nil {
		m.lo.Error("error fetching overview message volume", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return stats, nil
}

// GetOverviewTagDistribution returns tag distribution metrics for the overview dashboard
func (m *Manager) GetOverviewTagDistribution(tenantID, days int) (json.RawMessage, error) {
	var stats = json.RawMessage{}
	tx, err := m.db.BeginTxx(context.Background(), &sql.TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		m.lo.Error("error starting db txn", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	defer tx.Rollback()

	query := fmt.Sprintf(m.q.GetOverviewTagDistribution, days, days, days, days)
	if err := tx.Get(&stats, query, tenantID); err != nil {
		m.lo.Error("error fetching overview tag distribution", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return stats, nil
}
