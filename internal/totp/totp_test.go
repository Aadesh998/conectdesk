package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// RFC 6238 appendix B, SHA-1, with the last 6 of its 8 digits.
func TestRFC6238(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for unix, want := range map[int64]string{
		59:          "287082",
		1111111109:  "081804",
		1111111111:  "050471",
		1234567890:  "005924",
		2000000000:  "279037",
		20000000000: "353130",
	} {
		got, err := Code(secret, time.Unix(unix, 0))
		if err != nil || got != want {
			t.Errorf("Code(%d) = %q, %v; want %q", unix, got, err, want)
		}
	}
}

func TestVerify(t *testing.T) {
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0)
	code, _ := Code(secret, now)
	step, ok := Verify(secret, code, now)
	if !ok || step != uint64(now.Unix())/30 {
		t.Fatalf("current code: step %d ok %v", step, ok)
	}
	if _, ok := Verify(secret, code[:3]+" "+code[3:], now); !ok {
		t.Fatal("a code typed with a space is refused")
	}
	if _, ok := Verify(secret, code, now.Add(30*time.Second)); !ok {
		t.Fatal("the previous step's code is refused")
	}
	if _, ok := Verify(secret, code, now.Add(90*time.Second)); ok {
		t.Fatal("a code three steps old is accepted")
	}
	if _, ok := Verify(secret, "000000", now); ok && code != "000000" {
		t.Fatal("a wrong code is accepted")
	}
	if _, ok := Verify(secret, "12345", now); ok {
		t.Fatal("a short code is accepted")
	}
	uri := URI(secret, "Acme Desk", "alice@acme.io")
	if !strings.HasPrefix(uri, "otpauth://totp/Acme%20Desk:alice@acme.io?") || !strings.Contains(uri, "secret="+secret) {
		t.Fatalf("URI = %s", uri)
	}
}
