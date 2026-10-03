package sealed

import (
	"encoding/base64"
	"testing"
)

func TestSealOpen(t *testing.T) {
	k, err := NewKey("k1")
	if err != nil {
		t.Fatal(err)
	}
	env, err := Seal("k1", k.PublicKey(), "whatsapp_manual", []byte(`{"access_token":"EAAG"}`))
	if err != nil {
		t.Fatal(err)
	}

	// The key survives a round trip through storage.
	restored, err := ParseKey("k1", k.PrivateBytes())
	if err != nil {
		t.Fatal(err)
	}
	got, err := restored.Open(env, "whatsapp_manual")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"access_token":"EAAG"}` {
		t.Fatalf("got %q", got)
	}
}

func TestOpenRejects(t *testing.T) {
	k, _ := NewKey("k1")
	other, _ := NewKey("k2")
	env, _ := Seal("k1", k.PublicKey(), "whatsapp_manual", []byte("secret"))

	tampered := env
	ct, _ := base64.StdEncoding.DecodeString(env.Ciphertext)
	ct[0] ^= 1
	tampered.Ciphertext = base64.StdEncoding.EncodeToString(ct)

	wrongID := env
	wrongID.KeyID = "k2"

	cases := map[string]struct {
		key     Key
		env     Envelope
		purpose string
	}{
		"other purpose": {k, env, "telegram"},
		"tampered":      {k, tampered, "whatsapp_manual"},
		"other key":     {other, wrongID, "whatsapp_manual"},
		"key id swap":   {Key{ID: "k2", priv: k.priv}, wrongID, "whatsapp_manual"},
		"bad epk":       {k, Envelope{KeyID: "k1", EPK: "AAAA", IV: env.IV, Ciphertext: env.Ciphertext}, "whatsapp_manual"},
		"short iv":      {k, Envelope{KeyID: "k1", EPK: env.EPK, IV: "AAAA", Ciphertext: env.Ciphertext}, "whatsapp_manual"},
	}
	for name, c := range cases {
		if _, err := c.key.Open(c.env, c.purpose); err != ErrOpen {
			t.Errorf("%s: got %v, want ErrOpen", name, err)
		}
	}
}
