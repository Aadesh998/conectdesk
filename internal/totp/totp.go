// Package totp implements time-based one-time passwords (RFC 6238) as
// authenticator apps show them: HMAC-SHA1, 30 second steps, 6 digits.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// Step is how long a code is valid.
	Step = 30 * time.Second
	// Digits is the length of a code.
	Digits = 6
	// Skew is how many steps before and after now a code is accepted, for
	// clocks that drift.
	Skew = 1
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a random 160-bit secret in base32, as authenticator apps take it.
func NewSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

// URI returns the otpauth:// URI an authenticator app scans, labelled
// "issuer (account)".
func URI(secret, issuer, account string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprint(Digits))
	v.Set("period", fmt.Sprint(int(Step.Seconds())))
	label := url.PathEscape(issuer + ":" + account)
	return "otpauth://totp/" + label + "?" + v.Encode()
}

// Code returns the code for the step t falls in.
func Code(secret string, t time.Time) (string, error) {
	return codeAt(secret, uint64(t.Unix())/uint64(Step.Seconds()))
}

// Verify checks code against the steps around t and returns the step it
// matched, so the caller can refuse a code used before (step <= the last used
// one). ok is false for a wrong code.
func Verify(secret, code string, t time.Time) (step uint64, ok bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != Digits {
		return 0, false
	}
	now := uint64(t.Unix()) / uint64(Step.Seconds())
	for d := -Skew; d <= Skew; d++ {
		s := now + uint64(d)
		want, err := codeAt(secret, s)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

func codeAt(secret string, step uint64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil {
		return "", fmt.Errorf("invalid secret: %w", err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], step)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", Digits, bin%1000000), nil
}
