package slack

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/chatmsg"
)

type keys struct{}

func (keys) T(k string) string { return k }

func sign(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + ts + ":"))
	mac.Write(body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"type":"event_callback"}`)
	good := sign("s3cret", ts, body)

	if !VerifySignature("s3cret", body, ts, good, now) {
		t.Error("a good signature is rejected")
	}
	if VerifySignature("other", body, ts, good, now) {
		t.Error("a signature from another secret is accepted")
	}
	if VerifySignature("s3cret", []byte(`{"type":"x"}`), ts, good, now) {
		t.Error("a changed body is accepted")
	}
	if VerifySignature("s3cret", body, ts, good, now.Add(6*time.Minute)) {
		t.Error("an old request is accepted")
	}
	if VerifySignature("", body, ts, good, now) {
		t.Error("a request is accepted with no signing secret set")
	}
}

func TestIsThreadReply(t *testing.T) {
	reply := Event{Type: "message", User: "U1", TS: "2.0", ThreadTS: "1.0"}
	if !reply.IsThreadReply("UBOT") {
		t.Error("a person's thread reply is ignored")
	}
	for name, e := range map[string]Event{
		"top level":   {Type: "message", User: "U1", TS: "1.0"},
		"thread root": {Type: "message", User: "U1", TS: "1.0", ThreadTS: "1.0"},
		"bot":         {Type: "message", User: "U1", BotID: "B1", TS: "2.0", ThreadTS: "1.0"},
		"own post":    {Type: "message", User: "UBOT", TS: "2.0", ThreadTS: "1.0"},
		"edit":        {Type: "message", Subtype: "message_changed", TS: "2.0", ThreadTS: "1.0"},
	} {
		if e.IsThreadReply("UBOT") {
			t.Errorf("%s is taken as a reply", name)
		}
	}
}

func TestTextToHTML(t *testing.T) {
	cases := map[string]string{
		"Refund sent &amp; logged":             "<p>Refund sent &amp; logged</p>",
		"see <https://x.test/a|the order> now": "<p>see the order (https://x.test/a) now</p>",
		"<https://x.test>":                     "<p>https://x.test</p>",
		"thanks <@U123|sam>, <!here>":          "<p>thanks @sam, @here</p>",
		"a &lt;script&gt;\nb\n\nc":             "<p>a &lt;script&gt;<br>b</p><p>c</p>",
	}
	for in, want := range cases {
		if got := TextToHTML(in); got != want {
			t.Errorf("TextToHTML(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReplyModalRoundTrip(t *testing.T) {
	view := ReplyModal(keys{}, 3, 42, "C1", "111.222", "Conversation #9")
	if view["private_metadata"] != "3|42|C1|111.222" {
		t.Fatalf("metadata = %v", view["private_metadata"])
	}
	payload := `{"type":"view_submission","user":{"id":"U1"},"team":{"id":"T1"},"view":{"callback_id":"cd_reply_modal","private_metadata":"3|42|C1|111.222",
		"state":{"values":{"cd_mode":{"mode":{"selected_option":{"value":"note"}}},"cd_text":{"text":{"value":"  hello  "}}}}}}`
	in, err := ParseInteraction(payload)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := in.ReplyInput()
	if !ok || r.IntegrationID != 3 || r.ConversationID != 42 || r.Channel != "C1" || r.ThreadTS != "111.222" || r.Mode != ModeNote || r.Text != "hello" {
		t.Errorf("reply input = %+v, %v", r, ok)
	}
}

func TestRootBlocksHaveActions(t *testing.T) {
	blocks := RootBlocks(keys{}, chatmsg.Post{Title: "T", ConversationID: 42, Link: "https://desk.test/c", LinkText: "open"})
	b, _ := json.Marshal(blocks)
	for _, id := range []string{ActionAssign, ActionReply, ActionResolve, "cd_open"} {
		if !strings.Contains(string(b), `"action_id":"`+id+`"`) {
			t.Errorf("no %s button: %s", id, b)
		}
	}
	if !strings.Contains(string(b), `"value":"42"`) {
		t.Errorf("buttons don't carry the conversation: %s", b)
	}
}

// TestClient checks the calls the app makes reach Slack's methods with the
// bot token, and that Slack's errors come back as errors.
func TestClient(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/api/")
		calls = append(calls, method+" "+r.Header.Get("Authorization"))
		body, _ := io.ReadAll(r.Body)
		switch method {
		case "oauth.v2.access":
			form, _ := url.ParseQuery(string(body))
			if form.Get("code") != "c0de" || form.Get("client_secret") != "cs" {
				w.Write([]byte(`{"ok":false,"error":"invalid_code"}`))
				return
			}
			w.Write([]byte(`{"ok":true,"access_token":"xoxb-1","bot_user_id":"UBOT","team":{"id":"T1","name":"Acme"},"incoming_webhook":{"channel":"#support","channel_id":"C1"}}`))
		case "chat.postMessage":
			var msg Message
			json.Unmarshal(body, &msg)
			if msg.Channel != "C1" {
				w.Write([]byte(`{"ok":false,"error":"channel_not_found"}`))
				return
			}
			w.Write([]byte(`{"ok":true,"ts":"123.456"}`))
		case "users.info":
			w.Write([]byte(`{"ok":true,"user":{"id":"U1","profile":{"email":"sam@acme.test"}}}`))
		case "reactions.add":
			w.Write([]byte(`{"ok":false,"error":"already_reacted"}`))
		default:
			w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer srv.Close()

	c := New(App{ClientID: "ci", ClientSecret: "cs", SigningSecret: "ss"}, srv.Client(), srv.URL+"/api")
	ctx := context.Background()

	inst, err := c.ExchangeCode(ctx, "c0de", "https://desk.test/cb")
	if err != nil || inst.AccessToken != "xoxb-1" || inst.Team.ID != "T1" || inst.IncomingWebhook.ChannelID != "C1" || inst.BotUserID != "UBOT" {
		t.Fatalf("install = %+v, %v", inst, err)
	}
	if _, err := c.ExchangeCode(ctx, "bad", "https://desk.test/cb"); !IsCode(err, "invalid_code") {
		t.Errorf("bad code error = %v", err)
	}
	ts, err := c.PostMessage(ctx, "xoxb-1", Message{Channel: "C1", Text: "hi"})
	if err != nil || ts != "123.456" {
		t.Errorf("post = %q, %v", ts, err)
	}
	if _, err := c.PostMessage(ctx, "xoxb-1", Message{Channel: "C9", Text: "hi"}); !IsCode(err, "channel_not_found") {
		t.Errorf("post to a missing channel error = %v", err)
	}
	u, err := c.UserInfo(ctx, "xoxb-1", "U1")
	if err != nil || u.Profile.Email != "sam@acme.test" {
		t.Errorf("user = %+v, %v", u, err)
	}
	if err := c.AddReaction(ctx, "xoxb-1", "C1", "1.0", "white_check_mark"); err != nil {
		t.Errorf("reacting twice is an error: %v", err)
	}
	if calls[2] != "chat.postMessage Bearer xoxb-1" {
		t.Errorf("post call = %q", calls[2])
	}

	if u := c.AuthorizeURL("https://desk.test/cb", "st"); !strings.Contains(u, "client_id=ci") || !strings.Contains(u, "incoming-webhook") || !strings.Contains(u, "state=st") {
		t.Errorf("authorize url = %s", u)
	}
	if err := c.Respond(ctx, "https://evil.test/hook", map[string]any{}); err == nil {
		t.Error("answered a response url that isn't Slack's")
	}
}
