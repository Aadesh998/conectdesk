package main

import (
	"strconv"

	"github.com/zerodha/fastglue"
)

// handleOverviewCounts retrieves general dashboard counts for all users.
func handleOverviewCounts(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
	)
	counts, err := app.report.GetOverViewCounts(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(counts)
}

// handleCallStats returns phone call metrics for the reports page.
func handleCallStats(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		days, _ = strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("days")))
	)
	if days < 0 || days > 365 {
		days = 30
	}
	stats, err := app.report.GetCallStats(tenantID(r), days)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(stats)
}

// handleCallLog returns a page of the call log, each call with a playable
// link to its recording.
func handleCallLog(r *fastglue.Request) error {
	var (
		app        = r.Context.(*App)
		tid        = tenantID(r)
		q          = r.RequestCtx.QueryArgs()
		days, _    = strconv.Atoi(string(q.Peek("days")))
		page, _    = strconv.Atoi(string(q.Peek("page")))
		perPage, _ = strconv.Atoi(string(q.Peek("page_size")))
		recorded   = string(q.Peek("recorded")) == "true"
	)
	if days < 0 || days > 365 {
		days = 30
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 25
	}
	calls, total, err := app.report.GetCallLog(tid, days, recorded, page, perPage)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	for i := range calls {
		if calls[i].RecordingUUID.Valid {
			calls[i].RecordingURL = app.media.GetURL(tid, calls[i].RecordingUUID.String, calls[i].RecordingContentType.String, calls[i].RecordingName.String)
		}
	}
	return r.SendEnvelope(map[string]any{
		"results":   calls,
		"total":     total,
		"page":      page,
		"page_size": perPage,
	})
}

// handleOverviewCharts retrieves general dashboard chart data.
func handleOverviewCharts(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		days, _ = strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("days")))
	)
	charts, err := app.report.GetOverviewChart(tenantID(r), days)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(charts)
}

// handleOverviewSLA retrieves SLA data for the dashboard.
func handleOverviewSLA(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		days, _ = strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("days")))
	)
	sla, err := app.report.GetOverviewSLA(tenantID(r), days)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(sla)
}

// handleOverviewCSAT retrieves CSAT metrics for the dashboard.
func handleOverviewCSAT(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		days, _ = strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("days")))
	)
	csat, err := app.report.GetOverviewCSAT(tenantID(r), days)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(csat)
}

// handleOverviewMessageVolume retrieves message volume metrics for the dashboard.
func handleOverviewMessageVolume(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		days, _ = strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("days")))
	)
	volume, err := app.report.GetOverviewMessageVolume(tenantID(r), days)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(volume)
}

// handleOverviewTagDistribution retrieves tag distribution metrics for the dashboard.
func handleOverviewTagDistribution(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		days, _ = strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("days")))
	)
	tags, err := app.report.GetOverviewTagDistribution(tenantID(r), days)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(tags)
}
