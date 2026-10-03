package tenant_test

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	activitylog "github.com/conectdesk/conectdesk/internal/activity_log"
	businesshours "github.com/conectdesk/conectdesk/internal/business_hours"
	contextlink "github.com/conectdesk/conectdesk/internal/context_link"
	clmodels "github.com/conectdesk/conectdesk/internal/context_link/models"
	"github.com/conectdesk/conectdesk/internal/conversation/priority"
	"github.com/conectdesk/conectdesk/internal/conversation/status"
	"github.com/conectdesk/conectdesk/internal/csat"
	customattribute "github.com/conectdesk/conectdesk/internal/custom_attribute"
	camodels "github.com/conectdesk/conectdesk/internal/custom_attribute/models"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/macro"
	"github.com/conectdesk/conectdesk/internal/media"
	notification "github.com/conectdesk/conectdesk/internal/notification"
	"github.com/conectdesk/conectdesk/internal/report"
	"github.com/conectdesk/conectdesk/internal/role"
	rmodels "github.com/conectdesk/conectdesk/internal/role/models"
	"github.com/conectdesk/conectdesk/internal/search"
	"github.com/conectdesk/conectdesk/internal/setting"
	"github.com/conectdesk/conectdesk/internal/tag"
	"github.com/conectdesk/conectdesk/internal/team"
	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/conectdesk/conectdesk/internal/view"
	"github.com/conectdesk/conectdesk/internal/webhook"
	wmodels "github.com/conectdesk/conectdesk/internal/webhook/models"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

// Two tenants: acme owns the rows, foo tries to reach them.
const acme, foo = 2, 3

type env struct {
	db   *sqlx.DB
	lo   *logf.Logger
	i18n *i18ncat.Catalog
}

