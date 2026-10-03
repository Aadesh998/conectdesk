package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/messenger"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/whatsapp"
	imodels "github.com/conectdesk/conectdesk/internal/inbox/models"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const (
	// metaOAuthProvider is the provider name the Meta connect flow stores its
	// state under, and the path segment of its shared redirect URL.
	metaOAuthProvider = "meta"

	// maxMetaWebhookBody caps a webhook body. Meta sends metadata only; media is
	// fetched separately, so a payload this large is malformed or hostile.
	maxMetaWebhookBody = 3 << 20

	// metaConnectTimeout bounds the Graph API calls that connect accounts.
	metaConnectTimeout = 90 * time.Second

	// metaWebhookTimeout bounds handling one webhook, media downloads included.
	// Meta waits about 20 seconds before counting a delivery as failed.
	metaWebhookTimeout = 18 * time.Second

	// metaDeletionTTL is how long a data deletion confirmation stays looked up.
	metaDeletionTTL = 90 * 24 * time.Hour
)

var (
	metaOnce   sync.Once
	metaClient *meta.Client
)

// metaGraph returns the Graph API client for the platform's Meta app, set in
// [meta] in config. Its App reports Configured() false when it isn't set.
func metaGraph() *meta.Client {
	metaOnce.Do(func() {
		metaClient = meta.NewClient(meta.App{
			AppID:              strings.TrimSpace(ko.String("meta.app_id")),
			AppSecret:          strings.TrimSpace(ko.String("meta.app_secret")),
			GraphVersion:       strings.TrimSpace(ko.String("meta.graph_version")),
			WebhookVerifyToken: strings.TrimSpace(ko.String("meta.webhook_verify_token")),
			WhatsAppConfigID:   strings.TrimSpace(ko.String("meta.whatsapp_config_id")),
			LoginConfigID:      strings.TrimSpace(ko.String("meta.login_config_id")),
			HumanAgent:         ko.Bool("meta.human_agent"),
		})
	})
	return metaClient
}

// metaPublicBase is where Meta calls the webhook and the deauthorize and data
// deletion callbacks: the base domain with multi-tenancy, since one Meta app
// serves every workspace, and the app's root URL without.
func metaPublicBase(app *App) string {
	if app.tenant.Enabled() && app.tenant.BaseDomain() != "" {
		return app.tenant.BaseURL()
	}
	return strings.TrimRight(tenantConsts(app, tenant.DefaultID).AppBaseURL, "/")
}

