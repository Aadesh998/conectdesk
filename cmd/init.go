package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"html/template"

	activitylog "github.com/conectdesk/conectdesk/internal/activity_log"
	"github.com/conectdesk/conectdesk/internal/ai"
	"github.com/conectdesk/conectdesk/internal/aiagent"
	auth_ "github.com/conectdesk/conectdesk/internal/auth"
	"github.com/conectdesk/conectdesk/internal/authz"
	"github.com/conectdesk/conectdesk/internal/autoassigner"
	"github.com/conectdesk/conectdesk/internal/automation"
	businesshours "github.com/conectdesk/conectdesk/internal/business_hours"
	"github.com/conectdesk/conectdesk/internal/callback"
	"github.com/conectdesk/conectdesk/internal/colorlog"
	contextlink "github.com/conectdesk/conectdesk/internal/context_link"
	"github.com/conectdesk/conectdesk/internal/conversation"
	"github.com/conectdesk/conectdesk/internal/conversation/priority"
	"github.com/conectdesk/conectdesk/internal/conversation/status"
	"github.com/conectdesk/conectdesk/internal/csat"
	customAttribute "github.com/conectdesk/conectdesk/internal/custom_attribute"
	"github.com/conectdesk/conectdesk/internal/export"
	"github.com/conectdesk/conectdesk/internal/helpcenter"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/importer"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/email"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/livechat"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/messenger"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/telegram"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/voice"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/whatsapp"
	imodels "github.com/conectdesk/conectdesk/internal/inbox/models"
	"github.com/conectdesk/conectdesk/internal/integration"
	"github.com/conectdesk/conectdesk/internal/integration/slack"
	"github.com/conectdesk/conectdesk/internal/integration/teams"
	"github.com/conectdesk/conectdesk/internal/lead"
	"github.com/conectdesk/conectdesk/internal/macro"
	"github.com/conectdesk/conectdesk/internal/media"
	fs "github.com/conectdesk/conectdesk/internal/media/stores/localfs"
	"github.com/conectdesk/conectdesk/internal/media/stores/s3"
	"github.com/conectdesk/conectdesk/internal/mfa"
	"github.com/conectdesk/conectdesk/internal/migrations"
	notifier "github.com/conectdesk/conectdesk/internal/notification"
	emailnotifier "github.com/conectdesk/conectdesk/internal/notification/providers/email"
	"github.com/conectdesk/conectdesk/internal/oidc"
	"github.com/conectdesk/conectdesk/internal/platform"
	pmodels "github.com/conectdesk/conectdesk/internal/platform/models"
	"github.com/conectdesk/conectdesk/internal/ratelimit"
	"github.com/conectdesk/conectdesk/internal/report"
	"github.com/conectdesk/conectdesk/internal/retention"
	"github.com/conectdesk/conectdesk/internal/role"
	"github.com/conectdesk/conectdesk/internal/search"
	"github.com/conectdesk/conectdesk/internal/setting"
	"github.com/conectdesk/conectdesk/internal/signup"
	"github.com/conectdesk/conectdesk/internal/sla"
	"github.com/conectdesk/conectdesk/internal/ssrf"
	"github.com/conectdesk/conectdesk/internal/tag"
	"github.com/conectdesk/conectdesk/internal/team"
	tmpl "github.com/conectdesk/conectdesk/internal/template"
	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/conectdesk/conectdesk/internal/user"
	"github.com/conectdesk/conectdesk/internal/view"
	"github.com/conectdesk/conectdesk/internal/webhook"
	"github.com/conectdesk/conectdesk/internal/ws"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/go-i18n"
	kjson "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	flag "github.com/spf13/pflag"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

// constants holds the app constants.
type constants struct {
	AppBaseURL                  string
	FaviconURL                  string
	LogoURL                     string
	SiteName                    string
	UploadProvider              string
	AllowedUploadFileExtensions []string
	MaxFileUploadSizeMB         int
	// Require2FA makes agents set up two-factor login before anything else.
	Require2FA bool
	// RequireSSO leaves password login to admins.
	RequireSSO bool
}

// Config loads config from files and environment variables into koanf.
func initConfig(ko *koanf.Koanf) {
	for _, f := range ko.Strings("config") {
		log.Println("reading config file:", f)
		if err := ko.Load(file.Provider(f), toml.Parser()); err != nil {
			if os.IsNotExist(err) {
				log.Printf("WARNING: Config file not found. Continuing with defaults and environment variables.")
				continue
			}
			log.Fatalf("error loading config from file: %v.", err)
		}
	}
	// Load environment variables: the legacy LIBREDESK_ prefix first, then
	// CONECTDESK_ over it.
	// The legacy prefix is still read so existing deployments keep working
	// until their variables are renamed.
	for _, prefix := range []string{legacyEnvPrefix, envPrefix} {
		prefix := prefix
		ko.Load(env.Provider(".", env.Opt{
			Prefix: prefix,
			TransformFunc: func(key, val string) (string, any) {
				key = strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(key, prefix)), "__", ".")
				return key, val
			},
		}), nil)
	}
}

const (
	// envPrefix prefixes the environment variables that configure the app.
	envPrefix = "CONECTDESK_"
	// legacyEnvPrefix is the prefix before the rename, still read.
	legacyEnvPrefix = "LIBREDESK_"
)

// getenv returns the environment variable name under envPrefix, else under
// the legacy prefix.
func getenv(name string) string {
	if v := os.Getenv(envPrefix + name); v != "" {
		return v
	}
	return os.Getenv(legacyEnvPrefix + name)
}

// validateConfig logs warnings/fatals for invalid config values.
func validateConfig(ko *koanf.Koanf) {
	encKey := ko.MustString("app.encryption_key")

	if len(encKey) != 32 {
		log.Fatalf("encryption_key must be exactly 32 characters, got %d", len(encKey))
	}

	// Warn if using sample config value.
	if encKey == sampleEncKey {
		colorlog.Red("WARNING: You are using the sample encryption_key from config.sample.toml. Change it immediately. Generate a secure key with `openssl rand -hex 16`")
	}

	if ko.Bool("app.tenancy.enabled") {
		if ko.String("app.tenancy.base_domain") == "" {
			log.Fatalf("app.tenancy.base_domain is required when app.tenancy.enabled is true")
		}
		colorlog.Green("Multi-tenancy is on: workspaces are served on *.%s", ko.String("app.tenancy.base_domain"))
	}
}

