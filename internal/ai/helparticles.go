package ai

import (
	"database/sql"

	"github.com/conectdesk/conectdesk/internal/ai/models"
)

// helpArticleSource indexes published, AI-enabled help center articles.
type helpArticleSource struct {
	m        *Manager
	tenantID int
}

func (s helpArticleSource) sourceType() string {
	return models.SourceHelpArticle
}

func (s helpArticleSource) list() ([]embedItem, error) {
	rows := make([]models.HelpArticleItem, 0)
	if err := s.m.q.GetEmbeddableHelpArticles.Select(&rows, s.tenantID); err != nil {
		s.m.lo.Error("error fetching help articles for reconcile", "error", err)
		return nil, err
	}
	items := make([]embedItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, helpArticleItem(row))
	}
	return items, nil
}

func (s helpArticleSource) get(id int) (embedItem, error) {
	var row models.HelpArticleItem
	if err := s.m.q.GetEmbeddableHelpArticle.Get(&row, s.tenantID, id); err != nil {
		if err != sql.ErrNoRows {
			s.m.lo.Error("error fetching help article for reindex", "error", err, "id", id)
		}
		return embedItem{}, err
	}
	return helpArticleItem(row), nil
}

func (s helpArticleSource) exists(id int) (bool, error) {
	var exists bool
	if err := s.m.q.HelpArticleExists.Get(&exists, s.tenantID, id); err != nil {
		return false, err
	}
	return exists, nil
}

func (s helpArticleSource) setFingerprint(id int, fingerprint string) {
	if _, err := s.m.q.SetHelpArticleFingerprint.Exec(s.tenantID, id, fingerprint); err != nil {
		s.m.lo.Error("error setting help article embedded fingerprint", "error", err, "id", id)
	}
}

func (s helpArticleSource) deleteOrphans() ([]int, error) {
	var ids []int
	if err := s.m.q.DeleteOrphanArticleVectors.Select(&ids, s.tenantID); err != nil {
		return nil, err
	}
	return ids, nil
}

// GetHelpArticleTitle returns a help article's title, for attributing a search hit to its source.
func (m *Manager) GetHelpArticleTitle(tenantID, id int) (string, error) {
	var row models.HelpArticleItem
	if err := m.q.GetEmbeddableHelpArticle.Get(&row, tenantID, id); err != nil {
		return "", err
	}
	return row.Title, nil
}

// ReindexHelpArticle re-syncs a help article's embeddings in the background, indexing or removing
// based on its current status and AI flag.
func (m *Manager) ReindexHelpArticle(tenantID, id int) {
	m.reindexItemByID(helpArticleSource{m, tenantID}, id)
}

// RemoveHelpArticleEmbeddings drops a help article's vectors from the DB and memory.
func (m *Manager) RemoveHelpArticleEmbeddings(tenantID, id int) error {
	m.dropGen(models.SourceHelpArticle, id)
	return m.RemoveEmbeddings(tenantID, models.SourceHelpArticle, id)
}

func helpArticleItem(row models.HelpArticleItem) embedItem {
	return embedItem{
		ID:          row.ID,
		Title:       row.Title,
		Content:     row.Content,
		Fingerprint: row.EmbeddedFingerprint,
		// An article inside an unpublished collection is 404 on the public site, so the
		// assistant must not cite it.
		Eligible: row.Status == "published" && row.AIEnabled && row.IsReachable,
	}
}

func (s helpArticleSource) tenant() int { return s.tenantID }
