package main

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/whatsapp"
	imodels "github.com/conectdesk/conectdesk/internal/inbox/models"
	"github.com/conectdesk/conectdesk/internal/sealed"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

var (
	// Meta's IDs are numeric. Checking them keeps a pasted value from steering
	// a Graph API path anywhere else.
	reMetaID     = regexp.MustCompile(`^[0-9]{5,25}$`)
	reAppSecret  = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
	reTwoStepPIN = regexp.MustCompile(`^[0-9]{6}$`)
)

// whatsAppManualCredentials is what the admin copies from their own Meta app
// and WhatsApp Manager. It only ever arrives sealed.
type whatsAppManualCredentials struct {
	Name          string `json:"name"`
	WABAID        string `json:"waba_id"`
	PhoneNumberID string `json:"phone_number_id"`
	AppID         string `json:"app_id"`
	AppSecret     string `json:"app_secret"`
	AccessToken   string `json:"access_token"`
	// TwoStepPIN is the number's existing two-step verification PIN, if it has
	// one. Without it a number that isn't on the Cloud API yet is registered
	// with a new PIN.
	TwoStepPIN string `json:"two_step_pin"`
}

func (c *whatsAppManualCredentials) normalize() {
	for _, f := range []*string{&c.Name, &c.WABAID, &c.PhoneNumberID, &c.AppID, &c.AppSecret, &c.AccessToken, &c.TwoStepPIN} {
		*f = strings.TrimSpace(*f)
	}
}

func (c whatsAppManualCredentials) valid() bool {
	return reMetaID.MatchString(c.WABAID) &&
		reMetaID.MatchString(c.PhoneNumberID) &&
		reMetaID.MatchString(c.AppID) &&
		reAppSecret.MatchString(c.AppSecret) &&
		c.AccessToken != "" && len(c.AccessToken) <= 1024 && !strings.ContainsAny(c.AccessToken, " \t\r\n") &&
		(c.TwoStepPIN == "" || reTwoStepPIN.MatchString(c.TwoStepPIN))
}

// whatsAppGraph returns the Graph API client an inbox calls through: the
// business's own app for a number connected manually, the platform's otherwise.
func whatsAppGraph(cfg whatsapp.Config) *meta.Client {
	if !cfg.Manual() {
		return metaGraph()
	}
	return meta.NewClient(meta.App{
		AppID:        cfg.AppID,
		AppSecret:    cfg.AppSecret,
		GraphVersion: metaGraph().App().GraphVersion,
	})
}

// whatsAppWebhookURL is a manually connected inbox's own webhook, on the
// workspace's address, which the business's app delivers its events to.
func whatsAppWebhookURL(app *App, record imodels.Inbox) string {
	return strings.TrimRight(tenantConsts(app, record.TenantID).AppBaseURL, "/") + "/webhook/whatsapp/" + record.UUID
}

// handleWhatsAppManualKey issues the one-time key the browser seals the manual
// connection's credentials to.
func handleWhatsAppManualKey(r *fastglue.Request) error {
	app := r.Context.(*App)
	if err := checkChannelAllowed(app, tenantID(r), inbox.ChannelWhatsApp); err != nil {
		return sendErrorEnvelope(r, err)
	}
	return issueSealedKey(r, sealedPurposeWhatsAppManual)
}