// initFlags initializes the commandline flags.
func initFlags() {
	f := flag.NewFlagSet("config", flag.ContinueOnError)

	// Registering `--help` handler.
	f.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		f.PrintDefaults()
	}

	// Register the commandline flags and parse them.
	f.StringSlice("config", []string{"config.toml"},
		"path to one or more config files (will be merged in order)")
	f.Bool("version", false, "show current version of the build")
	f.Bool("install", false, "setup database")
	f.Bool("idempotent-install", false, "run idempotent installation, i.e., skip installion if schema is already installed useful for the first time setup")
	f.Bool("yes", false, "skip confirmation prompt")
	f.Bool("upgrade", false, "upgrade the database schema")
	f.Bool("tenant-list", false, "list tenants")
	f.String("tenant-suspend", "", "suspend the tenant with this slug")
	f.String("tenant-activate", "", "activate the tenant with this slug")
	f.String("tenant-delete", "", "delete the tenant with this slug and all its data, at once")
	f.String("tenant-restore", "", "restore the tenant with this slug from its scheduled deletion")
	f.String("tenant-create", "", "create a workspace with this slug and its admin, without email verification; takes --tenant-name, --tenant-admin-name and --tenant-admin-email, and the password on stdin")
	f.String("tenant-name", "", "the name of the workspace --tenant-create creates")
	f.String("tenant-admin-name", "", "the full name of the admin --tenant-create creates")
	f.String("tenant-admin-email", "", "the email of the admin --tenant-create creates")
	f.String("seed-demo", "", "fill the workspace with this slug, which must have no conversations, with a coaching institute's demo data")
	f.String("platform-admin-add", "", "create a platform dashboard admin with this email, or reset their password")
	f.String("platform-admin-remove", "", "remove the platform dashboard admin with this email")
	f.Bool("platform-admin-list", false, "list platform dashboard admins")
	f.String("platform-admin-reset-2fa", "", "turn off two-factor login of the platform dashboard admin with this email, for a lost phone")
	f.Bool("set-system-user-password", false, "set password for the system user")
	f.Bool("copy-uploads-to-s3", false, "copy files from the fs upload path into the s3 bucket, skipping ones already there, when moving uploads to s3 or MinIO")
	f.String("static-dir", "", "path to a directory with custom static files and templates to override the defaults")

	if err := f.Parse(os.Args[1:]); err != nil {
		log.Fatalf("loading flags: %v", err)
	}

	if err := ko.Load(posflag.Provider(f, ".", ko), nil); err != nil {
		log.Fatalf("loading config: %v", err)
	}
}

// initConstants initializes the app constants.
func initConstants() *constants {
	return &constants{
		AppBaseURL:                  ko.String("app.root_url"),
		FaviconURL:                  ko.String("app.favicon_url"),
		LogoURL:                     ko.String("app.logo_url"),
		SiteName:                    ko.String("app.site_name"),
		UploadProvider:              ko.MustString("upload.provider"),
		AllowedUploadFileExtensions: ko.Strings("app.allowed_file_upload_extensions"),
		MaxFileUploadSizeMB:         ko.Int("app.max_file_upload_size"),
		Require2FA:                  ko.Bool("app.require_2fa"),
		RequireSSO:                  ko.Bool("app.require_sso"),
	}
}

// initFS initializes the stuffbin FileSystem. If staticDir is set, files from
// that directory are merged into the FS, overriding embedded defaults.
func initFS(staticDir string) stuffbin.FileSystem {
	// Custom static dir mirrors the structure of the built-in static/ directory.
	staticFiles := []string{
		"./:static",
	}

	// Get self executable path.
	path, err := os.Executable()
	if err != nil {
		log.Fatalf("error initializing FS: %v", err)
	}

	// Load embedded files in the executable.
	fs, err := stuffbin.UnStuff(path)

	if err != nil {
		if err == stuffbin.ErrNoID {
			// Running in local/dev mode, use the local filesystem.
			// Only include frontend dirs if they exist (frontend build is optional in dev).
			colorlog.Red("binary unstuff failed, using local filesystem for static files")
			files := []string{"i18n", "static", "schema"}
			for _, d := range []string{"frontend/dist/main", "frontend/dist/widget"} {
				if _, err := os.Stat(d); err == nil {
					files = append(files, d)
				}
			}
			fs, err = stuffbin.NewLocalFS("/", files...)
			if err != nil {
				log.Fatalf("error initializing local FS: %v", err)
			}
		} else {
			log.Fatalf("error initializing FS: %v", err)
		}
	}

	// Merge custom static files if a custom static dir is provided.
	if staticDir != "" {
		// Only include paths that exist in the custom dir.
		var sf []string
		for _, def := range staticFiles {
			src, _, _ := strings.Cut(def, ":")
			if _, err := os.Stat(filepath.Join(staticDir, src)); err == nil {
				sf = append(sf, def)
			}
		}

		if len(sf) > 0 {
			files := joinFSPaths(staticDir, sf)
			fLocal, err := stuffbin.NewLocalFS("/", files...)
			if err != nil {
				log.Fatalf("error loading custom static files from '%s': %v", staticDir, err)
			}
			if err := fs.Merge(fLocal); err != nil {
				log.Fatalf("error merging custom static files from '%s': %v", staticDir, err)
			}
		}
	}

	return fs
}

// joinFSPaths joins a root directory with stuffbin path specs (local:virtual).
func joinFSPaths(root string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		f := strings.Split(p, ":")
		out = append(out, filepath.Join(root, f[0])+":"+f[1])
	}
	return out
}

// loadSettings loads settings from the DB into Koanf map.
func loadSettings(m *setting.Manager) {
	j, err := m.GetAllJSON(tenant.DefaultID)
	if err != nil {
		log.Fatalf("error parsing settings from DB: %v", err)
	}

	// Setting keys are dot separated, eg: app.favicon_url. Unflatten them into
	// nested maps {app: {favicon_url}}.
	var out map[string]any

	if err := json.Unmarshal(j, &out); err != nil {
		log.Fatalf("error unmarshalling settings from DB: %v", err)
	}
	if err := ko.Load(confmap.Provider(out, "."), nil); err != nil {
		log.Fatalf("error parsing settings from DB: %v", err)
	}
}

// initSettings inits setting manager.
func initSettings(db *sqlx.DB) *setting.Manager {
	s, err := setting.New(setting.Opts{
		DB:            db,
		Lo:            initLogger("settings"),
		EncryptionKey: ko.MustString("app.encryption_key"),
	})
	if err != nil {
		log.Fatalf("error initializing setting manager: %v", err)
	}
	return s
}

// initUser inits user manager.
func initUser(i18n *i18ncat.Catalog, DB *sqlx.DB) *user.Manager {
	mgr, err := user.New(i18n, user.Opts{
		DB: DB,
		Lo: initLogger("user_manager"),
	})
	if err != nil {
		log.Fatalf("error initializing user manager: %v", err)
	}
	return mgr
}

// initConversations inits conversation manager.
func initConversations(
	i18n *i18ncat.Catalog,
	sla *sla.Manager,
	status *status.Manager,
	priority *priority.Manager,
	hub *ws.Hub,
	db *sqlx.DB,
	inboxStore *inbox.Manager,
	userStore *user.Manager,
	teamStore *team.Manager,
	mediaStore *media.Manager,
	settings *setting.Manager,
	csat *csat.Manager,
	automationEngine *automation.Engine,
	template *tmpl.Manager,
	webhook *webhook.Manager,
	dispatcher *notifier.Dispatcher,
) *conversation.Manager {
	continuityConfig := &conversation.ContinuityConfig{}
	if ko.Exists("conversation.continuity_scan_interval") {
		continuityConfig.BatchCheckInterval = ko.MustDuration("conversation.continuity_scan_interval")
	}

	c, err := conversation.New(hub, i18n, sla, status, priority, inboxStore, userStore, teamStore, mediaStore, settings, csat, automationEngine, template, webhook, dispatcher, conversation.Opts{
		DB:                         db,
		Lo:                         initLogger("conversation_manager"),
		OutgoingMessageQueueSize:   ko.MustInt("message.outgoing_queue_size"),
		IncomingMessageQueueSize:   ko.MustInt("message.incoming_queue_size"),
		IncomingMaxQueuedPerTenant: tenantQueueShare(ko.MustInt("message.incoming_queue_size")),
		ContinuityConfig:           continuityConfig,
		SubjectRefFormat:           ko.String("conversation.subject_ref_format"),
	})
	if err != nil {
		log.Fatalf("error initializing conversation manager: %v", err)
	}
	return c
}

