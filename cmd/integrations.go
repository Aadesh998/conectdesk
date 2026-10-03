package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/chatmsg"
	cmodels "github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/whatsapp"
	"github.com/conectdesk/conectdesk/internal/integration"
	"github.com/conectdesk/conectdesk/internal/integration/slack"
	"github.com/conectdesk/conectdesk/internal/integration/teams"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

const (
	// slackOAuthProvider is the provider the Slack install stores its state
	// under, and the path segment of its redirect URL.
	slackOAuthProvider = "slack"

	// maxChatWebhookBody caps what Slack and Teams post; their payloads are small.
	maxChatWebhookBody = 1 << 20

	// chatActionTimeout bounds acting on a conversation from Slack or Teams.
	chatActionTimeout = 20 * time.Second

	// teamsLinkTTL is how long a code to link a Teams channel stays usable.
	teamsLinkTTL = 30 * time.Minute

	// chatEmailTTL is how long a Slack or Teams user's email is remembered.
	chatEmailTTL = 10 * time.Minute
)

// Actions agents take on a conversation from Slack or Teams.
const (
	chatActionAssign  = "assign"
	chatActionResolve = "resolve"
	chatActionReply   = "reply"
	chatActionNote    = "note"
)

// chatPublicBase is where Slack and Teams call: the base domain with
// multi-tenancy, since one app serves every workspace, else the app's address.
func chatPublicBase(app *App) string {
	return metaPublicBase(app)
}

// slackRedirectURI is where Slack sends the admin back after installing. With
// multi-tenancy it is the shared relay on the base domain, which forwards to
// the workspace the install started in.
func slackRedirectURI(app *App, tid int) string {
	if app.tenant.Enabled() && app.tenant.BaseDomain() != "" {
		return app.tenant.BaseURL() + "/oauth/" + slackOAuthProvider + "/callback"
	}
	return tenantConsts(app, tid).AppBaseURL + "/api/v1/integrations/slack/callback"
}

// chatSetup is what the admin page shows about the platform's Slack app and
// Teams bot, and the addresses to set them up with.
type chatSetup struct {
	Slack struct {
		Configured      bool   `json:"configured"`
		RedirectURL     string `json:"redirect_url"`
		EventsURL       string `json:"events_url"`
		InteractionsURL string `json:"interactions_url"`
	} `json:"slack"`
	Teams struct {
		Configured        bool   `json:"configured"`
		BotName           string `json:"bot_name"`
		MessagingEndpoint string `json:"messaging_endpoint"`
	} `json:"teams"`
	Events []string `json:"events"`
}

// handleGetChatSetup returns whether Slack and Teams can be connected.
func handleGetChatSetup(r *fastglue.Request) error {
	var (
		app  = r.Context.(*App)
		base = chatPublicBase(app)
		out  chatSetup
	)
	out.Slack.Configured = app.integration.Slack().App().Configured()
	out.Slack.RedirectURL = slackRedirectURI(app, tenantID(r))
	out.Slack.EventsURL = base + "/webhook/slack/events"
	out.Slack.InteractionsURL = base + "/webhook/slack/interactions"
	out.Teams.Configured = app.integration.Teams().App().Configured()
	out.Teams.BotName = teamsBotName(app)
	out.Teams.MessagingEndpoint = base + "/webhook/teams"
	out.Events = integration.Events
	return r.SendEnvelope(out)
}

// handleGetChatIntegrations returns the workspace's connected channels.
func handleGetChatIntegrations(r *fastglue.Request) error {
	app := r.Context.(*App)
	out, err := app.integration.GetAll(tenantID(r))
	if err != nil {
		app.lo.Error("error fetching chat integrations", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(out)
}

// handleUpdateChatIntegration changes what a channel posts and whether it is on.
func handleUpdateChatIntegration(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		req   struct {
			Events   []string `json:"events"`
			IsActive bool     `json:"is_active"`
		}
	)
	if err := r.Decode(&req, "json"); err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	if len(req.Events) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).Ts("globals.messages.empty", "name", "`events`"), nil, envelope.InputError)
	}
	out, err := app.integration.Update(tid, id, req.Events, req.IsActive)
	if errors.Is(err, integration.ErrNotFound) {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tid).T("globals.messages.notFound"), nil, envelope.NotFoundError)
	}
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("validation.invalid"), nil, envelope.InputError)
	}
	return r.SendEnvelope(out)
}

