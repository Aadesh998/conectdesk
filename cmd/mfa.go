package main

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/mfa"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/conectdesk/conectdesk/internal/totp"
	realip "github.com/ferluci/fast-realip"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"rsc.io/qr"
)

const (
	// A password login waits this long for its two-factor code.
	twoFactorChallengeTTL = 5 * time.Minute
	// A challenge allows this many wrong codes before it is dropped.
	twoFactorMaxAttempts = 5
	// A secret being set up waits this long for its first code.
	twoFactorSetupTTL = 10 * time.Minute
)

// twoFactorSetupPaths stay open to an agent whose workspace requires
// two-factor login they haven't set up, so they can set it up.
var twoFactorSetupPaths = []string{"/api/v1/agents/me", "/api/v1/agents/me/2fa", "/api/v1/auth/logout", "/logout"}

// twoFactorSetupRequired reports whether the agent must set up two-factor
// login before using anything else.
func twoFactorSetupRequired(r *fastglue.Request, totpEnabled bool) bool {
	app := r.Context.(*App)
	if totpEnabled || !tenantConsts(app, tenantID(r)).Require2FA {
		return false
	}
	path := string(r.RequestCtx.Path())
	for _, p := range twoFactorSetupPaths {
		if path == p || strings.HasPrefix(path, p+"/") {
			return false
		}
	}
	return true
}

// startTwoFactorChallenge records a password login waiting for its code and
// returns the token the code comes back with.
func startTwoFactorChallenge(app *App, tenantID, userID int) (string, error) {
	token, err := stringutil.RandomAlphanumeric(40)
	if err != nil {
		return "", err
	}
	key := "mfa_login:" + token
	if err := app.redis.HSet(ctx, key, map[string]any{"tenant_id": tenantID, "user_id": userID}).Err(); err != nil {
		return "", err
	}
	return token, app.redis.Expire(ctx, key, twoFactorChallengeTTL).Err()
}

// handleLoginTwoFactor finishes a password login with a code from the agent's
// authenticator app or a recovery code.
func handleLoginTwoFactor(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		ip  = realip.FromRequest(r.RequestCtx)
		tid = tenantID(r)
		req struct {
			Token string `json:"two_factor_token"`
			Code  string `json:"code"`
		}
	)
	if err := r.Decode(&req, "json"); err != nil || req.Token == "" || req.Code == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	key := "mfa_login:" + req.Token
	vals, err := app.redis.HMGet(ctx, key, "tenant_id", "user_id").Result()
	if err != nil || len(vals) != 2 || vals[0] == nil || vals[0] != strconv.Itoa(tid) {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, app.i18n.For(tid).T("auth.twoFactorExpired"), map[string]bool{"two_factor_expired": true}, envelope.PermissionError)
	}
	userID, _ := strconv.Atoi(vals[1].(string))

	// Each wrong code counts; too many and the password is needed again.
	if n, _ := app.redis.HIncrBy(ctx, key, "attempts", 1).Result(); n > twoFactorMaxAttempts {
		app.redis.Del(ctx, key)
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, app.i18n.For(tid).T("auth.twoFactorExpired"), map[string]bool{"two_factor_expired": true}, envelope.PermissionError)
	}
	usedRecovery, err := app.mfa.Verify(tid, userID, req.Code)
	if err != nil {
		if errors.Is(err, mfa.ErrInvalidCode) {
			return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, app.i18n.For(tid).T("auth.twoFactorInvalidCode"), nil, envelope.PermissionError)
		}
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	app.redis.Del(ctx, key)

	user, err := app.user.GetAgent(tid, userID, "")
	if err != nil || !user.Enabled {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, app.i18n.For(tid).T("auth.twoFactorExpired"), map[string]bool{"two_factor_expired": true}, envelope.PermissionError)
	}
	if usedRecovery {
		app.lo.Info("agent logged in with a recovery code", "tenant_id", tid, "user_id", userID)
	}
	return finishLogin(r, user, ip)
}

