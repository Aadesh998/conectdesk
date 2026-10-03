package main

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/email/oauth"
	imodels "github.com/conectdesk/conectdesk/internal/inbox/models"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"golang.org/x/oauth2"
)

const (
	FlowTypeNewInbox  = "new_inbox"
	FlowTypeReconnect = "reconnect"
)

// OAuthCredentialsRequest represents the OAuth credentials from the request body.
type OAuthCredentialsRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	TenantID     string `json:"tenant_id,omitempty"` // Optional for Microsoft
	FlowType     string `json:"flow_type,omitempty"` // "new_inbox" or "reconnect"
	InboxID      int    `json:"inbox_id,omitempty"`  // Required for reconnect flow
	// UsePlatformApp connects through the installation's own Google or
	// Microsoft app (oauth.<provider> in config) instead of the client's.
	UsePlatformApp bool `json:"use_platform_app,omitempty"`
}

// platformOAuthApp returns the installation's own OAuth app for provider, set
// in oauth.google or oauth.microsoft, which every workspace can connect through.
func platformOAuthApp(provider string) (clientID, clientSecret, azureTenantID string, ok bool) {
	clientID = ko.String("oauth." + provider + ".client_id")
	clientSecret = ko.String("oauth." + provider + ".client_secret")
	if clientID == "" || clientSecret == "" {
		return "", "", "", false
	}
	if provider == string(oauth.ProviderMicrosoft) {
		azureTenantID = cmp.Or(ko.String("oauth.microsoft.tenant_id"), "common")
	}
	return clientID, clientSecret, azureTenantID, true
}

// platformOAuthProviders lists the providers with a platform OAuth app.
func platformOAuthProviders() []string {
	out := []string{}
	for _, p := range []string{string(oauth.ProviderGoogle), string(oauth.ProviderMicrosoft)} {
		if _, _, _, ok := platformOAuthApp(p); ok {
			out = append(out, p)
		}
	}
	return out
}

// oauthRedirectBase is where OAuth providers send the browser back to, before
// the provider name and /callback. With multi-tenancy it is one URL on the
// base domain for every workspace, so an OAuth app registers a single
// redirect URL; that relays the browser on to the workspace that started.
func oauthRedirectBase(app *App, tenantID int) string {
	if app.tenant.Enabled() && app.tenant.BaseDomain() != "" {
		return app.tenant.BaseURL() + "/oauth"
	}
	return tenantConsts(app, tenantID).AppBaseURL + "/api/v1/inboxes/oauth"
}