// handleDeleteChatIntegration disconnects a channel. The Slack app stays
// installed in the Slack workspace; removing it is done in Slack.
func handleDeleteChatIntegration(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	if err := app.integration.Delete(tenantID(r), id); err != nil {
		app.lo.Error("error deleting chat integration", "id", id, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(true)
}

// handleTestChatIntegration posts a message to the channel, answering with why
// it failed if it did.
func handleTestChatIntegration(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		t     = app.i18n.For(tid)
	)
	in, err := app.integration.Get(tid, id)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, t.T("globals.messages.notFound"), nil, envelope.NotFoundError)
	}
	c, cancel := context.WithTimeout(app.ctx, 15*time.Second)
	defer cancel()

	text := t.T("chat.connected.title") + ". " + t.T("chat.connected.body")
	switch in.Provider {
	case integration.ProviderSlack:
		_, err = app.integration.Slack().PostMessage(c, in.AccessToken, slack.Message{Channel: in.ChannelID, Text: text})
		if slack.IsCode(err, "not_in_channel") {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, t.T("chat.error.slackNotInChannel"), nil, envelope.InputError)
		}
	case integration.ProviderTeams:
		err = app.integration.Teams().Send(c, in.ServiceURL, in.ChannelID, teams.TextMessage(text))
	}
	if err != nil {
		app.lo.Error("chat integration test failed", "id", id, "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusBadGateway, t.Ts("webhook.testFailed", "error", err.Error()), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(true)
}

// handleSlackAuthorize starts installing the Slack app, returning Slack's install
// page. The admin picks the channel to post to there.
func handleSlackAuthorize(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
	)
	if !app.integration.Slack().App().Configured() {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("chat.error.slackNotConfigured"), nil, envelope.InputError)
	}
	state, err := stringutil.RandomAlphanumeric(32)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	user, _ := r.RequestCtx.UserValue("user").(amodels.User)
	redirectURI := slackRedirectURI(app, tid)
	key := "inbox_oauth:" + state
	if err := app.redis.HSet(ctx, key, map[string]any{
		"provider":     slackOAuthProvider,
		"redirect_uri": redirectURI,
		"workspace_id": tid,
		"user_id":      user.ID,
	}).Err(); err != nil {
		app.lo.Error("error storing slack oauth state", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	app.redis.Expire(ctx, key, 15*time.Minute)
	return r.SendEnvelope(app.integration.Slack().AuthorizeURL(redirectURI, state))
}

// handleSlackCallback finishes installing the Slack app: it checks the state
// belongs to this workspace and admin, trades the code for the bot token, and
// connects the channel picked, joining it when it is public.
func handleSlackCallback(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		args  = r.RequestCtx.QueryArgs()
		code  = string(args.Peek("code"))
		state = string(args.Peek("state"))
		back  = "/admin/integrations/slack"
	)
	if code == "" || state == "" {
		return r.Redirect(back+"?error=slack_denied", fasthttp.StatusFound, nil, "")
	}
	key := "inbox_oauth:" + state
	data, err := app.redis.HGetAll(ctx, key).Result()
	if err != nil || data["provider"] != slackOAuthProvider {
		return r.Redirect(back+"?error=slack_expired", fasthttp.StatusFound, nil, "")
	}
	user, _ := r.RequestCtx.UserValue("user").(amodels.User)
	if data["workspace_id"] != strconv.Itoa(tid) || data["user_id"] != strconv.Itoa(user.ID) {
		app.lo.Warn("slack oauth state used by another workspace or user", "workspace_id", tid, "user_id", user.ID)
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, app.i18n.For(tid).T("inbox.oauthStateMismatch"), nil, envelope.PermissionError)
	}
	app.redis.Del(ctx, key)

	c, cancel := context.WithTimeout(app.ctx, 30*time.Second)
	defer cancel()
	sc := app.integration.Slack()
	inst, err := sc.ExchangeCode(c, code, data["redirect_uri"])
	if err != nil {
		app.lo.Error("error finishing slack install", "error", err)
		return r.Redirect(back+"?error=slack_failed", fasthttp.StatusFound, nil, "")
	}
	// Public channels are joined here; a private one needs the bot invited.
	joinErr := sc.JoinChannel(c, inst.AccessToken, inst.IncomingWebhook.ChannelID)

	_, err = app.integration.Connect(tid, integration.Connect{
		Provider:    integration.ProviderSlack,
		TeamID:      inst.Team.ID,
		TeamName:    inst.Team.Name,
		ChannelID:   inst.IncomingWebhook.ChannelID,
		ChannelName: inst.IncomingWebhook.Channel,
		AccessToken: inst.AccessToken,
		BotUserID:   inst.BotUserID,
		ConnectedBy: user.ID,
	})
	if errors.Is(err, integration.ErrChannelTaken) {
		return r.Redirect(back+"?error=channel_taken", fasthttp.StatusFound, nil, "")
	}
	if err != nil {
		app.lo.Error("error saving slack connection", "error", err)
		return r.Redirect(back+"?error=slack_failed", fasthttp.StatusFound, nil, "")
	}
	app.lo.Info("slack channel connected", "tenant_id", tid, "team_id", inst.Team.ID, "channel_id", inst.IncomingWebhook.ChannelID)
	if joinErr != nil {
		return r.Redirect(back+"?success=connected&invite=1", fasthttp.StatusFound, nil, "")
	}
	return r.Redirect(back+"?success=connected", fasthttp.StatusFound, nil, "")
}