// handleGetTwoFactor returns the agent's two-factor login status and whether
// the workspace requires it.
func handleGetTwoFactor(r *fastglue.Request) error {
	app := r.Context.(*App)
	user := r.RequestCtx.UserValue("user").(amodels.User)
	st, err := app.mfa.Status(tenantID(r), user.ID)
	if err != nil && !errors.Is(err, mfa.ErrNotEnabled) {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	return r.SendEnvelope(map[string]any{
		"enabled":             st.Enabled,
		"enabled_at":          st.EnabledAt,
		"recovery_codes_left": st.RecoveryCodesLeft,
		"required":            tenantConsts(app, tenantID(r)).Require2FA,
	})
}

// handleSetupTwoFactor starts setting up two-factor login: a new secret, as a
// QR code for the authenticator app and as text to type in. It takes effect
// once the agent confirms a code from it.
func handleSetupTwoFactor(r *fastglue.Request) error {
	app := r.Context.(*App)
	tid := tenantID(r)
	user := r.RequestCtx.UserValue("user").(amodels.User)
	secret, err := totp.NewSecret()
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	if err := app.redis.Set(ctx, twoFactorSetupKey(tid, user.ID), secret, twoFactorSetupTTL).Err(); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	uri := totp.URI(secret, tenantConsts(app, tid).SiteName, user.Email)
	code, err := qr.Encode(uri, qr.M)
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	code.Scale = 6
	return r.SendEnvelope(map[string]any{
		"secret": secret,
		"uri":    uri,
		"qr":     "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG()),
	})
}

// handleEnableTwoFactor switches two-factor login on once the agent types a
// code from the secret being set up, and returns their recovery codes.
func handleEnableTwoFactor(r *fastglue.Request) error {
	var (
		app  = r.Context.(*App)
		tid  = tenantID(r)
		user = r.RequestCtx.UserValue("user").(amodels.User)
		req  struct {
			Code string `json:"code"`
		}
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	secret, err := app.redis.Get(ctx, twoFactorSetupKey(tid, user.ID)).Result()
	if err != nil || secret == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("auth.twoFactorSetupExpired"), nil, envelope.InputError)
	}
	codes, err := app.mfa.Enable(tid, user.ID, secret, req.Code)
	if err != nil {
		if errors.Is(err, mfa.ErrInvalidCode) {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("auth.twoFactorInvalidCode"), nil, envelope.InputError)
		}
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	app.redis.Del(ctx, twoFactorSetupKey(tid, user.ID))
	app.user.InvalidateAgentCache(user.ID)
	app.activityLog.TwoFactorChanged(tid, user.ID, user.Email, user.ID, user.Email, true, realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(map[string]any{"recovery_codes": codes})
}

// handleDisableTwoFactor switches the agent's two-factor login off, with their
// password. A workspace that requires it doesn't allow that.
func handleDisableTwoFactor(r *fastglue.Request) error {
	app := r.Context.(*App)
	tid := tenantID(r)
	user := r.RequestCtx.UserValue("user").(amodels.User)
	if tenantConsts(app, tid).Require2FA {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.For(tid).T("auth.twoFactorRequiredByWorkspace"), nil, envelope.PermissionError)
	}
	if !checkOwnPassword(r, user) {
		return nil
	}
	if err := app.mfa.Disable(tid, user.ID); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	app.user.InvalidateAgentCache(user.ID)
	app.activityLog.TwoFactorChanged(tid, user.ID, user.Email, user.ID, user.Email, false, realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(true)
}

// handleNewRecoveryCodes replaces the agent's recovery codes, with their password.
func handleNewRecoveryCodes(r *fastglue.Request) error {
	app := r.Context.(*App)
	tid := tenantID(r)
	user := r.RequestCtx.UserValue("user").(amodels.User)
	if !checkOwnPassword(r, user) {
		return nil
	}
	codes, err := app.mfa.NewRecoveryCodes(tid, user.ID)
	if err != nil {
		if errors.Is(err, mfa.ErrNotEnabled) {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("auth.twoFactorNotEnabled"), nil, envelope.InputError)
		}
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	return r.SendEnvelope(map[string]any{"recovery_codes": codes})
}

// handleResetAgentTwoFactor switches another agent's two-factor login off,
// for an agent who lost their phone and recovery codes. They set it up again
// at their next login when the workspace requires it.
func handleResetAgentTwoFactor(r *fastglue.Request) error {
	app := r.Context.(*App)
	tid := tenantID(r)
	actor := r.RequestCtx.UserValue("user").(amodels.User)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("globals.messages.badRequest"), nil, envelope.InputError)
	}
	agent, err := app.user.GetAgent(tid, id, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if err := app.mfa.Disable(tid, agent.ID); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	app.user.InvalidateAgentCache(agent.ID)
	app.activityLog.TwoFactorChanged(tid, actor.ID, actor.Email, agent.ID, agent.Email.String, false, realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(true)
}

// checkOwnPassword reports whether the password in the request body is the
// agent's; when it isn't, it has sent the error.
func checkOwnPassword(r *fastglue.Request, user amodels.User) bool {
	app := r.Context.(*App)
	var req struct {
		Password string `json:"password"`
	}
	if err := r.Decode(&req, "json"); err != nil || req.Password == "" {
		r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("auth.passwordRequired"), nil, envelope.InputError)
		return false
	}
	if _, err := app.user.VerifyPassword(tenantID(r), user.Email, []byte(req.Password)); err != nil {
		r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.For(tenantID(r)).T("auth.wrongPassword"), nil, envelope.PermissionError)
		return false
	}
	return true
}

func twoFactorSetupKey(tenantID, userID int) string {
	return "mfa_setup:" + strconv.Itoa(tenantID) + ":" + strconv.Itoa(userID)
}