// initTag inits tag manager.
func initTag(db *sqlx.DB, i18n *i18ncat.Catalog) *tag.Manager {
	var lo = initLogger("tag_manager")
	mgr, err := tag.New(tag.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing tags: %v", err)
	}
	return mgr
}

// initSignup inits workspace creation, nil without multi-tenancy. Platform
// admins create workspaces with it; publicSignup says whether anyone can too.
func initSignup(db *sqlx.DB, tenants *tenant.Manager) *signup.Manager {
	if !tenants.Enabled() {
		return nil
	}
	return signup.New(signup.Opts{
		DB:      db,
		Lo:      initLogger("signup"),
		Tenants: tenants,
		Plan:    cmp.Or(ko.String("app.tenancy.default_plan"), "free"),
	})
}

// tenantQueueShare returns how many items of a shared queue of queueSize one
// tenant may hold: app.tenancy.queue_share_percent of it, 10% by default. It is
// 0, no cap, without multi-tenancy.
func tenantQueueShare(queueSize int) int {
	if !ko.Bool("app.tenancy.enabled") {
		return 0
	}
	pct := ko.Int("app.tenancy.queue_share_percent")
	if pct <= 0 || pct > 100 {
		pct = 10
	}
	return max(queueSize*pct/100, 1)
}

// initExport inits workspace data exports.
func initExport(db *sqlx.DB, app *App) *export.Manager {
	m, err := export.New(export.Opts{
		DB:            db,
		Lo:            initLogger("export"),
		Files:         exportFiles{app: app},
		Tables:        migrations.AllTenantTables(),
		MaxFilesBytes: exportMaxFilesBytes(),
	})
	if err != nil {
		log.Fatalf("error initializing data export: %v", err)
	}
	return m
}

// initRetention inits workspaces' data retention.
func initRetention(db *sqlx.DB) *retention.Manager {
	m, err := retention.New(retention.Opts{DB: db, Lo: initLogger("retention")})
	if err != nil {
		log.Fatalf("error initializing data retention: %v", err)
	}
	return m
}

// initMFA inits two-factor login.
func initMFA(db *sqlx.DB) *mfa.Manager {
	m, err := mfa.New(mfa.Opts{DB: db, Lo: initLogger("mfa"), EncryptionKey: ko.MustString("app.encryption_key")})
	if err != nil {
		log.Fatalf("error initializing two-factor login: %v", err)
	}
	return m
}

// initPlatform inits the platform admin dashboard and returns the host it is
// served on, app.tenancy.platform_host or admin.<base_domain>. It is off
// without multi-tenancy.
func initPlatform(db *sqlx.DB, tenants *tenant.Manager) (*platform.Manager, string) {
	if !tenants.Enabled() || tenants.BaseDomain() == "" {
		return nil, ""
	}
	m, err := platform.New(platform.Opts{DB: db, Lo: initLogger("platform"), EncryptionKey: ko.MustString("app.encryption_key")})
	if err != nil {
		log.Fatalf("error initializing platform dashboard: %v", err)
	}
	host := strings.ToLower(cmp.Or(ko.String("app.tenancy.platform_host"), "admin."+tenants.BaseDomain()))
	return m, host
}

// initTenant inits the tenant manager.
func initTenant(db *sqlx.DB) *tenant.Manager {
	mgr, err := tenant.New(tenant.Opts{
		DB:         db,
		Lo:         initLogger("tenant_manager"),
		Enabled:    ko.Bool("app.tenancy.enabled"),
		BaseDomain: ko.String("app.tenancy.base_domain"),
		URLScheme:  ko.String("app.tenancy.url_scheme"),
		DNSServer:  ko.String("app.tenancy.dns_server"),
	})
	if err != nil {
		log.Fatalf("error initializing tenants: %v", err)
	}
	return mgr
}

// initViews inits view manager.
func initView(db *sqlx.DB, i18n *i18ncat.Catalog) *view.Manager {
	var lo = initLogger("view_manager")
	m, err := view.New(view.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing view manager: %v", err)
	}
	return m
}

// initMacro inits macro manager.
func initMacro(db *sqlx.DB, i18n *i18ncat.Catalog) *macro.Manager {
	var lo = initLogger("macro")
	m, err := macro.New(macro.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing macro manager: %v", err)
	}
	return m
}

// initBusinessHours inits business hours manager.
func initBusinessHours(db *sqlx.DB, i18n *i18ncat.Catalog) *businesshours.Manager {
	var lo = initLogger("business-hours")
	m, err := businesshours.New(businesshours.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing business hours manager: %v", err)
	}
	return m
}

// initSLA inits SLA manager.
func initSLA(db *sqlx.DB, teamManager *team.Manager, settings *setting.Manager, businessHours *businesshours.Manager, template *tmpl.Manager, userManager *user.Manager, i18n *i18ncat.Catalog, dispatcher *notifier.Dispatcher) *sla.Manager {
	var lo = initLogger("sla")
	m, err := sla.New(sla.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	}, teamManager, settings, businessHours, template, userManager, dispatcher)
	if err != nil {
		log.Fatalf("error initializing SLA manager: %v", err)
	}
	return m
}

// initCSAT inits CSAT manager.
func initCSAT(db *sqlx.DB, i18n *i18ncat.Catalog) *csat.Manager {
	var lo = initLogger("csat")
	m, err := csat.New(csat.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing CSAT manager: %v", err)
	}
	return m
}

// initWS inits websocket hub.
func initWS(user *user.Manager) *ws.Hub {
	return ws.NewHub(initLogger("ws"), user)
}

// getCustomStaticDir returns the custom static directory path from CLI flag or config.
func getCustomStaticDir() string {
	dir := ko.String("static-dir")
	if dir == "" {
		dir = ko.String("app.static_dir")
	}
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}

// initTemplates inits template manager.
func initTemplate(db *sqlx.DB, fs stuffbin.FileSystem, consts *constants, i18n *i18ncat.Catalog) *tmpl.Manager {
	var (
		lo      = initLogger("template")
		funcMap = getTmplFuncs(consts, i18n, fs)
	)
	tpls, err := stuffbin.ParseTemplatesGlob(funcMap, fs, "/static/email-templates/*.html")
	if err != nil {
		log.Fatalf("error parsing e-mail templates: %v", err)
	}
	webTpls, err := parseWebTemplates(funcMap, fs)
	if err != nil {
		log.Fatalf("error parsing web templates: %v", err)
	}

	m, err := tmpl.New(lo, db, webTpls, tpls, funcMap, i18n)
	if err != nil {
		log.Fatalf("error initializing template manager: %v", err)
	}
	return m
}

// getTmplFuncs returns the template functions.
func getTmplFuncs(consts *constants, i18n *i18ncat.Catalog, fs stuffbin.FileSystem) template.FuncMap {
	lucideIcons := loadLucideIcons(fs)
	return template.FuncMap{
		"LucideIcon": func(name string) template.HTML {
			return lucideIcons[name]
		},
		"RootURL": func() string {
			return consts.AppBaseURL
		},
		"FaviconURL": func() string {
			return consts.FaviconURL
		},
		"AssetVer": func() string {
			return assetVersion
		},
		"Date": func(layout string) string {
			if layout == "" {
				layout = time.ANSIC
			}
			return time.Now().Format(layout)
		},
		"LogoURL": func() string {
			return consts.LogoURL
		},
		"SiteName": func() string {
			return consts.SiteName
		},
		"L": func() any {
			return i18n
		},
		"map": func(pairs ...any) (map[string]any, error) {
			if len(pairs)%2 != 0 {
				return nil, fmt.Errorf("map: odd number of arguments")
			}
			out := make(map[string]any, len(pairs)/2)
			for i := 0; i < len(pairs); i += 2 {
				key, ok := pairs[i].(string)
				if !ok {
					return nil, fmt.Errorf("map: key %v is not a string", pairs[i])
				}
				out[key] = pairs[i+1]
			}
			return out, nil
		},
	}
}

