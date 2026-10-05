package ai

import (
	"context"
	"database/sql"
	"strings"

	"github.com/conectdesk/conectdesk/internal/ai/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
)

// snippetSource indexes enabled knowledge base snippets.
type snippetSource struct {
	m        *Manager
	tenantID int
}

func (s snippetSource) sourceType() string {
	return models.SourceSnippet
}

func (s snippetSource) list() ([]embedItem, error) {
	rows, err := s.m.GetKnowledgeBaseItems(s.tenantID)
	if err != nil {
		return nil, err
	}
	items := make([]embedItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, snippetItem(row))
	}
	return items, nil
}

func (s snippetSource) get(id int) (embedItem, error) {
	row, err := s.m.GetKnowledgeBaseItem(s.tenantID, id)
	if err != nil {
		return embedItem{}, err
	}
	return snippetItem(row), nil
}

func (s snippetSource) exists(id int) (bool, error) {
	var exists bool
	if err := s.m.q.KnowledgeBaseItemExists.Get(&exists, s.tenantID, id); err != nil {
		return false, err
	}
	return exists, nil
}

func (s snippetSource) setFingerprint(id int, fingerprint string) {
	if _, err := s.m.q.SetKnowledgeBaseFingerprint.Exec(s.tenantID, id, fingerprint); err != nil {
		s.m.lo.Error("error setting snippet embedded fingerprint", "error", err, "id", id)
	}
}

// deleteOrphans is a no-op: snippet rows and their vectors are deleted in one transaction.
func (s snippetSource) deleteOrphans() ([]int, error) {
	return nil, nil
}

// GetKnowledgeBaseItems returns all snippet knowledge base items.
func (m *Manager) GetKnowledgeBaseItems(tenantID int) ([]models.KnowledgeBaseItem, error) {
	items := make([]models.KnowledgeBaseItem, 0)
	if err := m.q.GetKnowledgeBaseItems.Select(&items, tenantID); err != nil {
		m.lo.Error("error fetching knowledge base items", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return items, nil
}

func (m *Manager) GetKnowledgeBaseItem(tenantID, id int) (models.KnowledgeBaseItem, error) {
	var item models.KnowledgeBaseItem
	if err := m.q.GetKnowledgeBaseItem.Get(&item, tenantID, id); err != nil {
		if err == sql.ErrNoRows {
			return item, envelope.NewError(envelope.NotFoundError, m.i18n.For(tenantID).T("globals.messages.notFound"), nil)
		}
		m.lo.Error("error fetching knowledge base item", "error", err)
		return item, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return item, nil
}

func (m *Manager) CreateKnowledgeBaseItem(tenantID int, title, content, source, sourceURL string, enabled bool) (models.KnowledgeBaseItem, error) {
	if strings.TrimSpace(title) == "" {
		return models.KnowledgeBaseItem{}, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).Ts("globals.messages.empty", "name", m.i18n.For(tenantID).T("globals.terms.title")), nil)
	}
	if strings.TrimSpace(content) == "" {
		return models.KnowledgeBaseItem{}, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).Ts("globals.messages.empty", "name", m.i18n.For(tenantID).T("globals.terms.content")), nil)
	}
	if source == "" {
		source = models.KnowledgeSourceManual
	}
	var item models.KnowledgeBaseItem
	if err := m.q.InsertKnowledgeBaseItem.Get(&item, tenantID, models.KnowledgeTypeSnippet, title, content, enabled, source, sourceURL); err != nil {
		m.lo.Error("error creating knowledge base item", "error", err)
		return item, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	m.reindexItem(snippetSource{m, tenantID}, snippetItem(item))
	return item, nil
}

func (m *Manager) UpdateKnowledgeBaseItem(tenantID, id int, title, content string, enabled bool) (models.KnowledgeBaseItem, error) {
	if strings.TrimSpace(title) == "" {
		return models.KnowledgeBaseItem{}, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).Ts("globals.messages.empty", "name", m.i18n.For(tenantID).T("globals.terms.title")), nil)
	}
	if strings.TrimSpace(content) == "" {
		return models.KnowledgeBaseItem{}, envelope.NewError(envelope.InputError, m.i18n.For(tenantID).Ts("globals.messages.empty", "name", m.i18n.For(tenantID).T("globals.terms.content")), nil)
	}
	var item models.KnowledgeBaseItem
	if err := m.q.UpdateKnowledgeBaseItem.Get(&item, tenantID, id, title, content, enabled); err != nil {
		if err == sql.ErrNoRows {
			return item, envelope.NewError(envelope.NotFoundError, m.i18n.For(tenantID).T("globals.messages.notFound"), nil)
		}
		m.lo.Error("error updating knowledge base item", "error", err)
		return item, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	m.reindexItem(snippetSource{m, tenantID}, snippetItem(item))
	return item, nil
}

// DeleteKnowledgeBaseItem removes the snippet and its embeddings in one transaction.
func (m *Manager) DeleteKnowledgeBaseItem(tenantID, id int) error {
	m.reindexMu.Lock()
	defer m.reindexMu.Unlock()

	// Supersede any in-flight reindex for this snippet so a slower embed can't re-insert its vectors
	// after the delete commits.
	m.dropGen(models.SourceSnippet, id)

	tx, err := m.db.BeginTxx(context.Background(), &sql.TxOptions{})
	if err != nil {
		m.lo.Error("error beginning knowledge base delete transaction", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Stmtx(m.q.DeleteKnowledgeBaseItem).Exec(tenantID, id); err != nil {
		m.lo.Error("error deleting knowledge base item", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if _, err := tx.Stmtx(m.q.DeleteEmbeddingsBySource).Exec(tenantID, models.SourceSnippet, id); err != nil {
		m.lo.Error("error deleting snippet embeddings", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if err := tx.Commit(); err != nil {
		m.lo.Error("error committing knowledge base delete transaction", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// ReindexAll triggers a reconcile so snippets are re-embedded against the current model, e.g. after the embedding model changed.
func (m *Manager) ReindexAll() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.reconcile(m.ctx)
	}()
}

func snippetItem(row models.KnowledgeBaseItem) embedItem {
	return embedItem{
		ID:          row.ID,
		Title:       row.Title,
		Content:     row.Content,
		Fingerprint: row.EmbeddedFingerprint,
		Eligible:    row.Enabled,
	}
}

func (s snippetSource) tenant() int { return s.tenantID }
