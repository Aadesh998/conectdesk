package main

import (
	"fmt"
	"mime/multipart"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	auth_ "github.com/conectdesk/conectdesk/internal/auth"
	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/image"
	mmodels "github.com/conectdesk/conectdesk/internal/media/models"
	notifier "github.com/conectdesk/conectdesk/internal/notification"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	tmpl "github.com/conectdesk/conectdesk/internal/template"
	"github.com/conectdesk/conectdesk/internal/user"
	"github.com/conectdesk/conectdesk/internal/user/models"
	realip "github.com/ferluci/fast-realip"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
)

const (
	maxAvatarSizeMB = 2
)

type resetPasswordRequest struct {
	Email string `json:"email"`
}

type setPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type availabilityRequest struct {
	Status string `json:"status"`
	Source string `json:"source"`
}

const availabilitySourceIdle = "idle"

type agentReq struct {
	FirstName          string   `json:"first_name"`
	LastName           string   `json:"last_name"`
	Email              string   `json:"email"`
	SendWelcomeEmail   bool     `json:"send_welcome_email"`
	Teams              []string `json:"teams"`
	Roles              []string `json:"roles"`
	Enabled            bool     `json:"enabled"`
	AvailabilityStatus string   `json:"availability_status"`
	NewPassword        string   `json:"new_password,omitempty"`
	// PhoneNumber is where the agent takes calls; nil leaves it as it is.
	PhoneNumber *string `json:"phone_number,omitempty"`
}

