package main

import (
	"encoding/json"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/httputil"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/voice"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/email/oauth"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/livechat"
	imodels "github.com/conectdesk/conectdesk/internal/inbox/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// handleGetInboxes returns all inboxes
func handleGetInboxes(r *fastglue.Request) error {
	var app = r.Context.(*App)
	inboxes, err := app.inbox.GetAll(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	for i := range inboxes {
		if err := inboxes[i].ClearPasswords(); err != nil {
			app.lo.Error("error clearing inbox passwords from response", "error", err)
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
		}
	}
	return r.SendEnvelope(inboxes)
}

// handleGetInbox returns an inbox by ID
func handleGetInbox(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	inbox, err := app.inbox.GetDBRecord(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := inbox.ClearPasswords(); err != nil {
		app.lo.Error("error clearing inbox passwords from response", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	return r.SendEnvelope(inbox)
}

// handleCreateInbox creates a new inbox
func handleCreateInbox(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		inbox = imodels.Inbox{}
	)
	if err := r.Decode(&inbox, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}

	// Trim whitespace from inbox fields and config.
	if err := trimInboxFields(&inbox); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}

	// WhatsApp, Messenger and Instagram inboxes are created by connecting the
	// account, which is where their config comes from.
	if isMetaChannel(inbox.Channel) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("inbox.metaConnectOnly"), nil, envelope.InputError)
	}
	// So is a Telegram inbox, from its bot token.
	if inbox.Channel == "telegram" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("telegram.connectOnly"), nil, envelope.InputError)
	}
	// A new voice inbox needs its API token; on an edit, blank keeps it.
	if inbox.Channel == voice.ChannelVoice {
		var cfg voice.Config
		if json.Unmarshal(inbox.Config, &cfg) != nil || strings.TrimSpace(cfg.AccessToken) == "" {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.empty", "name", "API token"), nil, envelope.InputError)
		}
	}
	if err := validateInbox(app, tenantID(r), inbox); err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := checkChannelAllowed(app, tenantID(r), inbox.Channel); err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := checkPlanLimit(app, tenantID(r), planInboxes, 1); err != nil {
		return sendErrorEnvelope(r, err)
	}

	createdInbox, err := app.inbox.Create(tenantID(r), inbox)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err := reloadInbox(app, tenantID(r), createdInbox.ID); err != nil {
		app.lo.Error("error reloading inbox", "id", createdInbox.ID, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Clear passwords before returning.
	if err := createdInbox.ClearPasswords(); err != nil {
		app.lo.Error("error clearing inbox passwords from response", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}

	return r.SendEnvelope(createdInbox)
}

// handleUpdateInbox updates an inbox
func handleUpdateInbox(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		inbox = imodels.Inbox{}
	)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	if err := r.Decode(&inbox, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}

	// Trim whitespace from inbox fields and config.
	if err := trimInboxFields(&inbox); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}

	if err := validateInbox(app, tenantID(r), inbox); err != nil {
		return sendErrorEnvelope(r, err)
	}
	// An inbox of a channel the platform disabled stays off.
	if inbox.Enabled {
		if err := checkChannelAllowed(app, tenantID(r), inbox.Channel); err != nil {
			return sendErrorEnvelope(r, err)
		}
	}

	updatedInbox, err := app.inbox.Update(tenantID(r), id, inbox)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err := reloadInbox(app, tenantID(r), id); err != nil {
		app.lo.Error("error reloading inbox", "id", id, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Clear passwords before returning.
	if err := updatedInbox.ClearPasswords(); err != nil {
		app.lo.Error("error clearing inbox passwords from response", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}

	return r.SendEnvelope(updatedInbox)
}

// handleToggleInbox toggles an inbox
func handleToggleInbox(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
	)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// An inbox of a channel the platform disabled can't be switched back on.
	record, err := app.inbox.GetDBRecord(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if !record.Enabled {
		if err := checkChannelAllowed(app, tenantID(r), record.Channel); err != nil {
			return sendErrorEnvelope(r, err)
		}
	}

	toggledInbox, err := app.inbox.Toggle(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err := reloadInbox(app, tenantID(r), id); err != nil {
		app.lo.Error("error reloading inbox", "id", id, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Clear passwords before returning
	if err := toggledInbox.ClearPasswords(); err != nil {
		app.lo.Error("error clearing inbox passwords from response", "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}

	return r.SendEnvelope(toggledInbox)
}

// handleDeleteInbox deletes an inbox
func handleDeleteInbox(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	// A deleted Telegram inbox's bot stops calling us. Read before the delete,
	// which clears the config and its token.
	if record, err := app.inbox.GetDBRecord(tenantID(r), id); err == nil && record.Channel == "telegram" {
		defer removeTelegramWebhook(app, record)
	}
	err := app.inbox.SoftDelete(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := reloadInbox(app, tenantID(r), id); err != nil {
		app.lo.Error("error reloading inbox", "id", id, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(true)
}

// validateInbox validates the inbox
func validateInbox(app *App, tenantID int, inbox imodels.Inbox) error {
	// Validate from address only for email channels.
	if inbox.Channel == "email" {
		if _, err := mail.ParseAddress(inbox.From); err != nil {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("validation.invalidFromAddress"), nil)
		}
		var cfg imodels.Config
		if len(inbox.Config) > 0 {
			if err := json.Unmarshal(inbox.Config, &cfg); err == nil && cfg.ReplyTo != "" {
				if _, err := mail.ParseAddress(cfg.ReplyTo); err != nil {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalidEmail"), nil)
				}
			}
		}
	}
	if len(inbox.Config) == 0 {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "config"), nil)
	}
	if inbox.Name == "" {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "name"), nil)
	}
	if inbox.Channel == "" {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "channel"), nil)
	}

	// Validate livechat-specific configuration
	if inbox.Channel == livechat.ChannelLiveChat {
		var config livechat.Config
		if err := json.Unmarshal(inbox.Config, &config); err == nil {
			// ShowOfficeHoursAfterAssignment cannot be enabled if ShowOfficeHoursInChat is disabled
			if config.ShowOfficeHoursAfterAssignment && !config.ShowOfficeHoursInChat {
				return envelope.NewError(envelope.InputError, "`show_office_hours_after_assignment` cannot be enabled when `show_office_hours_in_chat` is disabled", nil)
			}
			// Validate continuity settings - required when linked email inbox is set.
			if inbox.LinkedEmailInboxID.Valid && inbox.LinkedEmailInboxID.Int > 0 {
				if config.Continuity.OfflineThreshold == "" {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "offline_threshold"), nil)
				}
				if config.Continuity.MinEmailInterval == "" {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "min_email_interval"), nil)
				}
				if config.Continuity.MaxMessagesPerEmail == 0 {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "max_messages_per_email"), nil)
				}
			}
			if config.Continuity.OfflineThreshold != "" {
				d, err := time.ParseDuration(config.Continuity.OfflineThreshold)
				if err != nil {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("validation.invalidDuration", "name", "offline_threshold"), nil)
				}
				if d < time.Minute {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("validation.minDuration", "name", "offline_threshold", "min", "1m"), nil)
				}
			}
			if config.Continuity.MinEmailInterval != "" {
				d, err := time.ParseDuration(config.Continuity.MinEmailInterval)
				if err != nil {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("validation.invalidDuration", "name", "min_email_interval"), nil)
				}
				if d < time.Minute {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("validation.minDuration", "name", "min_email_interval", "min", "1m"), nil)
				}
			}
			if config.Continuity.MaxMessagesPerEmail != 0 {
				if config.Continuity.MaxMessagesPerEmail < 1 || config.Continuity.MaxMessagesPerEmail > 100 {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("validation.minmaxNumber", "min", "1", "max", "100"), nil)
				}
			}

			// Validate colors.
			hexColorRegex := regexp.MustCompile(`^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$`)
			if config.Colors.Primary == "" {
				return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "primary color"), nil)
			}
			if !hexColorRegex.MatchString(config.Colors.Primary) {
				return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalidColor"), nil)
			}

			// Validate launcher position.
			if config.Launcher.Position != "left" && config.Launcher.Position != "right" {
				return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalidValue"), nil)
			}

			// Validate launcher spacing: clamp to a sane range so a fat-fingered value doesn't push the launcher off-screen.
			if config.Launcher.Spacing.Side < 0 || config.Launcher.Spacing.Side > 200 ||
				config.Launcher.Spacing.Bottom < 0 || config.Launcher.Spacing.Bottom > 200 {
				return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalidValue"), nil)
			}

			// Validate home apps.
			for _, ha := range config.HomeApps {
				if ha.URL != "" && !httputil.IsValidHTTPURL(ha.URL) {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalidUrl"), nil)
				}
				if ha.Type == livechat.HomeAppAnnouncement && ha.ImageURL != "" && !httputil.IsValidHTTPURL(ha.ImageURL) {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalidUrl"), nil)
				}
			}

			// Validate home screen background image URL.
			if config.HomeScreen.Background.ImageURL != "" && !httputil.IsValidHTTPURL(config.HomeScreen.Background.ImageURL) {
				return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalidUrl"), nil)
			}

			// Validate URLs if set.
			for _, u := range []string{config.LogoURL, config.Launcher.LogoURL, config.WebsiteURL} {
				if u != "" && !httputil.IsValidHTTPURL(u) {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalidUrl"), nil)
				}
			}

			// Validate trusted domains.
			// Valid formats: example.com, *.example.com, sub.example.com, example.com:8080
			for _, domain := range config.TrustedDomains {
				d := strings.TrimSpace(domain)
				if d == "" {
					continue
				}
				if strings.Contains(d, "://") || strings.Contains(d, "/") || strings.Contains(d, " ") {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("validation.invalidDomain", "domain", d), nil)
				}
				// Wildcard must be at the start followed by a dot.
				if strings.Contains(d, "*") && !strings.HasPrefix(d, "*.") {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("validation.invalidDomain", "domain", d), nil)
				}
			}

			// Validate blocked IPs entries.
			for _, entry := range config.BlockedIPs {
				if !httputil.ValidateIPOrCIDR(entry) {
					return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("validation.invalidIPOrCIDR", "entry", entry), nil)
				}
			}
		}

		// Validate linked email inbox if specified
		if inbox.LinkedEmailInboxID.Valid {
			linkedInbox, err := app.inbox.GetDBRecord(tenantID, int(inbox.LinkedEmailInboxID.Int))
			if err != nil {
				return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
			}
			// Ensure linked inbox is an email channel
			if linkedInbox.Channel != "email" {
				return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
			}
			// Ensure linked inbox is enabled
			if !linkedInbox.Enabled {
				return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)

			}
		}
	}

	// Validate email channel config.
	if inbox.Channel == "email" {
		if err := validateEmailConfig(app, tenantID, inbox.Config); err != nil {
			return err
		}
	}

	if inbox.Channel == voice.ChannelVoice {
		if err := validateVoiceConfig(app, tenantID, inbox.Config); err != nil {
			return err
		}
	}

	// Validate WhatsApp, Messenger, Instagram and Telegram options.
	if isMetaChannel(inbox.Channel) || inbox.Channel == "telegram" {
		if err := validateMetaConfig(app, tenantID, inbox.Config); err != nil {
			return err
		}
	}
	return nil
}

