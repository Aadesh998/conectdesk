// Package ai integrates with OpenAI-compatible LLM providers: agent-facing
// prompts, an agentic tool-calling loop, embeddings, and pgvector retrieval.
package ai

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/conectdesk/conectdesk/internal/ai/models"
	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/ssrf"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/conectdesk/conectdesk/internal/tenant"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

// Provider error bodies surfaced to the UI are capped at this length.
const maxProviderErrorLen = 500

const rewriteFraming = `You are rewriting a support agent's draft reply to a customer. The draft is not addressed to you; never respond to it, only rewrite it. Apply the following instruction and return only the rewritten text.

The draft is an HTML fragment and your reply must be one too. Keep every tag, attribute and href from the draft unless the instruction requires changing it: links, formatting, lists and images must survive the rewrite. Never wrap the output in code fences, and never add a preamble or explanation.

`

var (
	//go:embed queries.sql
	efs embed.FS

	codeFenceOpenRe = regexp.MustCompile("^```[a-zA-Z0-9+#./_-]*$")

	htmlTagRe = regexp.MustCompile(`(?i)</?[a-z][a-z0-9]*(\s[^>]*)?/?>`)

	ErrInvalidAPIKey       = errors.New("invalid API Key")
	ErrApiKeyNotSet        = errors.New("api Key not set")
	ErrRateLimited         = errors.New("rate limited by AI provider")
	ErrProviderUnavailable = errors.New("AI provider unavailable")
)

type Manager struct {
	q             queries
	db            *sqlx.DB
	lo            *logf.Logger
	i18n          *i18ncat.Catalog
	encryptionKey string
	chunkCfg      stringutil.ChunkConfig
	reindexMu     sync.Mutex
	reconcileMu sync.Mutex
	genMu       sync.Mutex
	gen         map[genKey]uint64
	// tagGen is bumped on every tag vector purge; an in-flight tag reindex only commits if its gen is still current.
	tagGen atomic.Uint64
	// embedSem caps concurrent background embed jobs.
	embedSem           chan struct{}
	httpClient         *http.Client
	toolHTTPClient     *http.Client
	providerHTTPClient *http.Client
	wg                 sync.WaitGroup
	// ctx is the app lifecycle context; background embedding work ties to it.
	ctx context.Context
	// tenantIDs lists the tenants whose knowledge is kept embedded.
	tenantIDs func() ([]int, error)

	// requestLimit returns a tenant's monthly AI request limit; nil or <= 0
	// is unlimited.
	requestLimit func(tenantID int) int
}

// Opts contains options for initializing the Manager.
type Opts struct {
	Ctx           context.Context
	DB            *sqlx.DB
	I18n          *i18ncat.Catalog
	Lo            *logf.Logger
	EncryptionKey string
	DialControl   ssrf.Control
	// TenantIDs lists the tenants whose knowledge is kept embedded.
	TenantIDs func() ([]int, error)
}

// ProviderConfigView is the sanitized provider config returned to admins, with the API key dummy-masked.
type ProviderConfigView struct {
	models.ProviderConfig
	HasAPIKey bool `json:"has_api_key"`
}