// reloadSettings reloads the settings from the database into the Koanf instance.
func reloadSettings(app *App) error {
	app.lo.Info("reloading settings")
	j, err := app.setting.GetAllJSON(tenant.DefaultID)
	if err != nil {
		app.lo.Error("error parsing settings from DB", "error", err)
		return err
	}
	var out map[string]any
	if err := json.Unmarshal(j, &out); err != nil {
		app.lo.Error("error unmarshalling settings from DB", "error", err)
		return err
	}
	app.Lock()
	err = ko.Load(confmap.Provider(out, "."), nil)
	app.Unlock()
	if err != nil {
		app.lo.Error("error loading settings into koanf", "error", err)
		return err
	}
	newConsts := initConstants()
	app.consts.Store(newConsts)
	return nil
}

// reloadTemplates reloads the templates from the filesystem.
func reloadTemplates(app *App) error {
	app.lo.Info("reloading templates")
	funcMap := getTmplFuncs(app.consts.Load().(*constants), app.i18n, app.fs)
	tpls, err := stuffbin.ParseTemplatesGlob(funcMap, app.fs, "/static/email-templates/*.html")
	if err != nil {
		app.lo.Error("error parsing email templates", "error", err)
		return err
	}
	webTpls, err := parseWebTemplates(funcMap, app.fs)
	if err != nil {
		app.lo.Error("error parsing web templates", "error", err)
		return err
	}

	return app.tmpl.Reload(webTpls, tpls, funcMap)
}

// parseWebTemplates parses the top-level web templates and the per-template help center pages.
func parseWebTemplates(funcMap template.FuncMap, fs stuffbin.FileSystem) (*template.Template, error) {
	var paths []string
	for _, pattern := range []string{
		"/static/public/web-templates/*.html",
		"/static/public/web-templates/help/*/*.html",
	} {
		p, err := fs.Glob(pattern)
		if err != nil {
			return nil, err
		}
		paths = append(paths, p...)
	}
	return stuffbin.ParseTemplates(funcMap, fs, paths...)
}

// initTeam inits team manager.
func initTeam(db *sqlx.DB, i18n *i18ncat.Catalog) *team.Manager {
	var lo = initLogger("team-manager")
	mgr, err := team.New(team.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing team manager: %v", err)
	}
	return mgr
}

// initMedia inits media manager.
func initMedia(db *sqlx.DB, i18n *i18ncat.Catalog, rootURL func(tenantID int) string) *media.Manager {
	var (
		store media.Store
		err   error
		lo    = initLogger("media")
	)
	switch s := ko.MustString("upload.provider"); s {
	case "s3":
		store, err = s3.New(s3.Opt{
			URL:        ko.String("upload.s3.url"),
			PublicURL:  ko.String("upload.s3.public_url"),
			PresignURL: ko.String("upload.s3.presign_url"),
			AccessKey:  ko.String("upload.s3.access_key"),
			SecretKey:  ko.String("upload.s3.secret_key"),
			Region:     ko.String("upload.s3.region"),
			Bucket:     ko.String("upload.s3.bucket"),
			BucketPath: ko.String("upload.s3.bucket_path"),
			// All files are private by default.
			BucketType: "private",
			Expiry:     ko.Duration("upload.s3.expiry"),
		})
		if err != nil {
			log.Fatalf("error initializing s3 media store: %v", err)
		}
		// A new MinIO starts without the bucket.
		if err := store.(*s3.Client).EnsureBucket(); err != nil {
			lo.Error("error checking s3 bucket, uploads may fail", "bucket", ko.String("upload.s3.bucket"), "error", err)
		}
	case "fs":
		// Default expiry to 1h if not set.
		fsExpiry := ko.Duration("upload.fs.expiry")
		if fsExpiry == 0 {
			fsExpiry = 1 * time.Hour
		}
		store, err = fs.New(fs.Opts{
			UploadURI:  media.PublicURI,
			UploadPath: filepath.Clean(ko.String("upload.fs.upload_path")),
			RootURL:    rootURL,
			SigningKey: ko.MustString("app.encryption_key"),
			Expiry:     fsExpiry,
		})
		if err != nil {
			log.Fatalf("error initializing fs media store: %v", err)
		}
	default:
		log.Fatalf("unknown media store: %s", s)
	}

	media, err := media.New(media.Opts{
		Store:   store,
		Lo:      lo,
		DB:      db,
		I18n:    i18n,
		RootURL: rootURL,
	})
	if err != nil {
		log.Fatalf("error initializing media: %v", err)
	}
	return media
}

// initInbox initializes the inbox manager without registering inboxes.
func initInbox(db *sqlx.DB, i18n *i18ncat.Catalog) *inbox.Manager {
	// Inboxes connected through the installation's own Google or Microsoft
	// app refresh their tokens with its credentials from config.
	email.SetPlatformOAuthApp(platformOAuthApp)

	var lo = initLogger("inbox-manager")
	mgr, err := inbox.New(lo, db, i18n, ko.MustString("app.encryption_key"))
	if err != nil {
		log.Fatalf("error initializing inbox manager: %v", err)
	}
	return mgr
}

// initAutomationEngine initializes the automation engine.
func initAutomationEngine(db *sqlx.DB, i18n *i18ncat.Catalog) *automation.Engine {
	var lo = initLogger("automation_engine")
	engine, err := automation.New(automation.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,

		MaxQueuedPerTenant: tenantQueueShare(automation.MaxQueueSize),
	})
	if err != nil {
		log.Fatalf("error initializing automation engine: %v", err)
	}
	return engine
}

// initAutoAssigner initializes the auto assigner.
func initAutoAssigner(teamManager *team.Manager, userManager *user.Manager, conversationManager *conversation.Manager, tenants *tenant.Manager) *autoassigner.Engine {
	e, err := autoassigner.New(teamManager, conversationManager, tenants.ActiveIDs, userManager.GetSystemUser, initLogger("autoassigner"))
	if err != nil {
		log.Fatalf("error initializing auto assigner: %v", err)
	}
	return e
}

// initNotifier initializes the notifier service with available providers.
// initNotifier initializes the notifier service. Email goes out through each
// tenant's own mailer, or the default tenant's for tenants without one.
func initNotifier(mailer *emailnotifier.TenantRouter) *notifier.Service {
	notifierProviders := map[string]notifier.Notifier{
		mailer.Name(): mailer,
	}
	svc := notifier.NewService(notifierProviders, ko.MustInt("notification.concurrency"), ko.MustInt("notification.queue_size"), initLogger("notifier"))
	svc.SetMaxQueuedPerTenant(tenantQueueShare(ko.MustInt("notification.queue_size")))
	return svc
}

// initMailer initializes the router that picks each tenant's notification mailer.
func initMailer(settings *setting.Manager, tenants *tenant.Manager) *emailnotifier.TenantRouter {
	return emailnotifier.NewTenantRouter(emailnotifier.TenantRouterOpts{
		Lo: initLogger("email-notifier"),
		// Without multi-tenancy the installation's mailer is the default
		// tenant's; with it, the platform dashboard sets it (see initPlatform).
		PlatformSettings: func() (emailnotifier.Settings, error) {
			return mailerSettings(settings, tenant.DefaultID)
		},
		Settings: func(tenantID int) (emailnotifier.Settings, error) {
			return mailerSettings(settings, tenantID)
		},
		TenantName: func(tenantID int) string {
			t, err := tenants.GetCached(tenantID)
			if err != nil {
				return ""
			}
			return t.Name
		},
	})
}

