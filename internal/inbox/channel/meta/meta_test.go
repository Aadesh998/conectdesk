package meta

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"object":"page","entry":[]}`)
	good := sign("s3cret", body)

	cases := []struct {
		name   string
		secret string
		body   []byte
		header string
		want   bool
	}{
		{"valid", "s3cret", body, good, true},
		{"other secret", "other", body, good, false},
		{"tampered body", "s3cret", []byte(`{"object":"page","entry":[{}]}`), good, false},
		{"no prefix", "s3cret", body, strings.TrimPrefix(good, "sha256="), false},
		{"sha1 header", "s3cret", body, "sha1=" + strings.TrimPrefix(good, "sha256="), false},
		{"not hex", "s3cret", body, "sha256=zz", false},
		{"empty header", "s3cret", body, "", false},
		// An app with no secret configured must reject everything, not accept it.
		{"no secret", "", body, sign("", body), false},
	}
	for _, c := range cases {
		if got := VerifySignature(c.secret, c.body, c.header); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestVerifyChallenge(t *testing.T) {
	if got, ok := VerifyChallenge("tok", "subscribe", "tok", "12345"); !ok || got != "12345" {
		t.Fatalf("valid challenge: got %q %v", got, ok)
	}
	for _, c := range [][4]string{
		{"tok", "subscribe", "wrong", "1"},
		{"tok", "unsubscribe", "tok", "1"},
		{"tok", "subscribe", "tok", ""},
		{"", "subscribe", "", "1"},
	} {
		if _, ok := VerifyChallenge(c[0], c[1], c[2], c[3]); ok {
			t.Errorf("accepted %v", c)
		}
	}
}

func signedRequest(secret, payload string) string {
	p := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(p))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) + "." + p
}

func TestParseSignedRequest(t *testing.T) {
	sr, err := ParseSignedRequest("s3cret", signedRequest("s3cret", `{"algorithm":"HMAC-SHA256","issued_at":1,"user_id":"42"}`))
	if err != nil || sr.UserID != "42" {
		t.Fatalf("valid request: %+v %v", sr, err)
	}

	bad := []string{
		signedRequest("other", `{"algorithm":"HMAC-SHA256","user_id":"42"}`),
		signedRequest("s3cret", `{"algorithm":"none","user_id":"42"}`),
		signedRequest("s3cret", `{"algorithm":"HMAC-SHA256"}`),
		"no-dot",
		"",
	}
	for _, b := range bad {
		if _, err := ParseSignedRequest("s3cret", b); err == nil {
			t.Errorf("accepted %q", b)
		}
	}
	if _, err := ParseSignedRequest("", signedRequest("", `{"algorithm":"HMAC-SHA256","user_id":"42"}`)); err == nil {
		t.Error("accepted a request with no app secret configured")
	}
}

func TestIsMetaMediaHost(t *testing.T) {
	for _, h := range []string{"scontent.xx.fbcdn.net", "lookaside.fbsbx.com", "scontent.cdninstagram.com", "fbcdn.net", "LOOKASIDE.FBSBX.COM."} {
		if !IsMetaMediaHost(h) {
			t.Errorf("%s should be a meta media host", h)
		}
	}
	for _, h := range []string{"evilfbcdn.net", "fbcdn.net.evil.com", "localhost", "169.254.169.254", ""} {
		if IsMetaMediaHost(h) {
			t.Errorf("%s should not be a meta media host", h)
		}
	}
}

func TestSplitText(t *testing.T) {
	if got := SplitText("", 10); got != nil {
		t.Fatalf("empty: %v", got)
	}
	got := SplitText("hello world again", 11)
	if len(got) != 2 || got[0] != "hello world" || got[1] != "again" {
		t.Fatalf("split on space: %q", got)
	}
	// A cut without a break must not land inside a multi-byte rune.
	for _, chunk := range SplitText(strings.Repeat("é", 10), 5) {
		if !strings.HasPrefix(chunk, "é") || strings.ContainsRune(chunk, '�') {
			t.Fatalf("broke a rune: %q", chunk)
		}
	}
}

func TestIsOutsideWindow(t *testing.T) {
	if !IsOutsideWindow(&Error{Code: 10, Subcode: 2018278}) || !IsOutsideWindow(&Error{Code: 10, Subcode: 2534022}) {
		t.Error("window errors not recognised")
	}
	if IsOutsideWindow(&Error{Code: 100}) || IsOutsideWindow(errors.New("x")) {
		t.Error("unrelated error taken for a window error")
	}
}

func TestAuthorizeURL(t *testing.T) {
	c := NewClient(App{AppID: "123", AppSecret: "s", WhatsAppConfigID: "cfg"})
	u, err := c.AuthorizeURL(ProductWhatsApp, "https://example.com/oauth/meta/callback", "state1")
	if err != nil || !strings.Contains(u, "config_id=cfg") || !strings.Contains(u, "state=state1") {
		t.Fatalf("whatsapp url: %s %v", u, err)
	}
	u, err = c.AuthorizeURL(ProductInstagram, "https://example.com/cb", "s")
	if err != nil || !strings.Contains(u, "instagram_manage_messages") {
		t.Fatalf("instagram url: %s %v", u, err)
	}
	if _, err := NewClient(App{AppID: "1", AppSecret: "s"}).AuthorizeURL(ProductWhatsApp, "x", "y"); err == nil {
		t.Error("whatsapp without a config id should fail")
	}
}

func TestProducts(t *testing.T) {
	if got := (App{}).Products(); len(got) != 0 {
		t.Errorf("unconfigured app: %v", got)
	}
	if got := (App{AppID: "1", AppSecret: "s"}).Products(); strings.Join(got, ",") != "messenger,instagram" {
		t.Errorf("without whatsapp config: %v", got)
	}
	if got := (App{AppID: "1", AppSecret: "s", WhatsAppConfigID: "c"}).Products(); strings.Join(got, ",") != "whatsapp,messenger,instagram" {
		t.Errorf("with whatsapp config: %v", got)
	}
}