// handleGetAgents returns all agents.
func handleGetAgents(r *fastglue.Request) error {
	var app = r.Context.(*App)
	agents, err := app.user.GetAgents(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(agents)
}

// handleGetAgentsCompact returns all agents in a compact format.
func handleGetAgentsCompact(r *fastglue.Request) error {
	var app = r.Context.(*App)
	agents, err := app.user.GetAgentsCompact(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(agents)
}

// handleGetAgent returns an agent.
func handleGetAgent(r *fastglue.Request) error {
	var app = r.Context.(*App)
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	if err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}
	agent, err := app.user.GetAgent(tenantID(r), id, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(agent)
}

// handleUpdateAgentAvailability updates the current agent availability.
func handleUpdateAgentAvailability(r *fastglue.Request) error {
	var (
		app      = r.Context.(*App)
		auser    = r.RequestCtx.UserValue("user").(amodels.User)
		ip       = realip.FromRequest(r.RequestCtx)
		availReq availabilityRequest
	)

	if err := r.Decode(&availReq, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	agent, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	if agent.AvailabilityStatus == availReq.Status {
		return r.SendEnvelope(agent)
	}

	// Idle-driven transitions must never overwrite manual-away states.
	if availReq.Source == availabilitySourceIdle &&
		(agent.AvailabilityStatus == models.AwayManual || agent.AvailabilityStatus == models.AwayAndReassigning) {
		return r.SendEnvelope(agent)
	}

	if err := app.user.UpdateAvailability(tenantID(r), auser.ID, availReq.Status); err != nil {
		return sendErrorEnvelope(r, err)
	}
	app.user.InvalidateAgentCache(auser.ID)

	go app.conversation.BroadcastAgentAvailability(tenantID(r), auser.ID, availReq.Status)

	// Skip activity log when returning online from idle-away to avoid log spam.
	if !(agent.AvailabilityStatus == models.Away && availReq.Status == models.Online) {
		if err := app.activityLog.UserAvailability(tenantID(r), auser.ID, auser.Email, availReq.Status, ip, "", 0); err != nil {
			app.lo.Error("error creating activity log", "error", err)
		}
	}

	agent, err = app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(agent)
}

// handleGetCurrentAgentTeams returns the teams of current agent.
func handleGetCurrentAgentTeams(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	teams, err := app.team.GetUserTeams(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(teams)
}

// handleUpdateCurrentAgent updates the current agent.
func handleUpdateCurrentAgent(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	form, err := r.RequestCtx.MultipartForm()
	if err != nil {
		app.lo.Error("error parsing form data", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.GeneralError)
	}

	// Change name?
	if first, ok := form.Value["first_name"]; ok && len(first) > 0 {
		last := ""
		if v := form.Value["last_name"]; len(v) > 0 {
			last = v[0]
		}
		if err := app.user.UpdateName(tenantID(r), auser.ID, first[0], last); err != nil {
			return sendErrorEnvelope(r, err)
		}
	}

	// The number the agent takes phone calls on.
	if phone, ok := form.Value["phone_number"]; ok && len(phone) > 0 {
		if err := app.user.UpdatePhone(tenantID(r), auser.ID, phone[0]); err != nil {
			return sendErrorEnvelope(r, err)
		}
	}

	files, ok := form.File["files"]

	// Upload avatar?
	if ok && len(files) > 0 {
		agent, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		if err := uploadUserAvatar(r, agent, files); err != nil {
			return sendErrorEnvelope(r, err)
		}
	}

	// Fetch updated agent and return.
	agent, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(agent)
}

// handleCreateAgent creates a new agent.
func handleCreateAgent(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req = agentReq{}
	)

	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	if err := validateAgentRequest(app, tenantID(r), &req); err != nil {
		return sendErrorEnvelope(r, err)
	}
	if req.PhoneNumber != nil && !user.ValidAgentPhone(*req.PhoneNumber) {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("voice.invalidAgentPhone"), nil, envelope.InputError)
	}
	if err := checkPlanLimit(app, tenantID(r), planAgents, 1); err != nil {
		return sendErrorEnvelope(r, err)
	}

	agent, err := app.user.CreateAgent(tenantID(r), req.FirstName, req.LastName, req.Email, req.Roles)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if req.PhoneNumber != nil && *req.PhoneNumber != "" {
		if err := app.user.UpdatePhone(tenantID(r), agent.ID, *req.PhoneNumber); err != nil {
			app.lo.Error("error setting new agent's phone", "agent_id", agent.ID, "error", err)
		}
	}

	// Upsert user teams.
	if len(req.Teams) > 0 {
		app.team.UpsertUserTeams(tenantID(r), agent.ID, req.Teams)
	}

	if req.SendWelcomeEmail {
		// Generate reset token.
		resetToken, err := app.user.SetResetPasswordToken(tenantID(r), agent.ID)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}

		// Render template and send email.
		content, err := app.tmpl.RenderInMemoryTemplate(tenantID(r), tmpl.TmplWelcome, map[string]any{
			"ResetToken": resetToken,
			"Email":      req.Email,
		})
		if err != nil {
			app.lo.Error("error rendering template", "error", err)
		}

		if err := app.notifier.Send(notifier.Message{
			TenantID:        tenantID(r),
			RecipientEmails: []string{req.Email},
			Subject:         app.i18n.For(tenantID(r)).T("globals.messages.welcomeToConectDesk"),
			Content:         content,
			Provider:        notifier.ProviderEmail,
		}); err != nil {
			app.lo.Error("error sending notification message", "error", err)
		}
	}

	// Refetch agent as other details might've changed.
	agent, err = app.user.GetAgent(tenantID(r), agent.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(agent)
}

// handleUpdateAgent updates an agent.
func handleUpdateAgent(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		req   = agentReq{}
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		ip    = realip.FromRequest(r.RequestCtx)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.empty", "name", "`id`"), nil, envelope.InputError)
	}

	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	if err := validateAgentRequest(app, tenantID(r), &req); err != nil {
		return sendErrorEnvelope(r, err)
	}

	agent, err := app.user.GetAgent(tenantID(r), id, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	// AI assistant identity users are managed via the AI assistant endpoints only.
	if agent.Type != models.UserTypeAgent {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).Ts("globals.messages.notFound", "name", app.i18n.For(tenantID(r)).T("globals.terms.agent")), nil, envelope.NotFoundError)
	}
	oldAvailabilityStatus := agent.AvailabilityStatus

	if req.PhoneNumber != nil {
		if err := app.user.UpdatePhone(tenantID(r), id, *req.PhoneNumber); err != nil {
			return sendErrorEnvelope(r, err)
		}
	}

	// Update agent with individual fields
	if err = app.user.UpdateAgent(tenantID(r), id, req.FirstName, req.LastName, req.Email, req.Roles, req.Enabled, req.AvailabilityStatus, req.NewPassword); err != nil {
		return sendErrorEnvelope(r, err)
	}

	app.user.InvalidateAgentCache(id)
	app.wsHub.KickUser(id)

	// Create activity log if user availability status changed.
	if oldAvailabilityStatus != req.AvailabilityStatus {
		if err := app.activityLog.UserAvailability(tenantID(r), auser.ID, auser.Email, req.AvailabilityStatus, ip, req.Email, id); err != nil {
			app.lo.Error("error creating activity log", "error", err)
		}
	}

	// Log activity if password was changed.
	if req.NewPassword != "" {
		if err := app.activityLog.PasswordSet(tenantID(r), auser.ID, auser.Email, ip, id, req.Email); err != nil {
			app.lo.Error("error creating activity log", "error", err)
		}
	}

	// Upsert agent teams.
	if err := app.team.UpsertUserTeams(tenantID(r), id, req.Teams); err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Refetch agent and return.
	agent, err = app.user.GetAgent(tenantID(r), id, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(agent)
}

// handleDeleteAgent soft deletes an agent.
func handleDeleteAgent(r *fastglue.Request) error {
	var (
		app     = r.Context.(*App)
		id, err = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		auser   = r.RequestCtx.UserValue("user").(amodels.User)
	)
	if err != nil || id == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.empty", "name", "{globals.terms.user} `id`"), nil, envelope.InputError)
	}

	// Disallow if self-deleting.
	if id == auser.ID {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("user.userCannotDeleteSelf"), nil, envelope.InputError)
	}

	// Soft delete user.
	if err = app.user.SoftDeleteAgent(tenantID(r), id); err != nil {
		return sendErrorEnvelope(r, err)
	}

	defer app.wsHub.KickUser(id)
	defer app.user.InvalidateAgentCache(id)

	// Unassign all open conversations assigned to the user.
	if err := app.conversation.UnassignOpen(tenantID(r), id); err != nil {
		return sendErrorEnvelope(r, err)
	}

	return r.SendEnvelope(true)
}