func newEnv(t *testing.T, name string) env {
	t.Helper()
	db := testutil.NewDB(t, "isolation_"+name)
	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'acme', 'Acme'), (3, 'foo', 'Foo')`)
	lo := logf.New(logf.Opts{})
	return env{db: db, lo: &lo, i18n: testutil.NewI18n(t)}
}

// agent inserts an agent into the tenant and returns its ID.
func (e env) agent(t *testing.T, tenantID int) int {
	t.Helper()
	var id int
	if err := e.db.Get(&id, `INSERT INTO users (tenant_id, type, email, first_name) VALUES ($1, 'agent', $2, 'A') RETURNING id`,
		tenantID, t.Name()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	return id
}

func mustOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// mustFail checks foo's attempt at acme's row errored.
func mustFail(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: foo reached acme's row", what)
	}
}

func TestIsolationTags(t *testing.T) {
	e := newEnv(t, "tags")
	m, err := tag.New(tag.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)

	a, err := m.Create(acme, "vip")
	mustOK(t, err)
	_, err = m.Create(foo, "vip")
	mustOK(t, err) // names are per tenant

	_, err = m.Update(foo, a.ID, "hacked")
	mustFail(t, "update", err)
	mustOK(t, m.Delete(foo, a.ID))

	list, err := m.GetAll(acme)
	mustOK(t, err)
	if len(list) != 1 || list[0].Name != "vip" {
		t.Fatalf("acme tags after foo's writes: %+v", list)
	}
	fooList, _ := m.GetAll(foo)
	if len(fooList) != 1 || fooList[0].ID == a.ID {
		t.Fatalf("foo sees %+v", fooList)
	}
}

func TestIsolationRoles(t *testing.T) {
	e := newEnv(t, "roles")
	m, err := role.New(role.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)

	a, err := m.Create(acme, rmodels.Role{Name: "Support", Description: "d", Permissions: []string{"conversations:read"}})
	mustOK(t, err)
	_, err = m.Get(foo, a.ID)
	mustFail(t, "get", err)
	_, err = m.Update(foo, a.ID, rmodels.Role{Name: "Owned", Description: "d", Permissions: []string{"conversations:read"}})
	mustFail(t, "update", err)
	mustFail(t, "delete", m.Delete(foo, a.ID))
	if got, err := m.Get(acme, a.ID); err != nil || got.Name != "Support" {
		t.Fatalf("acme role after foo's writes: %+v, %v", got, err)
	}
	fooRoles, _ := m.GetAll(foo)
	if len(fooRoles) != 0 {
		t.Fatalf("foo sees roles %+v", fooRoles)
	}
}

func TestIsolationStatusesAndPriorities(t *testing.T) {
	e := newEnv(t, "statuses")
	s, err := status.New(status.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)
	a, err := s.Create(acme, "Escalated", "open")
	mustOK(t, err)
	_, err = s.Get(foo, a.ID)
	mustFail(t, "get status", err)
	_, err = s.Update(foo, a.ID, "Owned", "open")
	mustFail(t, "update status", err)
	mustFail(t, "delete status", s.Delete(foo, a.ID))
	if all, _ := s.GetAll(foo); len(all) != 0 {
		t.Fatalf("foo sees statuses %+v", all)
	}

	p, err := priority.New(priority.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)
	var pid int
	mustOK(t, e.db.Get(&pid, `INSERT INTO conversation_priorities (tenant_id, name) VALUES (2, 'Urgent') RETURNING id`))
	_, err = p.Get(foo, pid)
	mustFail(t, "get priority", err)
	if all, _ := p.GetAll(foo); len(all) != 0 {
		t.Fatalf("foo sees priorities %+v", all)
	}
}

func TestIsolationMacrosAndViews(t *testing.T) {
	e := newEnv(t, "macros")
	mm, err := macro.New(macro.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)
	acmeAgent, fooAgent := e.agent(t, acme), e.agent(t, foo)

	a, err := mm.Create(acme, "Greet", "Hi", nil, nil, "all", []string{"replying"}, []byte("[]"))
	mustOK(t, err)
	_, err = mm.Get(foo, a.ID)
	mustFail(t, "get macro", err)
	_, err = mm.Update(foo, a.ID, "Owned", "x", nil, nil, "all", []string{"replying"}, []byte("[]"))
	mustFail(t, "update macro", err)
	mustFail(t, "delete macro", mm.Delete(foo, a.ID))
	// A macro can't be scoped to another tenant's agent.
	_, err = mm.Create(foo, "Spy", "x", &acmeAgent, nil, "user", []string{"replying"}, []byte("[]"))
	mustFail(t, "macro for acme's agent", err)

	vm, err := view.New(view.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)
	v, err := vm.Create(acme, "Mine", []byte("[]"), acmeAgent)
	mustOK(t, err)
	_, err = vm.Get(foo, v.ID)
	mustFail(t, "get view", err)
	if views, _ := vm.GetUsersViews(foo, acmeAgent); len(views) != 0 {
		t.Fatalf("foo lists acme's agent's views: %+v", views)
	}
	_, err = vm.Create(foo, "Spy", []byte("[]"), acmeAgent)
	mustFail(t, "view for acme's agent", err)
	_, err = vm.Create(foo, "Own", []byte("[]"), fooAgent)
	mustOK(t, err)
}

func TestIsolationBusinessHoursAndAttributes(t *testing.T) {
	e := newEnv(t, "hours")
	bh, err := businesshours.New(businesshours.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)
	a, err := bh.Create(acme, "Office", null.String{}, true, types.JSONText(`{}`), types.JSONText(`[]`))
	mustOK(t, err)
	_, err = bh.Get(foo, a.ID)
	mustFail(t, "get business hours", err)
	_, err = bh.Update(foo, a.ID, "Owned", null.String{}, true, types.JSONText(`{}`), types.JSONText(`[]`))
	mustFail(t, "update business hours", err)
	if all, _ := bh.GetAll(foo); len(all) != 0 {
		t.Fatalf("foo sees business hours %+v", all)
	}

	ca, err := customattribute.New(customattribute.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)
	attr := camodels.CustomAttribute{Name: "Plan", AppliesTo: "contact", Key: "plan", DataType: "text", Values: []string{}}
	c, err := ca.Create(acme, attr)
	mustOK(t, err)
	_, err = ca.Create(foo, attr)
	mustOK(t, err) // keys are per tenant
	_, err = ca.Get(foo, c.ID)
	mustFail(t, "get custom attribute", err)
	if all, _ := ca.GetAll(foo, ""); len(all) != 1 || all[0].ID == c.ID {
		t.Fatalf("foo sees custom attributes %+v", all)
	}
}

// conversation inserts a conversation, with its contact, inbox and status, into
// the tenant and returns its ID.
func (e env) conversation(t *testing.T, tenantID int, email string) int {
	t.Helper()
	var contact, inbox, status, id int
	mustOK(t, e.db.Get(&contact, `INSERT INTO users (tenant_id, type, email, first_name, last_name) VALUES ($1, 'contact', $2, 'C', '') RETURNING id`, tenantID, email))
	mustOK(t, e.db.Get(&inbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES ($1, 'Mail', 'email', '{}') RETURNING id`, tenantID))
	if err := e.db.Get(&status, `SELECT id FROM conversation_statuses WHERE tenant_id = $1 AND name = 'Open'`, tenantID); err != nil {
		mustOK(t, e.db.Get(&status, `INSERT INTO conversation_statuses (tenant_id, name) VALUES ($1, 'Open') RETURNING id`, tenantID))
	}
	mustOK(t, e.db.Get(&id, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
		VALUES ($1, $2, $3, $4, generate_reference_number($1, '')) RETURNING id`, tenantID, contact, inbox, status))
	return id
}

func TestIsolationTeams(t *testing.T) {
	e := newEnv(t, "teams")
	m, err := team.New(team.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)
	a, err := m.Create(acme, "Sales", "", "Manual", null.Int{}, null.Int{}, "", 0)
	mustOK(t, err)
	_, err = m.Create(foo, "Sales", "", "Manual", null.Int{}, null.Int{}, "", 0)
	mustOK(t, err) // names are per tenant

	_, err = m.Get(foo, a.ID)
	mustFail(t, "get team", err)
	_, err = m.Update(foo, a.ID, "Owned", "", "Manual", null.Int{}, null.Int{}, "", 0)
	mustFail(t, "update team", err)

	// Adding a foo agent to teams by name only reaches foo's "Sales".
	fooAgent := e.agent(t, foo)
	mustOK(t, m.UpsertUserTeams(foo, fooAgent, []string{"Sales"}))
	var n int
	mustOK(t, e.db.Get(&n, `SELECT COUNT(*) FROM team_members WHERE team_id = $1`, a.ID))
	if n != 0 {
		t.Fatal("foo's agent joined acme's team")
	}
	members, err := m.GetMembers(foo, a.ID)
	mustOK(t, err)
	if len(members) != 0 {
		t.Fatalf("foo lists acme's team members: %+v", members)
	}
	if all, _ := m.GetAll(foo); len(all) != 1 || all[0].ID == a.ID {
		t.Fatalf("foo sees teams %+v", all)
	}
}

func TestIsolationCSATSettingsSearchReports(t *testing.T) {
	e := newEnv(t, "csat")
	conv := e.conversation(t, acme, "ann@acme.test")

	cs, err := csat.New(csat.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)
	resp, err := cs.Create(acme, conv)
	mustOK(t, err)
	_, err = cs.Get(foo, resp.UUID)
	mustFail(t, "get CSAT by UUID", err)
	_, err = cs.Create(foo, conv)
	mustFail(t, "CSAT for acme's conversation", err)

	st, err := setting.New(setting.Opts{DB: e.db, Lo: e.lo, EncryptionKey: "0123456789abcdef0123456789abcdef"})
	mustOK(t, err)
	mustOK(t, e.db.Select(new([]int), `INSERT INTO settings (tenant_id, key, value) VALUES (2, 'app.site_name', '"Acme Desk"'), (3, 'app.site_name', '"Foo Desk"') RETURNING 1`))
	mustOK(t, st.Update(foo, map[string]any{"app.site_name": "Foo Help"}))
	if v, _ := st.Get(acme, "app.site_name"); string(v) != `"Acme Desk"` {
		t.Fatalf("acme site name after foo's update: %s", v)
	}
	if v, _ := st.Get(foo, "app.site_name"); string(v) != `"Foo Help"` {
		t.Fatalf("foo site name: %s", v)
	}

	se, err := search.New(search.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)
	if res, err := se.Contacts(foo, "ann@acme"); err != nil || len(res) != 0 {
		t.Fatalf("foo's contact search found acme's contact: %+v, %v", res, err)
	}
	if res, err := se.Contacts(acme, "ann@acme"); err != nil || len(res) != 1 {
		t.Fatalf("acme's contact search: %+v, %v", res, err)
	}

	rp, err := report.New(report.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)
	counts, err := rp.GetOverViewCounts(foo)
	mustOK(t, err)
	var c struct{ Open int }
	mustOK(t, json.Unmarshal(counts, &c))
	if c.Open != 0 {
		t.Fatalf("foo's report counts acme's conversation: %s", counts)
	}

	al, err := activitylog.New(activitylog.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)
	mustOK(t, al.Login(acme, e.agent(t, acme), "a@acme.test", "127.0.0.1"))
	logs, err := al.GetAll(foo, "desc", "activity_logs.created_at", "", 1, 10, "")
	mustOK(t, err)
	if len(logs) != 0 {
		t.Fatalf("foo sees acme's activity logs: %+v", logs)
	}
}

func TestIsolationWebhooksAndContextLinks(t *testing.T) {
	e := newEnv(t, "webhooks")
	wh, err := webhook.New(webhook.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n, Workers: 1, QueueSize: 1, Timeout: time.Second, EncryptionKey: "0123456789abcdef0123456789abcdef"})
	mustOK(t, err)
	a, err := wh.Create(acme, wmodels.Webhook{Name: "CRM", URL: "https://crm.acme.test/hook", Events: []string{"conversation.created"}, IsActive: true})
	mustOK(t, err)
	_, err = wh.Get(foo, a.ID)
	mustFail(t, "get webhook", err)
	_, err = wh.Update(foo, a.ID, wmodels.Webhook{Name: "Owned", URL: "https://evil.test", Events: []string{"conversation.created"}, IsActive: true})
	mustFail(t, "update webhook", err)
	if all, _ := wh.GetAll(foo); len(all) != 0 {
		t.Fatalf("foo sees webhooks %+v", all)
	}

	cl, err := contextlink.New(contextlink.Opts{DB: e.db, Lo: e.lo, I18n: e.i18n, EncryptionKey: "0123456789abcdef0123456789abcdef"})
	mustOK(t, err)
	link, err := cl.Create(acme, clmodels.ContextLink{Name: "CRM", URLTemplate: "https://crm.acme.test/{{.Contact.Email}}", TokenExpirySeconds: 60, IsActive: true})
	mustOK(t, err)
	_, err = cl.Get(foo, link.ID)
	mustFail(t, "get context link", err)
	if all, _ := cl.GetAll(foo); len(all) != 0 {
		t.Fatalf("foo sees context links %+v", all)
	}
}

// fakeStore is a media store that keeps nothing.
type fakeStore struct{}

func (fakeStore) Put(name, _ string, _ io.ReadSeeker) (string, error) { return name, nil }
func (fakeStore) Delete(string) error                                 { return nil }
func (fakeStore) GetURL(tenantID int, name, _, _ string) string {
	return fmt.Sprintf("tenant-%d/%s", tenantID, name)
}
func (fakeStore) GetBlob(string) ([]byte, error)                       { return nil, nil }
func (fakeStore) Name() string                                         { return "fs" }
func (fakeStore) SignedURLValidator() func(string, string, int64) bool { return nil }

func TestIsolationMediaInboxesNotifications(t *testing.T) {
	e := newEnv(t, "media")
	md, err := media.New(media.Opts{Store: fakeStore{}, Lo: e.lo, DB: e.db, I18n: e.i18n,
		RootURL: func(id int) string { return fmt.Sprintf("https://t%d.example.com", id) }})
	mustOK(t, err)
	f, err := md.UploadAndInsert(acme, "a.png", "image/png", "", null.String{}, null.Int{}, strings.NewReader("x"), 1, null.String{}, []byte("{}"), false)
	mustOK(t, err)
	if f.URL != "https://t2.example.com/uploads/"+f.UUID {
		t.Fatalf("acme media URL %q is not on acme's host", f.URL)
	}
	_, err = md.Get(foo, 0, f.UUID)
	mustFail(t, "get media by UUID", err)
	_, err = md.Get(foo, f.ID, "")
	mustFail(t, "get media by ID", err)
	mustOK(t, md.Delete(foo, f.UUID))
	if _, err := md.Get(acme, f.ID, ""); err != nil {
		t.Fatalf("foo deleted acme's media: %v", err)
	}

	ib, err := inbox.New(e.lo, e.db, e.i18n, "0123456789abcdef0123456789abcdef")
	mustOK(t, err)
	var inboxID int
	var inboxUUID string
	mustOK(t, e.db.QueryRow(`INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (2, 'Mail', 'email', '{}') RETURNING id, uuid`).Scan(&inboxID, &inboxUUID))
	_, err = ib.GetDBRecord(foo, inboxID)
	mustFail(t, "get inbox by ID", err)
	_, err = ib.GetDBRecord(foo, inboxUUID)
	mustFail(t, "get inbox by UUID", err)
	_, err = ib.Toggle(foo, inboxID)
	mustFail(t, "toggle inbox", err)
	if all, _ := ib.GetAll(foo); len(all) != 0 {
		t.Fatalf("foo sees inboxes %+v", all)
	}

	un, err := notification.NewUserNotificationManager(notification.UserNotificationOpts{DB: e.db, Lo: e.lo, I18n: e.i18n})
	mustOK(t, err)
	acmeAgent := e.agent(t, acme)
	n, err := un.Create(acme, acmeAgent, "mention", "hi", null.String{}, null.Int{}, null.Int{}, null.Int{}, []byte("{}"))
	mustOK(t, err)
	// Marking read is idempotent, so check the effect rather than the error.
	mustOK(t, un.MarkAsRead(foo, n.ID, acmeAgent))
	if stats, _ := un.GetStats(acme, acmeAgent); stats.UnreadCount != 1 {
		t.Fatalf("foo marked acme's notification read: %+v", stats)
	}
	if list, _ := un.GetAll(foo, acmeAgent, 10, 0); len(list) != 0 {
		t.Fatalf("foo lists acme's agent's notifications: %+v", list)
	}
	_, err = un.Create(foo, acmeAgent, "mention", "spy", null.String{}, null.Int{}, null.Int{}, null.Int{}, []byte("{}"))
	mustFail(t, "notify acme's agent from foo", err)
}
