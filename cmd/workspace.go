package main

import (
	"encoding/json"
	"path/filepath"
	"strconv"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/retention"
	smodels "github.com/conectdesk/conectdesk/internal/setting/models"
	"github.com/conectdesk/conectdesk/internal/tenant"
	tmodels "github.com/conectdesk/conectdesk/internal/tenant/models"
	realip "github.com/ferluci/fast-realip"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// workspaceUsage is what a workspace uses against its plan's limits. A 0
// limit is unlimited.
type workspaceUsage struct {
	Usage  tmodels.Usage  `json:"usage"`
	Limits map[string]int `json:"limits"`
	// FairUse is what the plan meters besides agents and inboxes.
	FairUse map[string]any `json:"fair_use"`
}

// planUsage returns what the tenant uses against its plan, as the platform
// dashboard and the workspace's own usage page show it.
func planUsage(app *App, ctx *fasthttp.RequestCtx, tenantID int) (workspaceUsage, error) {
	var out workspaceUsage
	usage, err := app.tenant.Usage(tenantID)
	if err != nil {
		return out, err
	}
	storage, err := app.media.StorageUsed(tenantID)
	if err != nil {
		return out, err
	}
	aiRequests, err := app.ai.Usage(tenantID)
	if err != nil {
		return out, err
	}

	// The default workspace has no plan limits.
	limits := map[string]int{}
	for _, k := range []string{planAgents, planInboxes, "storage_mb", "file_upload_mb"} {
		limits[k] = planLimit(app, tenantID, "max_"+k)
	}
	limits["ai_requests_per_month"] = planLimit(app, tenantID, "ai_requests_per_month")
	limits["api_requests_per_minute"] = planLimit(app, tenantID, "api_requests_per_minute")
	return workspaceUsage{
		Usage:  usage,
		Limits: limits,
		FairUse: map[string]any{
			"storage_bytes":     storage,
			"ai_requests_month": aiRequests,
			"throttled_today":   app.rateLimit.TenantHits(ctx, tenantID),
		},
	}, nil
}

// handleGetWorkspaceUsage returns the workspace's plan and what it uses
// against the plan's limits.
func handleGetWorkspaceUsage(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
	)
	usage, err := planUsage(app, r.RequestCtx, tid)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	// Plans only exist with multi-tenancy, and the default workspace has none.
	plan := ""
	if app.tenant.Enabled() && tid != tenant.DefaultID {
		plan = tenantFromReq(r).Plan
	}
	return r.SendEnvelope(map[string]any{
		"plan":        plan,
		"upgrade_url": ko.String("app.tenancy.upgrade_url"),
		"usage":       usage.Usage,
		"limits":      usage.Limits,
		"fair_use":    usage.FairUse,
	})
}

// onboardingDismissed is the workspace setting that hides the setup checklist.
const onboardingDismissed = "onboarding.dismissed"

// handleGetOnboarding returns the setup checklist: which first steps the
// workspace has done, and whether an admin hid the checklist.
func handleGetOnboarding(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
	)
	steps, err := app.tenant.Onboarding(tid)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	var dismissed bool
	if b, err := app.setting.Get(tid, onboardingDismissed); err == nil {
		json.Unmarshal(b, &dismissed)
	}
	return r.SendEnvelope(map[string]any{"steps": steps, "dismissed": dismissed})
}

// handleDismissOnboarding hides the setup checklist for the workspace.
func handleDismissOnboarding(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
	)
	if err := app.setting.Update(tid, map[string]any{onboardingDismissed: true}); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(true)
}

// handleGetPrivacySettings returns the workspace's data retention and the
// installation's legal documents.
func handleGetPrivacySettings(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
		out smodels.Privacy
	)
	b, err := app.setting.GetByPrefix(tid, "privacy")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	return r.SendEnvelope(map[string]any{
		"privacy.retention_days":            out.RetentionDays,
		"privacy.retention_delete_contacts": out.RetentionDeleteContacts,
		"min_days":                          retention.MinDays,
		"max_days":                          retention.MaxDays,
		"legal": map[string]string{
			"privacy_policy_url": ko.String("app.legal.privacy_policy_url"),
			"dpa_url":            ko.String("app.legal.dpa_url"),
			"subprocessors_url":  ko.String("app.legal.subprocessors_url"),
		},
	})
}

// handleUpdatePrivacySettings sets the workspace's data retention.
func handleUpdatePrivacySettings(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		req   smodels.Privacy
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	if req.RetentionDays != 0 && (req.RetentionDays < retention.MinDays || req.RetentionDays > retention.MaxDays) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).Ts("admin.privacy.invalidDays",
			"min", strconv.Itoa(retention.MinDays), "max", strconv.Itoa(retention.MaxDays)), nil, envelope.InputError)
	}
	if req.RetentionDays == 0 {
		req.RetentionDeleteContacts = false
	}
	if err := app.setting.Update(tid, req); err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := app.activityLog.RetentionPolicyChanged(tid, auser.ID, auser.Email, realip.FromRequest(r.RequestCtx),
		req.RetentionDays, req.RetentionDeleteContacts); err != nil {
		app.lo.Error("error recording retention policy change", "error", err)
	}
	return r.SendEnvelope(true)
}

// onRetentionApplied returns what runs after data retention deleted a
// workspace's data: the deleted contacts' avatars are removed from the store
// and the deletion goes to the workspace's activity log.
func onRetentionApplied(app *App) func(retention.Policy, retention.Result) {
	return func(p retention.Policy, res retention.Result) {
		for _, a := range res.Avatars {
			if err := app.media.Delete(p.TenantID, filepath.Base(a)); err != nil {
				app.lo.Error("error deleting avatar of expired contact", "tenant_id", p.TenantID, "error", err)
			}
		}
		sys, err := app.user.GetSystemUser(p.TenantID)
		if err != nil {
			return
		}
		if err := app.activityLog.RetentionApplied(p.TenantID, sys.ID, res.Conversations, res.Contacts); err != nil {
			app.lo.Error("error recording retention", "tenant_id", p.TenantID, "error", err)
		}
	}
}