// verifySlack checks a request is signed by Slack, and returns its body.
func verifySlack(r *fastglue.Request) ([]byte, bool) {
	app := r.Context.(*App)
	body := r.RequestCtx.PostBody()
	if len(body) > maxChatWebhookBody {
		return nil, false
	}
	h := &r.RequestCtx.Request.Header
	if !slack.VerifySignature(app.integration.Slack().App().SigningSecret, body,
		string(h.Peek("X-Slack-Request-Timestamp")), string(h.Peek("X-Slack-Signature")), time.Now()) {
		app.lo.Warn("rejected slack request with a bad signature")
		return nil, false
	}
	return body, true
}

// handleSlackEvents receives the Events API: the URL check when the app is set
// up, replies in the threads the app posts, and the app being removed.
func handleSlackEvents(r *fastglue.Request) error {
	app := r.Context.(*App)
	body, ok := verifySlack(r)
	if !ok {
		r.RequestCtx.SetStatusCode(fasthttp.StatusUnauthorized)
		return nil
	}
	var env slack.EventEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusBadRequest)
		return nil
	}
	if env.Type == "url_verification" {
		r.RequestCtx.SetContentType("text/plain; charset=utf-8")
		r.RequestCtx.SetBodyString(env.Challenge)
		return nil
	}
	// Slack retries an event it thinks timed out; it was handled the first time.
	if len(r.RequestCtx.Request.Header.Peek("X-Slack-Retry-Num")) > 0 {
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	}

	switch env.Event.Type {
	case "app_uninstalled", "tokens_revoked":
		app.lo.Info("slack app removed from workspace", "team_id", env.TeamID)
		app.integration.DeactivateTeam(integration.ProviderSlack, env.TeamID)
	case "message":
		// Slack wants an answer within three seconds, so the reply is handled after.
		go handleSlackThreadReply(app, env.TeamID, env.Event)
	}
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	return nil
}

// handleSlackThreadReply adds a reply in a conversation's thread as an internal
// note by the agent who wrote it, and ticks it.
func handleSlackThreadReply(app *App, teamID string, ev slack.Event) {
	in, err := app.integration.FindByChannel(integration.ProviderSlack, teamID, ev.Channel)
	if err != nil || !in.IsActive || !ev.IsThreadReply(in.BotUserID) {
		return
	}
	convID, err := app.integration.ThreadConversation(in, ev.ThreadTS)
	if err != nil || convID == 0 {
		return
	}
	c, cancel := context.WithTimeout(app.ctx, chatActionTimeout)
	defer cancel()
	sc := app.integration.Slack()
	t := app.i18n.For(in.TenantID)

	agent, err := slackAgent(c, app, in, ev.User)
	if err != nil {
		sc.PostEphemeral(c, in.AccessToken, ev.Channel, ev.User, ev.ThreadTS, chatErrorText(t, err))
		return
	}
	content := slack.TextToHTML(ev.Text)
	if content == "" {
		return
	}
	if _, err := doChatAction(app, in, agent, convID, chatActionNote, content, true); err != nil {
		sc.PostEphemeral(c, in.AccessToken, ev.Channel, ev.User, ev.ThreadTS, chatErrorText(t, err))
		return
	}
	if err := sc.AddReaction(c, in.AccessToken, ev.Channel, ev.TS, "white_check_mark"); err != nil {
		app.lo.Debug("error reacting to slack reply", "error", err)
	}
}

