package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	cmodels "github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/dailyreport"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/whatsapp"
	lmodels "github.com/conectdesk/conectdesk/internal/lead/models"
	"github.com/conectdesk/conectdesk/internal/phone"
	"github.com/jmoiron/sqlx"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// initDailyReport inits the owner's daily report.
func initDailyReport(db *sqlx.DB, app *App) *dailyreport.Manager {
	m, err := dailyreport.New(dailyreport.Opts{
		DB:        db,
		Settings:  app.setting,
		WonStages: func(tid int) []string { return stageKeys(app, tid, lmodels.KindWon) },
		Lo:        initLogger("daily_report"),
	})
	if err != nil {
		log.Fatalf("error initializing daily report: %v", err)
	}
	return m
}

// stageKeys returns the keys of a workspace's lead stages of a kind.
func stageKeys(app *App, tid int, kind string) []string {
	stages, err := app.lead.Stages(tid)
	if err != nil {
		return nil
	}
	var keys []string
	for _, st := range stages {
		if st.Kind == kind {
			keys = append(keys, st.Key)
		}
	}
	return keys
}

// sendDailyReport sends a workspace's report to each recipient on WhatsApp,
// with the report's approved template. The report isn't part of any
// conversation, so it goes straight to WhatsApp rather than through the
// outgoing queue; a failed send is recorded and shown on the settings page.
func sendDailyReport(app *App) dailyreport.Sender {
	return func(tid int, s dailyreport.Settings, sum dailyreport.Summary) error {
		wa, err := runningWhatsAppInbox(app, tid, s.WhatsAppInbox)
		if err != nil {
			return errors.New(callBackErrText(err))
		}
		c, cancel := context.WithTimeout(app.ctx, whatsAppGraphTimeout)
		t, err := wa.FindTemplate(c, s.TemplateName, s.TemplateLang)
		cancel()
		if err != nil {
			return fmt.Errorf("template %s (%s): %w", s.TemplateName, s.TemplateLang, err)
		}
		req := whatsapp.TemplateRequest{
			Name:     s.TemplateName,
			Language: s.TemplateLang,
			Body:     sum.Values(len(whatsapp.Summarize(t).BodyParams)),
		}
		_, meta, err := prepareWhatsAppTemplate(app, tid, s.WhatsAppInbox, req, nil)
		if err != nil {
			return errors.New(callBackErrText(err))
		}
		metaJSON, _ := json.Marshal(meta)

		var errs []string
		for _, to := range s.Recipients {
			msg := cmodels.OutboundMessage{UUID: "daily-report", To: []string{whatsapp.WAIDFromPhone(to)}, Meta: metaJSON}
			if _, _, err := wa.SendTracked(msg); err != nil {
				errs = append(errs, to+": "+err.Error())
			}
		}
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "; "))
		}
		app.lo.Info("daily report sent", "tenant_id", tid, "recipients", len(s.Recipients))
		return nil
	}
}

// callBackErrText is an error as the text an admin sees.
func callBackErrText(err error) string {
	var envErr envelope.Error
	if errors.As(err, &envErr) {
		return envErr.Message
	}
	return err.Error()
}

// handleGetDailyReportSettings returns the daily report's settings and the
// last week's reports.
func handleGetDailyReportSettings(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
	)
	s, err := app.dailyReport.Settings(tid)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	runs, _ := app.dailyReport.LastRuns(tid)
	return r.SendEnvelope(map[string]any{"settings": s, "runs": runs, "timezone": app.setting.GetAppTimezone(tid)})
}

// handleUpdateDailyReportSettings saves the daily report's settings.
func handleUpdateDailyReportSettings(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
		s   dailyreport.Settings
	)
	if err := r.Decode(&s, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if err := normalizeDailyReport(app, tid, &s); err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := app.setting.Update(tid, s); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return handleGetDailyReportSettings(r)
}

// normalizeDailyReport puts the recipients in international form and checks
// the settings.
func normalizeDailyReport(app *App, tid int, s *dailyreport.Settings) error {
	t := app.i18n.For(tid)
	numbers := []string{}
	for _, n := range s.Recipients {
		if strings.TrimSpace(n) == "" {
			continue
		}
		e := phone.E164(n)
		if len(e) < 9 {
			return envelope.NewError(envelope.InputError, t.Ts("dailyReport.invalidNumber", "number", n), nil)
		}
		numbers = append(numbers, e)
	}
	s.Recipients = numbers
	s.TemplateName, s.TemplateLang = strings.TrimSpace(s.TemplateName), strings.TrimSpace(s.TemplateLang)
	if err := s.Validate(); err != nil {
		return envelope.NewError(envelope.InputError, t.T("dailyReport.invalid"), nil)
	}
	if s.WhatsAppInbox > 0 {
		if _, err := runningWhatsAppInbox(app, tid, s.WhatsAppInbox); err != nil {
			return err
		}
	}
	return nil
}

// handleTestDailyReport sends today's report so far now, with the settings in
// the request, without counting as the day's report.
func handleTestDailyReport(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
		s   dailyreport.Settings
	)
	if err := r.Decode(&s, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	s.Enabled = true
	if err := normalizeDailyReport(app, tid, &s); err != nil {
		return sendErrorEnvelope(r, err)
	}
	sum, err := app.dailyReport.Summarize(tid, time.Now())
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	if err := sendDailyReport(app)(tid, s, sum); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tid).Ts("dailyReport.testFailed", "error", err.Error()), nil))
	}
	return r.SendEnvelope(sum)
}

// handleGetTodaySummary returns today's numbers so far, in the workspace's
// time zone.
func handleGetTodaySummary(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
	)
	sum, err := app.dailyReport.Summarize(tid, time.Now())
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	return r.SendEnvelope(sum)
}