type queries struct {
	ConsumeAIRequest             *sqlx.Stmt `query:"consume-ai-request"`
	GetAIUsage                   *sqlx.Stmt `query:"get-ai-usage"`
	GetProviderByType            *sqlx.Stmt `query:"get-provider-by-type"`
	UpdateProviderConfig         *sqlx.Stmt `query:"update-provider-config"`
	GetPrompt                    *sqlx.Stmt `query:"get-prompt"`
	GetPrompts                   *sqlx.Stmt `query:"get-prompts"`
	GetKnowledgeBaseItems        *sqlx.Stmt `query:"get-knowledge-base-items"`
	GetKnowledgeBaseItem         *sqlx.Stmt `query:"get-knowledge-base-item"`
	KnowledgeBaseItemExists      *sqlx.Stmt `query:"knowledge-base-item-exists"`
	InsertKnowledgeBaseItem      *sqlx.Stmt `query:"insert-knowledge-base-item"`
	UpdateKnowledgeBaseItem      *sqlx.Stmt `query:"update-knowledge-base-item"`
	DeleteKnowledgeBaseItem      *sqlx.Stmt `query:"delete-knowledge-base-item"`
	SetKnowledgeBaseFingerprint  *sqlx.Stmt `query:"set-knowledge-base-embedded-fingerprint"`
	GetEmbeddableHelpArticles    *sqlx.Stmt `query:"get-embeddable-help-articles"`
	GetEmbeddableHelpArticle     *sqlx.Stmt `query:"get-embeddable-help-article"`
	HelpArticleExists            *sqlx.Stmt `query:"help-article-exists"`
	SetHelpArticleFingerprint    *sqlx.Stmt `query:"set-help-article-embedded-fingerprint"`
	DeleteOrphanArticleVectors   *sqlx.Stmt `query:"delete-orphan-help-article-embeddings"`
	InsertEmbedding              *sqlx.Stmt `query:"insert-embedding"`
	DeleteEmbeddingsBySource     *sqlx.Stmt `query:"delete-embeddings-by-source"`
	DeleteEmbeddingsBySourceIDs  *sqlx.Stmt `query:"delete-embeddings-by-source-ids"`
	DeleteEmbeddingsBySourceType *sqlx.Stmt `query:"delete-embeddings-by-source-type"`
	SearchEmbeddings             *sqlx.Stmt `query:"search-embeddings"`
	CountMismatchedEmbeddings    *sqlx.Stmt `query:"count-mismatched-embeddings"`
	GetTagEmbeddingTexts         *sqlx.Stmt `query:"get-tag-embedding-texts"`
	GetTags                      *sqlx.Stmt `query:"get-tags"`
	GetTools                     *sqlx.Stmt `query:"get-tools"`
	GetTool                      *sqlx.Stmt `query:"get-tool"`
	GetEnabledToolsByIDs         *sqlx.Stmt `query:"get-enabled-tools-by-ids"`
	GetToolAuth                  *sqlx.Stmt `query:"get-tool-auth"`
	InsertTool                   *sqlx.Stmt `query:"insert-tool"`
	UpdateTool                   *sqlx.Stmt `query:"update-tool"`
	DeleteTool                   *sqlx.Stmt `query:"delete-tool"`
	GetCopilotMessages           *sqlx.Stmt `query:"get-copilot-messages"`
	InsertCopilotMessage         *sqlx.Stmt `query:"insert-copilot-message"`
	DeleteCopilotMessages        *sqlx.Stmt `query:"delete-copilot-messages"`
	GetReportCopilotMessages     *sqlx.Stmt `query:"get-report-copilot-messages"`
	InsertReportCopilotMessage   *sqlx.Stmt `query:"insert-report-copilot-message"`
	DeleteReportCopilotMessages  *sqlx.Stmt `query:"delete-report-copilot-messages"`
}

// defaultTenantOnly lists just the default tenant, for when no tenant list is given.
func defaultTenantOnly() ([]int, error) {
	return []int{tenant.DefaultID}, nil
}

// New creates and returns a new instance of the Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	transport := ssrf.NewTransport(opts.DialControl, 5*time.Second)
	m := &Manager{
		q:             q,
		ctx:           opts.Ctx,
		tenantIDs:     opts.TenantIDs,
		db:            opts.DB,
		lo:            opts.Lo,
		i18n:          opts.I18n,
		encryptionKey: opts.EncryptionKey,
		chunkCfg:      stringutil.DefaultChunkConfig(),
		gen:           make(map[genKey]uint64),
		embedSem:      make(chan struct{}, maxConcurrentEmbeds),
		httpClient: &http.Client{
			Timeout:   20 * time.Second,
			Transport: transport,
		},
		toolHTTPClient: &http.Client{
			Timeout:   20 * time.Second,
			Transport: transport,
			// Following a redirect would forward custom auth and contact identity headers to another host.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		providerHTTPClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: transport,
		},
	}
	if m.tenantIDs == nil {
		m.tenantIDs = defaultTenantOnly
	}
	m.chunkCfg.TokenizerFunc = newTokenCounter(opts.Lo)
	return m, nil
}

// SetRequestLimit sets the lookup of each tenant's monthly AI request limit.
func (m *Manager) SetRequestLimit(fn func(tenantID int) int) {
	m.requestLimit = fn
}