// handleSlackInteractions receives button clicks on the app's posts and the
// reply modal being submitted.
func handleSlackInteractions(r *fastglue.Request) error {
	app := r.Context.(*App)
	body, ok := verifySlack(r)
	if !ok {
		r.RequestCtx.SetStatusCode(fasthttp.StatusUnauthorized)
		return nil
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusBadRequest)
		return nil
	}
	in, err := slack.ParseInteraction(form.Get("payload"))
	if err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusBadRequest)
		return nil
	}

	switch in.Type {
	case "block_actions":
		handleSlackButton(app, in)
		r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	case "view_submission":
		if in.View.CallbackID != slack.CallbackReply {
			break
		}
		if resp := handleSlackReplyModal(app, in); resp != nil {
			b, _ := json.Marshal(resp)
			r.RequestCtx.SetContentType("application/json")
			r.RequestCtx.SetBody(b)
			return nil
		}
	}
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	return nil
}

// handleSlackButton acts on a button: Reply opens the modal at once, as Slack
// requires; Assign to me and Resolve run after answering, and tell only the
// person who clicked how it went.
func handleSlackButton(app *App, in slack.Interaction) {
	if len(in.Actions) == 0 {
		return
	}
	action := in.Actions[0]
	var chatAction string
	switch action.ActionID {
	case slack.ActionAssign:
		chatAction = chatActionAssign
	case slack.ActionResolve:
		chatAction = chatActionResolve
	case slack.ActionReply:
		chatAction = chatActionReply
	default:
		return
	}
	conn, convID, ok := slackThreadConn(app, in.Team.ID, in.Channel.ID, action.Value)
	if !ok {
		return
	}
	t := app.i18n.For(conn.TenantID)
	sc := app.integration.Slack()

	if chatAction == chatActionReply {
		c, cancel := context.WithTimeout(app.ctx, 2500*time.Millisecond)
		defer cancel()
		title := ""
		if conv, err := app.conversation.GetConversation(conn.TenantID, convID, "", ""); err == nil {
			title = chatmsg.Format(t.T("chat.conversationRef"), "ref", conv.ReferenceNumber)
			if conv.Subject.String != "" {
				title += " · " + conv.Subject.String
			}
		}
		thread := in.Container.ThreadTS
		if thread == "" {
			thread = in.Container.MessageTS
		}
		if err := sc.OpenView(c, conn.AccessToken, in.TriggerID, slack.ReplyModal(t, conn.ID, convID, in.Channel.ID, thread, title)); err != nil {
			app.lo.Error("error opening slack reply modal", "error", err)
		}
		return
	}

	go func() {
		c, cancel := context.WithTimeout(app.ctx, chatActionTimeout)
		defer cancel()
		text := ""
		agent, err := slackAgent(c, app, conn, in.User.ID)
		if err == nil {
			text, err = doChatAction(app, conn, agent, convID, chatAction, "", false)
		}
		if err != nil {
			text = chatErrorText(t, err)
		}
		if err := sc.Respond(c, in.ResponseURL, map[string]any{"response_type": "ephemeral", "replace_original": false, "text": text}); err != nil {
			app.lo.Error("error answering slack button", "error", err)
		}
	}()
}