// handleConnectWhatsAppManual connects a number with a permanent token from
// the business's own Meta app, for businesses that can't or won't use
// Embedded Signup. It checks the token and secret against the Graph API,
// creates the inbox (or reconnects the one it has), registers the number if it
// needs it, and points the account's webhooks at the inbox.
//
// The body is a sealed envelope: the credentials are never in plain text on
// the wire, even inside TLS.
func handleConnectWhatsAppManual(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
		T   = app.i18n.For(tid)
		env sealed.Envelope
	)
	if err := r.Decode(&env, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, T.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if err := checkChannelAllowed(app, tid, inbox.ChannelWhatsApp); err != nil {
		return sendErrorEnvelope(r, err)
	}

	plain, err := openSealed(r, env, sealedPurposeWhatsAppManual)
	if err != nil {
		return sendSealedError(r, err)
	}
	var creds whatsAppManualCredentials
	err = json.Unmarshal(plain, &creds)
	clear(plain)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, T.T("errors.parsingRequest"), nil, envelope.InputError)
	}
	creds.normalize()
	if !creds.valid() {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, T.T("whatsapp.manual.invalidFields"), nil, envelope.InputError)
	}

	c, cancel := context.WithTimeout(app.ctx, metaConnectTimeout)
	defer cancel()

	// Every call carries an appsecret_proof made with the secret given, which
	// Meta refuses unless the secret is the token's app's. So listing the
	// account's numbers proves the token can manage the account and the secret
	// will verify its webhooks.
	cfg := whatsapp.Config{Mode: whatsapp.ModeManual, AppID: creds.AppID, AppSecret: creds.AppSecret}
	graph := whatsAppGraph(cfg)
	numbers, err := graph.PhoneNumbers(c, creds.AccessToken, creds.WABAID)
	if err != nil {
		app.lo.Warn("whatsapp manual credentials rejected", "waba_id", creds.WABAID, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, T.Ts("whatsapp.manual.rejected", "error", graphErrorText(err)), nil, envelope.InputError)
	}
	var number *meta.PhoneNumber
	for i := range numbers {
		if numbers[i].ID == creds.PhoneNumberID {
			number = &numbers[i]
		}
	}
	if number == nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, T.T("whatsapp.manual.numberNotFound"), nil, envelope.InputError)
	}
	if owner, _, found, err := app.inbox.AccountOwner(inbox.ChannelWhatsApp, number.ID); err != nil {
		return sendErrorEnvelope(r, err)
	} else if found && owner != tid {
		return r.SendErrorEnvelope(fasthttp.StatusConflict, T.T("whatsapp.manual.numberConnectedElsewhere"), nil, envelope.InputError)
	}

	// A reconnect keeps the verify token the business's app already has, and
	// the PIN the number was registered with.
	existing, _ := existingWhatsAppConfig(app, tid, number.ID)
	cfg.AccountID = number.ID
	cfg.WABAID = creds.WABAID
	cfg.DisplayPhoneNumber = number.DisplayPhoneNumber
	cfg.VerifiedName = number.VerifiedName
	cfg.AccessToken = creds.AccessToken
	cfg.MaxMediaSizeMB = whatsapp.DefaultMaxMediaSizeMB
	cfg.MarkSeen = true
	cfg.WebhookVerifyToken = existing.WebhookVerifyToken
	if cfg.WebhookVerifyToken == "" {
		if cfg.WebhookVerifyToken, err = stringutil.RandomAlphanumeric(32); err != nil {
			return sendErrorEnvelope(r, err)
		}
	}
	cfg.TwoStepPIN = cmpOr(creds.TwoStepPIN, existing.TwoStepPIN)
	if cfg.TwoStepPIN == "" {
		if cfg.TwoStepPIN, err = randomPIN(); err != nil {
			return sendErrorEnvelope(r, err)
		}
	}

	// A number the business already sends from is on the Cloud API; registering
	// it again with a PIN it doesn't know would fail. Register only one that
	// isn't, or when the admin gave its PIN.
	if creds.TwoStepPIN != "" || !strings.EqualFold(number.Status, "CONNECTED") {
		if err := graph.RegisterPhoneNumber(c, creds.AccessToken, number.ID, cfg.TwoStepPIN); err != nil {
			app.lo.Warn("error registering whatsapp number", "phone_number_id", number.ID, "error", err)
			cfg.RegistrationError = graphErrorText(err)
		}
	}

	name := cmpOr(creds.Name, number.VerifiedName, number.DisplayPhoneNumber)
	if err := upsertMetaInbox(app, tid, inbox.ChannelWhatsApp, number.ID, name, number.DisplayPhoneNumber, cfg); err != nil {
		if errors.Is(err, errMetaAccountTaken) {
			return r.SendErrorEnvelope(fasthttp.StatusConflict, T.T("whatsapp.manual.numberConnectedElsewhere"), nil, envelope.InputError)
		}
		return sendErrorEnvelope(r, err)
	}
	_, inboxID, _, err := app.inbox.AccountOwner(inbox.ChannelWhatsApp, number.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	record, err := app.inbox.GetDBRecord(tid, inboxID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Point the account's webhooks at this inbox. Meta checks the URL before it
	// answers, so this needs the inbox saved, and the workspace reachable over
	// HTTPS. Failing isn't fatal: the admin can enter the URL in their app.
	webhookURL := whatsAppWebhookURL(app, record)
	webhookErr := ""
	if err := graph.SubscribeWABAOverride(c, cfg.AccessToken, cfg.WABAID, webhookURL, cfg.WebhookVerifyToken); err != nil {
		app.lo.Warn("error subscribing whatsapp webhook", "inbox_id", inboxID, "waba_id", cfg.WABAID, "error", err)
		webhookErr = graphErrorText(err)
	}

	app.lo.Info("whatsapp number connected manually", "tenant_id", tid, "inbox_id", inboxID, "phone_number_id", number.ID, "webhook_subscribed", webhookErr == "")
	return r.SendEnvelope(map[string]any{
		"inbox_id":           inboxID,
		"webhook_url":        webhookURL,
		"verify_token":       cfg.WebhookVerifyToken,
		"webhook_subscribed": webhookErr == "",
		"webhook_error":      webhookErr,
		"registration_error": cfg.RegistrationError,
	})
}

// graphErrorText is what Meta said was wrong, fit to show an admin. Neither the
// token nor the secret is ever in it: the token travels in a header.
func graphErrorText(err error) string {
	var ge *meta.Error
	if errors.As(err, &ge) {
		return cmpOr(ge.UserMsg, ge.Message)
	}
	return err.Error()
}

// existingWhatsAppConfig returns the config of a number's inbox in this
// workspace, if it has one.
func existingWhatsAppConfig(app *App, tid int, phoneNumberID string) (whatsapp.Config, bool) {
	var cfg whatsapp.Config
	owner, inboxID, found, err := app.inbox.AccountOwner(inbox.ChannelWhatsApp, phoneNumberID)
	if err != nil || !found || owner != tid {
		return cfg, false
	}
	record, err := app.inbox.GetDBRecord(tid, inboxID)
	if err != nil {
		return cfg, false
	}
	if json.Unmarshal(record.Config, &cfg) != nil {
		return cfg, false
	}
	return cfg, true
}

// handleGetWhatsAppWebhook shows a manually connected inbox's webhook URL and
// verify token, to enter in the business's Meta app.
func handleGetWhatsAppWebhook(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	record, cfg, ok := manualWhatsAppRecord(app, tid, id)
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tid).T("validation.notFoundInbox"), nil, envelope.NotFoundError)
	}
	return r.SendEnvelope(map[string]any{
		"webhook_url":  whatsAppWebhookURL(app, record),
		"verify_token": cfg.WebhookVerifyToken,
	})
}

