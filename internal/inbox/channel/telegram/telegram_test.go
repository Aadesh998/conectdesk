package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/conectdesk/conectdesk/internal/attachment"
	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/zerodha/logf"
)

const token = "123456:SECRET-TOKEN"

type fakeAPI struct {
	mu    sync.Mutex
	calls []string
}

func newTestInbox(t *testing.T, f *fakeAPI) *Telegram {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.calls = append(f.calls, r.URL.Path+" "+string(body))
		f.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/getFile"):
			w.Write([]byte(`{"ok": true, "result": {"file_path": "photos/file_1.jpg", "file_size": 3}}`))
		case strings.HasPrefix(r.URL.Path, "/file/bot"+token+"/"):
			w.Write([]byte("jpg"))
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			w.Write([]byte(`{"ok": true, "result": {"id": 42, "is_bot": true, "first_name": "Acme", "username": "acme_bot"}}`))
		case strings.HasSuffix(r.URL.Path, "/bad"):
			w.Write([]byte(`{"ok": false, "error_code": 401, "description": "Unauthorized"}`))
		default:
			w.Write([]byte(`{"ok": true, "result": {"message_id": 1}}`))
		}
	}))
	t.Cleanup(srv.Close)
	lo := logf.New(logf.Opts{})
	tg, err := New(Opts{ID: 5, TenantID: 2, Config: Config{AccessToken: token, BotID: 42, Username: "acme_bot"}, Lo: &lo})
	if err != nil {
		t.Fatal(err)
	}
	tg.api.SetBaseURL(srv.URL)
	return tg
}