// handleSlackReplyModal sends the reply or adds the note written in the modal.
// It returns the answer that keeps the modal open with an error, or nil to close it.
func handleSlackReplyModal(app *App, in slack.Interaction) map[string]any {
	input, ok := in.ReplyInput()
	if !ok {
		return nil
	}
	conn, convID, ok := slackThreadConn(app, in.Team.ID, input.Channel, strconv.Itoa(input.ConversationID))
	if !ok || conn.ID != input.IntegrationID {
		return nil
	}
	t := app.i18n.For(conn.TenantID)
	if input.Text == "" {
		return slack.ModalError(t.T("chat.error.emptyReply"))
	}
	c, cancel := context.WithTimeout(app.ctx, 2500*time.Millisecond)
	defer cancel()
	agent, err := slackAgent(c, app, conn, in.User.ID)
	if err != nil {
		return slack.ModalError(chatErrorText(t, err))
	}
	action := chatActionReply
	if input.Mode == slack.ModeNote {
		action = chatActionNote
	}
	if _, err := doChatAction(app, conn, agent, convID, action, slack.PlainToHTML(input.Text), false); err != nil {
		return slack.ModalError(chatErrorText(t, err))
	}
	return nil
}

// slackThreadConn finds the connected channel a click came from, and checks
// the conversation it names has a thread there.
func slackThreadConn(app *App, teamID, channelID, value string) (integration.Integration, int, bool) {
	conn, err := app.integration.FindByChannel(integration.ProviderSlack, teamID, channelID)
	if err != nil || !conn.IsActive {
		return conn, 0, false
	}
	convID, err := strconv.Atoi(value)
	if err != nil || convID <= 0 {
		return conn, 0, false
	}
	if ref, err := app.integration.Thread(conn, convID); err != nil || ref == "" {
		return conn, 0, false
	}
	return conn, convID, true
}

// chatEmails remembers Slack and Teams users' emails, so each click doesn't
// look them up again.
var chatEmails sync.Map

type cachedEmail struct {
	email string
	exp   time.Time
}

// slackAgent is the agent a Slack user is, matched by email.
func slackAgent(c context.Context, app *App, in integration.Integration, slackUser string) (umodels.User, error) {
	key := "slack:" + in.ExternalTeamID + ":" + slackUser
	email := cachedChatEmail(key)
	if email == "" {
		u, err := app.integration.Slack().UserInfo(c, in.AccessToken, slackUser)
		if err != nil {
			return umodels.User{}, err
		}
		if u.Deleted || u.IsBot {
			return umodels.User{}, errNoChatAgent{}
		}
		email = u.Profile.Email
		chatEmails.Store(key, cachedEmail{email, time.Now().Add(chatEmailTTL)})
	}
	return chatAgent(app, in.TenantID, email)
}

// teamsAgent is the agent a Teams user is, matched by email or sign-in name.
func teamsAgent(c context.Context, app *App, in integration.Integration, act teams.Activity) (umodels.User, error) {
	key := "teams:" + in.ExternalTeamID + ":" + act.From.ID
	email := cachedChatEmail(key)
	if email == "" {
		m, err := app.integration.Teams().GetMember(c, in.ServiceURL, act.Conversation.ID, act.From.ID)
		if err != nil {
			return umodels.User{}, err
		}
		email = m.Email
		if email == "" {
			email = m.UserPrincipalName
		}
		chatEmails.Store(key, cachedEmail{email, time.Now().Add(chatEmailTTL)})
	}
	return chatAgent(app, in.TenantID, email)
}

func cachedChatEmail(key string) string {
	if v, ok := chatEmails.Load(key); ok {
		if ce := v.(cachedEmail); time.Now().Before(ce.exp) {
			return ce.email
		}
		chatEmails.Delete(key)
	}
	return ""
}

// errNoChatAgent is a Slack or Teams user with no enabled agent of the same email.
type errNoChatAgent struct{ email string }

func (e errNoChatAgent) Error() string { return "no agent with email " + e.email }

// errChatDenied is an agent without the permission an action needs.
var errChatDenied = errors.New("permission denied")

// chatAgent loads the enabled agent with the email, with their permissions.
func chatAgent(app *App, tenantID int, email string) (umodels.User, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return umodels.User{}, errNoChatAgent{}
	}
	a, err := app.user.GetAgent(tenantID, 0, email)
	if err != nil {
		return umodels.User{}, errNoChatAgent{email}
	}
	agent, err := app.user.GetAgentCachedOrLoad(tenantID, a.ID)
	if err != nil || !agent.Enabled || agent.Type != umodels.UserTypeAgent {
		return umodels.User{}, errNoChatAgent{email}
	}
	return agent, nil
}

