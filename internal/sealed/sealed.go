// Package sealed opens payloads the browser encrypts to a one-time server key,
// so credentials an admin pastes are unreadable anywhere between the page and
// the handler that uses them: in a proxy that terminates TLS, a load
// balancer's request log, or a browser extension that records form posts.
//
// The scheme is ECIES on P-256 with the Web Crypto API's primitives:
//
//  1. The server generates an ephemeral P-256 key pair for one submission and
//     sends the browser its public key and an ID (Key).
//  2. The browser generates its own ephemeral key pair, derives the ECDH shared
//     secret, and stretches it with HKDF-SHA256 (salt: the key ID, info: Info)
//     into an AES-256-GCM key.
//  3. It encrypts the JSON payload with a random 96-bit IV, authenticating the
//     key ID and the purpose as additional data, and posts an Envelope.
//
// The server deletes the private key as it reads it, so an envelope can be
// opened once: a captured request can't be replayed, and a key leaked later
// opens nothing. Binding the purpose stops an envelope sealed for one form
// being submitted to another.
package sealed

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

const (
	// Info is the HKDF info string, versioning the scheme.
	Info = "conectdesk-sealed-v1"

	// Algorithm names the scheme to the browser.
	Algorithm = "ECDH-P256+HKDF-SHA256+A256GCM"

	ivSize = 12
)

// ErrOpen is returned for any envelope that doesn't open. It says no more, so
// a caller can't tell a bad key from a tampered ciphertext.
var ErrOpen = errors.New("sealed: envelope could not be opened")

// Envelope is what the browser posts: its ephemeral public key, the IV and the
// ciphertext with its GCM tag, all base64 (standard, padded).
type Envelope struct {
	KeyID      string `json:"key_id"`
	EPK        string `json:"epk"`
	IV         string `json:"iv"`
	Ciphertext string `json:"ciphertext"`
}

// Key is a one-time key pair for one submission.
type Key struct {
	ID   string
	priv *ecdh.PrivateKey
}

// NewKey generates a key pair under id.
func NewKey(id string) (Key, error) {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return Key{}, err
	}
	return Key{ID: id, priv: priv}, nil
}

// ParseKey restores a key from its private bytes.
func ParseKey(id string, private []byte) (Key, error) {
	priv, err := ecdh.P256().NewPrivateKey(private)
	if err != nil {
		return Key{}, err
	}
	return Key{ID: id, priv: priv}, nil
}

// PrivateBytes returns the private scalar, for storing until the key is used.
func (k Key) PrivateBytes() []byte {
	return k.priv.Bytes()
}

// PublicKey returns the public key as an uncompressed point, base64: the "raw"
// format Web Crypto's importKey takes.
func (k Key) PublicKey() string {
	return base64.StdEncoding.EncodeToString(k.priv.PublicKey().Bytes())
}

// Open decrypts env, which must have been sealed to k for purpose.
func (k Key) Open(env Envelope, purpose string) ([]byte, error) {
	if env.KeyID != k.ID {
		return nil, ErrOpen
	}
	epk, err1 := base64.StdEncoding.DecodeString(env.EPK)
	iv, err2 := base64.StdEncoding.DecodeString(env.IV)
	ct, err3 := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err1 != nil || err2 != nil || err3 != nil || len(iv) != ivSize {
		return nil, ErrOpen
	}
	peer, err := ecdh.P256().NewPublicKey(epk)
	if err != nil {
		return nil, ErrOpen
	}
	gcm, err := k.aead(peer)
	if err != nil {
		return nil, ErrOpen
	}
	plain, err := gcm.Open(nil, iv, ct, aad(k.ID, purpose))
	if err != nil {
		return nil, ErrOpen
	}
	return plain, nil
}

func (k Key) aead(peer *ecdh.PublicKey) (cipher.AEAD, error) {
	shared, err := k.priv.ECDH(peer)
	if err != nil {
		return nil, err
	}
	return deriveAEAD(shared, k.ID)
}

func deriveAEAD(shared []byte, keyID string) (cipher.AEAD, error) {
	key, err := hkdf.Key(sha256.New, shared, []byte(keyID), Info, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func aad(keyID, purpose string) []byte {
	return []byte(keyID + "|" + purpose)
}

// Seal encrypts plaintext to the public key publicKey (base64, uncompressed)
// as the browser does. The app only opens envelopes; this is for tests and
// for clients written in Go.
func Seal(keyID, publicKey, purpose string, plaintext []byte) (Envelope, error) {
	raw, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return Envelope{}, fmt.Errorf("decoding public key: %w", err)
	}
	server, err := ecdh.P256().NewPublicKey(raw)
	if err != nil {
		return Envelope{}, fmt.Errorf("parsing public key: %w", err)
	}
	eph, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return Envelope{}, err
	}
	shared, err := eph.ECDH(server)
	if err != nil {
		return Envelope{}, err
	}
	gcm, err := deriveAEAD(shared, keyID)
	if err != nil {
		return Envelope{}, err
	}
	iv := make([]byte, ivSize)
	if _, err := rand.Read(iv); err != nil {
		return Envelope{}, err
	}
	return Envelope{
		KeyID:      keyID,
		EPK:        base64.StdEncoding.EncodeToString(eph.PublicKey().Bytes()),
		IV:         base64.StdEncoding.EncodeToString(iv),
		Ciphertext: base64.StdEncoding.EncodeToString(gcm.Seal(nil, iv, plaintext, aad(keyID, purpose))),
	}, nil
}
