package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/telegram"
	imodels "github.com/conectdesk/conectdesk/internal/inbox/models"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const (
	// telegramTimeout bounds a Bot API call made while an admin waits.
	telegramTimeout = 20 * time.Second

	// maxTelegramWebhookBody caps an update. They carry file IDs, not files.
	maxTelegramWebhookBody = 1 << 20
)

// telegramWebhookURL is where Telegram posts a bot inbox's updates. It is on
// the workspace's own address, which Telegram needs to reach over HTTPS.
func telegramWebhookURL(app *App, record imodels.Inbox) string {
	return strings.TrimRight(tenantConsts(app, record.TenantID).AppBaseURL, "/") + "/webhook/telegram/" + record.UUID
}

// connectTelegramRequest is the bot token pasted from @BotFather.
type connectTelegramRequest struct {
	BotToken string `json:"bot_token"`
	Name     string `json:"name"`
}

// handleConnectTelegram creates a Telegram inbox from a bot token: it checks
// the token, creates the inbox, and points the bot's webhook at it.
func handleConnectTelegram(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
		req connectTelegramRequest
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	token := strings.TrimSpace(req.BotToken)
	if token == "" || !strings.Contains(token, ":") {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("telegram.invalidToken"), nil, envelope.InputError)
	}

	c, cancel := context.WithTimeout(app.ctx, telegramTimeout)
	defer cancel()

	api := telegram.NewClient(token)
	bot, err := api.GetMe(c)
	if err != nil || !bot.IsBot {
		app.lo.Warn("telegram token rejected", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("telegram.invalidToken"), nil, envelope.InputError)
	}

	accountID := strconv.FormatInt(bot.ID, 10)
	if owner, _, found, err := app.inbox.AccountOwner(inbox.ChannelTelegram, accountID); err != nil {
		return sendErrorEnvelope(r, err)
	} else if found {
		key := "telegram.botConnectedElsewhere"
		if owner == tid {
			key = "telegram.botAlreadyConnected"
		}
		return r.SendErrorEnvelope(fasthttp.StatusConflict, app.i18n.For(tid).T(key), nil, envelope.InputError)
	}
	if err := checkPlanLimit(app, tid, planInboxes, 1); err != nil {
		return sendErrorEnvelope(r, err)
	}

	cfg, _ := json.Marshal(telegram.Config{
		AccountID:      accountID,
		AccessToken:    token,
		BotID:          bot.ID,
		Username:       bot.Username,
		MaxMediaSizeMB: telegram.DefaultMaxMediaSizeMB,
	})
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = bot.FirstName
	}
	created, err := app.inbox.Create(tid, imodels.Inbox{
		Name:    name,
		From:    "@" + bot.Username,
		Channel: inbox.ChannelTelegram,
		Enabled: true,
		Config:  cfg,
	})
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Create generated the webhook secret; the record returned holds it.
	if err := api.SetWebhook(c, telegramWebhookURL(app, created), created.Secret.String); err != nil {
		app.lo.Error("error setting telegram webhook", "inbox_id", created.ID, "error", err)
		if delErr := app.inbox.SoftDelete(tid, created.ID); delErr != nil {
			app.lo.Error("error removing telegram inbox after webhook failure", "inbox_id", created.ID, "error", delErr)
		}
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).Ts("telegram.webhookFailed", "error", telegramErrorText(err)), nil, envelope.InputError)
	}
	if err := reloadInbox(app, tid, created.ID); err != nil {
		app.lo.Error("error loading telegram inbox", "inbox_id", created.ID, "error", err)
	}
	app.lo.Info("telegram bot connected", "inbox_id", created.ID, "bot", bot.Username)

	if err := created.ClearPasswords(); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	return r.SendEnvelope(created)
}

// telegramRecord returns a tenant's Telegram inbox record for a route's id.
func telegramRecord(r *fastglue.Request) (imodels.Inbox, error) {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	record, err := app.inbox.GetDBRecord(tenantID(r), id)
	if err != nil {
		return record, err
	}
	if record.Channel != inbox.ChannelTelegram {
		return record, envelope.NewError(envelope.InputError, app.i18n.For(tenantID(r)).T("validation.notFoundInbox"), nil)
	}
	return record, nil
}

func telegramClient(record imodels.Inbox) (*telegram.Client, error) {
	var cfg telegram.Config
	if err := json.Unmarshal(record.Config, &cfg); err != nil || cfg.AccessToken == "" {
		return nil, errors.New("telegram inbox has no bot token")
	}
	return telegram.NewClient(cfg.AccessToken), nil
}

// handleGetTelegramWebhook reports the bot's webhook: where it points, how
// many updates are waiting, and the last delivery error, if any.
func handleGetTelegramWebhook(r *fastglue.Request) error {
	app := r.Context.(*App)
	record, err := telegramRecord(r)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	api, err := telegramClient(record)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tenantID(r)).T("telegram.invalidToken"), nil))
	}
	c, cancel := context.WithTimeout(app.ctx, telegramTimeout)
	defer cancel()
	info, err := api.GetWebhookInfo(c)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).Ts("telegram.webhookFailed", "error", telegramErrorText(err)), nil))
	}
	return r.SendEnvelope(map[string]any{
		"connected":            info.URL == telegramWebhookURL(app, record),
		"pending_update_count": info.PendingUpdateCount,
		"last_error_message":   info.LastErrorMessage,
		"last_error_date":      info.LastErrorDate,
	})
}