// doChatAction acts on a conversation as an agent, with the same checks the
// desk makes: the agent must be able to see the conversation and have the
// permission for the action. content is HTML for replies and notes. fromThread
// marks a note written in the channel's thread, which isn't posted back to it.
// It returns what to tell the agent.
func doChatAction(app *App, in integration.Integration, agent umodels.User, convID int, action, content string, fromThread bool) (string, error) {
	tid := in.TenantID
	t := app.i18n.For(tid)
	conv, err := app.conversation.GetConversation(tid, convID, "", "")
	if err != nil {
		return "", err
	}
	allowed, err := app.authz.EnforceConversationAccess(agent, conv)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", errChatDenied
	}
	can := func(obj, act string) error {
		ok, err := app.authz.Enforce(agent, obj, act)
		if err != nil {
			return err
		}
		if !ok {
			return errChatDenied
		}
		return nil
	}
	ref := chatmsg.Format(t.T("chat.conversationRef"), "ref", conv.ReferenceNumber)

	switch action {
	case chatActionAssign:
		if err := can("conversations", "update_user_assignee"); err != nil {
			return "", err
		}
		if conv.AssignedUserID.Int == agent.ID {
			return chatmsg.Format(t.T("chat.done.alreadyYours"), "ref", ref), nil
		}
		if err := app.conversation.UpdateConversationUserAssignee(tid, conv.UUID, agent.ID, agent); err != nil {
			return "", err
		}
		return chatmsg.Format(t.T("chat.done.assigned"), "ref", ref), nil

	case chatActionResolve:
		if err := can("conversations", "update_status"); err != nil {
			return "", err
		}
		if err := app.conversation.UpdateConversationStatus(tid, conv.UUID, 0, cmodels.StatusResolved, "", agent); err != nil {
			return "", err
		}
		return chatmsg.Format(t.T("chat.done.resolved"), "ref", ref), nil

	case chatActionNote:
		if err := can("messages", "write"); err != nil {
			return "", err
		}
		if fromThread {
			app.integration.ExpectNote(in.ID, conv.UUID, agent.ID)
		}
		if _, err := app.conversation.SendPrivateNote(tid, nil, agent.ID, conv.UUID, content, nil); err != nil {
			if fromThread {
				app.integration.ForgetNote(in.ID, conv.UUID, agent.ID)
			}
			return "", err
		}
		return chatmsg.Format(t.T("chat.done.noted"), "ref", ref), nil

	case chatActionReply:
		if err := can("messages", "write"); err != nil {
			return "", err
		}
		inb, err := app.inbox.GetDBRecord(tid, conv.InboxID)
		if err != nil {
			return "", err
		}
		var to []string
		switch inb.Channel {
		case inbox.ChannelEmail:
			if conv.Contact.Email.String == "" {
				return "", errors.New(t.T("chat.error.noEmail"))
			}
			to = []string{conv.Contact.Email.String}
		case whatsapp.ChannelWhatsApp:
			// Only a template may be sent after 24 hours, and those are picked in the desk.
			window, err := whatsAppWindow(app, tid, conv.ContactID, conv.InboxID)
			if err != nil {
				return "", err
			}
			if !window.Open {
				return "", errors.New(t.T("whatsapp.windowClosed"))
			}
		}
		if _, err := app.conversation.QueueReply(tid, nil, conv.InboxID, agent.ID, conv.ContactID, conv.UUID, content, to, nil, nil, map[string]any{}); err != nil {
			return "", err
		}
		return chatmsg.Format(t.T("chat.done.replied"), "ref", ref), nil
	}
	return "", fmt.Errorf("unknown chat action %q", action)
}

// chatErrorText is what to tell an agent whose action didn't go through.
func chatErrorText(t interface{ T(string) string }, err error) string {
	var noAgent errNoChatAgent
	var env envelope.Error
	var envp *envelope.Error
	switch {
	case errors.As(err, &noAgent):
		return chatmsg.Format(t.T("chat.error.noAgent"), "email", orText(noAgent.email, "?"))
	case errors.Is(err, errChatDenied):
		return t.T("chat.error.denied")
	case errors.As(err, &envp):
		return envp.Message
	case errors.As(err, &env):
		return env.Message
	}
	return chatmsg.Format(t.T("chat.error.failed"), "error", err.Error())
}

