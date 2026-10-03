package report

import (
	"fmt"
	"os"
	"testing"

	"github.com/conectdesk/conectdesk/internal/report/models"
	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/goyesql/v2"
)

// slaFixture seeds two agents in two teams, each with a conversation under one
// SLA policy, plus a resolved conversation and a result from before the window.
type slaFixture struct {
	db *sqlx.DB
	q  goyesql.Queries
}

func newSLAFixture(t *testing.T) slaFixture {
	t.Helper()
	db := testutil.NewDB(t, "report_sla")
	b, err := os.ReadFile("queries.sql")
	if err != nil {
		t.Fatal(err)
	}
	q, err := goyesql.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}

	var contact, asha, ravi, inbox, billing, support, policy, open, resolved int
	db.Get(&contact, `INSERT INTO users (tenant_id, type, first_name) VALUES (1, 'contact', 'C') RETURNING id`)
	db.Get(&asha, `INSERT INTO users (tenant_id, type, first_name, last_name, email) VALUES (1, 'agent', 'Asha', 'Rao', 'asha@acme.example') RETURNING id`)
	db.Get(&ravi, `INSERT INTO users (tenant_id, type, first_name, last_name, email) VALUES (1, 'agent', 'Ravi', 'Kumar', 'ravi@acme.example') RETURNING id`)
	db.Get(&inbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Support mail', 'email', '{}') RETURNING id`)
	db.Get(&billing, `INSERT INTO teams (tenant_id, name, conversation_assignment_type) VALUES (1, 'Billing', 'Manual') RETURNING id`)
	db.Get(&support, `INSERT INTO teams (tenant_id, name, conversation_assignment_type) VALUES (1, 'Support', 'Manual') RETURNING id`)
	db.Get(&policy, `INSERT INTO sla_policies (tenant_id, name, first_response_time, next_response_time, resolution_time) VALUES (1, 'Standard', '1h', '2h', '1d') RETURNING id`)
	db.Get(&open, `SELECT id FROM conversation_statuses WHERE tenant_id = 1 AND category = 'open' LIMIT 1`)
	db.Get(&resolved, `SELECT id FROM conversation_statuses WHERE tenant_id = 1 AND category = 'resolved' LIMIT 1`)

	conv := func(ref string, agent, team, status int) int {
		var id int
		if err := db.Get(&id, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number, assigned_user_id, assigned_team_id, subject)
			VALUES (1, $1, $2, $3, $4, $5, $6, 'Fee refund') RETURNING id`, contact, inbox, status, ref, agent, team); err != nil {
			t.Fatal(err)
		}
		return id
	}
	c1 := conv("101", asha, billing, open)
	c2 := conv("102", ravi, support, open)
	c3 := conv("103", ravi, support, resolved)

	// c1: first response met in 10m, resolution breached, one next response breached.
	var a1 int
	db.Get(&a1, `INSERT INTO applied_slas (tenant_id, conversation_id, sla_policy_id, created_at, first_response_deadline_at, first_response_met_at, resolution_deadline_at, resolution_breached_at)
		VALUES (1, $1, $2, NOW() - INTERVAL '2 days', NOW() - INTERVAL '2 days' + INTERVAL '1 hour', NOW() - INTERVAL '2 days' + INTERVAL '10 minutes', NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day') RETURNING id`, c1, policy)
	db.MustExec(`INSERT INTO sla_events (tenant_id, applied_sla_id, sla_policy_id, type, created_at, deadline_at, breached_at, status)
		VALUES (1, $1, $2, 'next_response', NOW() - INTERVAL '36 hours', NOW() - INTERVAL '34 hours', NOW() - INTERVAL '34 hours', 'breached')`, a1, policy)
	// c2: first response breached, resolution due in 30 minutes.
	db.MustExec(`INSERT INTO applied_slas (tenant_id, conversation_id, sla_policy_id, created_at, first_response_deadline_at, first_response_breached_at, resolution_deadline_at)
		VALUES (1, $1, $2, NOW() - INTERVAL '1 day', NOW() - INTERVAL '23 hours', NOW() - INTERVAL '23 hours', NOW() + INTERVAL '30 minutes')`, c2, policy)
	// c3 is resolved, so its open first response deadline isn't at risk.
	db.MustExec(`INSERT INTO applied_slas (tenant_id, conversation_id, sla_policy_id, created_at, first_response_deadline_at)
		VALUES (1, $1, $2, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '10 minutes')`, c3, policy)
	// Outside a 7 day window.
	db.MustExec(`INSERT INTO applied_slas (tenant_id, conversation_id, sla_policy_id, created_at, status, first_response_breached_at, resolution_breached_at)
		VALUES (1, $1, $2, NOW() - INTERVAL '40 days', 'breached', NOW() - INTERVAL '40 days', NOW() - INTERVAL '40 days')`, c2, policy)

	return slaFixture{db: db, q: q}
}

func TestSLAQueries(t *testing.T) {
	f := newSLAFixture(t)

	t.Run("breakdown matches the dashboard totals", func(t *testing.T) {
		var overview models.OverviewSLA
		if err := f.db.Get(&overview, fmt.Sprintf(f.q["get-overview-sla-counts"].Query, 7, 7, 7, 7), 1); err != nil {
			t.Fatal(err)
		}
		var rows []SLAGroupStats
		if err := f.db.Select(&rows, f.q["get-sla-breakdown"].Query, 1, 7, "team", ""); err != nil {
			t.Fatal(err)
		}
		met, breached := map[string]int{}, map[string]int{}
		for _, r := range rows {
			met[r.Metric] += r.Met
			breached[r.Metric] += r.Breached
		}
		if met["first_response"] != overview.FirstResponseMetCount || breached["first_response"] != overview.FirstResponseBreachedCount ||
			met["resolution"] != overview.ResolutionMetCount || breached["resolution"] != overview.ResolutionBreachedCount ||
			met["next_response"] != overview.NextResponseMetCount || breached["next_response"] != overview.NextResponseBreachedCount {
			t.Fatalf("breakdown %v met %v breached differs from overview %+v", met, breached, overview)
		}
		if breached["first_response"] != 1 || met["first_response"] != 1 {
			t.Fatalf("the 40 day old result leaked into 7 days: %+v", rows)
		}
	})

	t.Run("breakdown by agent", func(t *testing.T) {
		var rows []SLAGroupStats
		if err := f.db.Select(&rows, f.q["get-sla-breakdown"].Query, 1, 7, "agent", "first_response"); err != nil {
			t.Fatal(err)
		}
		got := map[string]SLAGroupStats{}
		for _, r := range rows {
			if r.Metric != "first_response" {
				t.Fatalf("metric filter let %s through", r.Metric)
			}
			got[r.GroupName] = r
		}
		if a := got["Asha Rao"]; a.Met != 1 || a.Breached != 0 || int(a.AvgMetSeconds) != 600 {
			t.Fatalf("Asha = %+v", a)
		}
		// Ravi breached one and has one pending (the resolved conversation's).
		if r := got["Ravi Kumar"]; r.Breached != 1 || r.Pending != 1 {
			t.Fatalf("Ravi = %+v", r)
		}
		if rows[0].GroupName != "Ravi Kumar" {
			t.Fatalf("most breaches should come first, got %+v", rows)
		}
	})

	t.Run("breaches filter by team and agent", func(t *testing.T) {
		var rows []SLAConversation
		if err := f.db.Select(&rows, f.q["get-sla-breaches"].Query, 1, 7, "", "", "", 25); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 3 || rows[0].Total != 3 {
			t.Fatalf("all breaches = %+v", rows)
		}
		// Newest first: c2's first response (23h ago) before c1's resolution (1d ago).
		if rows[0].ReferenceNumber != "102" || rows[0].Metric != "first_response" {
			t.Fatalf("newest breach = %+v", rows[0])
		}

		if err := f.db.Select(&rows, f.q["get-sla-breaches"].Query, 1, 7, "", "billing", "", 25); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0].TeamName != "Billing" {
			t.Fatalf("billing breaches = %+v", rows)
		}

		rows = nil
		if err := f.db.Select(&rows, f.q["get-sla-breaches"].Query, 1, 7, "resolution", "", "asha", 25); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].ReferenceNumber != "101" || rows[0].AgentName != "Asha Rao" {
			t.Fatalf("asha's resolution breaches = %+v", rows)
		}

		if err := f.db.Select(&rows, f.q["get-sla-breaches"].Query, 1, 7, "", "", "", 1); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].Total != 3 {
			t.Fatalf("a limited page should still count every match, got %+v", rows)
		}
	})

	t.Run("at risk skips resolved conversations", func(t *testing.T) {
		var rows []SLAConversation
		if err := f.db.Select(&rows, f.q["get-sla-at-risk"].Query, 1, 60, 25); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].ReferenceNumber != "102" || rows[0].Metric != "resolution" {
			t.Fatalf("at risk in 60m = %+v", rows)
		}
		if err := f.db.Select(&rows, f.q["get-sla-at-risk"].Query, 1, 15, 25); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Fatalf("at risk in 15m = %+v", rows)
		}
	})

	t.Run("trend per day", func(t *testing.T) {
		var rows []SLATrendPoint
		if err := f.db.Select(&rows, f.q["get-sla-trend"].Query, 1, 7, "day"); err != nil {
			t.Fatal(err)
		}
		breached := 0
		for _, r := range rows {
			breached += r.Breached
		}
		if breached != 3 {
			t.Fatalf("trend breaches = %d, want 3: %+v", breached, rows)
		}
	})

	t.Run("policies", func(t *testing.T) {
		var rows []SLAPolicySummary
		if err := f.db.Select(&rows, f.q["get-sla-policies"].Query, 1); err != nil {
			t.Fatal(err)
		}
		// The seeded Default policy, then Standard, by name.
		if len(rows) != 2 || rows[1].Name != "Standard" || rows[1].NextResponseTime != "2h" || rows[0].NextResponseTime != "" {
			t.Fatalf("policies = %+v", rows)
		}
	})
}