// handleSetTelegramWebhook points the bot's webhook at this inbox again, e.g.
// after the workspace's address changed or another app took the bot over.
func handleSetTelegramWebhook(r *fastglue.Request) error {
	app := r.Context.(*App)
	record, err := telegramRecord(r)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	api, err := telegramClient(record)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tenantID(r)).T("telegram.invalidToken"), nil))
	}
	c, cancel := context.WithTimeout(app.ctx, telegramTimeout)
	defer cancel()
	if err := api.SetWebhook(c, telegramWebhookURL(app, record), record.Secret.String); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tenantID(r)).Ts("telegram.webhookFailed", "error", telegramErrorText(err)), nil))
	}
	return r.SendEnvelope(true)
}

// removeTelegramWebhook stops a deleted inbox's bot from calling us. Best
// effort: the inbox is gone either way.
func removeTelegramWebhook(app *App, record imodels.Inbox) {
	api, err := telegramClient(record)
	if err != nil {
		return
	}
	c, cancel := context.WithTimeout(app.ctx, telegramTimeout)
	defer cancel()
	if err := api.DeleteWebhook(c); err != nil {
		app.lo.Warn("error removing telegram webhook", "inbox_id", record.ID, "error", err)
	}
}

// handleTelegramWebhook receives a bot's updates. The route names the inbox;
// the X-Telegram-Bot-Api-Secret-Token header proves Telegram sent it.
//
// Anything but a bad secret or a full queue answers 200: Telegram retries a
// failed delivery, and an update we skipped on purpose would be retried for
// nothing.
func handleTelegramWebhook(r *fastglue.Request) error {
	var (
		app       = r.Context.(*App)
		inboxUUID = r.RequestCtx.UserValue("uuid").(string)
		body      = r.RequestCtx.PostBody()
	)
	if _, err := uuid.Parse(inboxUUID); err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		return nil
	}
	record, err := app.inbox.GetDBRecord(tenantID(r), inboxUUID)
	if err != nil || record.Channel != inbox.ChannelTelegram {
		r.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		return nil
	}
	if !telegram.VerifySecret(string(r.RequestCtx.Request.Header.Peek("X-Telegram-Bot-Api-Secret-Token")), record.Secret.String) {
		app.lo.Warn("rejected telegram webhook with a bad secret", "inbox_uuid", inboxUUID)
		r.RequestCtx.SetStatusCode(fasthttp.StatusUnauthorized)
		return nil
	}
	if len(body) > maxTelegramWebhookBody {
		r.RequestCtx.SetStatusCode(fasthttp.StatusRequestEntityTooLarge)
		return nil
	}
	if !record.Enabled {
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	}

	var update telegram.Update
	if err := json.Unmarshal(body, &update); err != nil {
		app.lo.Error("error decoding telegram update", "inbox_id", record.ID, "error", err)
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	}
	// The business linked the bot in Telegram's Business settings, or unlinked
	// it: keep the connection its customers' replies go through.
	if link, enabled, ok := telegram.BusinessUpdate(update); ok {
		var value any
		if enabled {
			value = link
		}
		if err := setInboxConfigKey(app, record.TenantID, record.ID, "business", value); err != nil {
			app.lo.Error("error saving telegram business link", "inbox_id", record.ID, "error", err)
			r.RequestCtx.SetStatusCode(fasthttp.StatusServiceUnavailable)
			return nil
		}
		if err := reloadInbox(app, record.TenantID, record.ID); err != nil {
			app.lo.Error("error reloading telegram inbox", "inbox_id", record.ID, "error", err)
		}
		app.lo.Info("telegram business link changed", "inbox_id", record.ID, "enabled", enabled, "business", link.Username)
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	}

	inb, err := app.inbox.Get(record.ID)
	if err != nil {
		// Not running yet, e.g. mid-reload: let Telegram retry.
		r.RequestCtx.SetStatusCode(fasthttp.StatusServiceUnavailable)
		return nil
	}
	tg, ok := inb.(*telegram.Telegram)
	if !ok {
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	}

	c, cancel := context.WithTimeout(app.ctx, metaWebhookTimeout)
	defer cancel()
	msg, err := tg.ParseUpdate(c, update)
	if err != nil {
		var ignored meta.ErrIgnored
		if errors.As(err, &ignored) {
			app.lo.Debug("skipping telegram update", "inbox_id", record.ID, "reason", ignored.Reason)
		} else {
			app.lo.Error("error parsing telegram update", "inbox_id", record.ID, "error", err)
		}
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	}
	if err := app.conversation.EnqueueIncoming(msg); err != nil {
		app.lo.Error("error enqueuing telegram message", "inbox_id", record.ID, "error", err)
		r.RequestCtx.SetStatusCode(fasthttp.StatusServiceUnavailable)
		return nil
	}
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	return nil
}

// telegramErrorText is the Bot API's description of an error, for the admin.
func telegramErrorText(err error) string {
	var te *telegram.Error
	if errors.As(err, &te) {
		return te.Description
	}
	return err.Error()
}