// handleGetCurrentAgent returns the current logged in agent.
func handleGetCurrentAgent(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)
	u, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	// A "log in as" session names the platform admin, for the app to show.
	var impersonatedBy string
	if r.RequestCtx.UserValue("auth_method") == authMethodSession {
		v, _ := app.auth.GetSessionValue(r, auth_.SessImpersonatedBy)
		impersonatedBy, _ = v.(string)
	}
	return r.SendEnvelope(struct {
		models.User
		ImpersonatedBy string `json:"impersonated_by,omitempty"`
	}{u, impersonatedBy})
}

// handleDeleteCurrentAgentAvatar deletes the current agent's avatar.
func handleDeleteCurrentAgentAvatar(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
	)

	// Get user
	agent, err := app.user.GetAgentCachedOrLoad(tenantID(r), auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Valid str?
	if agent.AvatarURL.String == "" {
		return r.SendEnvelope(true)
	}

	fileName := filepath.Base(agent.AvatarURL.String)

	// Delete file from the store.
	if err := app.media.Delete(tenantID(r), fileName); err != nil {
		return sendErrorEnvelope(r, err)
	}

	if err = app.user.UpdateAvatar(tenantID(r), agent.ID, ""); err != nil {
		return sendErrorEnvelope(r, err)
	}
	app.user.InvalidateAgentCache(agent.ID)
	return r.SendEnvelope(true)
}