// Usage returns how many AI requests the tenant made this month.
func (m *Manager) Usage(tenantID int) (int, error) {
	var n int
	if err := m.q.GetAIUsage.Get(&n, tenantID); err != nil {
		m.lo.Error("error fetching ai usage", "tenant_id", tenantID, "error", err)
		return 0, err
	}
	return n, nil
}

// consumeRequest counts one AI request, a completion or an agent run, against
// the tenant's monthly limit. Embeddings aren't counted: they keep search
// working and aren't something a user asks for.
func (m *Manager) consumeRequest(tenantID int) error {
	limit := 0
	if m.requestLimit != nil {
		limit = m.requestLimit(tenantID)
	}
	var n int
	if err := m.q.ConsumeAIRequest.Get(&n, tenantID, limit); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return envelope.NewError(envelope.PermissionError, m.i18n.For(tenantID).Ts("tenant.aiLimitReached", "max", strconv.Itoa(limit)), nil)
		}
		// Counting is bookkeeping; don't fail the request over it.
		m.lo.Error("error counting ai request", "tenant_id", tenantID, "error", err)
	}
	return nil
}

// Completion runs the DB-stored prompt (by key) over the user content.
func (m *Manager) Completion(tenantID int, ctx context.Context, k string, prompt string) (string, error) {
	systemPrompt, err := m.getPrompt(tenantID, k)
	if err != nil {
		return "", err
	}

	client, err := m.getProviderClient(tenantID, models.ProviderTypeCompletion)
	if err != nil {
		return "", err
	}
	if err := m.consumeRequest(tenantID); err != nil {
		return "", err
	}

	response, err := client.SendPrompt(ctx, models.PromptPayload{SystemPrompt: rewriteFraming + systemPrompt, UserPrompt: prompt})
	if err != nil {
		return "", m.providerError(err)
	}
	return ensureHTMLFragment(stripCodeFence(response)), nil
}

// CompletionRaw runs an ad-hoc system+user prompt (no DB-stored prompt) and returns the text.
func (m *Manager) CompletionRaw(tenantID int, ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	client, err := m.getProviderClient(tenantID, models.ProviderTypeCompletion)
	if err != nil {
		return "", err
	}
	if err := m.consumeRequest(tenantID); err != nil {
		return "", err
	}
	response, err := client.SendPrompt(ctx, models.PromptPayload{SystemPrompt: systemPrompt, UserPrompt: userPrompt})
	if err != nil {
		return "", m.providerError(err)
	}
	return response, nil
}

// GetEmbeddings returns the embedding vector for text using the embedding provider.
func (m *Manager) GetEmbeddings(tenantID int, ctx context.Context, text string) ([]float32, error) {
	client, err := m.getProviderClient(tenantID, models.ProviderTypeEmbedding)
	if err != nil {
		return nil, err
	}
	vec, err := client.GetEmbeddings(ctx, text)
	if err != nil {
		return nil, m.providerError(err)
	}
	return vec, nil
}

// GetEmbeddingsBatch returns embedding vectors for all texts in a single provider request.
func (m *Manager) GetEmbeddingsBatch(tenantID int, ctx context.Context, texts []string) ([][]float32, error) {
	client, err := m.getProviderClient(tenantID, models.ProviderTypeEmbedding)
	if err != nil {
		return nil, err
	}
	vecs, err := client.GetEmbeddingsBatch(ctx, texts)
	if err != nil {
		return nil, m.providerError(err)
	}
	return vecs, nil
}