// validateVoiceConfig validates a voice inbox's Exotel settings.
func validateVoiceConfig(app *App, tenantID int, configJSON json.RawMessage) error {
	var cfg voice.Config
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if cfg.Provider != voice.ProviderExotel {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("voice.unknownProvider"), nil)
	}
	for name, v := range map[string]string{"account_sid": cfg.AccountSID, "api_key": cfg.APIKey} {
		if strings.TrimSpace(v) == "" || strings.ContainsAny(v, "/?#@: ") {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", name), nil)
		}
	}
	if cfg.Subdomain != voice.ExotelSingapore && cfg.Subdomain != voice.ExotelMumbai {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("voice.unknownRegion"), nil)
	}
	if len(voice.Digits(cfg.CallerID)) < 8 {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("voice.invalidCallerID"), nil)
	}
	for _, n := range cfg.FallbackNumbers {
		if len(voice.Digits(n)) < 8 {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("voice.invalidFallback"), nil)
		}
	}
	// Each menu digit is one key and routes to one of the workspace's teams.
	seen := map[string]bool{}
	for _, o := range cfg.Menu {
		if len(o.Digit) != 1 || !strings.Contains("0123456789", o.Digit) || seen[o.Digit] {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("voice.invalidMenu"), nil)
		}
		seen[o.Digit] = true
		if _, err := app.team.Get(tenantID, o.TeamID); err != nil {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("voice.invalidMenu"), nil)
		}
	}
	if cfg.RingSeconds != 0 && (cfg.RingSeconds < 10 || cfg.RingSeconds > 120) {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("validation.minmaxNumber", "min", "10", "max", "120"), nil)
	}
	return validateMissedCallConfig(app, tenantID, cfg.MissedCalls)
}