// platformMailerSettings turns the platform dashboard's mail settings into the
// platform mailer's.
func platformMailerSettings(ms pmodels.MailSettings) emailnotifier.Settings {
	return emailnotifier.Settings{
		Enabled: ms.Enabled && ms.Host != "",
		From:    ms.From,
		SMTP: imodels.SMTPConfig{
			Host:              ms.Host,
			Port:              ms.Port,
			Username:          ms.Username,
			Password:          ms.Password,
			AuthProtocol:      cmp.Or(ms.AuthProtocol, "login"),
			TLSType:           cmp.Or(ms.TLSType, "starttls"),
			TLSSkipVerify:     ms.TLSSkipVerify,
			HelloHostname:     ms.HelloHostname,
			MaxConns:          cmp.Or(ms.MaxConns, 5),
			MaxMessageRetries: 2,
			IdleTimeout:       cmp.Or(ms.IdleTimeout, "25s"),
			PoolWaitTimeout:   cmp.Or(ms.WaitTimeout, "60s"),
		},
	}
}

// mailerSettings reads a tenant's notification.email.* settings.
func mailerSettings(settings *setting.Manager, tenantID int) (emailnotifier.Settings, error) {
	b, err := settings.GetByPrefix(tenantID, "notification.email")
	if err != nil {
		return emailnotifier.Settings{}, err
	}
	var flat map[string]json.RawMessage
	if err := json.Unmarshal(b, &flat); err != nil {
		return emailnotifier.Settings{}, err
	}
	// The keys are flat ("notification.email.host"); SMTPConfig wants them bare.
	bare := make(map[string]json.RawMessage, len(flat))
	for k, v := range flat {
		bare[strings.TrimPrefix(k, "notification.email.")] = v
	}
	var s struct {
		Enabled bool   `json:"enabled"`
		From    string `json:"email_address"`
		imodels.SMTPConfig
	}
	nb, _ := json.Marshal(bare)
	if err := json.Unmarshal(nb, &s); err != nil {
		return emailnotifier.Settings{}, err
	}
	return emailnotifier.Settings{Enabled: s.Enabled, From: s.From, SMTP: s.SMTPConfig}, nil
}

// initEmailInbox loads inbox config from DB and initializes the email inbox.
func initEmailInbox(inboxRecord imodels.Inbox, msgStore inbox.MessageStore, usrStore inbox.UserStore, mgr *inbox.Manager) (inbox.Inbox, error) {
	var config imodels.Config

	// Load JSON data into Koanf.
	if err := ko.Load(rawbytes.Provider([]byte(inboxRecord.Config)), kjson.Parser()); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	if err := ko.UnmarshalWithConf("", &config, koanf.UnmarshalConf{Tag: "json"}); err != nil {
		return nil, fmt.Errorf("unmarshalling `%s` %s config: %w", inboxRecord.Channel, inboxRecord.Name, err)
	}

	if len(config.SMTP) == 0 {
		log.Printf("WARNING: Zero SMTP servers configured for `%s` inbox: Name: `%s`", inboxRecord.Channel, inboxRecord.Name)
	}

	if len(config.IMAP) == 0 {
		log.Printf("WARNING: Zero IMAP clients configured for `%s` inbox: Name: `%s`", inboxRecord.Channel, inboxRecord.Name)
	}

	config.From = inboxRecord.From
	config.FromNameTemplate = inboxRecord.FromNameTemplate

	if len(config.From) == 0 {
		log.Printf("WARNING: No `from` email address set for `%s` inbox: Name: `%s`", inboxRecord.Channel, inboxRecord.Name)
	}

	// Callback to persist refreshed tokens in DB.
	tokenRefreshCallback := func(inboxID int, updatedConfig imodels.Config) error {
		// Marshal updated config to JSON
		updatedConfigJSON, err := json.Marshal(updatedConfig)
		if err != nil {
			log.Printf("ERROR: Failed to marshal updated config during token refresh for inbox %d: %v", inboxID, err)
			return err
		}

		// Persist updated config to DB
		if err := mgr.UpdateConfig(inboxRecord.TenantID, inboxID, updatedConfigJSON); err != nil {
			log.Printf("ERROR: Failed to persist refreshed tokens during operation for inbox %d: %v", inboxID, err)
			return err
		}

		log.Printf("INFO: Successfully persisted refreshed tokens during operation for inbox: %d", inboxID)
		return nil
	}

	inbox, err := email.New(msgStore, usrStore, email.Opts{
		ID:                   inboxRecord.ID,
		TenantID:             inboxRecord.TenantID,
		Name:                 inboxRecord.Name,
		Config:               config,
		Lo:                   initLogger("email_inbox"),
		TokenRefreshCallback: tokenRefreshCallback,
	})

	if err != nil {
		return nil, fmt.Errorf("initializing `%s` inbox: `%s` error : %w", inboxRecord.Channel, inboxRecord.Name, err)
	}

	log.Printf("`%s` inbox successfully initialized", inboxRecord.Name)

	return inbox, nil
}

// initLiveChatInbox initializes the live chat inbox.
func initLiveChatInbox(inboxRecord imodels.Inbox, msgStore inbox.MessageStore, usrStore inbox.UserStore, signAvatarURL func(int, *null.String)) (inbox.Inbox, error) {
	var config livechat.Config

	// Load JSON data into Koanf.
	if err := ko.Load(rawbytes.Provider([]byte(inboxRecord.Config)), kjson.Parser()); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	if err := ko.UnmarshalWithConf("", &config, koanf.UnmarshalConf{Tag: "json"}); err != nil {
		return nil, fmt.Errorf("unmarshalling `%s` %s config: %w", inboxRecord.Channel, inboxRecord.Name, err)
	}

	inbox, err := livechat.New(msgStore, usrStore, livechat.Opts{
		ID:            inboxRecord.ID,
		TenantID:      inboxRecord.TenantID,
		Name:          inboxRecord.Name,
		Config:        config,
		Lo:            initLogger("livechat_inbox"),
		SignAvatarURL: func(u *null.String) { signAvatarURL(inboxRecord.TenantID, u) },
	})

	if err != nil {
		return nil, fmt.Errorf("initializing `%s` inbox: `%s` error : %w", inboxRecord.Channel, inboxRecord.Name, err)
	}

	log.Printf("`%s` inbox successfully initialized", inboxRecord.Name)

	return inbox, nil
}

// initWhatsAppInbox initializes a WhatsApp Cloud API inbox.
func initWhatsAppInbox(inboxRecord imodels.Inbox, msgStore inbox.MessageStore, usrStore inbox.UserStore) (inbox.Inbox, error) {
	// Decoded on its own rather than through the global koanf instance, which
	// holds the app's config.
	var config whatsapp.Config
	if err := json.Unmarshal(inboxRecord.Config, &config); err != nil {
		return nil, fmt.Errorf("unmarshalling `%s` %s config: %w", inboxRecord.Channel, inboxRecord.Name, err)
	}

	inb, err := whatsapp.New(msgStore, usrStore, whatsapp.Opts{
		ID:       inboxRecord.ID,
		TenantID: inboxRecord.TenantID,
		Name:     inboxRecord.Name,
		From:     inboxRecord.From,
		Config:   config,
		Graph:    whatsAppGraph(config),
		Lo:       initLogger("whatsapp_inbox"),
	})
	if err != nil {
		return nil, fmt.Errorf("initializing `%s` inbox: `%s` error : %w", inboxRecord.Channel, inboxRecord.Name, err)
	}

	log.Printf("`%s` inbox successfully initialized", inboxRecord.Name)

	return inb, nil
}

