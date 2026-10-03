package webhook

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/conectdesk/conectdesk/internal/chatmsg"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/conectdesk/conectdesk/internal/webhook/models"
)

var (
	ErrSlackURL = errors.New("not a Slack incoming webhook URL")
	ErrTeamsURL = errors.New("not a Microsoft Teams workflow or incoming webhook URL")
	ErrKind     = errors.New("unknown webhook kind")
)

// teamsHosts are the host suffixes Microsoft serves Teams webhooks on: Workflows
// (Power Automate) URLs and the older Office 365 connector URLs.
var teamsHosts = []string{
	".logic.azure.com",
	".powerautomate.com",
	".powerplatform.com",
	".webhook.office.com",
}

// ValidateURL checks that a webhook URL fits its kind, so a pasted URL meant for
// somewhere else is caught when the webhook is saved rather than when it fails.
func ValidateURL(kind, rawURL string) error {
	switch kind {
	case models.KindCustom:
		return nil
	case models.KindSlack:
		u, err := url.Parse(rawURL)
		if err != nil || u.Scheme != "https" || (u.Host != "hooks.slack.com" && u.Host != "hooks.slack-gov.com") ||
			!strings.HasPrefix(u.Path, "/services/") {
			return ErrSlackURL
		}
		return nil
	case models.KindTeams:
		u, err := url.Parse(rawURL)
		if err != nil || u.Scheme != "https" {
			return ErrTeamsURL
		}
		host := strings.ToLower(u.Hostname())
		for _, suffix := range teamsHosts {
			if strings.HasSuffix(host, suffix) {
				return nil
			}
		}
		return ErrTeamsURL
	}
	return ErrKind
}

// IsChatKind reports whether the webhook posts a chat message rather than the raw event.
func IsChatKind(kind string) bool {
	return kind == models.KindSlack || kind == models.KindTeams
}

// MaskURL hides the secret part of a Slack or Teams webhook URL, keeping the
// host and first path segment so the admin can tell which one it is. Anyone
// holding the full URL can post to the channel, so it is never sent back.
func MaskURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return strings.Repeat(stringutil.PasswordDummy, 10)
	}
	first := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 2)[0]
	masked := u.Scheme + "://" + u.Host + "/"
	if first != "" {
		masked += first + "/"
	}
	return masked + strings.Repeat(stringutil.PasswordDummy, 10)
}

// IsMaskedURL reports whether a URL is one MaskURL returned, sent back unchanged.
func IsMaskedURL(rawURL string) bool {
	return strings.Contains(rawURL, stringutil.PasswordDummy)
}

// chatBody builds the request body a Slack or Teams webhook receives for an event.
func chatBody(t chatmsg.Translator, kind string, event models.WebhookEvent, payload []byte, rootURL string) ([]byte, error) {
	post, err := chatmsg.Build(t, event, payload, rootURL)
	if err != nil {
		return nil, err
	}
	if kind == models.KindTeams {
		return json.Marshal(map[string]any{
			"type":        "message",
			"attachments": []map[string]any{chatmsg.CardAttachment(chatmsg.AdaptiveCard(post, nil, nil))},
		})
	}
	// text is the fallback shown in notifications and by clients without blocks.
	return json.Marshal(map[string]any{"text": post.Title, "blocks": chatmsg.SlackBlocks(post)})
}