// validateMissedCallConfig checks a phone line's missed-call settings: the
// call-back time, and a WhatsApp inbox of the workspace's with a template to
// send from it.
func validateMissedCallConfig(app *App, tenantID int, mc voice.MissedCallConfig) error {
	if mc.CallBackMinutes < 0 || mc.CallBackMinutes > 1440 {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("validation.minmaxNumber", "min", "1", "max", "1440"), nil)
	}
	if mc.WhatsAppInboxID == 0 {
		return nil
	}
	record, err := app.inbox.GetDBRecord(tenantID, mc.WhatsAppInboxID)
	if err != nil || record.Channel != inbox.ChannelWhatsApp {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("voice.missedCallInvalidInbox"), nil)
	}
	if strings.TrimSpace(mc.Template.Name) == "" || strings.TrimSpace(mc.Template.Language) == "" {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("voice.missedCallTemplateRequired"), nil)
	}
	if mc.AfterHoursTemplate.Name != "" && strings.TrimSpace(mc.AfterHoursTemplate.Language) == "" {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("voice.missedCallTemplateRequired"), nil)
	}
	return nil
}

// validateMetaConfig validates the options an admin can set on a WhatsApp,
// Messenger or Instagram inbox; the rest of the config comes from Meta.
func validateMetaConfig(app *App, tenantID int, configJSON json.RawMessage) error {
	var cfg struct {
		MaxMediaSizeMB int `json:"max_media_size_mb"`
		AIAssistantID  int `json:"ai_assistant_id"`
	}
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
	}
	maxUpload := tenantConsts(app, tenantID).MaxFileUploadSizeMB
	if cfg.MaxMediaSizeMB < 0 || cfg.MaxMediaSizeMB > maxUpload {
		return envelope.NewError(envelope.InputError,
			app.i18n.For(tenantID).Ts("validation.minmaxNumber", "min", "1", "max", strconv.Itoa(maxUpload)), nil)
	}
	// The assistant new conversations go to must be one of this workspace's.
	if cfg.AIAssistantID < 0 {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("ai.jev.invalidAssistant"), nil)
	}
	if cfg.AIAssistantID > 0 {
		if _, err := app.aiAgent.GetAssistant(tenantID, cfg.AIAssistantID); err != nil {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("ai.jev.invalidAssistant"), nil)
		}
	}
	return nil
}