// handleOAuthAuthorize initiates the OAuth authorization flow for creating a new email inbox.
func handleOAuthAuthorize(r *fastglue.Request) error {
	var (
		app      = r.Context.(*App)
		provider = r.RequestCtx.UserValue("provider").(string)
		req      OAuthCredentialsRequest
	)

	if provider != string(oauth.ProviderGoogle) && provider != string(oauth.ProviderMicrosoft) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.For(tenantID(r)).Ts("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Parse request body
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &req); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.For(tenantID(r)).Ts("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Validate credentials
	if req.UsePlatformApp {
		var ok bool
		if req.ClientID, req.ClientSecret, req.TenantID, ok = platformOAuthApp(provider); !ok {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("inbox.oauthPlatformAppMissing"), nil, envelope.InputError)
		}
	}
	if req.ClientID == "" || req.ClientSecret == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.empty", "name", "credentials"), nil, envelope.InputError)
	}

	if req.FlowType != FlowTypeNewInbox && req.FlowType != FlowTypeReconnect {
		req.FlowType = FlowTypeNewInbox
	}

	// Build redirect URI
	redirectURI := oauthRedirectBase(app, tenantID(r)) + "/" + provider + "/callback"

	// Generate secure random state
	state, err := stringutil.RandomAlphanumeric(32)
	if err != nil {
		app.lo.Error("Failed to generate OAuth state", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Store OAuth data in Redis with 15 min expiry
	redisKey := "inbox_oauth:" + state
	// The state is bound to the workspace and the admin who started, so only
	// they can finish: a flow started elsewhere can't connect a mailbox here.
	user, _ := r.RequestCtx.UserValue("user").(amodels.User)
	oauthData := map[string]any{
		"provider":     provider,
		"redirect_uri": redirectURI,
		"client_id":    req.ClientID,
		"flow_type":    req.FlowType,
		"inbox_id":     req.InboxID,
		"workspace_id": tenantID(r),
		"user_id":      user.ID,
	}
	if req.UsePlatformApp {
		// The platform app's secret stays in config.
		oauthData["platform"] = "1"
	} else {
		oauthData["client_secret"] = req.ClientSecret
	}

	// Add tenant ID for Microsoft if provided
	if provider == string(oauth.ProviderMicrosoft) && req.TenantID != "" {
		oauthData["tenant_id"] = req.TenantID
	}

	if err := app.redis.HSet(ctx, redisKey, oauthData).Err(); err != nil {
		app.lo.Error("Failed to store OAuth state in Redis", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).Ts("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Set 15 min expiry (auto-cleanup)
	if err := app.redis.Expire(ctx, redisKey, 15*time.Minute).Err(); err != nil {
		app.lo.Error("Failed to set expiry for OAuth state in Redis", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).Ts("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Build authorization URL with scopes
	authURL, err := oauth.BuildAuthorizationURL(
		oauth.Provider(provider),
		req.ClientID,
		redirectURI,
		state,
		req.TenantID,
	)
	if err != nil {
		app.lo.Error("Failed to build authorization URL", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, envelope.InputError)
	}

	return r.SendEnvelope(authURL)
}

// handleOAuthCallback handles the OAuth callback and auto-creates an inbox.
func handleOAuthCallback(r *fastglue.Request) error {
	var (
		app      = r.Context.(*App)
		provider = r.RequestCtx.UserValue("provider").(string)
		code     = string(r.RequestCtx.QueryArgs().Peek("code"))
		state    = string(r.RequestCtx.QueryArgs().Peek("state"))
	)

	// Check if user denied authorization
	if code == "" {
		errorMsg := string(r.RequestCtx.QueryArgs().Peek("error"))
		errorDesc := string(r.RequestCtx.QueryArgs().Peek("error_description"))
		app.lo.Error("OAuth authorization failed", "error", errorMsg, "description", errorDesc)
		return r.Redirect("/admin/inboxes?error=oauth_denied", fasthttp.StatusFound, nil, "")
	}

	if state == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Retrieve OAuth data from Redis
	redisKey := "inbox_oauth:" + state
	oauthData, err := app.redis.HGetAll(ctx, redisKey).Result()
	if err != nil || len(oauthData) == 0 {
		app.lo.Error("Invalid or expired OAuth state", "error", err, "state", state)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest,
			app.i18n.For(tenantID(r)).Ts("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Only the admin who started the flow, in the workspace they started it in,
	// can finish it.
	user, _ := r.RequestCtx.UserValue("user").(amodels.User)
	if oauthData["workspace_id"] != strconv.Itoa(tenantID(r)) || oauthData["user_id"] != strconv.Itoa(user.ID) {
		app.lo.Warn("OAuth state used by another workspace or user", "workspace_id", tenantID(r), "user_id", user.ID)
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.For(tenantID(r)).T("inbox.oauthStateMismatch"), nil, envelope.PermissionError)
	}

	// Delete key after retrieval (one-time use)
	app.redis.Del(ctx, redisKey)

	// WhatsApp, Messenger and Instagram share this callback and its state.
	if provider == metaOAuthProvider {
		if oauthData["provider"] != metaOAuthProvider {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.somethingWentWrong"), nil, envelope.InputError)
		}
		return finishMetaConnect(r, oauthData, code)
	}

	// Extract values from Redis hash
	storedProvider := oauthData["provider"]
	redirectURI := oauthData["redirect_uri"]
	clientID := oauthData["client_id"]
	clientSecret := oauthData["client_secret"]
	azureTenantID := oauthData["tenant_id"] // Empty string if not set
	flowType := oauthData["flow_type"]      // "new_inbox" or "reconnect"
	inboxIDStr := oauthData["inbox_id"]     // Inbox ID for reconnect flow
	platformApp := oauthData["platform"] == "1"
	if platformApp {
		var ok bool
		if clientID, clientSecret, azureTenantID, ok = platformOAuthApp(provider); !ok {
			app.lo.Error("platform OAuth app no longer configured", "provider", provider)
			return r.Redirect("/admin/inboxes?error=token_exchange_failed", fasthttp.StatusFound, nil, "")
		}
	}
	// Inboxes on the platform app don't keep its secret; refreshing tokens
	// reads it from config.
	storedSecret := clientSecret
	if platformApp {
		storedSecret = ""
	}

	// Validate provider matches URL parameter
	if storedProvider != provider {
		app.lo.Error("Provider mismatch", "stored", storedProvider, "url", provider)
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Exchange authorization code for tokens
	token, err := oauth.ExchangeCodeForToken(
		context.Background(),
		oauth.Provider(provider),
		clientID,
		clientSecret,
		code,
		redirectURI,
		azureTenantID,
	)
	if err != nil {
		app.lo.Error("Failed to exchange code for tokens", "error", err)
		return r.Redirect("/admin/inboxes?error=token_exchange_failed", fasthttp.StatusFound, nil, "")
	}

	// Get user email from provider
	userEmail, err := getUserEmailFromProvider(provider, token)
	if err != nil {
		app.lo.Error("Failed to get user email from provider", "error", err)
		return r.Redirect("/admin/inboxes?error=email_fetch_failed", fasthttp.StatusFound, nil, "")
	}

	if userEmail == "" {
		app.lo.Error("User email not found from provider")
		return r.Redirect("/admin/inboxes?error=email_fetch_failed", fasthttp.StatusFound, nil, "")
	}

	// Check if inbox with this email already exists
	existingInboxes, err := app.inbox.GetAll(tenantID(r))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).Ts("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	// Extract email address for comparison (handles "Name <email>" format)
	userEmailAddr, err := stringutil.ExtractEmail(userEmail)
	if err != nil {
		app.lo.Error("error extracting email address", "email", userEmail, "error", err)
		// Fallback
		userEmailAddr = userEmail
	}

	var existingInbox *imodels.Inbox
	for i, existing := range existingInboxes {
		existingEmailAddr, err := stringutil.ExtractEmail(existing.From)
		if err != nil {
			existingEmailAddr = existing.From
		}

		if existingEmailAddr == userEmailAddr {
			existingInbox = &existingInboxes[i]
			break
		}
	}

	// Validate flow type matches actual state
	// New inbox flow: reject if inbox already exists
	if flowType == FlowTypeNewInbox && existingInbox != nil {
		app.lo.Error("Inbox already exists for email", "email", userEmail)
		return r.Redirect("/admin/inboxes?error=inbox_already_exists", fasthttp.StatusFound, nil, "")
	}

	// Reconnect flow: validate inbox_id and email match
	if flowType == FlowTypeReconnect {
		// Find the target inbox by ID
		var targetInbox *imodels.Inbox
		for i, existing := range existingInboxes {
			if fmt.Sprintf("%d", existing.ID) == inboxIDStr {
				targetInbox = &existingInboxes[i]
				break
			}
		}

		if targetInbox == nil {
			app.lo.Error("Target inbox not found for reconnect", "inbox_id", inboxIDStr)
			return r.Redirect("/admin/inboxes?error=inbox_not_found", fasthttp.StatusFound, nil, "")
		}

		// Verify the authorized email matches the target inbox's email
		targetEmailAddr, _ := stringutil.ExtractEmail(targetInbox.From)
		if targetEmailAddr != userEmailAddr {
			app.lo.Error("Email mismatch during reconnect", "target_email", targetEmailAddr, "authorized_email", userEmailAddr)
			return r.Redirect("/admin/inboxes?error=email_mismatch", fasthttp.StatusFound, nil, "")
		}

		existingInbox = targetInbox
	}

	// If inbox exists, update it with new OAuth tokens (reconnect flow)
	if existingInbox != nil {
		app.lo.Info("Updating existing inbox with new OAuth tokens", "email", userEmail, "inbox_id", existingInbox.ID)

		// Parse existing config
		var existingConfig imodels.Config
		if err := json.Unmarshal(existingInbox.Config, &existingConfig); err != nil {
			app.lo.Error("Failed to unmarshal existing config", "error", err)
			return r.Redirect("/admin/inboxes?error=config_parse_failed", fasthttp.StatusFound, nil, "")
		}

		// Update OAuth section with new tokens
		oauthConfig := &imodels.OAuthConfig{
			Provider:     provider,
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			ExpiresAt:    token.Expiry,
			ClientID:     clientID,
			ClientSecret: storedSecret,
			TenantID:     azureTenantID,
			Platform:     platformApp,
		}
		existingConfig.OAuth = oauthConfig
		existingConfig.AuthType = imodels.AuthTypeOAuth2

		// Marshal updated config
		configJSON, err := json.Marshal(existingConfig)
		if err != nil {
			app.lo.Error("Failed to marshal updated config", "error", err)
			return r.Redirect("/admin/inboxes?error=config_update_failed", fasthttp.StatusFound, nil, "")
		}

		// Update inbox config directly (bypasses preservation logic that could corrupt OAuth tokens)
		if err := app.inbox.UpdateConfig(tenantID(r), existingInbox.ID, json.RawMessage(configJSON)); err != nil {
			app.lo.Error("Failed to update inbox config", "error", err)
			return r.Redirect("/admin/inboxes?error=inbox_update_failed", fasthttp.StatusFound, nil, "")
		}

		// Reload inbox to apply new tokens.
		if err := reloadInbox(app, tenantID(r), existingInbox.ID); err != nil {
			app.lo.Error("error reloading inbox", "id", existingInbox.ID, "error", err)
		}

		return r.Redirect("/admin/inboxes?success=oauth_reconnected", fasthttp.StatusFound, nil, "")
	}

	// Get provider-specific defaults
	smtpConfig, imapConfig := getProviderDefaults(provider, userEmail)

	// Create OAuth config for tokens
	oauthConfig := &imodels.OAuthConfig{
		Provider:     provider,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
		ClientID:     clientID,
		ClientSecret: storedSecret,
		TenantID:     azureTenantID,
		Platform:     platformApp,
	}

	// Create inbox config
	config := imodels.Config{
		SMTP:                 []imodels.SMTPConfig{smtpConfig},
		IMAP:                 []imodels.IMAPConfig{imapConfig},
		From:                 userEmail,
		AuthType:             imodels.AuthTypeOAuth2,
		OAuth:                oauthConfig,
		EnablePlusAddressing: true,
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		app.lo.Error("Failed to marshal inbox config", "error", err)
		return r.Redirect("/admin/inboxes?error=config_creation_failed", fasthttp.StatusFound, nil, "")
	}

	// Create inbox
	newInbox := imodels.Inbox{
		Name:              fmt.Sprintf("%s Inbox", userEmail),
		From:              userEmail,
		Channel:           inbox.ChannelEmail,
		Enabled:           true,
		CSATEnabled:       false,
		PromptTagsOnReply: false,
		Config:            json.RawMessage(configJSON),
	}

	if err := checkPlanLimit(app, tenantID(r), planInboxes, 1); err != nil {
		return r.Redirect("/admin/inboxes?error=plan_limit", fasthttp.StatusFound, nil, "")
	}
	createdInbox, err := app.inbox.Create(tenantID(r), newInbox)
	if err != nil {
		app.lo.Error("Failed to create inbox", "error", err)
		return r.Redirect("/admin/inboxes?error=inbox_creation_failed", fasthttp.StatusFound, nil, "")
	}

	// Reload inbox to start the new inbox.
	if err := reloadInbox(app, tenantID(r), createdInbox.ID); err != nil {
		app.lo.Error("error reloading inbox", "id", createdInbox.ID, "error", err)
	}

	return r.Redirect("/admin/inboxes?success=oauth_connected", fasthttp.StatusFound, nil, "")
}

// handleOAuthRelay is the redirect URL every workspace shares on the base
// domain. It only sends the browser on, with the provider's answer, to the
// workspace that started the flow, which checks and finishes it under the
// admin's session there.
func handleOAuthRelay(r *fastglue.Request) error {
	var (
		app      = r.Context.(*App)
		provider = r.RequestCtx.UserValue("provider").(string)
		state    = string(r.RequestCtx.QueryArgs().Peek("state"))
	)
	if !app.tenant.Enabled() || !app.tenant.IsBaseDomain(string(r.RequestCtx.Host())) || state == "" {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).T("globals.messages.pageNotFound"), nil, envelope.NotFoundError)
	}
	vals, err := app.redis.HMGet(ctx, "inbox_oauth:"+state, "workspace_id", "provider").Result()
	if err != nil || len(vals) != 2 || vals[0] == nil || vals[1] != provider {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("inbox.oauthStateExpired"), nil, envelope.InputError)
	}
	workspaceID, _ := strconv.Atoi(fmt.Sprint(vals[0]))
	t, err := app.tenant.Get(workspaceID)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("inbox.oauthStateExpired"), nil, envelope.InputError)
	}
	r.RequestCtx.Response.Header.Set("Referrer-Policy", "no-referrer")
	callback := "/api/v1/inboxes/oauth/" + provider + "/callback"
	if provider == slackOAuthProvider {
		callback = "/api/v1/integrations/slack/callback"
	}
	r.RequestCtx.Redirect(app.tenant.URLFor(t)+callback+"?"+string(r.RequestCtx.QueryArgs().QueryString()), fasthttp.StatusFound)
	return nil
}

// getUserEmailFromProvider fetches the user's email from the OAuth provider.
func getUserEmailFromProvider(provider string, token *oauth2.Token) (string, error) {
	switch provider {
	case string(oauth.ProviderMicrosoft):
		idToken, ok := token.Extra("id_token").(string)
		if !ok {
			return "", fmt.Errorf("no id_token")
		}

		parts := strings.Split(idToken, ".")
		if len(parts) < 2 {
			return "", fmt.Errorf("invalid id_token")
		}

		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", err
		}

		var claims map[string]any
		json.Unmarshal(payload, &claims)

		if email, ok := claims["email"].(string); ok && email != "" {
			return email, nil
		}
		if upn, ok := claims["preferred_username"].(string); ok {
			return upn, nil
		}
		return "", fmt.Errorf("email not found")
	case string(oauth.ProviderGoogle):
		req, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)

		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		if email, ok := result["email"].(string); ok {
			return email, nil
		}
		return "", fmt.Errorf("email not found")
	}

	return "", fmt.Errorf("unsupported provider")
}