// handleMetaAuthorize starts connecting WhatsApp numbers, Facebook Pages or
// Instagram accounts to the workspace, returning the Facebook Login URL.
//
// It shares the email OAuth flow's state store and redirect URL, so the
// browser comes back through the same relay on the base domain, bound to the
// workspace and admin who started.
func handleMetaAuthorize(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		product = r.RequestCtx.UserValue("product").(string)
		graph   = metaGraph()
		tid     = tenantID(r)
	)

	if product != meta.ProductWhatsApp && product != meta.ProductMessenger && product != meta.ProductInstagram {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	available := false
	for _, p := range graph.App().Products() {
		available = available || p == product
	}
	if !available {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("inbox.metaAppMissing"), nil, envelope.InputError)
	}
	if err := checkChannelAllowed(app, tid, product); err != nil {
		return sendErrorEnvelope(r, err)
	}

	state, err := stringutil.RandomAlphanumeric(32)
	if err != nil {
		app.lo.Error("error generating meta oauth state", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	redirectURI := oauthRedirectBase(app, tid) + "/" + metaOAuthProvider + "/callback"
	user, _ := r.RequestCtx.UserValue("user").(amodels.User)
	redisKey := "inbox_oauth:" + state
	if err := app.redis.HSet(ctx, redisKey, map[string]any{
		"provider":     metaOAuthProvider,
		"product":      product,
		"redirect_uri": redirectURI,
		"workspace_id": tid,
		"user_id":      user.ID,
	}).Err(); err != nil {
		app.lo.Error("error storing meta oauth state", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	if err := app.redis.Expire(ctx, redisKey, 15*time.Minute).Err(); err != nil {
		app.lo.Error("error setting meta oauth state expiry", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	authURL, err := graph.AuthorizeURL(product, redirectURI, state)
	if err != nil {
		app.lo.Error("error building meta authorize url", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("inbox.metaAppMissing"), nil, envelope.InputError)
	}
	return r.SendEnvelope(authURL)
}

// errMetaAccountTaken marks an account already connected to another workspace.
var errMetaAccountTaken = errors.New("account connected to another workspace")

// metaConnectResult counts what a connect run did, for the admin's toast.
type metaConnectResult struct {
	connected int
	taken     int
	failed    int
}

func (res metaConnectResult) redirect() string {
	switch {
	case res.connected > 0:
		return fmt.Sprintf("/admin/inboxes?success=meta_connected&count=%d&taken=%d&failed=%d", res.connected, res.taken, res.failed)
	case res.taken > 0:
		return "/admin/inboxes?error=meta_account_taken"
	case res.failed > 0:
		return "/admin/inboxes?error=meta_connect_failed"
	default:
		return "/admin/inboxes?error=meta_no_accounts"
	}
}

// finishMetaConnect completes the Facebook Login flow once handleOAuthCallback
// has checked the state: it trades the code for a token, then creates an inbox
// for each account shared, or reconnects the inbox it already has.
func finishMetaConnect(r *fastglue.Request, oauthData map[string]string, code string) error {
	var (
		app   = r.Context.(*App)
		graph = metaGraph()
		tid   = tenantID(r)
	)
	if !graph.App().Configured() {
		return r.Redirect("/admin/inboxes?error=meta_connect_failed", fasthttp.StatusFound, nil, "")
	}

	c, cancel := context.WithTimeout(app.ctx, metaConnectTimeout)
	defer cancel()

	token, err := graph.ExchangeCode(c, code, oauthData["redirect_uri"])
	if err != nil {
		app.lo.Error("error exchanging meta oauth code", "error", err)
		return r.Redirect("/admin/inboxes?error=token_exchange_failed", fasthttp.StatusFound, nil, "")
	}

	var res metaConnectResult
	switch product := oauthData["product"]; product {
	case meta.ProductWhatsApp:
		res, err = connectWhatsAppNumbers(c, app, tid, token.AccessToken)
	case meta.ProductMessenger, meta.ProductInstagram:
		res, err = connectPages(c, app, tid, product, token.AccessToken)
	default:
		err = fmt.Errorf("unknown meta product %q", product)
	}
	if err != nil {
		app.lo.Error("error connecting meta accounts", "product", oauthData["product"], "error", err)
		return r.Redirect("/admin/inboxes?error=meta_connect_failed", fasthttp.StatusFound, nil, "")
	}

	app.lo.Info("meta accounts connected", "tenant_id", tid, "product", oauthData["product"],
		"connected", res.connected, "taken", res.taken, "failed", res.failed)
	return r.Redirect(res.redirect(), fasthttp.StatusFound, nil, "")
}

// connectWhatsAppNumbers connects every number on the WhatsApp Business
// Accounts shared through Embedded Signup: it subscribes the app to each
// account's webhooks, registers each number for the Cloud API, and stores the
// business token on an inbox per number.
func connectWhatsAppNumbers(c context.Context, app *App, tid int, token string) (metaConnectResult, error) {
	var (
		graph = metaGraph()
		res   metaConnectResult
	)

	wabaIDs, err := graph.GrantedTargets(c, token, "whatsapp_business_management")
	if err != nil {
		return res, fmt.Errorf("reading shared whatsapp business accounts: %w", err)
	}
	connectedBy, err := graph.Me(c, token)
	if err != nil {
		app.lo.Warn("error reading meta user id", "error", err)
	}

	for _, wabaID := range wabaIDs {
		if err := graph.SubscribeWABA(c, token, wabaID); err != nil {
			app.lo.Error("error subscribing to whatsapp business account", "waba_id", wabaID, "error", err)
			res.failed++
			continue
		}
		numbers, err := graph.PhoneNumbers(c, token, wabaID)
		if err != nil {
			app.lo.Error("error listing whatsapp numbers", "waba_id", wabaID, "error", err)
			res.failed++
			continue
		}

		for _, n := range numbers {
			// Check before registering, so a number another workspace owns isn't
			// touched at all: registering sets its two-step verification PIN.
			if owner, _, found, err := app.inbox.AccountOwner(inbox.ChannelWhatsApp, n.ID); err == nil && found && owner != tid {
				res.taken++
				continue
			}

			cfg := whatsapp.Config{
				AccountID:          n.ID,
				WABAID:             wabaID,
				DisplayPhoneNumber: n.DisplayPhoneNumber,
				VerifiedName:       n.VerifiedName,
				AccessToken:        token,
				ConnectedBy:        connectedBy,
				MaxMediaSizeMB:     whatsapp.DefaultMaxMediaSizeMB,
				MarkSeen:           true,
			}

			// A number reconnected keeps the PIN it was registered with.
			if existing, _ := existingWhatsAppConfig(app, tid, n.ID); existing.TwoStepPIN != "" {
				cfg.TwoStepPIN = existing.TwoStepPIN
			} else if cfg.TwoStepPIN, err = randomPIN(); err != nil {
				return res, err
			}
			// Registering is what lets a number added through Embedded Signup send.
			// A failure is kept on the inbox rather than failing the connection: a
			// number already on the Cloud API with another PIN still receives.
			if err := graph.RegisterPhoneNumber(c, token, n.ID, cfg.TwoStepPIN); err != nil {
				app.lo.Warn("error registering whatsapp number", "phone_number_id", n.ID, "error", err)
				cfg.RegistrationError = err.Error()
			}

			name := cmpOr(n.VerifiedName, n.DisplayPhoneNumber)
			if err := upsertMetaInbox(app, tid, inbox.ChannelWhatsApp, n.ID, name, n.DisplayPhoneNumber, cfg); err != nil {
				if errors.Is(err, errMetaAccountTaken) {
					res.taken++
				} else {
					app.lo.Error("error saving whatsapp inbox", "phone_number_id", n.ID, "error", err)
					res.failed++
				}
				continue
			}
			res.connected++
		}
	}
	return res, nil
}

// connectPages connects the Facebook Pages, or the Instagram accounts linked to
// them, that the person shared, subscribing the app to each Page's webhooks.
func connectPages(c context.Context, app *App, tid int, product, shortToken string) (metaConnectResult, error) {
	var (
		graph = metaGraph()
		res   metaConnectResult
	)

	// Page tokens read with a long-lived user token don't expire.
	userToken := shortToken
	if ll, err := graph.LongLivedToken(c, shortToken); err == nil && ll.AccessToken != "" {
		userToken = ll.AccessToken
	} else if err != nil {
		app.lo.Warn("error exchanging for a long-lived meta token", "error", err)
	}

	connectedBy, err := graph.Me(c, userToken)
	if err != nil {
		app.lo.Warn("error reading meta user id", "error", err)
	}
	pages, err := graph.Pages(c, userToken)
	if err != nil {
		return res, fmt.Errorf("listing pages: %w", err)
	}

	for _, p := range pages {
		if p.AccessToken == "" {
			continue
		}
		cfg := messenger.Config{
			PageID:         p.ID,
			PageName:       p.Name,
			AccessToken:    p.AccessToken,
			ConnectedBy:    connectedBy,
			MaxMediaSizeMB: messenger.DefaultMaxMediaSizeMB,
		}
		var (
			channel = inbox.ChannelMessenger
			name    = p.Name
			from    = p.Name
			fields  = []string{"messages", "messaging_postbacks", "message_deliveries", "message_reads"}
		)
		if product == meta.ProductInstagram {
			if p.Instagram == nil || p.Instagram.ID == "" {
				continue
			}
			channel = inbox.ChannelInstagram
			cfg.AccountID = p.Instagram.ID
			cfg.Username = p.Instagram.Username
			name = cmpOr(p.Instagram.Name, "@"+p.Instagram.Username)
			from = "@" + p.Instagram.Username
			fields = []string{"messages"}
		} else {
			cfg.AccountID = p.ID
		}

		// Check before subscribing, so an account another workspace owns isn't
		// touched at all.
		if owner, _, found, err := app.inbox.AccountOwner(channel, cfg.AccountID); err == nil && found && owner != tid {
			res.taken++
			continue
		}
		if err := graph.SubscribePage(c, p.ID, p.AccessToken, fields); err != nil {
			app.lo.Error("error subscribing to page", "page_id", p.ID, "channel", channel, "error", err)
			res.failed++
			continue
		}
		if err := upsertMetaInbox(app, tid, channel, cfg.AccountID, name, from, cfg); err != nil {
			if errors.Is(err, errMetaAccountTaken) {
				res.taken++
			} else {
				app.lo.Error("error saving inbox", "channel", channel, "account_id", cfg.AccountID, "error", err)
				res.failed++
			}
			continue
		}
		res.connected++
	}
	return res, nil
}

// upsertMetaInbox creates the inbox for a connected account, or reconnects the
// workspace's existing one, keeping its name and message handling options. An
// account connected to another workspace is refused: webhooks name only the
// account, so it can belong to one workspace.
func upsertMetaInbox(app *App, tid int, channel, accountID, name, from string, cfg any) error {
	owner, inboxID, found, err := app.inbox.AccountOwner(channel, accountID)
	if err != nil {
		return err
	}
	if found && owner != tid {
		return errMetaAccountTaken
	}

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	if found {
		existing, err := app.inbox.GetDBRecord(tid, inboxID)
		if err != nil {
			return err
		}
		configJSON, err = keepEditableOptions(existing.Config, configJSON)
		if err != nil {
			return err
		}
		if err := app.inbox.Reconnect(tid, inboxID, configJSON); err != nil {
			return err
		}
		return reloadInbox(app, tid, inboxID)
	}

	if err := checkPlanLimit(app, tid, planInboxes, 1); err != nil {
		return err
	}
	created, err := app.inbox.Create(tid, imodels.Inbox{
		Name:    name,
		From:    from,
		Channel: channel,
		Enabled: true,
		Config:  configJSON,
	})
	if err != nil {
		return err
	}
	return reloadInbox(app, tid, created.ID)
}

// keepEditableOptions carries an existing inbox's message handling options over
// to the config a reconnect writes.
func keepEditableOptions(existing, fresh json.RawMessage) (json.RawMessage, error) {
	var old, next map[string]any
	if err := json.Unmarshal(fresh, &next); err != nil {
		return nil, err
	}
	if len(existing) > 0 && json.Unmarshal(existing, &old) == nil {
		for _, k := range []string{"max_media_size_mb", "mark_seen"} {
			if v, ok := old[k]; ok {
				next[k] = v
			}
		}
	}
	return json.Marshal(next)
}

// randomPIN returns a six-digit two-step verification PIN.
func randomPIN() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func cmpOr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" && strings.TrimSpace(v) != "@" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// handleMetaWebhookVerify answers the subscription check Meta makes when the
// webhook is set up in the App Dashboard.
func handleMetaWebhookVerify(r *fastglue.Request) error {
	args := r.RequestCtx.QueryArgs()
	challenge, ok := meta.VerifyChallenge(metaGraph().App().WebhookVerifyToken,
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

// handleMetaWebhook receives every WhatsApp, Messenger and Instagram event for
// every workspace. The body is signed with the app secret; each event is
// routed to the inbox of the account it is addressed to.
//
// Anything but a bad signature or a full queue answers 200, because Meta
// retries a non-2xx for hours and an event we've deliberately skipped would be
// retried for nothing. A message delivered twice is stored once: its source ID
// is unique.
func handleMetaWebhook(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		graph = metaGraph()
		body  = r.RequestCtx.PostBody()
	)

	if len(body) > maxMetaWebhookBody {
		r.RequestCtx.SetStatusCode(fasthttp.StatusRequestEntityTooLarge)
		return nil
	}
	if !meta.VerifySignature(graph.App().AppSecret, body, string(r.RequestCtx.Request.Header.Peek("X-Hub-Signature-256"))) {
		app.lo.Warn("rejected meta webhook with a bad signature")
		r.RequestCtx.SetStatusCode(fasthttp.StatusUnauthorized)
		return nil
	}

	var env meta.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		app.lo.Error("error decoding meta webhook", "error", err)
		r.RequestCtx.SetStatusCode(fasthttp.StatusBadRequest)
		return nil
	}

	c, cancel := context.WithTimeout(app.ctx, metaWebhookTimeout)
	defer cancel()

	retry := false
	for _, entry := range env.Entry {
		var err error
		switch env.Object {
		case meta.ObjectWhatsApp:
			err = handleWhatsAppEntry(c, app, entry, nil)
		case meta.ObjectPage:
			err = handleMessagingEntry(c, app, inbox.ChannelMessenger, entry)
		case meta.ObjectInstagram:
			err = handleMessagingEntry(c, app, inbox.ChannelInstagram, entry)
		default:
			app.lo.Debug("skipping meta webhook object", "object", env.Object)
		}
		if err != nil {
			retry = true
		}
	}

	if retry {
		r.RequestCtx.SetStatusCode(fasthttp.StatusServiceUnavailable)
		return nil
	}
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	return nil
}

// handleWhatsAppEntry stores the messages of one WhatsApp webhook entry, and
// disconnects the account's inboxes if the business removed the app. It
// returns an error only when a message couldn't be queued and should be retried.
//
// only is set for an entry from a manually connected inbox's own webhook. That
// is signed with the business's app secret, not the platform's, so it is
// trusted for that inbox alone: everything else in it is dropped.
func handleWhatsAppEntry(c context.Context, app *App, entry meta.Entry, only *whatsapp.WhatsApp) error {
	if only != nil && entry.ID != only.Config().WABAID {
		app.lo.Warn("dropping whatsapp webhook entry for another account", "inbox_id", only.Identifier(), "waba_id", entry.ID)
		return nil
	}
	var failed error
	for _, change := range entry.Changes {
		switch change.Field {
		case "messages":
			var v whatsapp.Value
			if err := json.Unmarshal(change.Value, &v); err != nil {
				app.lo.Error("error decoding whatsapp webhook change", "error", err)
				continue
			}
			var wa *whatsapp.WhatsApp
			if only != nil {
				if v.Metadata.PhoneNumberID != only.AccountID() {
					app.lo.Warn("dropping whatsapp webhook change for another number", "inbox_id", only.Identifier(), "phone_number_id", v.Metadata.PhoneNumberID)
					continue
				}
				wa = only
			} else {
				inb, ok := app.inbox.FindByAccount(inbox.ChannelWhatsApp, v.Metadata.PhoneNumberID)
				if !ok {
					app.lo.Debug("no whatsapp inbox for number", "phone_number_id", v.Metadata.PhoneNumberID)
					continue
				}
				if wa, ok = inb.(*whatsapp.WhatsApp); !ok {
					continue
				}
			}
			for _, rc := range wa.Receipts(v) {
				applyReceipt(app, wa.TenantID(), wa.Identifier(), rc)
			}
			for _, msg := range wa.ParseValue(c, v) {
				if err := app.conversation.EnqueueIncoming(msg); err != nil {
					app.lo.Error("error enqueuing whatsapp message", "inbox_id", wa.Identifier(), "source_id", msg.SourceID.String, "error", err)
					failed = err
					continue
				}
				// Send the read receipt only once the message is safely queued.
				wa.MarkSeen(c, msg.SourceID.String)
			}
		case "message_template_status_update":
			// A template was approved, rejected or paused: drop the cached list so
			// the picker shows it at once.
			for _, wa := range wabaInboxes(app, entry.ID, only) {
				wa.InvalidateTemplates()
			}
		case "account_update":
			var v struct {
				Event string `json:"event"`
			}
			if json.Unmarshal(change.Value, &v) == nil && v.Event == "PARTNER_REMOVED" {
				disconnectWABA(app, entry.ID, only)
				continue
			}
			if alert, ok := whatsapp.AccountAlert(change.Value); ok {
				recordHealthAlert(app, entry.ID, "", alert, only)
			}
		case "phone_number_quality_update":
			if number, alert, ok := whatsapp.QualityAlert(change.Value); ok {
				recordHealthAlert(app, entry.ID, number, alert, only)
			}
		case "phone_number_name_update":
			if number, alert, ok := whatsapp.NameAlert(change.Value); ok {
				recordHealthAlert(app, entry.ID, number, alert, only)
			}
		}
	}
	return failed
}

// wabaInboxes returns the running inboxes of a WhatsApp Business Account, or
// just only when an inbox's own webhook reported the event.
func wabaInboxes(app *App, wabaID string, only *whatsapp.WhatsApp) []*whatsapp.WhatsApp {
	if only != nil {
		if only.Config().WABAID == wabaID {
			return []*whatsapp.WhatsApp{only}
		}
		return nil
	}
	var out []*whatsapp.WhatsApp
	for _, inb := range app.inbox.Running(inbox.ChannelWhatsApp) {
		if wa, ok := inb.(*whatsapp.WhatsApp); ok && wa.Config().WABAID == wabaID {
			out = append(out, wa)
		}
	}
	return out
}

// recordHealthAlert keeps what Meta reported about a number's standing on the
// inboxes it concerns: those of the business account, or only the one for
// number when the report names it.
func recordHealthAlert(app *App, wabaID, number string, alert whatsapp.HealthAlert, only *whatsapp.WhatsApp) {
	for _, wa := range wabaInboxes(app, wabaID, only) {
		if number != "" && whatsapp.WAIDFromPhone(wa.Config().DisplayPhoneNumber) != number {
			continue
		}
		if err := setInboxConfigKey(app, wa.TenantID(), wa.Identifier(), "health_alert", alert); err != nil {
			app.lo.Error("error recording whatsapp health alert", "inbox_id", wa.Identifier(), "error", err)
			continue
		}
		app.lo.Warn("whatsapp number health changed", "inbox_id", wa.Identifier(), "event", alert.Event, "detail", alert.Detail)
	}
}

// setInboxConfigKey sets one key of an inbox's config, leaving the rest,
// secrets included, as stored.
func setInboxConfigKey(app *App, tid, id int, key string, value any) error {
	record, err := app.inbox.GetDBRecord(tid, id)
	if err != nil {
		return err
	}
	cfg := map[string]any{}
	if len(record.Config) > 0 {
		if err := json.Unmarshal(record.Config, &cfg); err != nil {
			return err
		}
	}
	cfg[key] = value
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return app.inbox.UpdateConfig(tid, id, b)
}

// disconnectWABA disconnects the inboxes of a WhatsApp Business Account whose
// business removed the app, deleting the token it no longer grants. From the
// platform's webhook that is the platform's app, which manually connected
// numbers don't go through; from an inbox's own webhook, the business's app.
func disconnectWABA(app *App, wabaID string, only *whatsapp.WhatsApp) {
	for _, wa := range wabaInboxes(app, wabaID, only) {
		if only == nil && wa.Config().Manual() {
			continue
		}
		if err := app.inbox.Disconnect(wa.TenantID(), wa.Identifier()); err != nil {
			app.lo.Error("error disconnecting whatsapp inbox", "inbox_id", wa.Identifier(), "error", err)
			continue
		}
		app.lo.Info("whatsapp inbox disconnected: business removed the app", "inbox_id", wa.Identifier(), "waba_id", wabaID)
	}
}

// handleMessagingEntry stores the messages of one Messenger or Instagram
// webhook entry. It returns an error only when a message couldn't be queued.
func handleMessagingEntry(c context.Context, app *App, channel string, entry meta.Entry) error {
	inb, ok := app.inbox.FindByAccount(channel, entry.ID)
	if !ok {
		app.lo.Debug("no inbox for account", "channel", channel, "account_id", entry.ID)
		return nil
	}
	m, ok := inb.(*messenger.Messenger)
	if !ok {
		return nil
	}

	var failed error
	for _, raw := range entry.Messaging {
		if rc, ok := m.ParseReceipt(raw); ok {
			applyReceipt(app, m.TenantID(), m.Identifier(), rc)
			continue
		}
		msg, err := m.ParseEvent(c, raw)
		if err != nil {
			var ignored meta.ErrIgnored
			if errors.As(err, &ignored) {
				app.lo.Debug("skipping meta event", "channel", channel, "inbox_id", m.Identifier(), "reason", ignored.Reason)
			} else {
				app.lo.Error("error parsing meta event", "channel", channel, "inbox_id", m.Identifier(), "error", err)
			}
			continue
		}
		if err := app.conversation.EnqueueIncoming(msg); err != nil {
			app.lo.Error("error enqueuing message", "channel", channel, "inbox_id", m.Identifier(), "source_id", msg.SourceID.String, "error", err)
			failed = err
		}
	}
	return failed
}

// applyReceipt shows a delivery or read receipt on the message it is for.
func applyReceipt(app *App, tenantID, inboxID int, rc meta.Receipt) {
	if len(rc.ExternalIDs) == 0 {
		app.conversation.ApplyReadWatermark(tenantID, inboxID, rc.Recipient, rc.Watermark)
		return
	}
	for _, id := range rc.ExternalIDs {
		app.conversation.ApplyDelivery(tenantID, inboxID, id, rc.Status, rc.Error)
	}
}

// handleMetaDeauthorize is the app's Deauthorize callback: Meta calls it when
// someone removes the app from their account. The inboxes they connected are
// disconnected and their tokens deleted; conversations stay with the workspace.
func handleMetaDeauthorize(r *fastglue.Request) error {
	app := r.Context.(*App)
	sr, err := meta.ParseSignedRequest(metaGraph().App().AppSecret, string(r.RequestCtx.PostArgs().Peek("signed_request")))
	if err != nil {
		app.lo.Warn("rejected meta deauthorize callback", "error", err)
		r.RequestCtx.SetStatusCode(fasthttp.StatusBadRequest)
		return nil
	}
	n := disconnectMetaUser(app, sr.UserID)
	app.lo.Info("meta app deauthorized", "inboxes_disconnected", n)
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	return nil
}

// handleMetaDataDeletion is the app's Data Deletion Request callback. It
// deletes what ConectDesk holds that came from the person's Meta login, the
// access tokens of the inboxes they connected, and answers with a URL where
// the request's status can be checked, as Meta requires.
//
// Messages customers sent to a connected account are the workspace's records,
// held on the business's behalf; deleting those is the workspace's decision,
// through its data retention settings or by deleting the conversations.
func handleMetaDataDeletion(r *fastglue.Request) error {
	app := r.Context.(*App)
	sr, err := meta.ParseSignedRequest(metaGraph().App().AppSecret, string(r.RequestCtx.PostArgs().Peek("signed_request")))
	if err != nil {
		app.lo.Warn("rejected meta data deletion callback", "error", err)
		r.RequestCtx.SetStatusCode(fasthttp.StatusBadRequest)
		return nil
	}

	n := disconnectMetaUser(app, sr.UserID)

	code, err := stringutil.RandomAlphanumeric(20)
	if err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusInternalServerError)
		return nil
	}
	status := map[string]any{"completed_at": time.Now().UTC().Format(time.RFC3339), "inboxes": n}
	if b, err := json.Marshal(status); err == nil {
		if err := app.redis.Set(ctx, "meta_deletion:"+code, b, metaDeletionTTL).Err(); err != nil {
			app.lo.Error("error storing meta data deletion status", "error", err)
		}
	}
	app.lo.Info("meta data deletion request processed", "confirmation_code", code, "inboxes_disconnected", n)

	resp, _ := json.Marshal(map[string]string{
		"url":               metaPublicBase(app) + "/meta/data-deletion/status?code=" + code,
		"confirmation_code": code,
	})
	r.RequestCtx.SetContentType("application/json")
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	r.RequestCtx.SetBody(resp)
	return nil
}

// handleMetaDataDeletionStatus shows the outcome of a data deletion request,
// the page Meta links the person to.
func handleMetaDataDeletionStatus(r *fastglue.Request) error {
	app := r.Context.(*App)
	code := string(r.RequestCtx.QueryArgs().Peek("code"))

	r.RequestCtx.SetContentType("text/plain; charset=utf-8")
	raw, err := app.redis.Get(ctx, "meta_deletion:"+code).Bytes()
	if code == "" || err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		r.RequestCtx.SetBodyString("No data deletion request was found for this confirmation code.\n")
		return nil
	}
	var status struct {
		CompletedAt string `json:"completed_at"`
		Inboxes     int    `json:"inboxes"`
	}
	_ = json.Unmarshal(raw, &status)
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	r.RequestCtx.SetBodyString(fmt.Sprintf(
		"Data deletion request %s: completed at %s.\nAccess tokens deleted and accounts disconnected: %s.\n",
		code, status.CompletedAt, strconv.Itoa(status.Inboxes)))
	return nil
}

// disconnectMetaUser disconnects every inbox a Meta user connected, in every
// workspace, and returns how many.
func disconnectMetaUser(app *App, metaUserID string) int {
	inboxes, err := app.inbox.ConnectedBy(metaUserID)
	if err != nil {
		return 0
	}
	n := 0
	for _, inb := range inboxes {
		if err := app.inbox.Disconnect(inb.TenantID, inb.ID); err != nil {
			app.lo.Error("error disconnecting meta inbox", "inbox_id", inb.ID, "error", err)
			continue
		}
		n++
	}
	return n
}

// isMetaChannel reports whether a channel is connected through the platform's
// Meta app, for handlers where a local variable shadows the inbox package.
func isMetaChannel(channel string) bool {
	return inbox.IsMetaChannel(channel)
}