// initMessengerInbox initializes a Messenger or Instagram inbox.
func initMessengerInbox(inboxRecord imodels.Inbox, msgStore inbox.MessageStore, usrStore inbox.UserStore) (inbox.Inbox, error) {
	var config messenger.Config
	if err := json.Unmarshal(inboxRecord.Config, &config); err != nil {
		return nil, fmt.Errorf("unmarshalling `%s` %s config: %w", inboxRecord.Channel, inboxRecord.Name, err)
	}

	inb, err := messenger.New(msgStore, usrStore, messenger.Opts{
		ID:       inboxRecord.ID,
		TenantID: inboxRecord.TenantID,
		Channel:  inboxRecord.Channel,
		Name:     inboxRecord.Name,
		From:     inboxRecord.From,
		Config:   config,
		Graph:    metaGraph(),
		Lo:       initLogger(inboxRecord.Channel + "_inbox"),
	})
	if err != nil {
		return nil, fmt.Errorf("initializing `%s` inbox: `%s` error : %w", inboxRecord.Channel, inboxRecord.Name, err)
	}

	log.Printf("`%s` inbox successfully initialized", inboxRecord.Name)
	return inb, nil
}

// initVoiceInbox initializes a phone inbox.
func initVoiceInbox(inboxRecord imodels.Inbox) (inbox.Inbox, error) {
	var config voice.Config
	if err := json.Unmarshal(inboxRecord.Config, &config); err != nil {
		return nil, fmt.Errorf("unmarshalling `%s` %s config: %w", inboxRecord.Channel, inboxRecord.Name, err)
	}
	inb, err := voice.New(voice.Opts{
		ID: inboxRecord.ID, TenantID: inboxRecord.TenantID,
		Name: inboxRecord.Name, From: inboxRecord.From,
		Config: config, Lo: initLogger("voice_inbox"),
	})
	if err != nil {
		return nil, fmt.Errorf("initializing `%s` inbox: `%s` error : %w", inboxRecord.Channel, inboxRecord.Name, err)
	}
	log.Printf("`%s` inbox successfully initialized", inboxRecord.Name)
	return inb, nil
}

// initTelegramInbox initializes a Telegram bot inbox.
func initTelegramInbox(inboxRecord imodels.Inbox) (inbox.Inbox, error) {
	var config telegram.Config
	if err := json.Unmarshal(inboxRecord.Config, &config); err != nil {
		return nil, fmt.Errorf("unmarshalling `%s` %s config: %w", inboxRecord.Channel, inboxRecord.Name, err)
	}
	inb, err := telegram.New(telegram.Opts{
		ID:       inboxRecord.ID,
		TenantID: inboxRecord.TenantID,
		Name:     inboxRecord.Name,
		From:     inboxRecord.From,
		Config:   config,
		Lo:       initLogger("telegram_inbox"),
	})
	if err != nil {
		return nil, fmt.Errorf("initializing `%s` inbox: `%s` error : %w", inboxRecord.Channel, inboxRecord.Name, err)
	}
	log.Printf("`%s` inbox successfully initialized", inboxRecord.Name)
	return inb, nil
}

// makeInboxInitializer creates an inbox initializer function.
func makeInboxInitializer(mgr *inbox.Manager, signAvatarURL func(int, *null.String)) func(imodels.Inbox, inbox.MessageStore, inbox.UserStore) (inbox.Inbox, error) {
	return func(inboxR imodels.Inbox, msgStore inbox.MessageStore, usrStore inbox.UserStore) (inbox.Inbox, error) {
		switch inboxR.Channel {
		case inbox.ChannelEmail:
			return initEmailInbox(inboxR, msgStore, usrStore, mgr)
		case inbox.ChannelLiveChat:
			return initLiveChatInbox(inboxR, msgStore, usrStore, signAvatarURL)
		case inbox.ChannelWhatsApp:
			return initWhatsAppInbox(inboxR, msgStore, usrStore)
		case inbox.ChannelMessenger, inbox.ChannelInstagram:
			return initMessengerInbox(inboxR, msgStore, usrStore)
		case inbox.ChannelTelegram:
			return initTelegramInbox(inboxR)
		case inbox.ChannelVoice:
			return initVoiceInbox(inboxR)
		default:
			return nil, fmt.Errorf("unknown inbox channel: %s", inboxR.Channel)
		}
	}
}

// reloadInbox reloads a single inbox by ID using the signal-aware context.
func reloadInbox(app *App, tenantID, id int) error {
	app.lo.Info("reloading inbox", "id", id)
	return app.inbox.ReloadInbox(app.ctx, tenantID, id, makeInboxInitializer(app.inbox, app.conversation.SignAvatarURL))
}

// startInboxes registers the active inboxes and starts receiver for each.
func startInboxes(ctx context.Context, mgr *inbox.Manager, msgStore inbox.MessageStore, usrStore inbox.UserStore, signAvatarURL func(int, *null.String)) {
	mgr.SetMessageStore(msgStore)
	mgr.SetUserStore(usrStore)

	if err := mgr.InitInboxes(makeInboxInitializer(mgr, signAvatarURL)); err != nil {
		log.Fatalf("error initializing inboxes: %v", err)
	}

	if err := mgr.Start(ctx); err != nil {
		log.Fatalf("error starting inboxes: %v", err)
	}
}

// initAuthz initializes authorization enforcer.
func initAuthz(i18n *i18ncat.Catalog) *authz.Enforcer {
	enforcer, err := authz.NewEnforcer(initLogger("authz"), i18n)
	if err != nil {
		log.Fatalf("error initializing authz: %v", err)
	}
	return enforcer
}

// initSSRFControl builds the shared outbound-request guard from config.
func initSSRFControl() ssrf.Control {
	lo := initLogger("ssrf")
	return ssrf.NewControl(ko.Bool("ssrf.enabled"), ko.Strings("ssrf.allowed_cidrs"), lo)
}

// initAuth initializes the authentication manager.
func initAuth(o *oidc.Manager, rd *redis.Client, i18n *i18ncat.Catalog, dialControl ssrf.Control) *auth_.Auth {
	lo := initLogger("auth")

	providers, err := buildProviders(o)
	if err != nil {
		log.Fatalf("error initializing auth: %v", err)
	}

	secure := !ko.Bool("app.server.disable_secure_cookies")
	sessionLifetime := ko.Duration("app.server.session_lifetime")
	auth, err := auth_.New(auth_.Config{Providers: providers, SecureCookies: secure, SessionLifetime: sessionLifetime}, i18n, rd, lo, dialControl)
	if err != nil {
		log.Fatalf("error initializing auth: %v", err)
	}

	return auth
}

// reloadAuth reloads the auth providers.
func reloadAuth(app *App) error {
	app.lo.Info("reloading auth manager")
	providers, err := buildProviders(app.oidc)
	if err != nil {
		log.Fatalf("error reloading auth: %v", err)
	}
	if err := app.auth.Reload(auth_.Config{Providers: providers}); err != nil {
		app.lo.Error("error reloading auth", "error", err)
		return err
	}
	return nil
}