// GetPrompts returns the list of agent-facing prompts.
func (m *Manager) GetPrompts(tenantID int) ([]models.Prompt, error) {
	prompts := make([]models.Prompt, 0)
	if err := m.q.GetPrompts.Select(&prompts, tenantID); err != nil {
		m.lo.Error("error fetching prompts", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return prompts, nil
}

// GetProviderConfig returns the sanitized config for a provider type (no API key).
func (m *Manager) GetProviderConfig(tenantID int, providerType string) (ProviderConfigView, error) {
	cfg, err := m.getProviderConfig(tenantID, providerType)
	if err != nil {
		return ProviderConfigView{}, err
	}
	view := ProviderConfigView{ProviderConfig: cfg, HasAPIKey: cfg.APIKey != ""}
	if cfg.APIKey != "" {
		view.APIKey = strings.Repeat(stringutil.PasswordDummy, 10)
	}
	return view, nil
}

// VisionEnabled reports whether the completion model is marked as accepting image input.
func (m *Manager) VisionEnabled(tenantID int) bool {
	cfg, err := m.getRawProviderConfig(tenantID, models.ProviderTypeCompletion)
	if err != nil {
		return false
	}
	return cfg.Vision
}

// UpdateProviderConfig updates a provider type's config; a masked api_key keeps the stored key, a blank one clears it.
func (m *Manager) UpdateProviderConfig(tenantID int, providerType string, in models.ProviderConfig) error {
	if providerType != models.ProviderTypeCompletion && providerType != models.ProviderTypeEmbedding {
		m.lo.Warn("invalid provider type", "providerType", providerType)
		return envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	existing, err := m.getRawProviderConfig(tenantID, providerType)
	if err != nil {
		return err
	}

	apiKey := ""
	switch {
	case strings.Contains(in.APIKey, stringutil.PasswordDummy):
		apiKey = existing.APIKey
	case in.APIKey == "":
	case crypto.IsEncrypted(in.APIKey):
		// crypto.Encrypt would store this user-supplied value as-is and Decrypt would then fail on every call.
		return envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("admin.ai.reservedSecretPrefix"), nil)
	default:
		enc, eerr := crypto.Encrypt(in.APIKey, m.encryptionKey)
		if eerr != nil {
			m.lo.Error("error encrypting API key", "error", eerr)
			return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
		apiKey = enc
	}

	cfg := in
	cfg.Provider = "openai"
	cfg.APIKey = apiKey
	b, err := json.Marshal(cfg)
	if err != nil {
		m.lo.Error("error marshaling provider config", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if _, err := m.q.UpdateProviderConfig.Exec(tenantID, providerType, b); err != nil {
		m.lo.Error("error updating provider config", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// A changed embedding model, dimension count, or backend URL produces vectors incomparable to the stored ones.
	if providerType == models.ProviderTypeEmbedding && (existing.Model != cfg.Model || existing.Dimensions != cfg.Dimensions || existing.BaseURL != cfg.BaseURL) {
		m.lo.Info("embedding provider changed, reindexing knowledge base", "old_model", existing.Model, "new_model", cfg.Model, "old_base_url", existing.BaseURL, "new_base_url", cfg.BaseURL)
		m.purgeTagEmbeddings(tenantID)
		m.ReindexAll()
	}
	return nil
}

// TestProviderConfig makes one live provider request with the given config; a masked api_key uses the stored key, matching what an update of the same form would store.
func (m *Manager) TestProviderConfig(tenantID int, providerType string, in models.ProviderConfig) error {
	if providerType != models.ProviderTypeCompletion && providerType != models.ProviderTypeEmbedding {
		return envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	cfg := in
	cfg.Provider = "openai"
	if strings.Contains(cfg.APIKey, stringutil.PasswordDummy) {
		stored, err := m.getProviderConfig(tenantID, providerType)
		if err != nil {
			return err
		}
		cfg.APIKey = stored.APIKey
	}
	client := NewOpenAIClient(cfg, m.lo, m.providerHTTPClient)

	if providerType == models.ProviderTypeCompletion {
		if _, err := client.SendPrompt(context.Background(), models.PromptPayload{SystemPrompt: "You are a connection test.", UserPrompt: "Reply with OK."}); err != nil {
			return m.testProviderError(err)
		}
		return nil
	}

	vec, err := client.GetEmbeddings(context.Background(), "connection test")
	if err != nil {
		return m.testProviderError(err)
	}
	if cfg.Dimensions > 0 && len(vec) != cfg.Dimensions {
		return envelope.NewError(envelope.InputError, m.i18n.For(tenantID).Ts("ai.testDimensionsMismatch",
			"configured", strconv.Itoa(cfg.Dimensions), "returned", strconv.Itoa(len(vec))), nil)
	}
	return nil
}

func (m *Manager) getPrompt(tenantID int, k string) (string, error) {
	var p models.Prompt
	if err := m.q.GetPrompt.Get(&p, tenantID, k); err != nil {
		if err == sql.ErrNoRows {
			return "", envelope.NewError(envelope.InputError, m.i18n.For(tenantID).T("validation.notFoundTemplate"), nil)
		}
		m.lo.Error("error fetching prompt", "error", err)
		return "", envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return p.Content, nil
}

// getRawProviderConfig returns the stored config with the API key still encrypted.
func (m *Manager) getRawProviderConfig(tenantID int, providerType string) (models.ProviderConfig, error) {
	var p models.Provider
	if err := m.q.GetProviderByType.Get(&p, tenantID, providerType); err != nil {
		// A tenant without the provider row has simply not configured it.
		if errors.Is(err, sql.ErrNoRows) {
			return models.ProviderConfig{}, nil
		}
		m.lo.Error("error fetching provider", "error", err, "type", providerType)
		return models.ProviderConfig{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	var cfg models.ProviderConfig
	if len(p.Config) > 0 {
		if err := json.Unmarshal(p.Config, &cfg); err != nil {
			m.lo.Error("error parsing provider config", "error", err)
			return models.ProviderConfig{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
	}
	return cfg, nil
}

func (m *Manager) getProviderConfig(tenantID int, providerType string) (models.ProviderConfig, error) {
	cfg, err := m.getRawProviderConfig(tenantID, providerType)
	if err != nil {
		return cfg, err
	}
	if cfg.APIKey != "" {
		decrypted, err := crypto.Decrypt(cfg.APIKey, m.encryptionKey)
		if err != nil {
			m.lo.Error("error decrypting API key", "error", err)
			return models.ProviderConfig{}, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
		cfg.APIKey = decrypted
	}
	return cfg, nil
}

func (m *Manager) getProviderClient(tenantID int, providerType string) (ProviderClient, error) {
	cfg, err := m.getProviderConfig(tenantID, providerType)
	if err != nil {
		return nil, err
	}
	return NewOpenAIClient(cfg, m.lo, m.providerHTTPClient), nil
}

func (m *Manager) providerError(err error) error {
	if errors.Is(err, ErrInvalidAPIKey) {
		m.lo.Error("invalid provider API key")
		return envelope.NewError(envelope.InputError, m.i18n.T("ai.invalidAPIKey"), nil)
	}
	if errors.Is(err, ErrApiKeyNotSet) {
		return envelope.NewError(envelope.InputError, m.i18n.T("ai.apiKeyNotSet"), nil)
	}
	if errors.Is(err, ErrRateLimited) {
		m.lo.Error("rate limited by AI provider", "error", err)
		return envelope.NewError(envelope.RateLimitError, m.i18n.T("ai.rateLimited"), nil)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		m.lo.Error("AI provider request timed out", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("ai.providerTimeout"), nil)
	}
	if errors.Is(err, ErrProviderUnavailable) {
		m.lo.Error("AI provider unavailable", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.T("ai.providerUnavailable"), nil)
	}
	m.lo.Error("error from AI provider", "error", err)
	return envelope.NewError(envelope.GeneralError, capProviderErrorMessage(err), nil)
}

// testProviderError surfaces the provider's own error message so the admin can act on it.
func (m *Manager) testProviderError(err error) error {
	if errors.Is(err, ErrApiKeyNotSet) {
		return envelope.NewError(envelope.InputError, m.i18n.T("ai.apiKeyNotSet"), nil)
	}
	return envelope.NewError(envelope.InputError, capProviderErrorMessage(err), nil)
}

func capProviderErrorMessage(err error) string {
	msg := err.Error()
	if len(msg) > maxProviderErrorLen {
		msg = trimToRuneBoundary(msg, maxProviderErrorLen) + "…"
	}
	return msg
}

// ensureHTMLFragment converts a plain-text response to HTML, which models return despite being told to answer in HTML.
func ensureHTMLFragment(s string) string {
	if htmlTagRe.MatchString(s) {
		return s
	}
	return strings.ReplaceAll(html.EscapeString(s), "\n", "<br>")
}

// stripCodeFence unwraps a whole-response ```lang fence, which models add around HTML output despite being told not to.
func stripCodeFence(s string) string {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "```") || !strings.HasSuffix(t, "```") || strings.Count(t, "```") != 2 {
		return s
	}
	open, body, found := strings.Cut(strings.TrimSuffix(t, "```"), "\n")
	if !found || !codeFenceOpenRe.MatchString(strings.TrimSpace(open)) {
		return s
	}
	return strings.TrimSpace(body)
}