// getProviderDefaults returns provider-specific SMTP and IMAP configurations.
func getProviderDefaults(provider, emailAddr string) (imodels.SMTPConfig, imodels.IMAPConfig) {
	var smtp imodels.SMTPConfig
	var imap imodels.IMAPConfig

	// Common settings
	smtp.Username = emailAddr
	smtp.AuthProtocol = "login"
	smtp.TLSSkipVerify = false
	smtp.MaxConns = 5
	smtp.MaxMessageRetries = 5
	smtp.IdleTimeout = "20s"
	smtp.PoolWaitTimeout = "120s"

	imap.Username = emailAddr
	imap.Mailbox = "INBOX"
	imap.ReadInterval = "5m"
	imap.ScanInboxSince = "24h"
	imap.TLSSkipVerify = false

	// Provider-specific settings
	switch provider {
	case string(oauth.ProviderGoogle):
		smtp.Host = "smtp.gmail.com"
		smtp.Port = 587
		smtp.TLSType = "starttls"
		imap.Host = "imap.gmail.com"
		imap.Port = 993
		imap.TLSType = "tls"
	case string(oauth.ProviderMicrosoft):
		smtp.Host = "smtp.office365.com"
		smtp.Port = 587
		smtp.TLSType = "starttls"
		imap.Host = "outlook.office365.com"
		imap.Port = 993
		imap.TLSType = "tls"
	}

	return smtp, imap
}