// validateEmailConfig validates the email inbox configuration.
func validateEmailConfig(app *App, tenantID int, configJSON json.RawMessage) error {
	var cfg imodels.Config
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Validate auth_type.
	if cfg.AuthType != "" && cfg.AuthType != imodels.AuthTypePassword && cfg.AuthType != imodels.AuthTypeOAuth2 {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Validate OAuth config if auth_type is oauth2.
	if cfg.AuthType == imodels.AuthTypeOAuth2 {
		if cfg.OAuth == nil {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "oauth"), nil)
		}
		if cfg.OAuth.Provider != string(oauth.ProviderGoogle) && cfg.OAuth.Provider != string(oauth.ProviderMicrosoft) {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
		if cfg.OAuth.ClientID == "" {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "oauth.client_id"), nil)
		}
	}

	// Validate SMTP configs.
	for i, smtp := range cfg.SMTP {
		if smtp.Host == "" {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "smtp.host"), nil)
		}
		if smtp.Port <= 0 {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalidPortValue"), nil)
		}
		// Validate auth_protocol for password auth.
		if cfg.AuthType != imodels.AuthTypeOAuth2 {
			validAuthProtocols := map[string]bool{"": true, "none": true, "plain": true, "login": true, "cram": true}
			if !validAuthProtocols[cfg.SMTP[i].AuthProtocol] {
				return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
			}
		}
	}

	// Validate IMAP configs.
	for _, imap := range cfg.IMAP {
		if imap.Host == "" {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "imap.host"), nil)
		}
		if imap.Port <= 0 {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
		if imap.Mailbox == "" {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "imap.mailbox"), nil)
		}
		// Validate tls_type.
		validTLSTypes := map[string]bool{"none": true, "starttls": true, "tls": true}
		if !validTLSTypes[imap.TLSType] {
			return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
	}

	return nil
}

