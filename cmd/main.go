package main

import (
	"cmp"
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	_ "time/tzdata"

	activitylog "github.com/conectdesk/conectdesk/internal/activity_log"
	"github.com/conectdesk/conectdesk/internal/ai"
	"github.com/conectdesk/conectdesk/internal/aiagent"
	auth_ "github.com/conectdesk/conectdesk/internal/auth"
	"github.com/conectdesk/conectdesk/internal/authz"
	businesshours "github.com/conectdesk/conectdesk/internal/business_hours"
	"github.com/conectdesk/conectdesk/internal/callback"
	"github.com/conectdesk/conectdesk/internal/colorlog"
	"github.com/conectdesk/conectdesk/internal/csat"
	customAttribute "github.com/conectdesk/conectdesk/internal/custom_attribute"
	"github.com/conectdesk/conectdesk/internal/dailyreport"
	"github.com/conectdesk/conectdesk/internal/docs"
	"github.com/conectdesk/conectdesk/internal/export"
	"github.com/conectdesk/conectdesk/internal/lead"
	"github.com/conectdesk/conectdesk/internal/macro"
	"github.com/conectdesk/conectdesk/internal/mfa"
	notifier "github.com/conectdesk/conectdesk/internal/notification"
	emailnotifier "github.com/conectdesk/conectdesk/internal/notification/providers/email"
	"github.com/conectdesk/conectdesk/internal/platform"
	"github.com/conectdesk/conectdesk/internal/report"
	"github.com/conectdesk/conectdesk/internal/search"
	"github.com/conectdesk/conectdesk/internal/sla"
	"github.com/conectdesk/conectdesk/internal/softphone"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/conectdesk/conectdesk/internal/view"
	"github.com/conectdesk/conectdesk/internal/workspace"
	"github.com/redis/go-redis/v9"

	"github.com/conectdesk/conectdesk/internal/automation"
	contextlink "github.com/conectdesk/conectdesk/internal/context_link"
	"github.com/conectdesk/conectdesk/internal/conversation"
	"github.com/conectdesk/conectdesk/internal/conversation/priority"
	"github.com/conectdesk/conectdesk/internal/conversation/status"
	"github.com/conectdesk/conectdesk/internal/helpcenter"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/importer"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/integration"
	"github.com/conectdesk/conectdesk/internal/integration/teams"
	"github.com/conectdesk/conectdesk/internal/media"
	"github.com/conectdesk/conectdesk/internal/oidc"
	"github.com/conectdesk/conectdesk/internal/ratelimit"
	"github.com/conectdesk/conectdesk/internal/role"
	"github.com/conectdesk/conectdesk/internal/setting"
	"github.com/conectdesk/conectdesk/internal/tag"
	"github.com/conectdesk/conectdesk/internal/team"
	"github.com/conectdesk/conectdesk/internal/template"
	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/conectdesk/conectdesk/internal/user"
	"github.com/conectdesk/conectdesk/internal/webhook"
	"github.com/conectdesk/conectdesk/internal/ws"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"github.com/zerodha/logf"
)

var (
	ko          = koanf.New(".")
	ctx         = context.Background()
	appName     = "conectdesk"
	frontendDir = "frontend/dist/main"
	widgetDir   = "frontend/dist/widget"

	// Injected at build time.
	buildString   string
	versionString string

	// assetVersion is hashed from buildString so public cache-bust URLs don't leak the version.
	assetVersion = func() string {
		src := buildString
		if src == "" {
			src = strconv.FormatInt(time.Now().Unix(), 36)
		}
		h := fnv.New32a()
		h.Write([]byte(src))
		return strconv.FormatUint(uint64(h.Sum32()), 36)
	}()
)

const (
	sampleEncKey = "your-32-char-random-string-here!"
)

