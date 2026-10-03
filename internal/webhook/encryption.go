package webhook

import (
	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/conectdesk/conectdesk/internal/webhook/models"
)

// encryptSecret encrypts webhook secret if present.
func (m *Manager) encryptSecret(secret string) (string, error) {
	encrypted, err := crypto.Encrypt(secret, m.encryptionKey)
	if err != nil {
		m.lo.Error("error encrypting webhook secret", "error", err)
		return "", err
	}
	return encrypted, nil
}

// decryptWebhook decrypts the webhook secret, and the URL of a Slack or Teams
// webhook, in-place. A value stored in the clear is left as it is.
func (m *Manager) decryptWebhook(webhook *models.Webhook) error {
	decrypted, err := crypto.Decrypt(webhook.Secret, m.encryptionKey)
	if err != nil {
		m.lo.Error("error decrypting webhook secret", "webhook_id", webhook.ID, "error", err)
		return err
	}
	webhook.Secret = decrypted

	url, err := crypto.Decrypt(webhook.URL, m.encryptionKey)
	if err != nil {
		m.lo.Error("error decrypting webhook url", "webhook_id", webhook.ID, "error", err)
		return err
	}
	webhook.URL = url
	return nil
}

// decryptWebhooks decrypts secrets for a slice of webhooks.
func (m *Manager) decryptWebhooks(webhooks []models.Webhook) {
	for i := range webhooks {
		if err := m.decryptWebhook(&webhooks[i]); err != nil {
			m.lo.Error("error decrypting webhook secret", "webhook_id", webhooks[i].ID, "error", err)
			continue
		}
	}
}
