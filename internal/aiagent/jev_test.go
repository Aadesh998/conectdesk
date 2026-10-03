package aiagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/aiagent/models"
	lmodels "github.com/conectdesk/conectdesk/internal/lead/models"
	"github.com/conectdesk/conectdesk/internal/systemone"
	"github.com/lib/pq"
)

func TestDecideGate(t *testing.T) {
	const thr = 0.7
	cases := []struct {
		name       string
		d          gateDecision
		handoff    bool
		hasReplied bool
		want       string
	}{
		{"confident reply", gateDecision{Action: actionReply, ActionConfidence: 0.9}, true, true, verdictLLM},
		{"unsure goes to llm", gateDecision{Action: actionNoReply, ActionConfidence: 0.6}, true, true, verdictLLM},
		{"thanks is skipped", gateDecision{Action: actionNoReply, ActionConfidence: 0.95, Settled: 0.4}, true, true, verdictSkip},
		{"settled closes", gateDecision{Action: actionNoReply, ActionConfidence: 0.95, Settled: 0.9}, true, true, verdictResolve},
		{"first message always answered", gateDecision{Action: actionNoReply, ActionConfidence: 0.95, Settled: 0.9}, true, false, verdictLLM},
		{"asks for a person", gateDecision{Action: actionHuman, ActionConfidence: 0.8}, true, true, verdictHandoff},
		{"no handoff allowed", gateDecision{Action: actionHuman, ActionConfidence: 0.8}, false, true, verdictLLM},
		{"angry even if unsure of action", gateDecision{Action: actionReply, ActionConfidence: 0.3, Frustration: 4, FrustrationConf: 0.8}, true, true, verdictHandoff},
		{"anger unsure", gateDecision{Action: actionReply, ActionConfidence: 0.9, Frustration: 5, FrustrationConf: 0.5}, true, true, verdictLLM},
	}
	for _, c := range cases {
		if got, _ := decideGate(c.d, thr, c.handoff, c.hasReplied); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestFollowupDue(t *testing.T) {
	last := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	hours := []int64{4, 24}
	cases := []struct {
		sent int
		now  time.Time
		want bool
	}{
		{0, last.Add(3 * time.Hour), false},
		{0, last.Add(4 * time.Hour), true},
		{1, last.Add(5 * time.Hour), false},
		{1, last.Add(24 * time.Hour), true},
		{2, last.Add(100 * time.Hour), false},
	}
	for _, c := range cases {
		if got := followupDue(hours, c.sent, last, c.now); got != c.want {
			t.Errorf("sent %d at +%s: got %v", c.sent, c.now.Sub(last), got)
		}
	}
}

func TestNormalizeJev(t *testing.T) {
	a := models.Assistant{
		JevConfidence: 1.4,
		FollowupHours: pq.Int64Array{24, 0, 4, 4, 900, 2, 3, 5, 6},
		FollowupTemplates: models.TemplateRefs{
			{Name: " visit_reminder ", Language: "en"},
			{Name: "", Language: "en"},
			{Name: "no_lang"},
		},
	}
	normalizeJev(&a)
	if a.JevConfidence != 0.99 {
		t.Errorf("confidence = %v", a.JevConfidence)
	}
	if want := (pq.Int64Array{2, 3, 4, 5, 6}); len(a.FollowupHours) != len(want) || a.FollowupHours[0] != 2 || a.FollowupHours[4] != 6 {
		t.Errorf("hours = %v", a.FollowupHours)
	}
	if len(a.FollowupTemplates) != 1 || a.FollowupTemplates[0].Name != "visit_reminder" || a.FollowupTemplates[0].Body == nil {
		t.Errorf("templates = %+v", a.FollowupTemplates)
	}

	empty := models.Assistant{}
	normalizeJev(&empty)
	if empty.JevConfidence != defaultJevConfidence || len(empty.FollowupHours) != 2 {
		t.Errorf("defaults = %+v", empty)
	}
}

// fakeJev answers every question from answers, recording what it was asked.
func fakeJev(t *testing.T, answers map[string]systemone.Answer, asked *map[string]systemone.Question) *systemone.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Questions map[string]systemone.Question `json:"questions"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if asked != nil {
			*asked = req.Questions
		}
		out := map[string]systemone.Answer{}
		for id := range req.Questions {
			out[id] = answers[id]
		}
		json.NewEncoder(w).Encode(systemone.Response{Answers: out})
	}))
	t.Cleanup(srv.Close)
	c := systemone.New("key", "")
	c.SetBaseURL(srv.URL)
	return c
}

func TestGuard(t *testing.T) {
	m := &Manager{}
	a := models.Assistant{Guardrails: "Never promise fee discounts."}

	var asked map[string]systemone.Question
	jev := fakeJev(t, map[string]systemone.Answer{
		reasonPolicy:      {Noul: 0.92},
		reasonTone:        {Noul: 0.05},
		reasonUnsupported: {Noul: 0.8},
	}, &asked)

	g, err := m.guard(context.Background(), jev, a, "Any discount?", "Sure, 20% off for you!", nil, 0.7)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := asked[reasonUnsupported]; ok {
		t.Error("asked about facts without any knowledge to check them against")
	}
	if !g.Blocked() || len(g.Reasons) != 1 || g.Reasons[0] != reasonPolicy {
		t.Errorf("reasons = %v", g.Reasons)
	}

	g, _ = m.guard(context.Background(), jev, a, "Fees?", "It's 50,000.", []string{"Fees are 45,000 per year."}, 0.7)
	if len(g.Reasons) != 2 || g.Reasons[1] != reasonUnsupported {
		t.Errorf("with knowledge, reasons = %v", g.Reasons)
	}
}

func TestPickLeadStage(t *testing.T) {
	m := &Manager{}
	stages := []lmodels.Stage{
		{Key: "new", Label: "New", Kind: lmodels.KindOpen},
		{Key: "visit_booked", Label: "Visit booked", Kind: lmodels.KindOpen},
		{Key: "admitted", Label: "Admitted", Kind: lmodels.KindWon},
	}
	pick := func(choice string, conf float64, current string) string {
		jev := fakeJev(t, map[string]systemone.Answer{"stage": {Choice: choice, Confidence: conf}}, nil)
		got, err := m.pickLeadStage(context.Background(), jev, nil, stages, current, 0.7)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := pick("visit_booked", 0.8, "new"); got != "visit_booked" {
		t.Errorf("open move: %q", got)
	}
	if got := pick("visit_booked", 0.6, "new"); got != "" {
		t.Errorf("unsure move: %q", got)
	}
	if got := pick("admitted", 0.8, "visit_booked"); got != "" {
		t.Errorf("won needs more confidence: %q", got)
	}
	if got := pick("admitted", 0.95, "visit_booked"); got != "admitted" {
		t.Errorf("confident won: %q", got)
	}
	if got := pick("new", 0.99, "admitted"); got != "" {
		t.Errorf("moved a contact out of a won stage: %q", got)
	}
}

func TestDecideFollowup(t *testing.T) {
	m := &Manager{}
	jev := fakeJev(t, map[string]systemone.Answer{
		"follow_up": {Noul: 0.9},
		"template":  {Choice: "t1", Confidence: 0.85},
	}, nil)
	d, err := m.decideFollowup(context.Background(), jev, nil, 26, 1, []string{"Fee reminder", "Visit reminder"}, 0.7)
	if err != nil || !d.FollowUp || d.Template != 1 {
		t.Fatalf("d = %+v err %v", d, err)
	}

	jev = fakeJev(t, map[string]systemone.Answer{
		"follow_up": {Noul: 0.9},
		"template":  {Choice: noTemplate, Confidence: 0.9},
	}, nil)
	if d, _ := m.decideFollowup(context.Background(), jev, nil, 26, 1, []string{"Fee reminder"}, 0.7); d.Template != -1 {
		t.Errorf("none picked a template: %+v", d)
	}
}
