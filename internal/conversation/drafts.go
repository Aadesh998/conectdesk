package conversation

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
)

func (m *Manager) UpsertConversationDraft(tenantID, conversationID, userID int, draftType, content string, meta json.RawMessage) (models.ConversationDraft, error) {
	var draft models.ConversationDraft
	content = rewriteInlineImagesToCID(content)

	if err := m.q.UpsertConversationDraft.Get(&draft, tenantID, conversationID, userID, draftType, content, meta); err != nil {
		m.lo.Error("error upserting conversation draft", "conversation_id", conversationID, "user_id", userID, "error", err)
		return draft, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	draft.Content = m.resolveDraftInlineCIDs(tenantID, conversationID, draft.Content)
	return draft, nil
}

func (m *Manager) GetAllUserDrafts(tenantID, userID int) ([]models.ConversationDraft, error) {
	var drafts = make([]models.ConversationDraft, 0)
	if err := m.q.GetAllUserDrafts.Select(&drafts, tenantID, userID); err != nil {
		m.lo.Error("error fetching user drafts", "user_id", userID, "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	for i := range drafts {
		drafts[i].Content = m.resolveDraftInlineCIDs(tenantID, int(drafts[i].ConversationID), drafts[i].Content)
	}
	return drafts, nil
}

// DeleteConversationDraft deletes a draft for a conversation by ID or UUID. An empty draftType deletes all types.
func (m *Manager) DeleteConversationDraft(tenantID, conversationID int, uuid string, userID int, draftType string) error {
	var uuidParam any
	if uuid != "" {
		uuidParam = uuid
	}

	if _, err := m.q.DeleteConversationDraft.Exec(tenantID, conversationID, uuidParam, userID, draftType); err != nil {
		m.lo.Error("error deleting conversation draft", "conversation_id", conversationID, "uuid", uuid, "user_id", userID, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	return nil
}

// DeleteStaleDrafts deletes drafts older than the specified retention period.
func (m *Manager) DeleteStaleDrafts(ctx context.Context, retentionPeriod time.Duration) error {
	cutoff := time.Now().Add(-retentionPeriod)
	res, err := m.q.DeleteStaleDrafts.ExecContext(ctx, cutoff)
	if err != nil {
		m.lo.Error("error deleting stale drafts", "error", err)
		return err
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected > 0 {
		m.lo.Info("deleted stale drafts", "count", rowsAffected)
	}

	return nil
}

// resolveDraftInlineCIDs rewrites inline cid: refs to media URLs, resolving only unattached media or media linked to the draft's own conversation.
func (m *Manager) resolveDraftInlineCIDs(tenantID, conversationID int, content string) string {
	cids := extractInlineContentIDs(content)
	for _, cid := range cids {
		uuid := strings.TrimPrefix(cid, "ldsk-")
		if uuid == "" {
			continue
		}
		media, err := m.mediaStore.GetDraftInlineMedia(tenantID, uuid, conversationID)
		if err != nil {
			continue
		}
		content = strings.ReplaceAll(content, "cid:"+cid, m.mediaStore.GetURL(tenantID, media.UUID, media.ContentType, media.Filename))
	}
	return content
}
