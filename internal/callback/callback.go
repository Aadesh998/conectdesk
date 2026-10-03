// Package callback keeps the call-back each missed call becomes: one open
// call-back per contact, which further missed calls add to, closed when a call
// with the contact connects, and escalated once when it goes overdue. Nothing
// in it is decided by AI; the rules are plain and the database enforces them.
package callback

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"time"

	"github.com/conectdesk/conectdesk/internal/callback/models"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

//go:embed queries.sql
var efs embed.FS

// DefaultDueIn is how long an agent has to call back, unless the phone line
// sets its own.
const DefaultDueIn = 15 * time.Minute

// maxPerPage caps a page of the call-back list.
const maxPerPage = 100

// Manager keeps call-backs.
type Manager struct {
	q    queries
	lo   *logf.Logger
	i18n *i18ncat.Catalog
}

// Opts holds the Manager's dependencies.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18ncat.Catalog
}

type queries struct {
	RecordMissed      *sqlx.Stmt `query:"record-missed"`
	CloseForContact   *sqlx.Stmt `query:"close-for-contact"`
	MarkDone          *sqlx.Stmt `query:"mark-done"`
	AddAttempt        *sqlx.Stmt `query:"add-attempt"`
	SetWhatsApp       *sqlx.Stmt `query:"set-whatsapp"`
	WhatsAppSentSince *sqlx.Stmt `query:"whatsapp-sent-since"`
	Get               *sqlx.Stmt `query:"get"`
	List              *sqlx.Stmt `query:"list"`
	Counts            *sqlx.Stmt `query:"counts"`
	PendingForContact *sqlx.Stmt `query:"pending-for-contact"`
	ClaimOverdue      *sqlx.Stmt `query:"claim-overdue"`
	GetManagerIDs     *sqlx.Stmt `query:"get-manager-ids"`
}

// New returns a Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, lo: opts.Lo, i18n: opts.I18n}, nil
}

// RecordMissed opens a call-back for a missed call, or adds the call to the
// contact's open one. created is true for a new call-back, which is when the
// agents are told and the caller is sent a WhatsApp message; a caller who
// keeps calling isn't messaged again for each call.
func (m *Manager) RecordMissed(tenantID int, in models.Missed) (models.CallBack, bool, error) {
	if in.DueIn <= 0 {
		in.DueIn = DefaultDueIn
	}
	var row struct {
		ID       int64 `db:"id"`
		Inserted bool  `db:"inserted"`
	}
	if err := m.q.RecordMissed.Get(&row, tenantID, in.ContactID, in.ConversationID, in.InboxID, in.Phone, in.CallSID,
		in.Voicemail, in.DueIn.Seconds()); err != nil {
		m.lo.Error("error recording missed call", "contact_id", in.ContactID, "call_sid", in.CallSID, "error", err)
		return models.CallBack{}, false, err
	}
	cb, err := m.Get(tenantID, row.ID)
	return cb, row.Inserted, err
}

// CloseForContact closes a contact's open call-back when a call with them
// connects: reason says whether an agent called back or the contact got
// through. byID is the agent on the call, or 0. It returns the closed
// call-back's ID, or 0 if none was open.
func (m *Manager) CloseForContact(tenantID, contactID int, reason string, byID int) (int64, error) {
	var id int64
	if err := m.q.CloseForContact.Get(&id, tenantID, contactID, reason, null.NewInt(byID, byID > 0)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		m.lo.Error("error closing call-back", "contact_id", contactID, "error", err)
		return 0, err
	}
	return id, nil
}

// AddAttempt counts an agent's call to a contact with an open call-back that
// didn't connect. The call-back stays open.
func (m *Manager) AddAttempt(tenantID, contactID int) (int64, error) {
	var id int64
	if err := m.q.AddAttempt.Get(&id, tenantID, contactID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		m.lo.Error("error counting call-back attempt", "contact_id", contactID, "error", err)
		return 0, err
	}
	return id, nil
}

