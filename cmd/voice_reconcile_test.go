package main

import (
	"slices"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/inbox/channel/voice"
)

// TestCallReconcilerReady checks a call is settled only once it has been
// listed as ended on two runs, leaving the flow's own webhooks time to come,
// and never again once settled.
func TestCallReconcilerReady(t *testing.T) {
	r := newCallReconciler(nil, 2*time.Hour)
	const exophone = "+918047112233"
	ended := voice.ExotelCall{Sid: "a", Direction: "inbound", Status: "no-answer", To: "08047112233"}
	other := voice.ExotelCall{Sid: "b", Direction: "inbound", Status: "completed", To: "08047119999"}
	live := voice.ExotelCall{Sid: "c", Direction: "inbound", Status: "in-progress", To: "08047112233"}
	sids := func(calls []voice.ExotelCall) []string {
		var out []string
		for _, c := range calls {
			out = append(out, c.Sid)
		}
		return out
	}

	t0 := time.Now()
	list := []voice.ExotelCall{ended, other, live}
	if got := r.ready(list, exophone, t0); len(got) != 0 {
		t.Fatalf("first run settled %v", sids(got))
	}
	got := r.ready(list, exophone, t0.Add(5*time.Minute))
	if !slices.Equal(sids(got), []string{"a"}) {
		t.Fatalf("second run settled %v, want [a]", sids(got))
	}

	// Settled: never again. The call that was still going is listed as ended
	// now, and waits a run.
	r.done["a"], live.Status = t0, "completed"
	list = []voice.ExotelCall{ended, other, live}
	if got := r.ready(list, exophone, t0.Add(10*time.Minute)); len(got) != 0 {
		t.Fatalf("third run settled %v", sids(got))
	}
	if got := r.ready(list, exophone, t0.Add(15*time.Minute)); !slices.Equal(sids(got), []string{"c"}) {
		t.Fatalf("fourth run settled %v, want [c]", sids(got))
	}

	r.forget(t0.Add(4 * time.Hour))
	if len(r.seen) != 0 || len(r.done) != 0 {
		t.Fatalf("old calls kept: seen %v, done %v", r.seen, r.done)
	}
}

func TestLoggedStatus(t *testing.T) {
	if loggedStatus(map[string]any{voice.MetaCallKey: map[string]any{"status": "ringing"}}) != voice.StatusRinging ||
		loggedStatus(map[string]any{}) != "" {
		t.Fatal("loggedStatus")
	}
}
