package email

import (
	"strings"
	"testing"

	"github.com/conectdesk/conectdesk/internal/inbox/models"
)

// TestRefreshPlatformApp checks an inbox connected through the platform app
// refreshes with the app's credentials from config, not its own copy, and
// fails plainly when the app is gone.
func TestRefreshPlatformApp(t *testing.T) {
	defer SetPlatformOAuthApp(nil)
	tok := &models.OAuthConfig{Provider: "google", RefreshToken: "rt", ClientID: "platform-id", Platform: true}

	SetPlatformOAuthApp(nil)
	if _, err := RefreshOAuthConfig(tok); err == nil || !strings.Contains(err.Error(), "isn't configured") {
		t.Fatalf("no lookup: %v", err)
	}
	SetPlatformOAuthApp(func(string) (string, string, string, bool) { return "", "", "", false })
	if _, err := RefreshOAuthConfig(tok); err == nil || !strings.Contains(err.Error(), "isn't configured") {
		t.Fatalf("app removed from config: %v", err)
	}

	// An inbox on its own app never uses the platform's.
	used := false
	SetPlatformOAuthApp(func(string) (string, string, string, bool) { used = true; return "p", "s", "", true })
	own := &models.OAuthConfig{Provider: "google", RefreshToken: "rt"}
	if _, err := RefreshOAuthConfig(own); err == nil || used {
		t.Fatalf("own-app inbox without credentials: err=%v, used platform app=%v", err, used)
	}
}
