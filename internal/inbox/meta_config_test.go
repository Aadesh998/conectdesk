package inbox

import (
	"encoding/json"
	"testing"
)

// The admin form can change a Meta inbox's options, never its account or token.
func TestMergeMetaConfig(t *testing.T) {
	current := `{"account_id":"PN1","waba_id":"W1","access_token":"tok","two_step_pin":"123456","max_media_size_mb":20,"mark_seen":true}`
	update := `{"account_id":"OTHER","access_token":"","two_step_pin":"","max_media_size_mb":5,"mark_seen":false}`

	merged, err := mergeMetaConfig(json.RawMessage(current), json.RawMessage(update))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	if got["account_id"] != "PN1" || got["access_token"] != "tok" || got["two_step_pin"] != "123456" || got["waba_id"] != "W1" {
		t.Errorf("account fields changed: %v", got)
	}
	if got["max_media_size_mb"] != float64(5) || got["mark_seen"] != false {
		t.Errorf("options not applied: %v", got)
	}
}
