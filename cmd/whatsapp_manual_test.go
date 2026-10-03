package main

import "testing"

func TestWhatsAppManualCredentialsValid(t *testing.T) {
	good := whatsAppManualCredentials{
		WABAID:        "102290129340398",
		PhoneNumberID: "106540352242922",
		AppID:         "1234567890123",
		AppSecret:     "0123456789abcdef0123456789ABCDEF",
		AccessToken:   "EAAGm0PX4ZCpsBA",
	}
	if !good.valid() {
		t.Fatal("valid credentials rejected")
	}

	cases := map[string]func(*whatsAppManualCredentials){
		"path in waba id":   func(c *whatsAppManualCredentials) { c.WABAID = "1023/../me" },
		"path in number id": func(c *whatsAppManualCredentials) { c.PhoneNumberID = "me?fields=x" },
		"short secret":      func(c *whatsAppManualCredentials) { c.AppSecret = "abc" },
		"no token":          func(c *whatsAppManualCredentials) { c.AccessToken = "" },
		"token with space":  func(c *whatsAppManualCredentials) { c.AccessToken = "EAAG x" },
		"bad pin":           func(c *whatsAppManualCredentials) { c.TwoStepPIN = "12ab56" },
	}
	for name, mutate := range cases {
		c := good
		mutate(&c)
		if c.valid() {
			t.Errorf("%s: accepted", name)
		}
	}

	padded := good
	padded.WABAID = "  " + good.WABAID + "\n"
	padded.TwoStepPIN = " 123456 "
	padded.normalize()
	if !padded.valid() {
		t.Error("whitespace around pasted values rejected")
	}
}