func orText(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// teamsBotName is what people @mention the Teams bot as.
func teamsBotName(app *App) string {
	if n := app.integration.Teams().App().Name; n != "" {
		return n
	}
	return "ConectDesk"
}

// handleGetTeamsPackage downloads the Teams app package, which an admin uploads
// in Teams to add the bot to their teams.
func handleGetTeamsPackage(r *fastglue.Request) error {
	app := r.Context.(*App)
	tc := app.integration.Teams()
	if !tc.App().Configured() {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("chat.error.teamsNotConfigured"), nil, envelope.InputError)
	}
	base := chatPublicBase(app)
	u, _ := url.Parse(base)
	b, err := teams.Package(teams.App{AppID: tc.App().AppID, Name: teamsBotName(app)}, base, u.Hostname())
	if err != nil {
		app.lo.Error("error building teams package", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	r.RequestCtx.SetContentType("application/zip")
	r.RequestCtx.Response.Header.Set("Content-Disposition", `attachment; filename="conectdesk-teams.zip"`)
	r.RequestCtx.SetBody(b)
	return nil
}

// handleCreateTeamsCode makes a one-time code that links a Teams channel to
// the workspace, sent to the bot in that channel.
func handleCreateTeamsCode(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
	)
	if !app.integration.Teams().App().Configured() {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("chat.error.teamsNotConfigured"), nil, envelope.InputError)
	}
	code, err := stringutil.RandomAlphanumeric(8)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	code = strings.ToUpper(code)
	user, _ := r.RequestCtx.UserValue("user").(amodels.User)
	if err := app.redis.Set(ctx, "teams_link:"+code, fmt.Sprintf("%d:%d", tid, user.ID), teamsLinkTTL).Err(); err != nil {
		app.lo.Error("error storing teams link code", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(map[string]any{
		"code":       code,
		"command":    "@" + teamsBotName(app) + " connect " + code,
		"expires_in": int(teamsLinkTTL.Seconds()),
	})
}

// handleTeamsMessages is the Teams bot's messaging endpoint. Each request is
// signed by the Bot Framework; it is answered at once and handled after.
func handleTeamsMessages(r *fastglue.Request) error {
	app := r.Context.(*App)
	body := r.RequestCtx.PostBody()
	if len(body) > maxChatWebhookBody || !app.integration.Teams().App().Configured() {
		r.RequestCtx.SetStatusCode(fasthttp.StatusBadRequest)
		return nil
	}
	var act teams.Activity
	if err := json.Unmarshal(body, &act); err != nil {
		r.RequestCtx.SetStatusCode(fasthttp.StatusBadRequest)
		return nil
	}
	c, cancel := context.WithTimeout(app.ctx, 10*time.Second)
	err := app.teamsVerifier.Verify(c, string(r.RequestCtx.Request.Header.Peek("Authorization")), act)
	cancel()
	if err != nil {
		app.lo.Warn("rejected teams request", "error", err)
		r.RequestCtx.SetStatusCode(fasthttp.StatusUnauthorized)
		return nil
	}
	if !teams.ValidServiceURL(act.ServiceURL) {
		r.RequestCtx.SetStatusCode(fasthttp.StatusBadRequest)
		return nil
	}
	go handleTeamsActivity(app, act)
	r.RequestCtx.SetStatusCode(fasthttp.StatusOK)
	return nil
}

// handleTeamsActivity handles what the bot receives: being added to a team, a
// link code, a card submitted, or an @mention in a conversation's thread.
func handleTeamsActivity(app *App, act teams.Activity) {
	c, cancel := context.WithTimeout(app.ctx, chatActionTimeout)
	defer cancel()
	tc := app.integration.Teams()
	reply := func(t interface{ T(string) string }, text string) {
		if err := tc.Send(c, act.ServiceURL, act.Conversation.ID, teams.TextMessage(text)); err != nil {
			app.lo.Error("error answering in teams", "error", err)
		}
	}

	switch act.Type {
	case "conversationUpdate":
		for _, m := range act.MembersAdded {
			if m.ID == act.Recipient.ID {
				t := app.i18n.Default()
				reply(t, chatmsg.Format(t.T("chat.teams.welcome"), "bot", teamsBotName(app)))
				return
			}
		}
		return
	case "message":
	default:
		return
	}

	channelID, msTenant := act.ChannelRef()
	text := teams.CleanText(act.Text)

	if code, ok := teams.LinkCode(text); ok && len(act.Value) == 0 {
		linkTeamsChannel(c, app, act, code, channelID, msTenant, reply)
		return
	}

	conn, err := app.integration.FindByChannel(integration.ProviderTeams, msTenant, channelID)
	if err != nil || !conn.IsActive {
		t := app.i18n.Default()
		reply(t, chatmsg.Format(t.T("chat.teams.notLinked"), "bot", teamsBotName(app)))
		return
	}
	t := app.i18n.For(conn.TenantID)

	// A card's buttons.
	if submit, ok := teams.ParseSubmit(act.Value); ok {
		if ref, err := app.integration.Thread(conn, submit.ConversationID); err != nil || ref == "" {
			return
		}
		action := map[string]string{
			teams.ActionAssign: chatActionAssign, teams.ActionResolve: chatActionResolve,
			teams.ActionReply: chatActionReply, teams.ActionNote: chatActionNote,
		}[submit.Action]
		if action == "" {
			return
		}
		if (action == chatActionReply || action == chatActionNote) && submit.Text == "" {
			reply(t, t.T("chat.error.emptyReply"))
			return
		}
		agent, err := teamsAgent(c, app, conn, act)
		if err == nil {
			var done string
			done, err = doChatAction(app, conn, agent, submit.ConversationID, action, slack.PlainToHTML(submit.Text), false)
			if err == nil {
				reply(t, done)
				return
			}
		}
		reply(t, chatErrorText(t, err))
		return
	}

	// An @mention in a conversation's thread is an internal note.
	convID, err := app.integration.ThreadConversation(conn, act.Conversation.ID)
	if err != nil || convID == 0 {
		reply(t, chatmsg.Format(t.T("chat.teams.help"), "bot", teamsBotName(app)))
		return
	}
	if text == "" {
		return
	}
	agent, err := teamsAgent(c, app, conn, act)
	if err == nil {
		var done string
		done, err = doChatAction(app, conn, agent, convID, chatActionNote, slack.PlainToHTML(text), true)
		if err == nil {
			reply(t, done)
			return
		}
	}
	reply(t, chatErrorText(t, err))
}

// linkTeamsChannel connects the channel a link code was sent in to the
// workspace the code was made for. A code works once.
func linkTeamsChannel(c context.Context, app *App, act teams.Activity, code, channelID, msTenant string, reply func(interface{ T(string) string }, string)) {
	def := app.i18n.Default()
	val, err := app.redis.GetDel(c, "teams_link:"+code).Result()
	if err != nil || val == "" {
		reply(def, def.T("chat.teams.codeExpired"))
		return
	}
	var tid, userID int
	if _, err := fmt.Sscanf(val, "%d:%d", &tid, &userID); err != nil || tid <= 0 {
		reply(def, def.T("chat.teams.codeExpired"))
		return
	}
	t := app.i18n.For(tid)
	channelName := act.ChannelName()
	if channelName == "" {
		channelName = "General"
	}
	_, err = app.integration.Connect(tid, integration.Connect{
		Provider:    integration.ProviderTeams,
		TeamID:      msTenant,
		TeamName:    act.TeamName(),
		ChannelID:   channelID,
		ChannelName: channelName,
		ServiceURL:  act.ServiceURL,
		ConnectedBy: userID,
	})
	if errors.Is(err, integration.ErrChannelTaken) {
		reply(t, t.T("chat.teams.channelTaken"))
		return
	}
	if err != nil {
		app.lo.Error("error linking teams channel", "error", err)
		reply(t, chatmsg.Format(t.T("chat.error.failed"), "error", err.Error()))
		return
	}
	name := ""
	if ten, err := app.tenant.Get(tid); err == nil {
		name = ten.Name
	}
	app.lo.Info("teams channel connected", "tenant_id", tid, "channel_id", channelID)
	reply(t, chatmsg.Format(t.T("chat.teams.linked"), "workspace", orText(name, tenantConsts(app, tid).SiteName)))
}
