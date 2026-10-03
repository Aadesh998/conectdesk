package meta

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Webhook object types, one per product.
const (
	ObjectWhatsApp  = "whatsapp_business_account"
	ObjectPage      = "page"
	ObjectInstagram = "instagram"
)

// ErrIgnored marks an event that parsed fine but carries nothing to store,
// e.g. an echo of our own reply, a read receipt, or a reaction. Callers log it
// and move on.
type ErrIgnored struct{ Reason string }

func (e ErrIgnored) Error() string { return "ignored meta webhook event: " + e.Reason }

// VerifySignature checks the X-Hub-Signature-256 header Meta signs every
// webhook with, an HMAC-SHA256 of the raw body keyed with the app secret.
// Anything unsigned or signed with another key is rejected.
func VerifySignature(appSecret string, body []byte, header string) bool {
	if appSecret == "" || header == "" {
		return false
	}
	sig, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return false
	}
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// VerifyChallenge answers the GET Meta sends when the webhook is set up in the
// App Dashboard. It returns the challenge to echo back, or false if the mode or
// token is wrong.
func VerifyChallenge(verifyToken, mode, token, challenge string) (string, bool) {
	if verifyToken == "" || mode != "subscribe" || challenge == "" {
		return "", false
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(verifyToken)) != 1 {
		return "", false
	}
	return challenge, true
}

// Envelope is the body of every webhook POST. Meta batches events, so one
// envelope can carry several entries, each for one account.
type Envelope struct {
	Object string  `json:"object"`
	Entry  []Entry `json:"entry"`
}

// Entry is the events for one account. WhatsApp puts them in Changes, keyed by
// the phone number in each change's metadata; Messenger and Instagram put them
// in Messaging, and ID is the Page or Instagram account.
type Entry struct {
	ID        string            `json:"id"`
	Time      int64             `json:"time"`
	Changes   []Change          `json:"changes"`
	Messaging []json.RawMessage `json:"messaging"`
}

// Change is one WhatsApp change notification.
type Change struct {
	Field string          `json:"field"`
	Value json.RawMessage `json:"value"`
}

// SignedRequest is the payload of a deauthorize or data deletion callback.
type SignedRequest struct {
	Algorithm string `json:"algorithm"`
	IssuedAt  int64  `json:"issued_at"`
	// UserID is the app-scoped ID of the person who removed the app or asked
	// for their data to be deleted.
	UserID string `json:"user_id"`
}

// ParseSignedRequest verifies and decodes the signed_request Meta posts to the
// deauthorize and data deletion callbacks: "<signature>.<payload>", both
// base64url, the signature an HMAC-SHA256 of the payload with the app secret.
func ParseSignedRequest(appSecret, signed string) (SignedRequest, error) {
	var sr SignedRequest
	if appSecret == "" {
		return sr, errors.New("no app secret")
	}
	sigPart, payloadPart, ok := strings.Cut(signed, ".")
	if !ok {
		return sr, errors.New("malformed signed_request")
	}
	sig, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(sigPart, "="))
	if err != nil {
		return sr, fmt.Errorf("decoding signature: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write([]byte(payloadPart))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return sr, errors.New("bad signed_request signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(payloadPart, "="))
	if err != nil {
		return sr, fmt.Errorf("decoding payload: %w", err)
	}
	if err := json.Unmarshal(payload, &sr); err != nil {
		return sr, fmt.Errorf("decoding payload: %w", err)
	}
	if !strings.EqualFold(sr.Algorithm, "HMAC-SHA256") {
		return sr, fmt.Errorf("unexpected algorithm %q", sr.Algorithm)
	}
	if sr.UserID == "" {
		return sr, errors.New("signed_request has no user_id")
	}
	return sr, nil
}

// Receipt is a delivery or read receipt for messages we sent: either for
// named messages (ExternalIDs), or, for a Messenger read receipt, for
// everything sent to Recipient up to Watermark.
type Receipt struct {
	ExternalIDs []string
	// Status is sent, delivered, read or failed.
	Status string
	// Error is why a message failed.
	Error     string
	Recipient string
	Watermark time.Time
}
