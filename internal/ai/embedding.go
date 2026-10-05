package ai

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/ai/models"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/lib/pq"
)

// reconcileInterval is how often knowledge base content that failed to embed is retried.
const reconcileInterval = 1 * time.Minute

// maxConcurrentEmbeds caps background snippet embed jobs hitting the provider at once.
const maxConcurrentEmbeds = 4

// defaultSearchLimit bounds a search that asks for no particular number of results.
const defaultSearchLimit = 10

// indexedChunk is one embedded chunk, ready to be written to the embeddings table.
type indexedChunk struct {
	sourceType string
	sourceID   int
	chunkText  string
	vec        []float32
}

// Search embeds the query and returns the tenant's top-k most similar knowledge chunks across snippets and help articles.
func (m *Manager) Search(tenantID int, ctx context.Context, query string, k int) ([]models.SearchResult, error) {
	return m.searchSources(tenantID, ctx, query, k, models.SourceSnippet, models.SourceHelpArticle)
}

func (m *Manager) searchSources(tenantID int, ctx context.Context, query string, k int, sourceTypes ...string) ([]models.SearchResult, error) {
	qvec, err := m.GetEmbeddings(tenantID, ctx, query)
	if err != nil {
		return nil, err
	}
	results, err := m.searchVectors(tenantID, ctx, qvec, k, sourceTypes...)
	if err != nil {
		return nil, err
	}
	m.lo.Debug("rag search", "source_types", sourceTypes, "query_len", len(query), "hits", len(results))
	for i, r := range results {
		m.lo.Debug("rag fetched chunk", "rank", i+1, "score", r.Score, "source_type", r.SourceType, "source_id", r.SourceID, "chunk_len", len(r.ChunkText))
	}
	return results, nil
}

// searchVectors returns the tenant's top-k chunks of the given source types most similar to qvec.
func (m *Manager) searchVectors(tenantID int, ctx context.Context, qvec []float32, k int, sourceTypes ...string) ([]models.SearchResult, error) {
	if len(qvec) == 0 {
		return nil, nil
	}
	if k <= 0 {
		k = defaultSearchLimit
	}
	results := make([]models.SearchResult, 0, k)
	if err := m.q.SearchEmbeddings.SelectContext(ctx, &results, tenantID, pq.Array(sourceTypes), vectorLiteral(qvec), len(qvec), k); err != nil {
		m.lo.Error("error searching embeddings", "error", err, "source_types", sourceTypes)
		return nil, err
	}
	if len(results) == 0 {
		// Vectors from a previous embedding model can't be compared with the query; say so instead of failing silently.
		var mismatched int
		if err := m.q.CountMismatchedEmbeddings.GetContext(ctx, &mismatched, tenantID, pq.Array(sourceTypes), len(qvec)); err == nil && mismatched > 0 {
			m.lo.Warn("skipped stale embeddings with mismatched dimensions; reindex after changing the embedding model", "source_types", sourceTypes, "count", mismatched, "query_dimensions", len(qvec))
		}
	}
	return results, nil
}

// Reindex re-chunks and re-embeds a source's content, replacing its stored vectors.
func (m *Manager) Reindex(tenantID int, sourceType string, sourceID int, title, htmlContent string) error {
	indexed, err := m.embedSource(tenantID, m.ctx, sourceType, sourceID, title, htmlContent)
	if err != nil {
		return err
	}
	m.reindexMu.Lock()
	defer m.reindexMu.Unlock()
	return m.commitEmbeddings(tenantID, sourceType, []int{sourceID}, indexed)
}

// embedSource chunks and embeds content without touching stored state, so it can run before taking reindexMu.
func (m *Manager) embedSource(tenantID int, ctx context.Context, sourceType string, sourceID int, title, htmlContent string) ([]indexedChunk, error) {
	rawChunks, err := stringutil.ChunkHTMLContent(title, htmlContent, m.chunkCfg)
	if err != nil {
		m.lo.Error("error chunking content for embedding", "error", err, "source_type", sourceType, "source_id", sourceID)
		return nil, err
	}
	// Drop blank chunks: they carry no signal and the embeddings API 400s on an empty input string.
	chunks := make([]string, 0, len(rawChunks))
	for _, c := range rawChunks {
		if strings.TrimSpace(c) != "" {
			chunks = append(chunks, c)
		}
	}

	vecs, err := m.GetEmbeddingsBatch(tenantID, ctx, chunks)
	if err != nil {
		m.lo.Error("error generating embeddings", "error", err)
		return nil, err
	}
	indexed := make([]indexedChunk, 0, len(chunks))
	for i, chunk := range chunks {
		indexed = append(indexed, indexedChunk{
			sourceType: sourceType,
			sourceID:   sourceID,
			chunkText:  chunk,
			vec:        vecs[i],
		})
	}
	return indexed, nil
}