// trimInboxFields trims whitespace from inbox fields and its email config if applicable.
func trimInboxFields(inb *imodels.Inbox) error {
	inb.Name = strings.TrimSpace(inb.Name)
	inb.From = strings.TrimSpace(inb.From)
	inb.FromNameTemplate = strings.TrimSpace(inb.FromNameTemplate)

	// A voice inbox's number is its account: one ExoPhone serves one workspace.
	if inb.Channel == voice.ChannelVoice && len(inb.Config) > 0 {
		var cfg voice.Config
		if err := json.Unmarshal(inb.Config, &cfg); err != nil {
			return err
		}
		cfg.Provider = voice.ProviderExotel
		cfg.AccountSID = strings.TrimSpace(cfg.AccountSID)
		cfg.APIKey = strings.TrimSpace(cfg.APIKey)
		cfg.AccessToken = strings.TrimSpace(cfg.AccessToken)
		cfg.CallerID = voice.E164(cfg.CallerID)
		cfg.AccountID = voice.Digits(cfg.CallerID)
		for i, n := range cfg.FallbackNumbers {
			cfg.FallbackNumbers[i] = voice.E164(n)
		}
		inb.From = cfg.CallerID
		b, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		inb.Config = b
	}

	// Trim email config fields if this is an email channel.
	if inb.Channel == inbox.ChannelEmail && len(inb.Config) > 0 {
		var cfg imodels.Config
		if err := json.Unmarshal(inb.Config, &cfg); err != nil {
			return err
		}
		trimEmailConfig(&cfg)
		trimmedConfig, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		inb.Config = trimmedConfig
	}
	return nil
}

// trimEmailConfig trims whitespace from email configuration fields.
// Passwords and secrets are intentionally NOT trimmed.
func trimEmailConfig(cfg *imodels.Config) {
	cfg.ReplyTo = strings.TrimSpace(cfg.ReplyTo)

	// Trim IMAP configs.
	for i := range cfg.IMAP {
		cfg.IMAP[i].Host = strings.TrimSpace(cfg.IMAP[i].Host)
		cfg.IMAP[i].Username = strings.TrimSpace(cfg.IMAP[i].Username)
		cfg.IMAP[i].Mailbox = strings.TrimSpace(cfg.IMAP[i].Mailbox)
	}

	// Trim SMTP configs.
	for i := range cfg.SMTP {
		cfg.SMTP[i].Host = strings.TrimSpace(cfg.SMTP[i].Host)
		cfg.SMTP[i].Username = strings.TrimSpace(cfg.SMTP[i].Username)
		cfg.SMTP[i].HelloHostname = strings.TrimSpace(cfg.SMTP[i].HelloHostname)
	}

	// Trim OAuth config.
	if cfg.OAuth != nil {
		cfg.OAuth.Provider = strings.TrimSpace(cfg.OAuth.Provider)
		cfg.OAuth.ClientID = strings.TrimSpace(cfg.OAuth.ClientID)
		cfg.OAuth.TenantID = strings.TrimSpace(cfg.OAuth.TenantID)
	}
}