// MarkDone closes a call-back by hand.
func (m *Manager) MarkDone(tenantID int, id int64, userID int) error {
	var got int64
	if err := m.q.MarkDone.Get(&got, tenantID, id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return envelope.NewError(envelope.NotFoundError, m.i18n.For(tenantID).T("callBack.notFound"), nil)
		}
		m.lo.Error("error marking call-back done", "id", id, "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// SetWhatsApp records the WhatsApp message sent to the caller, or why it
// couldn't be sent.
func (m *Manager) SetWhatsApp(tenantID int, id int64, messageID int, sendErr string) {
	if _, err := m.q.SetWhatsApp.Exec(tenantID, id, null.NewInt(messageID, messageID > 0), null.NewString(sendErr, sendErr != "")); err != nil {
		m.lo.Error("error recording call-back whatsapp message", "id", id, "error", err)
	}
}

// WhatsAppSentSince reports whether a contact was sent a missed-call message
// since t, so a caller hears from the business on WhatsApp once a day at most.
func (m *Manager) WhatsAppSentSince(tenantID, contactID int, t time.Time) (bool, error) {
	var sent bool
	if err := m.q.WhatsAppSentSince.Get(&sent, tenantID, contactID, t); err != nil {
		m.lo.Error("error checking missed-call whatsapp messages", "contact_id", contactID, "error", err)
		return false, err
	}
	return sent, nil
}

// Get returns a call-back.
func (m *Manager) Get(tenantID int, id int64) (models.CallBack, error) {
	var cb models.CallBack
	if err := m.q.Get.Get(&cb, tenantID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return cb, envelope.NewError(envelope.NotFoundError, m.i18n.For(tenantID).T("callBack.notFound"), nil)
		}
		m.lo.Error("error fetching call-back", "id", id, "error", err)
		return cb, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return cb, nil
}

// PendingForContact returns the ID of a contact's open call-back, or 0.
func (m *Manager) PendingForContact(tenantID, contactID int) (int64, error) {
	var id int64
	if err := m.q.PendingForContact.Get(&id, tenantID, contactID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return id, nil
}

// List returns a page of the call-backs scope may see, and how many there are.
func (m *Manager) List(tenantID int, scope models.Scope, opts models.ListOpts) ([]models.CallBack, int, error) {
	if opts.Status != models.StatusDone {
		opts.Status = models.StatusPending
	}
	if opts.PerPage <= 0 || opts.PerPage > maxPerPage {
		opts.PerPage = maxPerPage
	}
	if opts.Page < 1 {
		opts.Page = 1
	}
	out := []models.CallBack{}
	if err := m.q.List.Select(&out, tenantID, opts.Status, scope.All, scope.UserID, scope.Assigned, pq.Array(scope.TeamIDs),
		scope.TeamAll, scope.TeamInbox, scope.Unassigned, opts.MineOnly, opts.PerPage, (opts.Page-1)*opts.PerPage); err != nil {
		m.lo.Error("error listing call-backs", "error", err)
		return nil, 0, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	total := 0
	if len(out) > 0 {
		total = out[0].Total
	}
	return out, total, nil
}

// Counts returns how many call-backs scope may see are waiting, and overdue.
func (m *Manager) Counts(tenantID int, scope models.Scope) (models.Counts, error) {
	var c models.Counts
	if err := m.q.Counts.Get(&c, tenantID, scope.All, scope.UserID, scope.Assigned, pq.Array(scope.TeamIDs),
		scope.TeamAll, scope.TeamInbox, scope.Unassigned); err != nil {
		m.lo.Error("error counting call-backs", "error", err)
		return c, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return c, nil
}

// ManagerIDs returns the agents who see reports, who hear about overdue
// call-backs as well as the agent they belong to.
func (m *Manager) ManagerIDs(tenantID int) ([]int, error) {
	var ids []int
	if err := m.q.GetManagerIDs.Select(&ids, tenantID); err != nil {
		m.lo.Error("error fetching managers", "error", err)
		return nil, err
	}
	return ids, nil
}

// RunOverdueChecker escalates call-backs as they go overdue, every interval,
// calling onOverdue with each and its tenant, once. The claim is in the
// database, so each is escalated once however many instances run.
func (m *Manager) RunOverdueChecker(ctx context.Context, interval time.Duration, onOverdue func(tenantID int, cb models.CallBack)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.escalateOverdue(onOverdue)
		}
	}
}

func (m *Manager) escalateOverdue(onOverdue func(int, models.CallBack)) {
	var due []struct {
		TenantID int   `db:"tenant_id"`
		ID       int64 `db:"id"`
	}
	if err := m.q.ClaimOverdue.Select(&due); err != nil {
		m.lo.Error("error claiming overdue call-backs", "error", err)
		return
	}
	for _, d := range due {
		cb, err := m.Get(d.TenantID, d.ID)
		if err != nil {
			continue
		}
		onOverdue(d.TenantID, cb)
	}
}
