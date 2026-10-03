package lead

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/lead/models"
	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/jmoiron/sqlx/types"
	"github.com/zerodha/logf"
)

const tid = 1

// memSettings keeps lead settings in memory.
type memSettings struct{ s models.Settings }

func (m *memSettings) GetByPrefix(int, string) (types.JSONText, error) {
	b, err := json.Marshal(m.s)
	return b, err
}

func (m *memSettings) Update(_ int, s any) error {
	m.s = s.(models.Settings)
	return nil
}

var defaultStages = []models.Stage{
	{Key: "new", Label: "New", Kind: models.KindOpen},
	{Key: "interested", Label: "Interested", Kind: models.KindOpen},
	{Key: "admitted", Label: "Admitted", Kind: models.KindWon},
	{Key: "lost", Label: "Lost", Kind: models.KindLost},
}

func setup(t *testing.T) (*Manager, *memSettings, func(name string) int) {
	t.Helper()
	db := testutil.NewDB(t, "lead")
	lo := logf.New(logf.Opts{})
	st := &memSettings{s: models.Settings{Stages: defaultStages, AutoNew: true}}
	m, err := New(Opts{DB: db, Settings: st, Lo: &lo, I18n: testutil.NewI18n(t)})
	if err != nil {
		t.Fatal(err)
	}
	contact := func(name string) int {
		var id int
		db.QueryRow(`INSERT INTO users (tenant_id, type, first_name, phone_number) VALUES (1, 'contact', $1, '+919800000000') RETURNING id`, name).Scan(&id)
		return id
	}
	return m, st, contact
}

func TestStages(t *testing.T) {
	m, st, contact := setup(t)
	asha, ravi := contact("Asha"), contact("Ravi")

	// First contact puts them on the first stage, once.
	if s, _ := m.EnsureLead(tid, asha); s != "new" {
		t.Fatalf("EnsureLead = %q", s)
	}
	if s, _ := m.EnsureLead(tid, asha); s != "" {
		t.Fatalf("EnsureLead again = %q", s)
	}

	start := time.Now().Add(-time.Minute)
	if moved, err := m.SetStage(tid, asha, "admitted", 0); err != nil || !moved {
		t.Fatalf("SetStage: %v %v", moved, err)
	}
	if moved, _ := m.SetStage(tid, asha, "admitted", 0); moved {
		t.Fatal("moving to the same stage counted as a move")
	}
	if _, err := m.SetStage(tid, asha, "nope", 0); err == nil {
		t.Fatal("an unknown stage was accepted")
	}
	cl, err := m.ContactLead(tid, asha)
	if err != nil || cl.Stage.String != "admitted" || len(cl.History) != 2 || cl.History[0].FromStage.String != "new" {
		t.Fatalf("ContactLead = %+v, %v", cl, err)
	}
	if n, _ := m.CountMoves(tid, models.KindWon, start, time.Now().Add(time.Minute)); n != 1 {
		t.Fatalf("admissions = %d", n)
	}

	// With auto-new off, a new contact stays off the pipeline.
	st.s.AutoNew = false
	if s, _ := m.EnsureLead(tid, ravi); s != "" {
		t.Fatalf("EnsureLead with auto-new off = %q", s)
	}
	m.SetStage(tid, ravi, "interested", 0)

	cols, err := m.Board(tid, 10, "")
	if err != nil || len(cols) != 4 {
		t.Fatalf("Board: %v %v", cols, err)
	}
	got := map[string]int{}
	for _, c := range cols {
		got[c.Stage.Key] = c.Total
		if len(c.Cards) != c.Total {
			t.Fatalf("stage %s: %d cards, total %d", c.Stage.Key, len(c.Cards), c.Total)
		}
	}
	if got["admitted"] != 1 || got["interested"] != 1 || got["new"] != 0 {
		t.Fatalf("board totals %v", got)
	}
	if cols, _ := m.Board(tid, 10, "rav"); cols[1].Total != 1 || cols[2].Total != 0 {
		t.Fatal("search didn't filter the board")
	}

	// A stage with contacts can't be removed; an empty one can.
	if err := m.UpdateSettings(tid, models.Settings{Stages: []models.Stage{defaultStages[0], defaultStages[2], defaultStages[3]}}); err == nil {
		t.Fatal("removed a stage with contacts on it")
	}
	if err := m.UpdateSettings(tid, models.Settings{Stages: []models.Stage{defaultStages[1], defaultStages[2]}}); err != nil {
		t.Fatalf("removing empty stages: %v", err)
	}

	if _, err := m.SetStage(tid, ravi, "", 0); err != nil {
		t.Fatal(err)
	}
	if cl, _ := m.ContactLead(tid, ravi); cl.Stage.Valid {
		t.Fatal("clearing left a stage")
	}
}

func TestValidateStages(t *testing.T) {
	ok := func(s []models.Stage) bool { return ValidateStages(s) == nil }
	if !ok(defaultStages) {
		t.Fatal("default stages rejected")
	}
	for name, s := range map[string][]models.Stage{
		"empty":    {},
		"no won":   {{Key: "a", Label: "A", Kind: "open"}},
		"dup key":  {{Key: "a", Label: "A", Kind: "open"}, {Key: "a", Label: "B", Kind: "won"}},
		"bad key":  {{Key: "A B", Label: "A", Kind: "open"}, {Key: "b", Label: "B", Kind: "won"}},
		"bad kind": {{Key: "a", Label: "A", Kind: "maybe"}, {Key: "b", Label: "B", Kind: "won"}},
		"no label": {{Key: "a", Label: " ", Kind: "open"}, {Key: "b", Label: "B", Kind: "won"}},
	} {
		if ok(s) {
			t.Errorf("%s accepted", name)
		}
	}
	if KeyFor("Visit Booked!") != "visit_booked" {
		t.Errorf("KeyFor = %q", KeyFor("Visit Booked!"))
	}
}
