package main

import (
	"strconv"
	"strings"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	wh "github.com/conectdesk/conectdesk/internal/webhook"
	"github.com/conectdesk/conectdesk/internal/webhook/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// handleGetWebhooksCompact returns all webhooks in a compact format.
func handleGetWebhooksCompact(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
	)
	webhooks, err := app.webhook.GetAllCompact(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(webhooks)
}

// handleGetWebhooks returns all webhooks from the database.
func handleGetWebhooks(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
	)
	webhooks, err := app.webhook.GetAll(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	for i := range webhooks {
		hideWebhookSecrets(&webhooks[i])
	}
	return r.SendEnvelope(webhooks)
}

// handleGetWebhook returns a specific webhook by ID.
func handleGetWebhook(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	webhook, err := app.webhook.Get(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	hideWebhookSecrets(&webhook)
	return r.SendEnvelope(webhook)
}

// handleCreateWebhook creates a new webhook in the database.
func handleCreateWebhook(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		webhook = models.Webhook{}
	)
	if err := r.Decode(&webhook, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}

	// Validate webhook fields
	if err := validateWebhook(app, tenantID(r), webhook); err != nil {
		return sendErrorEnvelope(r, err)
	}

	webhook, err := app.webhook.Create(tenantID(r), webhook)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	hideWebhookSecrets(&webhook)
	return r.SendEnvelope(webhook)
}

// handleUpdateWebhook updates an existing webhook in the database.
func handleUpdateWebhook(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		webhook = models.Webhook{}
		id, _   = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)

	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	if err := r.Decode(&webhook, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}

	// A Slack or Teams URL comes back masked when the form is saved without
	// changing it; keep the one stored.
	if wh.IsMaskedURL(webhook.URL) {
		existing, err := app.webhook.Get(tenantID(r), id)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		webhook.URL = existing.URL
	}

	// Validate webhook fields
	if err := validateWebhook(app, tenantID(r), webhook); err != nil {
		return sendErrorEnvelope(r, err)
	}

	updatedWebhook, err := app.webhook.Update(tenantID(r), id, webhook)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	hideWebhookSecrets(&updatedWebhook)
	return r.SendEnvelope(updatedWebhook)
}

// handleDeleteWebhook deletes a webhook from the database.
func handleDeleteWebhook(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	if err := app.webhook.Delete(tenantID(r), id); err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(true)
}

// handleToggleWebhook toggles the active status of a webhook.
func handleToggleWebhook(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)

	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	toggledWebhook, err := app.webhook.Toggle(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	hideWebhookSecrets(&toggledWebhook)
	return r.SendEnvelope(toggledWebhook)
}

// handleTestWebhook sends a test payload to a webhook.
func handleTestWebhook(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)

	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	if err := app.webhook.SendTestWebhook(tenantID(r), id); err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(true)
}

// hideWebhookSecrets masks what must not leave the server: the signing secret,
// and the URL of a Slack or Teams webhook, which anyone could post to the
// channel with.
func hideWebhookSecrets(webhook *models.Webhook) {
	if webhook.Secret != "" {
		webhook.Secret = strings.Repeat(stringutil.PasswordDummy, 10)
	}
	if wh.IsChatKind(webhook.Kind) {
		webhook.URL = wh.MaskURL(webhook.URL)
	}
}

// validateWebhook validates the webhook data.
func validateWebhook(app *App, tenantID int, webhook models.Webhook) error {
	if webhook.Name == "" {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "`name`"), nil)
	}
	if webhook.URL == "" || wh.IsMaskedURL(webhook.URL) {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "`url`"), nil)
	}
	if len(webhook.Events) == 0 {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "`events`"), nil)
	}
	if webhook.Kind == "" {
		webhook.Kind = models.KindCustom
	}
	switch err := wh.ValidateURL(webhook.Kind, webhook.URL); err {
	case nil:
	case wh.ErrSlackURL:
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("admin.webhook.kind.slack.invalidURL"), nil)
	case wh.ErrTeamsURL:
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("admin.webhook.kind.teams.invalidURL"), nil)
	default:
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalid"), nil)
	}
	return nil
}