// buildProviders creates a list of auth providers from the OIDC manager.
// Providers of every tenant are loaded; handlers check a provider belongs to
// the request's tenant before using it.
func buildProviders(o *oidc.Manager) ([]auth_.Provider, error) {
	oidcConfigs, err := o.GetAllEnabled()
	if err != nil {
		return nil, err
	}

	providers := make([]auth_.Provider, 0, len(oidcConfigs))
	for _, config := range oidcConfigs {
		if !config.Enabled {
			continue
		}
		providers = append(providers, auth_.Provider{
			ID:           config.ID,
			Provider:     config.Provider,
			ProviderURL:  config.ProviderURL,
			RedirectURL:  config.RedirectURI,
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
		})
	}
	return providers, nil
}

// initOIDC initializes open id connect config manager.
func initOIDC(db *sqlx.DB, rootURL func(tenantID int) string, i18n *i18ncat.Catalog) *oidc.Manager {
	lo := initLogger("oidc")
	o, err := oidc.New(oidc.Opts{
		DB:            db,
		Lo:            lo,
		I18n:          i18n,
		EncryptionKey: ko.MustString("app.encryption_key"),
	}, rootURL)
	if err != nil {
		log.Fatalf("error initializing oidc: %v", err)
	}
	return o
}

// initI18n inits the catalog of language packs. Without a tenant it serves the
// installation's language, app.lang; each tenant gets its own app.lang once
// SetTenantLang is set.
func initI18n(fs stuffbin.FileSystem) *i18ncat.Catalog {
	lang := cmp.Or(ko.String("app.lang"), defLang)
	log.Printf("loading i18n language file: %s", lang)
	if _, err := fs.Read("/i18n/" + lang + ".json"); err != nil {
		log.Fatalf("error reading i18n language file `%s` : %v", lang, err)
	}
	cat := i18ncat.New(
		func(lang string) (*i18n.I18n, error) { return loadI18nLang(lang, fs) },
		func() string { return cmp.Or(ko.String("app.lang"), defLang) },
	)
	if cat.Default() == nil {
		log.Fatalf("error initializing i18n for `%s`", lang)
	}
	return cat
}

// initRedis inits redis DB.
func initRedis() *redis.Client {
	// Load options from redis URL if set.
	redisURL := ko.String("redis.url")
	if redisURL != "" {
		options, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Fatalf("error parsing redis url: %v", err)
		}
		return redis.NewClient(options)
	}
	// Load from individual config options.
	return redis.NewClient(&redis.Options{
		Addr:     ko.MustString("redis.address"),
		Username: ko.String("redis.user"),
		Password: ko.String("redis.password"),
		DB:       ko.Int("redis.db"),
	})
}

// initRedis inits postgres DB.
func initDB() *sqlx.DB {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s %s",
		ko.MustString("db.host"),
		ko.MustInt("db.port"),
		ko.MustString("db.user"),
		ko.MustString("db.password"),
		ko.MustString("db.database"),
		ko.String("db.ssl_mode"),
		ko.String("db.params"),
	)

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		log.Fatalf("error connecting to DB: %v", err)
	}

	db.SetMaxOpenConns(ko.MustInt("db.max_open"))
	db.SetMaxIdleConns(ko.MustInt("db.max_idle"))
	db.SetConnMaxLifetime(ko.MustDuration("db.max_lifetime"))

	return db
}

// initRedis inits role manager.
func initRole(db *sqlx.DB, i18n *i18ncat.Catalog) *role.Manager {
	var lo = initLogger("role_manager")
	r, err := role.New(role.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing role manager: %v", err)
	}
	return r
}