// manualWhatsAppRecord returns a manually connected WhatsApp inbox and its
// decrypted config.
func manualWhatsAppRecord(app *App, tid int, identifier any) (imodels.Inbox, whatsapp.Config, bool) {
	var cfg whatsapp.Config
	record, err := app.inbox.GetDBRecord(tid, identifier)
	if err != nil || record.Channel != inbox.ChannelWhatsApp {
		return record, cfg, false
	}
	if json.Unmarshal(record.Config, &cfg) != nil || !cfg.Manual() {
		return record, cfg, false
	}
	return record, cfg, true
}

// handleWhatsAppManualWebhookVerify answers the subscription check Meta makes
// for a manually connected inbox's own webhook.
func handleWhatsAppManualWebhookVerify(r *fastglue.Request) error {
	var (
		app       = r.Context.(*App)
		inboxUUID = r.RequestCtx.UserValue("uuid").(string)
		args      = r.RequestCtx.QueryArgs()
	)
	if _, err := uuid.Parse(inboxUUID); err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		return nil
	}
	_, cfg, ok := manualWhatsAppRecord(app, tenantID(r), inboxUUID)
	if !ok {
		r.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		return nil
	}
	challenge, ok := meta.VerifyChallenge(cfg.WebhookVerifyToken,
		string(args.Peek("hub.mode")), string(args.Peek("hub.verify_token")), string(args.Peek("hub.challenge")))
	if !ok {
		r.RequestCtx.SetStatusCode(fasthttp.StatusForbidden)
		return nil
	}
	r.RequestCtx.SetContentType("text/plain; charset=utf-8")
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	r.RequestCtx.SetBodyString(challenge)
	return nil
}

// handleWhatsAppManualWebhook receives a manually connected inbox's events
// from the business's own app. The body is signed with that app's secret, so
// it is trusted only for this inbox's number: handleWhatsAppEntry drops
// anything about another.
func handleWhatsAppManualWebhook(r *fastglue.Request) error {
	var (
		app       = r.Context.(*App)
		inboxUUID = r.RequestCtx.UserValue("uuid").(string)
		body      = r.RequestCtx.PostBody()
	)
	if _, err := uuid.Parse(inboxUUID); err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		return nil
	}
	if len(body) > maxMetaWebhookBody {
		r.RequestCtx.SetStatusCode(fasthttp.StatusRequestEntityTooLarge)
		return nil
	}
	record, cfg, ok := manualWhatsAppRecord(app, tenantID(r), inboxUUID)
	if !ok {
		r.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		return nil
	}
	if !meta.VerifySignature(cfg.AppSecret, body, string(r.RequestCtx.Request.Header.Peek("X-Hub-Signature-256"))) {
		app.lo.Warn("rejected whatsapp webhook with a bad signature", "inbox_id", record.ID)
		r.RequestCtx.SetStatusCode(fasthttp.StatusUnauthorized)
		return nil
	}
	if !record.Enabled {
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	}

	var env meta.Envelope
	if err := json.Unmarshal(body, &env); err != nil || env.Object != meta.ObjectWhatsApp {
		r.RequestCtx.SetStatusCode(fasthttp.StatusBadRequest)
		return nil
	}
	inb, err := app.inbox.Get(record.ID)
	if err != nil {
		// Not running yet, e.g. mid-reload: let Meta retry.
		r.RequestCtx.SetStatusCode(fasthttp.StatusServiceUnavailable)
		return nil
	}
	wa, ok := inb.(*whatsapp.WhatsApp)
	if !ok {
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	}

	c, cancel := context.WithTimeout(app.ctx, metaWebhookTimeout)
	defer cancel()
	for _, entry := range env.Entry {
		if err := handleWhatsAppEntry(c, app, entry, wa); err != nil {
			r.RequestCtx.SetStatusCode(fasthttp.StatusServiceUnavailable)
			return nil
		}
	}
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	return nil
}
