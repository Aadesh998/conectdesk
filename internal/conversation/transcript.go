package conversation

import (
	"fmt"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/stringutil"
)

const (
	transcriptTimeFormat = "Jan 02, 2006 03:04 PM MST"
	transcriptSeparator  = "------------------------------------------------------------"
)

func (m *Manager) BuildTranscript(conversation models.Conversation, messages []models.Message, downloadedAt time.Time) []byte {
	tr := m.i18n.For(conversation.TenantID)
	var b strings.Builder
	fmt.Fprintf(&b, "%s #%s\n", tr.T("globals.terms.conversation"), conversation.ReferenceNumber)
	if subject := conversation.Subject.String; subject != "" {
		fmt.Fprintf(&b, "%s: %s\n", tr.T("globals.terms.subject"), subject)
	}
	if contact := conversation.Contact.FullName(); contact != "" {
		fmt.Fprintf(&b, "%s: %s\n", tr.T("globals.terms.contact"), contact)
	}
	fmt.Fprintf(&b, "%s: %s\n", tr.T("globals.terms.createdAt"), conversation.CreatedAt.UTC().Format(transcriptTimeFormat))
	fmt.Fprintf(&b, "%s: %s\n\n", tr.T("globals.terms.downloadedAt"), downloadedAt.UTC().Format(transcriptTimeFormat))
	b.WriteString(transcriptSeparator + "\n")

	for _, message := range messages {
		content := message.TextContent
		if content == "" {
			content = stringutil.HTML2Text(message.Content)
		}
		fmt.Fprintf(&b, "\n[%s] %s (%s):\n%s\n",
			message.CreatedAt.UTC().Format(transcriptTimeFormat),
			message.Author.FullName(),
			m.senderTypeLabel(conversation.TenantID, message.SenderType),
			content,
		)
		if len(message.Attachments) > 0 {
			names := make([]string, 0, len(message.Attachments))
			for _, attachment := range message.Attachments {
				names = append(names, attachment.Name)
			}
			fmt.Fprintf(&b, "%s: %s\n", tr.Tc("globals.terms.attachment", len(message.Attachments)), strings.Join(names, ", "))
		}
	}
	return []byte(b.String())
}

func (m *Manager) senderTypeLabel(tenantID int, senderType string) string {
	switch senderType {
	case models.SenderTypeAgent:
		return m.i18n.For(tenantID).T("globals.terms.agent")
	case models.SenderTypeContact:
		return m.i18n.For(tenantID).T("globals.terms.contact")
	}
	return senderType
}