// initStatus inits conversation status manager.
func initStatus(db *sqlx.DB, i18n *i18ncat.Catalog) *status.Manager {
	manager, err := status.New(status.Opts{
		DB:   db,
		Lo:   initLogger("status-manager"),
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing status manager: %v", err)
	}
	return manager
}

// initPriority inits conversation priority manager.
func initPriority(db *sqlx.DB, i18n *i18ncat.Catalog) *priority.Manager {
	manager, err := priority.New(priority.Opts{
		DB:   db,
		Lo:   initLogger("priority-manager"),
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing priority manager: %v", err)
	}
	return manager
}

// initAI inits AI manager.
func initAI(ctx context.Context, db *sqlx.DB, i18n *i18ncat.Catalog, dialControl ssrf.Control, tenants *tenant.Manager) *ai.Manager {
	lo := initLogger("ai")
	m, err := ai.New(ai.Opts{
		Ctx:           ctx,
		DB:            db,
		Lo:            lo,
		I18n:          i18n,
		EncryptionKey: ko.MustString("app.encryption_key"),
		DialControl:   dialControl,
		TenantIDs:     tenants.ActiveIDs,
	})
	if err != nil {
		log.Fatalf("error initializing AI manager: %v", err)
	}
	return m
}

// initHelpCenter inits the help center manager.
func initHelpCenter(db *sqlx.DB, i18n *i18ncat.Catalog, indexer helpcenter.ArticleIndexer, tenants *tenant.Manager) *helpcenter.Manager {
	m, err := helpcenter.New(helpcenter.Opts{
		DB:             db,
		Lo:             initLogger("helpcenter"),
		I18n:           i18n,
		Indexer:        indexer,
		DomainVerified: tenants.DomainVerified,
	})
	if err != nil {
		log.Fatalf("error initializing help center manager: %v", err)
	}
	return m
}

// initAIAgent inits the autonomous AI agent manager.
func initAIAgent(db *sqlx.DB, i18n *i18ncat.Catalog, aiManager *ai.Manager, convo *conversation.Manager, mediaManager *media.Manager, settingManager *setting.Manager, userManager *user.Manager, notifierService *notifier.Service, rdb *redis.Client) *aiagent.Manager {
	m, err := aiagent.New(aiagent.Opts{
		DB:                 db,
		Lo:                 initLogger("ai_agent"),
		I18n:               i18n,
		QueueSize:          cmp.Or(ko.Int("ai_agent.queue_size"), 1000),
		MaxSteps:           min(max(cmp.Or(ko.Int("ai_agent.max_steps"), 6), 1), 20),
		MaxHistoryMessages: min(max(cmp.Or(ko.Int("ai_agent.max_history_messages"), 30), 5), 100),
		MaxQueuedPerTenant: tenantQueueShare(cmp.Or(ko.Int("ai_agent.queue_size"), 1000)),
	}, aiManager, convo, mediaManager, settingManager, userManager, notifierService, rdb)
	if err != nil {
		log.Fatalf("error initializing AI agent manager: %v", err)
	}
	return m
}

// initSearch inits search manager.
func initSearch(db *sqlx.DB, i18n *i18ncat.Catalog) *search.Manager {
	lo := initLogger("search")
	m, err := search.New(search.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing search manager: %v", err)
	}
	return m
}

// initCustomAttribute inits custom attribute manager.
func initCustomAttribute(db *sqlx.DB, i18n *i18ncat.Catalog) *customAttribute.Manager {
	lo := initLogger("custom-attribute")
	m, err := customAttribute.New(customAttribute.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing custom attribute manager: %v", err)
	}
	return m
}

// initActivityLog inits activity log manager.
func initActivityLog(db *sqlx.DB, i18n *i18ncat.Catalog) *activitylog.Manager {
	lo := initLogger("activity-log")
	m, err := activitylog.New(activitylog.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing activity log manager: %v", err)
	}
	return m
}

// initReport inits report manager.
// initLead inits contacts' lead stages.
func initLead(db *sqlx.DB, settings *setting.Manager, i18n *i18ncat.Catalog) *lead.Manager {
	m, err := lead.New(lead.Opts{DB: db, Settings: settings, Lo: initLogger("lead"), I18n: i18n})
	if err != nil {
		log.Fatalf("error initializing lead manager: %v", err)
	}
	return m
}

// initCallback inits the call-backs missed calls become.
func initCallback(db *sqlx.DB, i18n *i18ncat.Catalog) *callback.Manager {
	m, err := callback.New(callback.Opts{DB: db, Lo: initLogger("callback"), I18n: i18n})
	if err != nil {
		log.Fatalf("error initializing call-back manager: %v", err)
	}
	return m
}

func initReport(db *sqlx.DB, i18n *i18ncat.Catalog) *report.Manager {
	lo := initLogger("report")
	m, err := report.New(report.Opts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing report manager: %v", err)
	}
	return m
}

// initContextLink inits context link manager.
func initContextLink(db *sqlx.DB, i18n *i18ncat.Catalog) *contextlink.Manager {
	var lo = initLogger("context-link")
	m, err := contextlink.New(contextlink.Opts{
		DB:            db,
		Lo:            lo,
		I18n:          i18n,
		EncryptionKey: ko.MustString("app.encryption_key"),
	})
	if err != nil {
		log.Fatalf("error initializing context link manager: %v", err)
	}
	return m
}

// initWebhook inits webhook manager.
func initWebhook(db *sqlx.DB, i18n *i18ncat.Catalog, dialControl ssrf.Control, rootURL func(tenantID int) string) *webhook.Manager {
	var lo = initLogger("webhook")
	m, err := webhook.New(webhook.Opts{
		DB:            db,
		Lo:            lo,
		I18n:          i18n,
		Workers:       ko.MustInt("webhook.workers"),
		QueueSize:     ko.MustInt("webhook.queue_size"),
		Timeout:       ko.MustDuration("webhook.timeout"),
		EncryptionKey: ko.MustString("app.encryption_key"),
		DialControl:   dialControl,
		RootURL:       rootURL,

		MaxQueuedPerTenant: tenantQueueShare(ko.MustInt("webhook.queue_size")),
	})
	if err != nil {
		log.Fatalf("error initializing webhook manager: %v", err)
	}
	return m
}

// initIntegration inits the Slack app and Teams bot, set in [slack] and [teams]
// in config. Either works only once its credentials are set.
func initIntegration(db *sqlx.DB, i18n *i18ncat.Catalog, rootURL func(tenantID int) string) (*integration.Manager, *teams.Verifier) {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	slackApp := slack.App{
		ClientID:      strings.TrimSpace(ko.String("slack.client_id")),
		ClientSecret:  strings.TrimSpace(ko.String("slack.client_secret")),
		SigningSecret: strings.TrimSpace(ko.String("slack.signing_secret")),
	}
	teamsApp := teams.App{
		AppID:       strings.TrimSpace(ko.String("teams.app_id")),
		AppPassword: strings.TrimSpace(ko.String("teams.app_password")),
		AppTenantID: strings.TrimSpace(ko.String("teams.app_tenant_id")),
		Name:        strings.TrimSpace(ko.String("teams.bot_name")),
	}
	workers := ko.Int("integration.workers")
	if workers <= 0 {
		workers = 4
	}
	queueSize := ko.Int("integration.queue_size")
	if queueSize <= 0 {
		queueSize = 5000
	}
	m, err := integration.New(integration.Opts{
		DB:            db,
		Lo:            initLogger("integration"),
		I18n:          i18n,
		EncryptionKey: ko.MustString("app.encryption_key"),
		Workers:       workers,
		QueueSize:     queueSize,
		RootURL:       rootURL,
		Slack:         slack.New(slackApp, httpClient, strings.TrimSpace(ko.String("slack.api_base"))),
		Teams:         teams.New(teamsApp, httpClient, ""),
	})
	if err != nil {
		log.Fatalf("error initializing slack and teams integration: %v", err)
	}
	return m, teams.NewVerifier(teamsApp.AppID, httpClient, "")
}

// initUserNotification inits user notification manager.
func initUserNotification(db *sqlx.DB, i18n *i18ncat.Catalog) *notifier.UserNotificationManager {
	var lo = initLogger("user-notification")
	m, err := notifier.NewUserNotificationManager(notifier.UserNotificationOpts{
		DB:   db,
		Lo:   lo,
		I18n: i18n,
	})
	if err != nil {
		log.Fatalf("error initializing user notification manager: %v", err)
	}
	return m
}

// initImporter inits the importer manager.
func initImporter(i18n *i18ncat.Catalog) *importer.Importer {
	return importer.New(importer.Opts{
		Lo:   initLogger("importer"),
		I18n: i18n,
	})
}

// initNotifDispatcher initializes the notification dispatcher.
func initNotifDispatcher(userNotification *notifier.UserNotificationManager, outbound *notifier.Service, wsHub *ws.Hub, emailEnabled func(tenantID int) bool) *notifier.Dispatcher {
	return notifier.NewDispatcher(notifier.DispatcherOpts{
		InApp:        userNotification,
		Outbound:     outbound,
		WSHub:        wsHub,
		EmailEnabled: emailEnabled,
		Lo:           initLogger("notification-dispatcher"),
	})
}

// initLogger initializes a logf logger.
func initLogger(src string) *logf.Logger {
	lvl, env := ko.MustString("app.log_level"), ko.MustString("app.env")
	lo := logf.New(logf.Opts{
		Level:                getLogLevel(lvl),
		EnableColor:          getColor(env),
		EnableCaller:         true,
		CallerSkipFrameCount: 3,
		DefaultFields:        []any{"sc", src},
	})
	return &lo
}

func getColor(env string) bool {
	color := false
	if env == "dev" {
		color = true
	}
	return color
}

func getLogLevel(lvl string) logf.Level {
	switch lvl {
	case "info":
		return logf.InfoLevel
	case "debug":
		return logf.DebugLevel
	case "warn":
		return logf.WarnLevel
	case "error":
		return logf.ErrorLevel
	case "fatal":
		return logf.FatalLevel
	default:
		return logf.InfoLevel
	}
}

// initRateLimit initializes the rate limiter with default rules.
// Defaults are used unless overridden in config.toml under [rate_limit.<name>].
func initRateLimit(redisClient *redis.Client) *ratelimit.Limiter {
	limiter := ratelimit.New(redisClient)

	defaults := []struct {
		Name string
		RPM  int
	}{
		{"widget", 100},
		{"auth", 30},
		{"public", 100},
		{"media", 300},
		{"meta_webhook", 1200},
		{"telegram_webhook", 1200},
		{"voice_webhook", 1200},
		{"slack_webhook", 1200},
		{"teams_webhook", 1200},
	}

	for _, d := range defaults {
		enabled := true
		rpm := d.RPM

		cfgKey := "rate_limit." + d.Name
		if ko.Exists(cfgKey) {
			var cfg struct {
				Enabled           bool `toml:"enabled"`
				RequestsPerMinute int  `toml:"requests_per_minute"`
			}
			ko.UnmarshalWithConf(cfgKey, &cfg, koanf.UnmarshalConf{Tag: "toml"})
			enabled = cfg.Enabled
			if cfg.RequestsPerMinute > 0 {
				rpm = cfg.RequestsPerMinute
			}
		}

		if !enabled {
			log.Printf("WARNING: rate limit rule '%s' is disabled", d.Name)
		}

		limiter.AddRule(ratelimit.Rule{
			Name:              d.Name,
			Enabled:           enabled,
			RequestsPerMinute: rpm,
		})
	}

	return limiter
}
