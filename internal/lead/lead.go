// Package lead keeps where each contact is in the admissions pipeline: a stage
// from the workspace's list, with every move recorded, so the board shows
// where leads are and reports can count admissions.
package lead

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/lead/models"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

//go:embed queries.sql
var efs embed.FS

const (
	// MaxStages caps the pipeline's length.
	MaxStages = 12
	// maxPerStage caps the cards the board loads for a stage.
	maxPerStage = 200
)

var keyPattern = regexp.MustCompile(`^[a-z0-9_]{1,50}$`)

// SettingStore reads and writes the workspace's settings.
type SettingStore interface {
	GetByPrefix(tenantID int, prefix string) (types.JSONText, error)
	Update(tenantID int, s any) error
}

// Manager keeps contacts' lead stages.
type Manager struct {
	q        queries
	settings SettingStore
	lo       *logf.Logger
	i18n     *i18ncat.Catalog
}

// Opts holds the Manager's dependencies.
type Opts struct {
	DB       *sqlx.DB
	Settings SettingStore
	Lo       *logf.Logger
	I18n     *i18ncat.Catalog
}

type queries struct {
	SetStage       *sqlx.Stmt `query:"set-stage"`
	ClearStage     *sqlx.Stmt `query:"clear-stage"`
	EnsureStage    *sqlx.Stmt `query:"ensure-stage"`
	GetContactLead *sqlx.Stmt `query:"get-contact-lead"`
	GetHistory     *sqlx.Stmt `query:"get-history"`
	GetBoard       *sqlx.Stmt `query:"get-board"`
	CountByStage   *sqlx.Stmt `query:"count-by-stage"`
	CountMoves     *sqlx.Stmt `query:"count-moves"`
}

// New returns a Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, settings: opts.Settings, lo: opts.Lo, i18n: opts.I18n}, nil
}

// Settings returns the workspace's lead settings.
func (m *Manager) Settings(tenantID int) (models.Settings, error) {
	var s models.Settings
	b, err := m.settings.GetByPrefix(tenantID, "leads.")
	if err != nil {
		return s, err
	}
	if len(b) > 0 && string(b) != "null" {
		if err := json.Unmarshal(b, &s); err != nil {
			m.lo.Error("error reading lead settings", "error", err)
			return s, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
		}
	}
	return s, nil
}

// Stages returns the workspace's lead stages, in pipeline order.
func (m *Manager) Stages(tenantID int) ([]models.Stage, error) {
	s, err := m.Settings(tenantID)
	return s.Stages, err
}

// UpdateSettings saves the lead stages and whether new contacts go on the
// pipeline. A stage that still has contacts can't be removed.
func (m *Manager) UpdateSettings(tenantID int, s models.Settings) error {
	t := m.i18n.For(tenantID)
	if err := ValidateStages(s.Stages); err != nil {
		return envelope.NewError(envelope.InputError, t.T("lead.invalidStages"), nil)
	}
	counts, err := m.CountByStage(tenantID)
	if err != nil {
		return err
	}
	for key, n := range counts {
		if n > 0 && !slices.ContainsFunc(s.Stages, func(st models.Stage) bool { return st.Key == key }) {
			return envelope.NewError(envelope.InputError, t.Ts("lead.stageInUse", "stage", key), nil)
		}
	}
	return m.settings.Update(tenantID, s)
}

// ValidateStages checks a pipeline: 1 to MaxStages stages with unique keys
// and names, at least one open and one won.
func ValidateStages(stages []models.Stage) error {
	if len(stages) == 0 || len(stages) > MaxStages {
		return errors.New("wrong number of stages")
	}
	keys, kinds := map[string]bool{}, map[string]bool{}
	for i := range stages {
		st := &stages[i]
		st.Label = strings.TrimSpace(st.Label)
		if !keyPattern.MatchString(st.Key) || keys[st.Key] || st.Label == "" || len(st.Label) > 50 {
			return errors.New("bad stage")
		}
		if st.Kind != models.KindOpen && st.Kind != models.KindWon && st.Kind != models.KindLost {
			return errors.New("bad stage kind")
		}
		keys[st.Key], kinds[st.Kind] = true, true
	}
	if !kinds[models.KindOpen] || !kinds[models.KindWon] {
		return errors.New("a pipeline needs an open and a won stage")
	}
	return nil
}

// KeyFor makes a stage key from its label, for a new stage.
func KeyFor(label string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(label)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('_')
		}
	}
	k := strings.Trim(b.String(), "_")
	if len(k) > 50 {
		k = k[:50]
	}
	return k
}