// App is the global app context which is passed and injected in the http handlers.
type App struct {
	ctx      context.Context
	fs       stuffbin.FileSystem
	consts   atomic.Value
	auth     *auth_.Auth
	authz    *authz.Enforcer
	i18n     *i18ncat.Catalog
	lo       *logf.Logger
	oidc     *oidc.Manager
	media    *media.Manager
	setting  *setting.Manager
	role     *role.Manager
	user     *user.Manager
	team     *team.Manager
	status   *status.Manager
	priority *priority.Manager
	tag      *tag.Manager
	tenant   *tenant.Manager
	// rootURL returns the URL a tenant is served on.
	rootURL func(tenantID int) string
	// mailer sends each tenant's notification email.
	mailer *emailnotifier.TenantRouter
	// workspace creates workspaces, from the platform dashboard only; nil without
	// multi-tenancy.
	workspace *workspace.Manager
	// platform backs the platform admin dashboard, served on platformHost;
	// nil without multi-tenancy.
	platform     *platform.Manager
	platformHost string
	// docs is the internal documentation, served on docsHost to docs viewers.
	docs     *docs.Docs
	docsHost string
	// mfa manages agents' two-factor login.
	mfa *mfa.Manager

	// softphone keeps the SIP accounts agents take calls on in their browser.
	softphone *softphone.Manager
	// export builds workspaces' data exports.
	export *export.Manager
	// tenantConsts caches each tenant's constants; see tenantConsts().
	tenantConsts     sync.Map
	inbox            *inbox.Manager
	tmpl             *template.Manager
	macro            *macro.Manager
	conversation     *conversation.Manager
	automation       *automation.Engine
	businessHours    *businesshours.Manager
	sla              *sla.Manager
	csat             *csat.Manager
	view             *view.Manager
	ai               *ai.Manager
	aiAgent          *aiagent.Manager
	helpcenter       *helpcenter.Manager
	search           *search.Manager
	activityLog      *activitylog.Manager
	notifier         *notifier.Service
	userNotification *notifier.UserNotificationManager
	customAttribute  *customAttribute.Manager
	report           *report.Manager
	// callback keeps the call-backs missed calls become.
	callback *callback.Manager
	// lead keeps contacts' lead stages.
	lead *lead.Manager
	// dailyReport sends the owner the day's numbers.
	dailyReport *dailyreport.Manager
	// notifDispatcher sends in-app, websocket and email notifications.
	notifDispatcher *notifier.Dispatcher
	webhook         *webhook.Manager
	// integration is the Slack app and Teams bot; teamsVerifier checks the
	// Bot Framework signed what the bot receives.
	integration   *integration.Manager
	teamsVerifier *teams.Verifier
	contextLink   *contextlink.Manager
	rateLimit     *ratelimit.Limiter
	redis         *redis.Client
	importer      *importer.Importer
	wsHub         *ws.Hub

	// Global state that stores data on an available app update.
	update *AppUpdate
	// Flag to indicate if app restart is required for settings to take effect.
	restartRequired bool
	sync.Mutex
}

