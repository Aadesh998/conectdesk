package ai

import (
	"context"
	"slices"
	"strings"

	"github.com/conectdesk/conectdesk/internal/ai/models"
)

// reconcileTags embeds tags missing from the index, re-embeds renamed ones and drops the vectors of deleted tags.
func (m *Manager) reconcileTags(ctx context.Context, tenantID int) {
	tags, err := m.getTags(tenantID)
	if err != nil {
		return
	}

	gen := m.tagGen.Load()
	indexed, err := m.getTagEmbeddingTexts(tenantID)
	if err != nil {
		return
	}
	stale, removed := diffTagIndex(tags, indexed)
	if len(stale) == 0 && len(removed) == 0 {
		return
	}
	m.lo.Debug("reindex tags", "total", len(tags), "stale", len(stale), "removed", len(removed))

	touched := slices.Clone(removed)
	chunks := make([]indexedChunk, 0, len(stale))
	if len(stale) > 0 {
		names := tagNames(stale)
		vecs, err := m.GetEmbeddingsBatch(tenantID, ctx, names)
		if err != nil {
			m.lo.Error("error embedding tags", "error", err, "count", len(names))
			return
		}
		for i, t := range stale {
			touched = append(touched, t.ID)
			chunks = append(chunks, indexedChunk{
				sourceType: models.SourceTag,
				sourceID:   t.ID,
				chunkText:  t.Name,
				vec:        vecs[i],
			})
		}
	}

	m.reindexMu.Lock()
	defer m.reindexMu.Unlock()
	if m.tagGen.Load() != gen {
		return
	}
	if err := m.commitEmbeddings(tenantID, models.SourceTag, touched, chunks); err != nil {
		return
	}
	m.lo.Debug("reindex tags embedded", "embedded", len(chunks), "removed", len(removed), "names", strings.Join(tagNames(stale), ","))
	m.lo.Info("reconciled tag embeddings", "embedded", len(chunks), "removed", len(removed), "total", len(tags))
}

// purgeTagEmbeddings drops every tag vector so the next reconcile re-embeds the list with the current provider.
func (m *Manager) purgeTagEmbeddings(tenantID int) {
	m.reindexMu.Lock()
	defer m.reindexMu.Unlock()
	m.tagGen.Add(1)
	m.lo.Debug("reindex tags purged")
	if _, err := m.q.DeleteEmbeddingsBySourceType.Exec(tenantID, models.SourceTag); err != nil {
		m.lo.Error("error deleting tag embeddings", "error", err)
	}
}

// tagCandidates returns the current names of the tags most similar to text, most similar first; a name only the index still knows is dropped.
func (m *Manager) tagCandidates(tenantID int, ctx context.Context, text string, k int, tags []models.TagRef) ([]string, error) {
	results, err := m.searchSources(tenantID, ctx, text, k, models.SourceTag)
	if err != nil {
		return nil, err
	}
	byID := make(map[int]string, len(tags))
	for _, t := range tags {
		byID[t.ID] = t.Name
	}
	names := make([]string, 0, len(results))
	for _, r := range results {
		if name, ok := byID[r.SourceID]; ok {
			names = append(names, name)
		}
	}
	return names, nil
}

func (m *Manager) getTags(tenantID int) ([]models.TagRef, error) {
	tags := make([]models.TagRef, 0)
	if err := m.q.GetTags.Select(&tags, tenantID); err != nil {
		m.lo.Error("error fetching tags", "error", err)
		return nil, err
	}
	return tags, nil
}

// getTagEmbeddingTexts returns the text each embedded tag was embedded from, keyed by tag id.
func (m *Manager) getTagEmbeddingTexts(tenantID int) (map[int]string, error) {
	var rows []struct {
		SourceID  int    `db:"source_id"`
		ChunkText string `db:"chunk_text"`
	}
	if err := m.q.GetTagEmbeddingTexts.Select(&rows, tenantID); err != nil {
		m.lo.Error("error fetching tag embeddings", "error", err)
		return nil, err
	}
	out := make(map[int]string, len(rows))
	for _, r := range rows {
		out[r.SourceID] = r.ChunkText
	}
	return out, nil
}

func tagNames(tags []models.TagRef) []string {
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	return names
}

// diffTagIndex reports which tags need embedding and which indexed source ids no longer belong; a blank name is dropped, not embedded.
func diffTagIndex(tags []models.TagRef, indexed map[int]string) (stale []models.TagRef, removed []int) {
	live := make(map[int]bool, len(tags))
	for _, t := range tags {
		if strings.TrimSpace(t.Name) == "" {
			continue
		}
		live[t.ID] = true
		if text, ok := indexed[t.ID]; ok && text == t.Name {
			continue
		}
		stale = append(stale, t)
	}
	for id := range indexed {
		if !live[id] {
			removed = append(removed, id)
		}
	}
	slices.Sort(removed)
	return stale, removed
}
