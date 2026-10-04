// Package softphone keeps the SIP accounts agents take calls on in their
// browser. An admin enters the SIP user Exotel issued for an agent; the
// agent's browser signs in to it over a secure WebSocket, and calls that
// would ring their phone ring there instead.
package softphone

import (
	"database/sql"
	"embed"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS

	// ErrInvalidUsername, ErrInvalidDomain, ErrInvalidServer, ErrInvalidPort
	// and ErrNoPassword are an account that can't be saved.
	ErrInvalidUsername = errors.New("invalid SIP username")
	ErrInvalidDomain   = errors.New("invalid SIP domain")
	ErrInvalidServer   = errors.New("invalid SIP WebSocket server")
	ErrInvalidPort     = errors.New("invalid SIP port")
	ErrNoPassword      = errors.New("a SIP password is required")
	// ErrUsernameTaken is a SIP user another agent already has.
	ErrUsernameTaken = errors.New("SIP username is taken")

	usernameRe = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,100}$`)
	domainRe   = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)
)

// DefaultPort is the secure WebSocket port Exotel's SIP servers take.
const DefaultPort = 443

// Account is an agent's SIP account.
type Account struct {
	UserID   int    `db:"user_id" json:"user_id"`
	Username string `db:"username" json:"username"`
	// Password is decrypted. It is never sent back to admins, only to the
	// agent's own browser.
	Password string `db:"password" json:"-"`
	// Domain is the SIP domain. Server is the host of the secure WebSocket
	// the browser connects over, at Port; empty, it is Domain.
	Domain      string `db:"domain" json:"domain"`
	Server      string `db:"server" json:"server"`
	Port        int    `db:"port" json:"port"`
	RingBrowser bool   `db:"ring_browser" json:"ring_browser"`
}

type queries struct {
	GetAccount     *sqlx.Stmt `query:"get-account"`
	UpsertAccount  *sqlx.Stmt `query:"upsert-account"`
	DeleteAccount  *sqlx.Stmt `query:"delete-account"`
	SetRingBrowser *sqlx.Stmt `query:"set-ring-browser"`
}

// Manager manages agents' SIP accounts.
type Manager struct {
	q             queries
	lo            *logf.Logger
	encryptionKey string
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB            *sqlx.DB
	Lo            *logf.Logger
	EncryptionKey string
}

// New returns a Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{q: q, lo: opts.Lo, encryptionKey: opts.EncryptionKey}, nil
}

// Get returns an agent's SIP account, with its password decrypted, and
// whether they have one.
func (m *Manager) Get(tenantID, userID int) (Account, bool, error) {
	var a Account
	if err := m.q.GetAccount.Get(&a, tenantID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Account{}, false, nil
		}
		m.lo.Error("error fetching SIP account", "user_id", userID, "error", err)
		return Account{}, false, err
	}
	pw, err := crypto.Decrypt(a.Password, m.encryptionKey)
	if err != nil {
		m.lo.Error("error decrypting SIP password", "user_id", userID, "error", err)
		return Account{}, false, err
	}
	a.Password = pw
	return a, true, nil
}

// Normalize trims an account's fields and checks them. A new account needs a
// password; an existing one keeps its own when none is given.
func Normalize(a Account, isNew bool) (Account, error) {
	a.Username = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(a.Username), "sip:"))
	a.Domain = host(a.Domain)
	a.Server = host(a.Server)
	// A server entered as host:port gives the port.
	if h, p, ok := strings.Cut(a.Server, ":"); ok {
		port, err := strconv.Atoi(p)
		if err != nil {
			return a, ErrInvalidServer
		}
		a.Server, a.Port = h, port
	}
	if a.Server == a.Domain {
		a.Server = ""
	}
	if a.Port == 0 {
		a.Port = DefaultPort
	}
	switch {
	case !usernameRe.MatchString(a.Username):
		return a, ErrInvalidUsername
	case len(a.Domain) > 255 || !domainRe.MatchString(a.Domain):
		return a, ErrInvalidDomain
	case a.Server != "" && (len(a.Server) > 255 || !domainRe.MatchString(a.Server)):
		return a, ErrInvalidServer
	case a.Port < 1 || a.Port > 65535:
		return a, ErrInvalidPort
	case isNew && a.Password == "":
		return a, ErrNoPassword
	}
	return a, nil
}

// host is a hostname as typed: lowercase, without a scheme or a path.
func host(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, p := range []string{"wss://", "https://", "sip:"} {
		s = strings.TrimPrefix(s, p)
	}
	s, _, _ = strings.Cut(s, "/")
	return strings.TrimSuffix(s, ".")
}

// Save sets an agent's SIP account. An empty password keeps the saved one.
func (m *Manager) Save(tenantID, userID int, a Account) error {
	_, found, err := m.Get(tenantID, userID)
	if err != nil {
		return err
	}
	if a, err = Normalize(a, !found); err != nil {
		return err
	}
	enc, err := crypto.Encrypt(a.Password, m.encryptionKey)
	if err != nil {
		return err
	}
	if _, err := m.q.UpsertAccount.Exec(tenantID, userID, a.Username, enc, a.Domain, a.Server, a.Port); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return ErrUsernameTaken
		}
		m.lo.Error("error saving SIP account", "user_id", userID, "error", err)
		return err
	}
	return nil
}

// Delete removes an agent's SIP account: their calls ring their phone again.
func (m *Manager) Delete(tenantID, userID int) error {
	if _, err := m.q.DeleteAccount.Exec(tenantID, userID); err != nil {
		m.lo.Error("error deleting SIP account", "user_id", userID, "error", err)
		return err
	}
	return nil
}

// SetRingBrowser switches whether an agent's calls ring their browser. It
// reports false when they have no SIP account.
func (m *Manager) SetRingBrowser(tenantID, userID int, on bool) (bool, error) {
	res, err := m.q.SetRingBrowser.Exec(tenantID, userID, on)
	if err != nil {
		m.lo.Error("error switching browser calling", "user_id", userID, "error", err)
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