func main() {
	// Set up signal handler.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Load command line flags into Koanf.
	initFlags()

	if ko.Bool("version") {
		fmt.Println(buildString)
		os.Exit(0)
	}

	// Build string injected at build time.
	colorlog.Green("Build: %s", buildString)

	// Load the config files into Koanf.
	initConfig(ko)

	// Validate custom static directory if provided.
	if dir := getCustomStaticDir(); dir != "" {
		if _, err := os.Stat(dir); err != nil {
			log.Fatalf("--static-dir path not accessible: %s: %v", dir, err)
		}
		colorlog.Green("Using custom static directory: %s", dir)
	}

	// Init stuffbin fs with optional custom static dir overlay.
	fs := initFS(getCustomStaticDir())

	db := initDB()

	if ko.Bool("install") {
		install(ctx, db, fs, ko.Bool("idempotent-install"), !ko.Bool("yes"))
		os.Exit(0)
	}

	if ko.Bool("set-system-user-password") {
		setSystemUserPass(ctx, db)
		os.Exit(0)
	}

	// Check if schema is installed.
	installed, err := checkSchema(db)
	if err != nil {
		log.Fatalf("error checking db schema: %v", err)
	}
	if !installed {
		log.Println("database tables are missing. Use the `--install` flag to set up the database schema.")
		os.Exit(0)
	}

	if ko.Bool("upgrade") {
		upgrade(db, fs, !ko.Bool("yes"))
		os.Exit(0)
	}

	checkPendingUpgrade(db)

	if ko.Bool("copy-uploads-to-s3") {
		copyUploadsToS3(db)
		os.Exit(0)
	}

	if runPlatformAdminCommand(db) {
		os.Exit(0)
	}

	if runDocsViewerCommand(db) {
		os.Exit(0)
	}
	if runTenantCommand(db, fs) {
		os.Exit(0)
	}

	// Load app settings from DB into the Koanf instance.
	settings := initSettings(db)
	loadSettings(settings)

	validateConfig(ko)

	startPprof()

	// Fallback for config typo. Logs a warning but continues to work with the incorrect key.
	// Uses 'message.message_outgoing_scan_interval' (correct key) as default key, falls back to the common typo.
	msgOutgoingScanIntervalKey := "message.message_outgoing_scan_interval"
	if ko.String(msgOutgoingScanIntervalKey) == "" {
		if ko.String("message.message_outoing_scan_interval") != "" {
			colorlog.Red("WARNING: typo in config key 'message.message_outoing_scan_interval' detected. Use 'message.message_outgoing_scan_interval' instead in your config.toml file. Support for this incorrect key will be removed in a future release.")
			msgOutgoingScanIntervalKey = "message.message_outoing_scan_interval"
		}
	}

	var (
		autoAssignInterval          = ko.MustDuration("autoassigner.autoassign_interval")
		unsnoozeInterval            = ko.MustDuration("conversation.unsnooze_interval")
		draftRetentionDuration      = cmp.Or(ko.Duration("conversation.draft_retention_duration"), 360*time.Hour)
		automationWorkers           = ko.MustInt("automation.worker_count")
		messageOutgoingQWorkers     = ko.MustDuration("message.outgoing_queue_workers")
		messageIncomingQWorkers     = ko.MustDuration("message.incoming_queue_workers")
		messageOutgoingScanInterval = ko.MustDuration(msgOutgoingScanIntervalKey)
		slaEvaluationInterval       = ko.MustDuration("sla.evaluation_interval")
		lo                          = initLogger(appName)
		rdb                         = initRedis()
		constants                   = initConstants()
		i18n                        = initI18n(fs)
		csat                        = initCSAT(db, i18n)
		tenants                     = initTenant(db)
		rootURL                     = newTenantRootURL(tenants, settings)
		oidc                        = initOIDC(db, rootURL, i18n)
		status                      = initStatus(db, i18n)
		priority                    = initPriority(db, i18n)
		ssrfControl                 = initSSRFControl()
		auth                        = initAuth(oidc, rdb, i18n, ssrfControl)
		template                    = initTemplate(db, fs, constants, i18n)
		media                       = initMedia(db, i18n, rootURL)
		inbox                       = initInbox(db, i18n)
		team                        = initTeam(db, i18n)
		businessHours               = initBusinessHours(db, i18n)
		webhook                     = initWebhook(db, i18n, ssrfControl, rootURL)
		chatIntegration, teamsVerif = initIntegration(db, i18n, rootURL)
		user                        = initUser(i18n, db)
		wsHub                       = initWS(user)
		mailer                      = initMailer(settings, tenants)
		notifier                    = initNotifier(mailer)
		userNotification            = initUserNotification(db, i18n)
		notifDispatcher             = initNotifDispatcher(userNotification, notifier, wsHub, mailer.Configured)
		automation                  = initAutomationEngine(db, i18n)
		ai                          = initAI(ctx, db, i18n, ssrfControl, tenants)
		sla                         = initSLA(db, team, settings, businessHours, template, user, i18n, notifDispatcher)
		conversation                = initConversations(i18n, sla, status, priority, wsHub, db, inbox, user, team, media, settings, csat, automation, template, webhook, notifDispatcher)
		aiAgent                     = initAIAgent(db, i18n, ai, conversation, media, settings, user, notifier, rdb)
		helpCenter                  = initHelpCenter(db, i18n, ai, tenants)
		autoassigner                = initAutoAssigner(team, user, conversation, tenants)
		rateLimiter                 = initRateLimit(rdb)
	)

	wsHub.SetConversationStore(conversation)
	automation.SetConversationStore(conversation)
	if _, err := user.GetSystemUser(tenant.DefaultID); err != nil {
		log.Fatalf("error fetching system user: %v", err)
	}
	automation.SetSystemUserResolver(newSystemUserResolver(user))
	conversation.SetAIAgent(aiAgent)
	conversation.SetRootURL(rootURL)

	startInboxes(ctx, inbox, conversation, user, conversation.SignAvatarURL)

	go automation.Run(ctx, automationWorkers)
	go autoassigner.Run(ctx, autoAssignInterval)
	go conversation.Run(ctx, messageIncomingQWorkers, messageOutgoingQWorkers, messageOutgoingScanInterval)
	go conversation.RunUnsnoozer(ctx, unsnoozeInterval)
	go conversation.RunContinuity(ctx)
	webhook.SetEventSink(chatIntegration)
	webhook.SetEnabled(func(tid int) bool { return tenantIntegrationAllowed(tenants, tid, integrationWebhooks) })
	chatIntegration.SetEnabled(func(tid int, provider string) bool { return tenantIntegrationAllowed(tenants, tid, provider) })
	go webhook.Run(ctx)
	go chatIntegration.Run(ctx)
	go notifier.Run(ctx)
	go sla.Run(ctx, slaEvaluationInterval)
	go sla.SendNotifications(ctx)
	go media.DeleteUnlinkedMedia(ctx)
	go user.MonitorUserAvailability(ctx, onUsersOffline(conversation))
	go conversation.RunDraftCleaner(ctx, draftRetentionDuration)
	go userNotification.RunNotificationCleaner(ctx)
	go helpCenter.RunSearchLogCleaner(ctx)
	go aiAgent.Run(ctx, cmp.Or(ko.Int("ai_agent.worker_count"), 10))
	go ai.Run(ctx)

	var app = &App{
		ctx:              ctx,
		lo:               lo,
		fs:               fs,
		sla:              sla,
		oidc:             oidc,
		i18n:             i18n,
		auth:             auth,
		media:            media,
		setting:          settings,
		inbox:            inbox,
		user:             user,
		team:             team,
		csat:             csat,
		status:           status,
		priority:         priority,
		tmpl:             template,
		notifier:         notifier,
		consts:           atomic.Value{},
		conversation:     conversation,
		automation:       automation,
		businessHours:    businessHours,
		activityLog:      initActivityLog(db, i18n),
		customAttribute:  initCustomAttribute(db, i18n),
		authz:            initAuthz(i18n),
		view:             initView(db, i18n),
		report:           initReport(db, i18n),
		search:           initSearch(db, i18n),
		role:             initRole(db, i18n),
		tag:              initTag(db, i18n),
		tenant:           tenants,
		rootURL:          rootURL,
		macro:            initMacro(db, i18n),
		ai:               ai,
		aiAgent:          aiAgent,
		helpcenter:       helpCenter,
		importer:         initImporter(i18n),
		webhook:          webhook,
		integration:      chatIntegration,
		teamsVerifier:    teamsVerif,
		contextLink:      initContextLink(db, i18n),
		rateLimit:        rateLimiter,
		redis:            rdb,
		userNotification: userNotification,
		wsHub:            wsHub,
	}
	app.consts.Store(constants)
	template.SetTenantFuncs(tenantTemplateFuncs(app))
	app.mailer = mailer
	app.workspace = initWorkspace(db, tenants)
	app.platform, app.platformHost = initPlatform(db, tenants)
	app.docs, app.docsHost = initDocs(tenants, app.platform)
	if app.platform != nil {
		mailer.SetPlatformSettings(func() (emailnotifier.Settings, error) {
			ms, err := app.platform.MailSettings()
			return platformMailerSettings(ms), err
		})
		aiAgent.SetPlatformFrom(func() string {
			ms, _ := app.platform.MailSettings()
			return ms.From
		})
	}
	app.mfa = initMFA(db)
	app.softphone = initSoftphone(db)
	app.callback = initCallback(db, i18n)
	app.lead = initLead(db, settings, i18n)
	conversation.SetNewConversationHook(onNewContactConversation(app))
	aiAgent.SetHooks(aiAgentHooks(app))
	go aiAgent.RunFollowups(ctx, aiFollowupInterval())
	go aiAgent.RunHandBacks(ctx, time.Minute, aiHandBackAfter())
	conversation.SetIncomingMessageHook(onIncomingMessage(app))
	app.dailyReport = initDailyReport(db, app)
	go app.dailyReport.Run(ctx, time.Minute, sendDailyReport(app))
	app.notifDispatcher = notifDispatcher
	go app.callback.RunOverdueChecker(ctx, time.Minute, onCallBackOverdue(app))
	// Inbound calls Exotel's flow didn't report are logged from its call list.
	if d := callReconcileInterval(); d > 0 {
		go runCallReconciler(ctx, app, d, callReconcileLookback())
	}
	app.export = initExport(db, app)
	go app.export.RunCleaner(ctx)
	go initRetention(db).Run(ctx, time.Hour, onRetentionApplied(app))
	if tenants.Enabled() {
		go runTenantDeletions(ctx, app)
		go tenants.RunDomainRechecks(ctx, cmp.Or(ko.Duration("app.tenancy.domain_recheck_interval"), 24*time.Hour))
	}
	i18n.SetTenantLang(func(tenantID int) string { return tenantLang(app, tenantID) })
	ai.SetRequestLimit(func(tenantID int) int { return planLimit(app, tenantID, "ai_requests_per_month") })

	g := fastglue.NewGlue()
	g.SetContext(app)
	initHandlers(g, wsHub)
	g.Router.NotFound = helpCenterHostNotFound(app, g)

	s := &fasthttp.Server{
		Name: appName,

		Handler: compressHandler(tenantHandler(app, g.Handler())),

		ReadTimeout:          ko.MustDuration("app.server.read_timeout"),
		WriteTimeout:         ko.MustDuration("app.server.write_timeout"),
		MaxRequestBodySize:   ko.MustInt("app.server.max_body_size"),
		MaxKeepaliveDuration: ko.MustDuration("app.server.keepalive_timeout"),
		ReadBufferSize:       ko.Int("app.server.read_buffer_size"),
	}

	go func() {
		colorlog.Green("Server started at %s", ko.String("app.server.address"))
		if ko.String("app.server.socket") != "" {
			colorlog.Green("Unix socket created at %s", ko.String("app.server.socket"))
		}
		if err := g.ListenAndServe(ko.String("app.server.address"), ko.String("app.server.socket"), s); err != nil {
			log.Fatalf("error starting server: %v", err)
		}
	}()

	// Start the app update checker.
	if url := ko.String("app.update_check_url"); ko.Bool("app.check_updates") && url != "" {
		go checkUpdates(url, versionString, time.Hour*1, app)
	}

	// Wait for shutdown signal.
	<-ctx.Done()
	colorlog.Red("Shutting down HTTP server...")
	s.Shutdown()
	colorlog.Red("Shutting down AI agent...")
	aiAgent.Close()
	colorlog.Red("Shutting down AI...")
	ai.Close()
	colorlog.Red("Shutting down inboxes...")
	inbox.Close()
	colorlog.Red("Shutting down automation...")
	automation.Close()
	colorlog.Red("Shutting down autoassigner...")
	autoassigner.Close()
	colorlog.Red("Shutting down notifier...")
	notifier.Close()
	mailer.Close()
	colorlog.Red("Shutting down webhook...")
	webhook.Close()
	colorlog.Red("Shutting down Slack and Teams...")
	chatIntegration.Close()
	colorlog.Red("Shutting down conversation...")
	conversation.Close()
	colorlog.Red("Shutting down SLA...")
	sla.Close()
	colorlog.Red("Shutting down importer...")
	app.importer.Close()
	colorlog.Red("Shutting down database...")
	db.Close()
	colorlog.Red("Shutting down redis...")
	rdb.Close()
	colorlog.Green("Shutdown complete.")
}

// onUsersOffline returns a callback for MonitorUserAvailability that broadcasts
// offline status to the appropriate clients based on user type.
func onUsersOffline(conv *conversation.Manager) func([]umodels.OfflineUser) {
	return func(users []umodels.OfflineUser) {
		for _, u := range users {
			switch u.Type {
			case umodels.UserTypeAgent:
				conv.BroadcastAgentAvailability(u.TenantID, u.ID, umodels.Offline)
			case umodels.UserTypeContact, umodels.UserTypeVisitor:
				conv.BroadcastContactUpdate(u.TenantID, u.ID, map[string]any{"availability_status": umodels.Offline})
			}
		}
	}
}
