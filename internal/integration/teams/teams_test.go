package teams

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/chatmsg"
	"github.com/golang-jwt/jwt/v5"
)

type keys struct{}

func (keys) T(k string) string { return k }

func TestValidServiceURL(t *testing.T) {
	for u, ok := range map[string]bool{
		"https://smba.trafficmanager.net/amer/":           true,
		"https://smba.trafficmanager.net/teams/":          true,
		"https://europe.botframework.com/":                true,
		"https://smba.infra.gov.teams.microsoft.us/":      true,
		"http://smba.trafficmanager.net/amer/":            false,
		"https://smba.trafficmanager.net.evil.test/amer/": false,
		"https://evil.test/botframework.com":              false,
		"https://user@smba.trafficmanager.net/amer/":      false,
	} {
		if ValidServiceURL(u) != ok {
			t.Errorf("ValidServiceURL(%q) = %v", u, !ok)
		}
	}
}

// keyServer publishes an RSA key the way the Bot Framework does.
func keyServer(t *testing.T, key *rsa.PrivateKey, endorsements []string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openid":
			json.NewEncoder(w).Encode(map[string]any{"jwks_uri": srv.URL + "/keys"})
		case "/keys":
			json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
				"kty": "RSA", "kid": "k1",
				"n":            base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":            base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
				"endorsements": endorsements,
			}}})
		}
	}))
	return srv
}

func token(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "k1"
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + s
}

func TestVerify(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := keyServer(t, key, []string{"msteams"})
	defer srv.Close()

	v := NewVerifier("app-1", srv.Client(), srv.URL+"/openid")
	act := Activity{ChannelID: "msteams", ServiceURL: "https://smba.trafficmanager.net/amer/"}
	claims := func(mod func(jwt.MapClaims)) jwt.MapClaims {
		c := jwt.MapClaims{"iss": botIssuer, "aud": "app-1", "exp": time.Now().Add(time.Hour).Unix(), "serviceurl": act.ServiceURL}
		if mod != nil {
			mod(c)
		}
		return c
	}
	ctx := context.Background()

	if err := v.Verify(ctx, token(t, key, claims(nil)), act); err != nil {
		t.Fatalf("a good token is rejected: %v", err)
	}
	bad := map[string]string{
		"no header":         "",
		"other key":         token(t, other, claims(nil)),
		"other bot":         token(t, key, claims(func(c jwt.MapClaims) { c["aud"] = "app-2" })),
		"other issuer":      token(t, key, claims(func(c jwt.MapClaims) { c["iss"] = "https://evil.test" })),
		"expired":           token(t, key, claims(func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() })),
		"no expiry":         token(t, key, claims(func(c jwt.MapClaims) { delete(c, "exp") })),
		"other service url": token(t, key, claims(func(c jwt.MapClaims) { c["serviceurl"] = "https://evil.test/" })),
	}
	for name, h := range bad {
		if err := v.Verify(ctx, h, act); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := v.Verify(ctx, token(t, key, claims(nil)), Activity{ChannelID: "slack", ServiceURL: act.ServiceURL}); err == nil {
		t.Error("a key endorsed for msteams signed for another channel")
	}
}

