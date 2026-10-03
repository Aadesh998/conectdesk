package main

import (
	"slices"
	"testing"

	authzmodels "github.com/conectdesk/conectdesk/internal/authz/models"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/voice"
	tmodels "github.com/conectdesk/conectdesk/internal/team/models"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
)

func TestIsMissed(t *testing.T) {
	for status, want := range map[string]bool{
		voice.StatusNoAnswer:  true,
		voice.StatusBusy:      true,
		voice.StatusVoicemail: true,
		voice.StatusCompleted: false,
		voice.StatusFailed:    false,
		voice.StatusRinging:   false,
	} {
		if isMissed(status) != want {
			t.Errorf("isMissed(%q) = %v", status, !want)
		}
	}
}

// TestCallBackScope checks an agent sees the call-backs of the phone
// conversations they may read.
func TestCallBackScope(t *testing.T) {
	agent := umodels.User{
		ID:          7,
		Permissions: []string{authzmodels.PermConversationsRead, authzmodels.PermConversationsReadAssigned, authzmodels.PermConversationsReadTeamInbox},
		Teams:       tmodels.TeamsCompact{{ID: 3}},
	}
	s := callBackScope(agent)
	if s.All || !s.Assigned || !s.TeamInbox || s.TeamAll || s.Unassigned || s.UserID != 7 || !slices.Equal(s.TeamIDs, []int{3}) {
		t.Fatalf("scope %+v", s)
	}
	if s := callBackScope(umodels.User{ID: 1, Permissions: []string{authzmodels.PermConversationsReadAll}}); s.All {
		t.Fatal("read_all without conversations:read saw everything")
	}
	if s := callBackScope(umodels.User{ID: 1, Permissions: []string{authzmodels.PermConversationsRead, authzmodels.PermConversationsReadAll}}); !s.All || s.TeamIDs == nil {
		t.Fatalf("admin scope %+v", s)
	}
}

func TestMissedCallTemplateValues(t *testing.T) {
	ref := voice.TemplateRef{Body: []string{"{{name}}", "Call {{business}} back on {{phone}}", "fixed"}}
	got := ref.Values("Asha", "+919876543210", "Bright Classes")
	want := []string{"Asha", "Call Bright Classes back on +919876543210", "fixed"}
	if !slices.Equal(got, want) {
		t.Fatalf("Values = %q", got)
	}
	if (voice.MissedCallConfig{}).Due().Minutes() != 15 || (voice.MissedCallConfig{CallBackMinutes: 5}).Due().Minutes() != 5 {
		t.Fatal("Due")
	}
	if !looksLikeNumber("+91 98765-43210") || looksLikeNumber("Asha") {
		t.Fatal("looksLikeNumber")
	}
}