// commitEmbeddings replaces the stored vectors of the given source ids. Caller must hold reindexMu.
func (m *Manager) commitEmbeddings(tenantID int, sourceType string, sourceIDs []int, indexed []indexedChunk) error {
	tx, err := m.db.BeginTxx(context.Background(), &sql.TxOptions{})
	if err != nil {
		m.lo.Error("error beginning reindex transaction", "error", err)
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Stmtx(m.q.DeleteEmbeddingsBySourceIDs).Exec(tenantID, sourceType, pq.Array(sourceIDs)); err != nil {
		m.lo.Error("error clearing old embeddings", "error", err)
		return err
	}
	insert := tx.Stmtx(m.q.InsertEmbedding)
	for _, c := range indexed {
		if _, err := insert.Exec(tenantID, sourceType, c.sourceID, c.chunkText, vectorLiteral(c.vec), len(c.vec)); err != nil {
			m.lo.Error("error inserting embedding", "error", err, "source_id", c.sourceID)
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		m.lo.Error("error committing reindex transaction", "error", err)
		return err
	}
	return nil
}

// RemoveEmbeddings drops all stored vectors for a source.
func (m *Manager) RemoveEmbeddings(tenantID int, sourceType string, sourceID int) error {
	m.reindexMu.Lock()
	defer m.reindexMu.Unlock()
	return m.removeEmbeddings(tenantID, sourceType, sourceID)
}

// removeEmbeddings drops all stored vectors for a source. Caller must hold reindexMu.
func (m *Manager) removeEmbeddings(tenantID int, sourceType string, sourceID int) error {
	if _, err := m.q.DeleteEmbeddingsBySource.Exec(tenantID, sourceType, sourceID); err != nil {
		m.lo.Error("error deleting embeddings", "error", err)
		return err
	}
	return nil
}

// Run periodically reconciles knowledge base embeddings.
func (m *Manager) Run(ctx context.Context) {
	m.wg.Add(1)
	go m.reconcileLoop(ctx)
}

// Close waits for an in-flight reconcile to finish.
func (m *Manager) Close() {
	m.wg.Wait()
}

func (m *Manager) reconcileLoop(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	m.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.reconcile(ctx)
		}
	}
}

// reconcile brings every tenant's embedded sources back in line with its active embedding provider.
func (m *Manager) reconcile(ctx context.Context) {
	if !m.reconcileMu.TryLock() {
		return
	}
	defer m.reconcileMu.Unlock()

	tenantIDs, err := m.tenantIDs()
	if err != nil {
		m.lo.Error("error listing tenants to reconcile embeddings", "error", err)
		return
	}
	for _, tenantID := range tenantIDs {
		if ctx.Err() != nil {
			return
		}
		m.reconcileTenant(ctx, tenantID)
	}
}

// reconcileTenant brings a tenant's embedded sources back in line with its embedding provider.
func (m *Manager) reconcileTenant(ctx context.Context, tenantID int) {
	cfg, err := m.getRawProviderConfig(tenantID, models.ProviderTypeEmbedding)
	if err != nil {
		return
	}
	// The API key is stored encrypted, so a non-empty value is enough to know a provider is configured.
	if cfg.APIKey == "" {
		return
	}

	for _, src := range m.embedSources(tenantID) {
		if ctx.Err() != nil {
			return
		}
		m.reconcileSource(ctx, src, cfg.BaseURL, cfg.Model, cfg.Dimensions)
	}
	m.reconcileTags(ctx, tenantID)
}

// vectorLiteral formats a vector in pgvector's text form, e.g. [0.1,-0.2], for a $n::vector parameter.
func vectorLiteral(vec []float32) string {
	buf := make([]byte, 0, len(vec)*12+2)
	buf = append(buf, '[')
	for i, f := range vec {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = strconv.AppendFloat(buf, float64(f), 'g', -1, 32)
	}
	return string(append(buf, ']'))
}