// TestClient checks the bot gets a token once, and posts threads, replies and
// member lookups to the service URL with it.
func TestClient(t *testing.T) {
	var tokens atomic.Int32
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token") {
			tokens.Add(1)
			r.ParseForm()
			if r.Form.Get("client_secret") != "pw" || !strings.HasPrefix(r.URL.Path, "/tenant-1/") {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"invalid_client"}`))
				return
			}
			w.Write([]byte(`{"access_token":"bot-token","expires_in":3600}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		got = append(got, r.Method+" "+r.URL.EscapedPath()+" "+r.Header.Get("Authorization"))
		switch {
		case r.URL.Path == "/amer/v3/conversations":
			var req map[string]any
			json.Unmarshal(body, &req)
			if req["channelData"].(map[string]any)["channel"].(map[string]any)["id"] != "19:abc@thread.tacv2" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Write([]byte(`{"id":"19:abc@thread.tacv2;messageid=111","activityId":"111"}`))
		case strings.Contains(r.URL.Path, "/members/"):
			w.Write([]byte(`{"id":"29:u","name":"Sam","email":"sam@acme.test","userPrincipalName":"sam@acme.test"}`))
		default:
			w.Write([]byte(`{"id":"222"}`))
		}
	}))
	defer srv.Close()

	c := New(App{AppID: "app-1", AppPassword: "pw", AppTenantID: "tenant-1"}, srv.Client(), srv.URL)
	c.AllowServiceURLs(func(u string) bool { return strings.HasPrefix(u, srv.URL) })
	ctx := context.Background()
	su := srv.URL + "/amer/"

	thread, err := c.CreateThread(ctx, su, "19:abc@thread.tacv2", TextMessage("hi"))
	if err != nil || thread != "19:abc@thread.tacv2;messageid=111" {
		t.Fatalf("thread = %q, %v", thread, err)
	}
	if err := c.Send(ctx, su, thread, TextMessage("again")); err != nil {
		t.Fatal(err)
	}
	m, err := c.GetMember(ctx, su, thread, "29:u")
	if err != nil || m.Email != "sam@acme.test" {
		t.Fatalf("member = %+v, %v", m, err)
	}
	if n := tokens.Load(); n != 1 {
		t.Errorf("fetched %d tokens, want 1", n)
	}
	if !strings.Contains(got[1], "/v3/conversations/19%3Aabc%40thread.tacv2%3Bmessageid%3D111/activities Bearer bot-token") {
		t.Errorf("reply went to %q", got[1])
	}
	if !strings.Contains(got[2], "/v3/conversations/19%3Aabc%40thread.tacv2/members/29%3Au") {
		t.Errorf("member lookup went to %q", got[2])
	}

	c.AllowServiceURLs(ValidServiceURL)
	if err := c.Send(ctx, "https://evil.test/", thread, TextMessage("x")); err != ErrServiceURL {
		t.Errorf("sent to a service url that isn't microsoft's: %v", err)
	}
}

func TestParseSubmit(t *testing.T) {
	s, ok := ParseSubmit(json.RawMessage(`{"cd_action":"reply","conversation_id":"42","cd_text":"  on it "}`))
	if !ok || s.Action != ActionReply || s.ConversationID != 42 || s.Text != "on it" {
		t.Errorf("submit = %+v, %v", s, ok)
	}
	if _, ok := ParseSubmit(json.RawMessage(`{"cd_action":"reply"}`)); ok {
		t.Error("a submit without a conversation is read")
	}
	if _, ok := ParseSubmit(nil); ok {
		t.Error("an empty value is read as a submit")
	}
}

func TestCleanTextAndLinkCode(t *testing.T) {
	if got := CleanText(`<at>ConectDesk</at> Refund is on its way &amp; logged`); got != "Refund is on its way & logged" {
		t.Errorf("CleanText = %q", got)
	}
	if got := CleanText(`<p><at>ConectDesk</at> line one</p><p>line two</p>`); got != "line one\nline two" {
		t.Errorf("CleanText html = %q", got)
	}
	code, ok := LinkCode(CleanText(`<at>ConectDesk</at> connect ab12cd34`))
	if !ok || code != "AB12CD34" {
		t.Errorf("code = %q, %v", code, ok)
	}
	if _, ok := LinkCode("please connect me"); ok {
		t.Error("read a code out of a sentence")
	}
}

func TestRootCard(t *testing.T) {
	card := RootCard(keys{}, chatmsg.Post{Title: "New", ConversationID: 42, Link: "https://desk.test/c", LinkText: "Open"})
	b, _ := json.Marshal(card)
	for _, want := range []string{`"cd_action":"reply"`, `"cd_action":"note"`, `"cd_action":"assign"`, `"cd_action":"resolve"`, `"conversation_id":"42"`, `"id":"cd_text"`, `"Action.OpenUrl"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("card misses %s: %s", want, b)
		}
	}
}

func TestPackage(t *testing.T) {
	b, err := Package(App{AppID: "11111111-2222-3333-4444-555555555555", Name: "Acme Desk"}, "https://desk.test", "desk.test")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]*zip.File{}
	for _, f := range zr.File {
		names[f.Name] = f
	}
	for _, n := range []string{"manifest.json", "color.png", "outline.png"} {
		if names[n] == nil {
			t.Fatalf("package has no %s", n)
		}
	}
	rc, _ := names["manifest.json"].Open()
	var m struct {
		ID   string `json:"id"`
		Bots []struct {
			BotID string `json:"botId"`
		} `json:"bots"`
		ValidDomains []string `json:"validDomains"`
	}
	json.NewDecoder(rc).Decode(&m)
	if m.ID != "11111111-2222-3333-4444-555555555555" || len(m.Bots) != 1 || m.Bots[0].BotID != m.ID || m.ValidDomains[0] != "desk.test" {
		t.Errorf("manifest = %+v", m)
	}
}
