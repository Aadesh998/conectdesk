package main

import (
	"encoding/json"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/zerodha/fastglue"
)

// handleGetConfig returns the public configuration needed for app initialization, this includes minimal app settings and enabled SSO providers (without secrets).
func handleGetConfig(r *fastglue.Request) error {
	var app = r.Context.(*App)

	// Get app settings
	settingsJSON, err := app.setting.GetByPrefix(tenantID(r), "app")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Unmarshal settings
	var settings map[string]any
	if err := json.Unmarshal(settingsJSON, &settings); err != nil {
		app.lo.Error("error unmarshalling settings", "err", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}

	// Filter to only include public fields needed for initial app load
	publicSettings := map[string]any{
		"app.lang":        settings["app.lang"],
		"app.favicon_url": settings["app.favicon_url"],
		"app.logo_url":    settings["app.logo_url"],
		"app.site_name":   settings["app.site_name"],
		// Custom domains and other per-workspace features only exist with multi-tenancy.
		"app.tenancy_enabled": app.tenant.Enabled(),
		// The workspace address, typed to confirm deleting the workspace.
		"app.workspace_slug": tenantFromReq(r).Slug,
		// The login page leads with single sign-on when the workspace requires it.
		"app.require_sso": tenantConsts(app, tenantID(r)).RequireSSO,
		// Gmail and Outlook inboxes can connect through the installation's own
		// OAuth apps; an own app registers oauth_redirect_base/<provider>/callback.
		"app.oauth_platform_providers": platformOAuthProviders(),
		"app.oauth_redirect_base":      oauthRedirectBase(app, tenantID(r)),
		// Channels the platform's Meta app can connect: whatsapp, messenger, instagram.
		"app.meta_products": metaGraph().App().Products(),
	}
	// The inbox channels the platform admin enabled for the workspace; absent
	// when every channel is open.
	if channels, all := tenantFromReq(r).AllowedChannels(); !all && app.tenant.Enabled() {
		publicSettings["app.allowed_channels"] = channels
	}

	// Get all OIDC providers
	oidcProviders, err := app.oidc.GetAll(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Filter for enabled providers and remove client_secret
	enabledProviders := make([]map[string]any, 0)
	for _, provider := range oidcProviders {
		if provider.Enabled {
			providerMap := map[string]any{
				"id":           provider.ID,
				"name":         provider.Name,
				"provider":     provider.Provider,
				"provider_url": provider.ProviderURL,
				"client_id":    provider.ClientID,
				"logo_url":     provider.LogoURL,
				"enabled":      provider.Enabled,
				"redirect_uri": provider.RedirectURI,
			}
			enabledProviders = append(enabledProviders, providerMap)
		}
	}

	// Add SSO providers to the response
	publicSettings["app.sso_providers"] = enabledProviders

	if err := app.auth.SetCSRFCookie(r); err != nil {
		app.lo.Error("error setting csrf cookie", "error", err)
	}

	return r.SendEnvelope(publicSettings)
}
