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