func update(t *testing.T, s string) Update {
	t.Helper()
	var u Update
	if err := json.Unmarshal([]byte(s), &u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestParseText(t *testing.T) {
	tg := newTestInbox(t, &fakeAPI{})
	in, err := tg.ParseUpdate(context.Background(), update(t, `{"update_id": 1, "message": {
		"message_id": 7, "date": 1, "text": "My order is late",
		"from": {"id": 1001, "first_name": "Asha", "last_name": "Rao", "username": "asha"},
		"chat": {"id": 1001, "type": "private"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if in.TenantID != 2 || in.InboxID != 5 || in.Channel != ChannelTelegram || in.Content != "My order is late" {
		t.Errorf("message: %+v", in)
	}
	if in.SourceID.String != "telegram:42:1001:7" || in.Contact.ExternalUserID.String != "telegram:1001" || in.Contact.FirstName != "Asha" {
		t.Errorf("ids: %q %+v", in.SourceID.String, in.Contact)
	}
}

func TestParseIgnoredAndStart(t *testing.T) {
	tg := newTestInbox(t, &fakeAPI{})
	for name, raw := range map[string]string{
		"group":  `{"message": {"message_id": 1, "text": "hi", "from": {"id": 1}, "chat": {"id": -100, "type": "supergroup"}}}`,
		"bot":    `{"message": {"message_id": 1, "text": "hi", "from": {"id": 9, "is_bot": true}, "chat": {"id": 9, "type": "private"}}}`,
		"edited": `{"edited_message": {"message_id": 1}}`,
	} {
		_, err := tg.ParseUpdate(context.Background(), update(t, raw))
		var ignored meta.ErrIgnored
		if !errors.As(err, &ignored) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	in, err := tg.ParseUpdate(context.Background(), update(t, `{"message": {"message_id": 1, "text": "/start", "from": {"id": 1, "first_name": "A"}, "chat": {"id": 1, "type": "private"}}}`))
	if err != nil || in.Content != "[started the chat]" {
		t.Errorf("start: %q %v", in.Content, err)
	}
}

func TestParsePhotoDownloadsLargest(t *testing.T) {
	f := &fakeAPI{}
	tg := newTestInbox(t, f)
	in, err := tg.ParseUpdate(context.Background(), update(t, `{"message": {"message_id": 2, "caption": "see this",
		"photo": [{"file_id": "small", "file_size": 1}, {"file_id": "large", "file_size": 3}],
		"from": {"id": 1, "first_name": "A"}, "chat": {"id": 1, "type": "private"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Attachments) != 1 || string(in.Attachments[0].Content) != "jpg" || in.Attachments[0].ContentType != "image/jpeg" || in.Content != "see this" {
		t.Fatalf("got %+v", in)
	}
	if !strings.Contains(f.calls[0], `"file_id":"large"`) {
		t.Errorf("downloaded %s", f.calls[0])
	}
}

func TestSend(t *testing.T) {
	f := &fakeAPI{}
	tg := newTestInbox(t, f)
	err := tg.Send(models.OutboundMessage{
		UUID: "u", To: []string{"1001"}, Content: "<p>Here is your <b>invoice</b></p>",
		Attachments: attachment.Attachments{{Name: "inv.pdf", ContentType: "application/pdf", Content: []byte("%PDF")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// A short reply rides on the file as its caption.
	if len(f.calls) != 1 || !strings.HasSuffix(strings.Fields(f.calls[0])[0], "/sendDocument") || !strings.Contains(f.calls[0], "Here is your <b>invoice</b>") || !strings.Contains(f.calls[0], "HTML") {
		t.Fatalf("calls: %v", f.calls)
	}
	if err := tg.Send(models.OutboundMessage{UUID: "u", To: []string{"not-a-chat"}, Content: "x"}); err == nil {
		t.Error("sent to a bad chat id")
	}
}

func TestErrorsHideToken(t *testing.T) {
	f := &fakeAPI{}
	tg := newTestInbox(t, f)
	err := tg.api.Call(context.Background(), "bad", map[string]any{}, nil)
	var te *Error
	if !errors.As(err, &te) || te.Description != "Unauthorized" {
		t.Fatalf("got %v", err)
	}
	c := NewClient(token)
	c.SetBaseURL("http://127.0.0.1:1")
	if err := c.Call(context.Background(), "getMe", map[string]any{}, nil); err == nil || strings.Contains(err.Error(), "SECRET-TOKEN") {
		t.Fatalf("error leaks the token: %v", err)
	}
}

func TestVerifySecret(t *testing.T) {
	if !VerifySecret("abc", "abc") || VerifySecret("abd", "abc") || VerifySecret("", "") || VerifySecret("x", "") {
		t.Error("VerifySecret")
	}
}

func TestGetMe(t *testing.T) {
	tg := newTestInbox(t, &fakeAPI{})
	bot, err := tg.api.GetMe(context.Background())
	if err != nil || bot.ID != 42 || bot.Username != "acme_bot" || !bot.IsBot {
		t.Fatalf("got %+v %v", bot, err)
	}
}

func TestGroupsAndButtons(t *testing.T) {
	f := &fakeAPI{}
	tg := newTestInbox(t, f)
	group := `{"message": {"message_id": 3, "text": "Any update?", "from": {"id": 1001, "first_name": "Asha"}, "chat": {"id": -100555, "type": "supergroup", "title": "Acme customers"}}}`

	if _, err := tg.ParseUpdate(context.Background(), update(t, group)); err == nil {
		t.Fatal("group message taken with groups off")
	}
	tg.config.AllowGroups = true
	in, err := tg.ParseUpdate(context.Background(), update(t, group))
	if err != nil {
		t.Fatal(err)
	}
	if in.Content != "Asha: Any update?" || in.Contact.FirstName != "Acme customers" || in.Contact.ExternalUserID.String != "telegram:-100555" {
		t.Errorf("group: %q %+v", in.Content, in.Contact)
	}

	// A tap on a quick reply comes back as the customer's answer.
	in, err = tg.ParseUpdate(context.Background(), update(t, `{"callback_query": {"id": "cb1", "data": "r:Yes, cancel it",
		"from": {"id": 1001, "first_name": "Asha"}, "message": {"message_id": 9, "chat": {"id": 1001, "type": "private"}}}}`))
	if err != nil || in.Content != "Yes, cancel it" || in.SourceID.String != "telegram:42:cb:cb1" {
		t.Fatalf("tap: %+v %v", in, err)
	}
	if !strings.Contains(strings.Join(f.calls, " "), "answerCallbackQuery") {
		t.Error("tap not acknowledged")
	}
}

func TestSendButtons(t *testing.T) {
	f := &fakeAPI{}
	tg := newTestInbox(t, f)
	meta, _ := json.Marshal(map[string]any{MetaButtonsKey: []Button{{Text: "Yes"}, {Text: "Track", URL: "https://acme.example/t/1"}}})
	if err := tg.Send(models.OutboundMessage{UUID: "u", To: []string{"1001"}, Content: "<p>Cancel?</p>", Meta: meta}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || !strings.Contains(f.calls[0], `"callback_data":"r:Yes"`) || !strings.Contains(f.calls[0], `"url":"https://acme.example/t/1"`) {
		t.Fatalf("calls: %v", f.calls)
	}
	if ValidateButtons([]Button{{Text: strings.Repeat("x", 61)}}) == nil || ValidateButtons([]Button{{Text: "a", URL: "javascript:x"}}) == nil || ValidateButtons([]Button{{Text: "ok"}}) != nil {
		t.Error("ValidateButtons")
	}
}

func TestBusiness(t *testing.T) {
	f := &fakeAPI{}
	tg := newTestInbox(t, f)

	link, enabled, ok := BusinessUpdate(update(t, `{"business_connection": {"id": "BC1", "user": {"id": 777, "first_name": "Acme", "username": "acme"}, "can_reply": true, "is_enabled": true}}`))
	if !ok || !enabled || link.ConnectionID != "BC1" || link.UserID != 777 || !link.CanReply {
		t.Fatalf("link: %+v %v %v", link, enabled, ok)
	}
	biz := `{"business_message": {"message_id": 4, "business_connection_id": "BC1", "text": "Hello shop", "from": {"id": 1001, "first_name": "Asha"}, "chat": {"id": 1001, "type": "private"}}}`

	// Until the business links the bot, its messages are not ours.
	if _, err := tg.ParseUpdate(context.Background(), update(t, biz)); err == nil {
		t.Fatal("business message taken with no link")
	}
	tg.config.Business = &link
	in, err := tg.ParseUpdate(context.Background(), update(t, biz))
	if err != nil || in.Contact.ExternalUserID.String != "telegram:biz:1001" || in.SourceID.String != "telegram:42:biz:1001:4" {
		t.Fatalf("business message: %+v %v", in, err)
	}
	// The owner's own replies from their phone are skipped.
	owner := strings.Replace(biz, `"from": {"id": 1001`, `"from": {"id": 777`, 1)
	if _, err := tg.ParseUpdate(context.Background(), update(t, owner)); err == nil {
		t.Error("owner's message taken")
	}

	// Replies to a business customer go through the connection.
	if err := tg.Send(models.OutboundMessage{UUID: "u", To: []string{"biz:1001"}, Content: "Hi!"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.calls[len(f.calls)-1], `"business_connection_id":"BC1"`) {
		t.Errorf("send: %s", f.calls[len(f.calls)-1])
	}
	tg.config.Business = nil
	if err := tg.Send(models.OutboundMessage{UUID: "u", To: []string{"biz:1001"}, Content: "Hi!"}); err == nil {
		t.Error("sent to a business customer with no link")
	}
}

func TestSendTyping(t *testing.T) {
	f := &fakeAPI{}
	tg := newTestInbox(t, f)
	tg.SendTyping("1001")
	if len(f.calls) != 1 || !strings.Contains(f.calls[0], "sendChatAction") || !strings.Contains(f.calls[0], `"action":"typing"`) {
		t.Fatalf("calls: %v", f.calls)
	}
}
