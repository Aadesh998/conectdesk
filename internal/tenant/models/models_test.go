package models

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestAllowedChannels(t *testing.T) {
	for _, tc := range []struct {
		meta     string
		channels []string
		all      bool
	}{
		{"", nil, true},
		{`{}`, nil, true},
		{`{"channels": null}`, nil, true},
		{`{"channels": []}`, []string{}, false},
		{`{"channels": ["whatsapp", "voice"]}`, []string{"whatsapp", "voice"}, false},
	} {
		tn := Tenant{Meta: json.RawMessage(tc.meta)}
		channels, all := tn.AllowedChannels()
		if all != tc.all || !slices.Equal(channels, tc.channels) {
			t.Errorf("meta %q: got %v, %v; want %v, %v", tc.meta, channels, all, tc.channels, tc.all)
		}
	}

	tn := Tenant{Meta: json.RawMessage(`{"channels": ["whatsapp", "voice"]}`)}
	if !tn.ChannelAllowed("voice") || tn.ChannelAllowed("email") {
		t.Error("tuition-style workspace: want voice allowed and email not")
	}
	if !(Tenant{}).ChannelAllowed("email") {
		t.Error("unrestricted workspace: want every channel allowed")
	}
}

func TestAllowedIntegrations(t *testing.T) {
	for _, tc := range []struct {
		meta         string
		integrations []string
		all          bool
	}{
		{"", nil, true},
		{`{"channels": ["voice"]}`, nil, true},
		{`{"integrations": null}`, nil, true},
		{`{"integrations": []}`, []string{}, false},
		{`{"channels": ["voice"], "integrations": ["slack"]}`, []string{"slack"}, false},
	} {
		tn := Tenant{Meta: json.RawMessage(tc.meta)}
		integrations, all := tn.AllowedIntegrations()
		if all != tc.all || !slices.Equal(integrations, tc.integrations) {
			t.Errorf("meta %q: got %v, %v; want %v, %v", tc.meta, integrations, all, tc.integrations, tc.all)
		}
	}

	tn := Tenant{Meta: json.RawMessage(`{"integrations": ["slack"]}`)}
	if !tn.IntegrationAllowed("slack") || tn.IntegrationAllowed("webhooks") {
		t.Error("want slack allowed and webhooks not")
	}
	if !(Tenant{}).IntegrationAllowed("webhooks") {
		t.Error("unrestricted workspace: want every integration allowed")
	}
}