// SetStage moves a contact to a stage, or off the pipeline for "". actorID is
// the agent, or 0. It returns whether they moved.
func (m *Manager) SetStage(tenantID, contactID int, stage string, actorID int) (bool, error) {
	t := m.i18n.For(tenantID)
	if stage == "" {
		if _, err := m.q.ClearStage.Exec(tenantID, contactID); err != nil {
			m.lo.Error("error clearing lead stage", "contact_id", contactID, "error", err)
			return false, envelope.NewError(envelope.GeneralError, t.T("globals.messages.somethingWentWrong"), nil)
		}
		return true, nil
	}
	stages, err := m.Stages(tenantID)
	if err != nil {
		return false, err
	}
	if !slices.ContainsFunc(stages, func(st models.Stage) bool { return st.Key == stage }) {
		return false, envelope.NewError(envelope.InputError, t.T("lead.unknownStage"), nil)
	}
	var from null.String
	if err := m.q.SetStage.Get(&from, tenantID, contactID, stage, null.NewInt(actorID, actorID > 0)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		m.lo.Error("error setting lead stage", "contact_id", contactID, "stage", stage, "error", err)
		return false, envelope.NewError(envelope.GeneralError, t.T("globals.messages.somethingWentWrong"), nil)
	}
	return true, nil
}

// EnsureLead puts a contact on the first stage when they first get in touch,
// if the workspace has that on and they aren't on the pipeline yet. It
// returns the stage they were put on, or "".
func (m *Manager) EnsureLead(tenantID, contactID int) (string, error) {
	s, err := m.Settings(tenantID)
	if err != nil || !s.AutoNew || len(s.Stages) == 0 {
		return "", err
	}
	var stage string
	if err := m.q.EnsureStage.Get(&stage, tenantID, contactID, s.Stages[0].Key); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		m.lo.Error("error putting contact on the lead pipeline", "contact_id", contactID, "error", err)
		return "", err
	}
	return stage, nil
}

// ContactLead returns a contact's stage and its history.
func (m *Manager) ContactLead(tenantID, contactID int) (models.ContactLead, error) {
	t := m.i18n.For(tenantID)
	var cl models.ContactLead
	if err := m.q.GetContactLead.Get(&cl, tenantID, contactID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return cl, envelope.NewError(envelope.NotFoundError, t.T("validation.notFoundUser"), nil)
		}
		m.lo.Error("error fetching contact's lead stage", "contact_id", contactID, "error", err)
		return cl, envelope.NewError(envelope.GeneralError, t.T("globals.messages.somethingWentWrong"), nil)
	}
	cl.History = []models.Event{}
	if err := m.q.GetHistory.Select(&cl.History, tenantID, contactID); err != nil {
		m.lo.Error("error fetching lead history", "contact_id", contactID, "error", err)
		return cl, envelope.NewError(envelope.GeneralError, t.T("globals.messages.somethingWentWrong"), nil)
	}
	return cl, nil
}

// Board returns each stage's newest-moved contacts, perStage at most,
// matching search.
func (m *Manager) Board(tenantID, perStage int, search string) ([]models.Column, error) {
	t := m.i18n.For(tenantID)
	stages, err := m.Stages(tenantID)
	if err != nil {
		return nil, err
	}
	if perStage <= 0 || perStage > maxPerStage {
		perStage = 50
	}
	keys := make([]string, len(stages))
	for i, st := range stages {
		keys[i] = st.Key
	}
	var cards []models.Card
	if err := m.q.GetBoard.Select(&cards, tenantID, pq.Array(keys), perStage, strings.TrimSpace(search)); err != nil {
		m.lo.Error("error fetching leads board", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, t.T("globals.messages.somethingWentWrong"), nil)
	}
	cols := make([]models.Column, len(stages))
	idx := map[string]int{}
	for i, st := range stages {
		cols[i] = models.Column{Stage: st, Cards: []models.Card{}}
		idx[st.Key] = i
	}
	for _, c := range cards {
		i := idx[c.Stage]
		cols[i].Cards = append(cols[i].Cards, c)
		cols[i].Total = c.StageCount
	}
	return cols, nil
}

// CountByStage returns how many contacts each stage has.
func (m *Manager) CountByStage(tenantID int) (map[string]int, error) {
	var rows []struct {
		Stage string `db:"lead_stage"`
		Count int    `db:"count"`
	}
	if err := m.q.CountByStage.Select(&rows, tenantID); err != nil {
		m.lo.Error("error counting leads by stage", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.Stage] = r.Count
	}
	return out, nil
}

// CountMoves returns how many contacts moved onto a stage of the given kind
// between from and to, e.g. admissions today.
func (m *Manager) CountMoves(tenantID int, kind string, from, to time.Time) (int, error) {
	stages, err := m.Stages(tenantID)
	if err != nil {
		return 0, err
	}
	var keys []string
	for _, st := range stages {
		if st.Kind == kind {
			keys = append(keys, st.Key)
		}
	}
	if len(keys) == 0 {
		return 0, nil
	}
	var n int
	if err := m.q.CountMoves.Get(&n, tenantID, pq.Array(keys), from, to); err != nil {
		m.lo.Error("error counting lead moves", "kind", kind, "error", err)
		return 0, err
	}
	return n, nil
}
