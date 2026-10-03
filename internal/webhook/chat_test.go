package webhook

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/conectdesk/conectdesk/internal/webhook/models"
	"github.com/zerodha/logf"
)

func TestValidateURL(t *testing.T) {
	cases := []struct {
		kind, url string
		ok        bool
	}{
		{models.KindCustom, "https://example.com/hook", true},
		{models.KindSlack, "https://hooks.slack.com/services/T0/B0/xyz", true},
		{models.KindSlack, "https://hooks.slack-gov.com/services/T0/B0/xyz", true},
		{models.KindSlack, "http://hooks.slack.com/services/T0/B0/xyz", false},
		{models.KindSlack, "https://hooks.slack.com/triggers/T0/1/abc", false},
		{models.KindSlack, "https://hooks.slack.com.evil.com/services/x", false},
		{models.KindTeams, "https://prod-01.westus.logic.azure.com:443/workflows/abc/triggers/manual/paths/invoke", true},
		{models.KindTeams, "https://default123.4a.environment.api.powerplatform.com:443/powerautomate/automations/direct/workflows/abc", true},
		{models.KindTeams, "https://contoso.webhook.office.com/webhookb2/abc", true},
		{models.KindTeams, "https://example.com/logic.azure.com", false},
		{models.KindTeams, "http://prod-01.westus.logic.azure.com/workflows/abc", false},
		{"discord", "https://discord.com/api/webhooks/1", false},
	}
	for _, c := range cases {
		if err := ValidateURL(c.kind, c.url); (err == nil) != c.ok {
			t.Errorf("ValidateURL(%q, %q) = %v, want ok=%v", c.kind, c.url, err, c.ok)
		}
	}
}

func testConversation() map[string]any {
	return map[string]any{
		"uuid":             "c-1",
		"reference_number": "42",
		"subject":          "Refund <please>",
		"inbox_name":       "Support",
		"contact":          map[string]any{"first_name": "Jane", "last_name": "Doe"},
	}
}

func TestMaskURL(t *testing.T) {
	cases := map[string]string{
		"https://hooks.slack.com/services/T0/B0/xyz":                       "https://hooks.slack.com/services/",
		"https://prod-01.westus.logic.azure.com:443/workflows/abc/x?sig=1": "https://prod-01.westus.logic.azure.com:443/workflows/",
	}
	for in, prefix := range cases {
		got := MaskURL(in)
		if !strings.HasPrefix(got, prefix) || strings.Contains(got, "xyz") || strings.Contains(got, "sig") || !IsMaskedURL(got) {
			t.Errorf("MaskURL(%q) = %q", in, got)
		}
		// The masked URL still passes the kind check, so saving the form unchanged works.
		kind := models.KindSlack
		if strings.Contains(in, "azure") {
			kind = models.KindTeams
		}
		if err := ValidateURL(kind, got); err != nil {
			t.Errorf("masked %q fails validation: %v", got, err)
		}
	}
	if IsMaskedURL("https://hooks.slack.com/services/T0/B0/xyz") {
		t.Error("a real URL reads as masked")
	}
}

func TestChatBody(t *testing.T) {
	raw, _ := json.Marshal(testConversation())
	slack, err := chatBody(keyTranslator{}, models.KindSlack, models.EventConversationCreated, raw, "https://desk.test")
	if err != nil {
		t.Fatal(err)
	}
	if s := string(slack); !strings.Contains(s, `"blocks"`) || !strings.Contains(s, "Refund \\u0026lt;please\\u0026gt;") {
		t.Errorf("slack body = %s", s)
	}

	teams, err := chatBody(keyTranslator{}, models.KindTeams, models.EventConversationCreated, raw, "https://desk.test")
	if err != nil {
		t.Fatal(err)
	}
	var msg struct {
		Type        string `json:"type"`
		Attachments []struct {
			ContentType string `json:"contentType"`
			Content     struct {
				Type    string           `json:"type"`
				Actions []map[string]any `json:"actions"`
			} `json:"content"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(teams, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "message" || len(msg.Attachments) != 1 ||
		msg.Attachments[0].ContentType != "application/vnd.microsoft.card.adaptive" ||
		msg.Attachments[0].Content.Type != "AdaptiveCard" || len(msg.Attachments[0].Content.Actions) != 1 {
		t.Fatalf("not an adaptive card message: %s", teams)
	}
}

// TestDeliverChatWebhook checks a Slack webhook gets a chat message without the
// signature header, that a failing endpoint is reported, and that skipped
// events send nothing.
func TestDeliverChatWebhook(t *testing.T) {
	var (
		got    []byte
		sig    string
		calls  int
		status = http.StatusOK
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		got, _ = io.ReadAll(r.Body)
		sig = r.Header.Get("X-ConectDesk-Signature")
		w.WriteHeader(status)
		w.Write([]byte("no_service"))
	}))
	defer srv.Close()

	lo := logf.New(logf.Opts{})
	m := &Manager{lo: &lo, httpClient: srv.Client(), rootURL: func(int) string { return "https://desk.test" }}
	hook := models.Webhook{ID: 1, URL: srv.URL, Kind: models.KindSlack, Secret: "s3cret"}

	if err := m.deliverSingleWebhook(hook, DeliveryTask{TenantID: 1, Event: models.EventConversationCreated, Payload: testConversation()}); err != nil {
		t.Fatal(err)
	}
	if sig != "" {
		t.Error("Slack delivery carries a signature")
	}
	if !strings.Contains(string(got), `"blocks"`) {
		t.Errorf("Slack delivery isn't a chat message: %s", got)
	}

	if err := m.deliverSingleWebhook(hook, DeliveryTask{TenantID: 1, Event: models.EventMessageUpdated, Payload: map[string]any{}}); err != nil || calls != 1 {
		t.Errorf("message update was sent to Slack (calls=%d, err=%v)", calls, err)
	}

	status = http.StatusNotFound
	err := m.deliverSingleWebhook(hook, DeliveryTask{TenantID: 1, Event: models.EventWebhookTest, Payload: map[string]any{}})
	if err == nil || !strings.Contains(err.Error(), "404 no_service") {
		t.Errorf("failed delivery error = %v", err)
	}
}