// handleResetPassword generates a reset password token and sends an email to the agent.
func handleResetPassword(r *fastglue.Request) error {
	var (
		app       = r.Context.(*App)
		auser, ok = r.RequestCtx.UserValue("user").(amodels.User)
		resetReq  resetPasswordRequest
	)
	if ok && auser.ID > 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("user.userAlreadyLoggedIn"), nil, envelope.InputError)
	}

	// Decode JSON request
	if err := r.Decode(&resetReq, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	if resetReq.Email == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.empty", "name", "`email`"), nil, envelope.InputError)
	}

	agent, err := app.user.GetAgent(tenantID(r), 0, resetReq.Email)
	if err != nil {
		// Send 200 even if user not found, to prevent email enumeration.
		return r.SendEnvelope(true)
	}
	// Under required single sign-on only admins have passwords to reset; the
	// answer is the same, so it doesn't tell who is an admin.
	if tenantConsts(app, tenantID(r)).RequireSSO && !agent.HasAdminRole() {
		return r.SendEnvelope(true)
	}

	token, err := app.user.SetResetPasswordToken(tenantID(r), agent.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Send email.
	content, err := app.tmpl.RenderInMemoryTemplate(tenantID(r), tmpl.TmplResetPassword, map[string]string{
		"ResetToken": token,
	})
	if err != nil {
		app.lo.Error("error rendering template", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.errorSendingPasswordResetEmail"), nil, envelope.GeneralError)
	}

	if err := app.notifier.Send(notifier.Message{
		TenantID:        tenantID(r),
		RecipientEmails: []string{agent.Email.String},
		Subject:         "Reset Password",
		Content:         content,
		Provider:        notifier.ProviderEmail,
	}); err != nil {
		app.lo.Error("error sending password reset email", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.errorSendingPasswordResetEmail"), nil, envelope.GeneralError)
	}

	return r.SendEnvelope(true)
}

// handleSetPassword resets the password with the provided token.
func handleSetPassword(r *fastglue.Request) error {
	var (
		app       = r.Context.(*App)
		agent, ok = r.RequestCtx.UserValue("user").(amodels.User)
		req       setPasswordRequest
	)

	if ok && agent.ID > 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("user.userAlreadyLoggedIn"), nil, envelope.InputError)
	}

	if err := r.Decode(&req, "json"); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil))
	}

	if req.Password == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).Ts("globals.messages.empty", "name", "{globals.terms.password}"), nil, envelope.InputError)
	}

	id, err := app.user.ResetPassword(tenantID(r), req.Token, req.Password)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	app.user.InvalidateAgentCache(id)
	app.wsHub.KickUser(id)

	return r.SendEnvelope(true)
}

// validateAvatarFile checks avatar type and size without side effects.
func validateAvatarFile(r *fastglue.Request, files []*multipart.FileHeader) error {
	var app = r.Context.(*App)

	if len(files) == 0 {
		return nil
	}
	fileHeader := files[0]
	srcExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(stringutil.SanitizeFilename(fileHeader.Filename))), ".")
	if !slices.Contains(image.Exts, srcExt) {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID(r)).T("globals.messages.fileTypeisNotAnImage"), nil)
	}
	if bytesToMegabytes(fileHeader.Size) > maxAvatarSizeMB {
		return envelope.NewError(
			envelope.InputError,
			app.i18n.For(tenantID(r)).Ts("media.fileSizeTooLarge", "size", fmt.Sprintf("%dMB", maxAvatarSizeMB)),
			nil,
		)
	}
	return nil
}

// uploadUserAvatar uploads the user avatar.
func uploadUserAvatar(r *fastglue.Request, user models.User, files []*multipart.FileHeader) error {
	var app = r.Context.(*App)

	if err := validateAvatarFile(r, files); err != nil {
		return err
	}

	fileHeader := files[0]
	file, err := fileHeader.Open()
	if err != nil {
		app.lo.Error("error opening uploaded file", "user_id", user.ID, "error", err)
		return envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.errorUploadingFile"), nil)
	}
	defer file.Close()

	srcFileName := stringutil.SanitizeFilename(fileHeader.Filename)
	srcContentType := fileHeader.Header.Get("Content-Type")
	srcFileSize := fileHeader.Size
	if err := checkStorageLimit(app, tenantID(r), srcFileSize); err != nil {
		return sendErrorEnvelope(r, err)
	}

	// Reset ptr.
	file.Seek(0, 0)
	linkedModel := null.StringFrom(mmodels.ModelUser)
	linkedID := null.IntFrom(user.ID)
	disposition := null.NewString("", false)
	contentID := ""
	meta := []byte("{}")
	// Agent and AI assistant avatars are public: both appear as authors on the help center.
	private := user.Type != models.UserTypeAgent && user.Type != models.UserTypeAIAssistant
	media, err := app.media.UploadAndInsert(tenantID(r), srcFileName, srcContentType, contentID, linkedModel, linkedID, file, int(srcFileSize), disposition, meta, private)
	if err != nil {
		app.lo.Error("error uploading file", "user_id", user.ID, "error", err)
		return envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.errorUploadingFile"), nil)
	}

	// Delete current avatar.
	if user.AvatarURL.Valid {
		fileName := filepath.Base(user.AvatarURL.String)
		if err := app.media.Delete(tenantID(r), fileName); err != nil {
			app.lo.Error("error deleting user avatar", "user_id", user.ID, "error", err)
		}
	}

	if err := app.user.UpdateAvatar(tenantID(r), user.ID, "/uploads/"+media.UUID); err != nil {
		return sendErrorEnvelope(r, err)
	}
	app.user.InvalidateAgentCache(user.ID)
	return nil
}

// handleGenerateAPIKey generates a new API key for a user
func handleGenerateAPIKey(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Check if user exists
	user, err := app.user.GetAgent(tenantID(r), id, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if user.Type != models.UserTypeAgent {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).Ts("globals.messages.notFound", "name", app.i18n.For(tenantID(r)).T("globals.terms.agent")), nil, envelope.NotFoundError)
	}

	// Generate API key and secret
	apiKey, apiSecret, err := app.user.GenerateAPIKey(tenantID(r), user.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	app.user.InvalidateAgentCache(user.ID)

	// Return the API key and secret (only shown once)
	response := struct {
		APIKey    string `json:"api_key"`
		APISecret string `json:"api_secret"`
	}{
		APIKey:    apiKey,
		APISecret: apiSecret,
	}

	return r.SendEnvelope(response)
}

// handleRevokeAPIKey revokes a user's API key
func handleRevokeAPIKey(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.InputError)
	}

	// Check if user exists
	user, err := app.user.GetAgent(tenantID(r), id, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if user.Type != models.UserTypeAgent {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).Ts("globals.messages.notFound", "name", app.i18n.For(tenantID(r)).T("globals.terms.agent")), nil, envelope.NotFoundError)
	}

	// Revoke API key
	if err := app.user.RevokeAPIKey(tenantID(r), id); err != nil {
		return sendErrorEnvelope(r, err)
	}
	app.user.InvalidateAgentCache(id)

	return r.SendEnvelope(true)
}

func validateAgentRequest(app *App, tenantID int, req *agentReq) error {
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	if req.Email == "" {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "`email`"), nil)
	}

	if !stringutil.ValidEmail(req.Email) {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalidEmail"), nil)
	}

	if req.Roles == nil {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "`role`"), nil)
	}

	if req.FirstName == "" {
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).Ts("globals.messages.empty", "name", "`first_name`"), nil)
	}

	switch req.AvailabilityStatus {
	case "", models.Online, models.Offline, models.Away, models.AwayManual, models.AwayAndReassigning:
	default:
		return envelope.NewError(envelope.InputError, app.i18n.For(tenantID).T("validation.invalidAvailabilityStatus"), nil)
	}

	return nil
}
