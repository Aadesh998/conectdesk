// Package mfa manages agents' two-factor login: authenticator app (TOTP)
// codes and one-time recovery codes.
package mfa

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/totp"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS

	// ErrInvalidCode is returned for a wrong, reused or expired code.
	ErrInvalidCode = errors.New("invalid two-factor code")
	// ErrNotEnabled is returned when the agent has no two-factor login.
	ErrNotEnabled = errors.New("two-factor login is not enabled")
)

// RecoveryCodeCount is how many recovery codes an agent gets.
const RecoveryCodeCount = 10

// Status is an agent's two-factor login.
type Status struct {
	Enabled           bool      `json:"enabled"`
	EnabledAt         null.Time `json:"enabled_at"`
	RecoveryCodesLeft int       `json:"recovery_codes_left"`
}

type totpRow struct {
	Secret            string    `db:"secret"`
	EnabledAt         null.Time `db:"enabled_at"`
	RecoveryCodesLeft int       `db:"recovery_codes_left"`
}

type queries struct {
	GetTOTP          *sqlx.Stmt `query:"get-totp"`
	EnableTOTP       *sqlx.Stmt `query:"enable-totp"`
	DisableTOTP      *sqlx.Stmt `query:"disable-totp"`
	UseTOTPStep      *sqlx.Stmt `query:"use-totp-step"`
	UseRecoveryCode  *sqlx.Stmt `query:"use-recovery-code"`
	SetRecoveryCodes *sqlx.Stmt `query:"set-recovery-codes"`
}

// Manager manages two-factor login.
type Manager struct {
	q             queries
	lo            *logf.Logger
	encryptionKey string
	now           func() time.Time
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
	return &Manager{q: q, lo: opts.Lo, encryptionKey: opts.EncryptionKey, now: time.Now}, nil
}

// Status returns the agent's two-factor login status.
func (m *Manager) Status(tenantID, userID int) (Status, error) {
	row, err := m.row(tenantID, userID)
	if err != nil {
		return Status{}, err
	}
	return Status{Enabled: row.EnabledAt.Valid, EnabledAt: row.EnabledAt, RecoveryCodesLeft: row.RecoveryCodesLeft}, nil
}

// Enable switches two-factor login on with a secret the agent's app shows
// code for, and returns the recovery codes, shown to the agent this once.
func (m *Manager) Enable(tenantID, userID int, secret, code string) ([]string, error) {
	step, ok := totp.Verify(secret, code, m.now())
	if !ok {
		return nil, ErrInvalidCode
	}
	enc, err := crypto.Encrypt(secret, m.encryptionKey)
	if err != nil {
		return nil, err
	}
	codes, hashes, err := NewRecoveryCodeSet()
	if err != nil {
		return nil, err
	}
	if _, err := m.q.EnableTOTP.Exec(tenantID, userID, enc, pq.Array(hashes), int64(step)); err != nil {
		m.lo.Error("error enabling two-factor login", "user_id", userID, "error", err)
		return nil, err
	}
	return codes, nil
}

// Disable switches the agent's two-factor login off.
func (m *Manager) Disable(tenantID, userID int) error {
	if _, err := m.q.DisableTOTP.Exec(tenantID, userID); err != nil {
		m.lo.Error("error disabling two-factor login", "user_id", userID, "error", err)
		return err
	}
	return nil
}

// Verify checks a code from the agent's authenticator app, or one of their
// recovery codes, which then can't be used again. A code is accepted once.
func (m *Manager) Verify(tenantID, userID int, code string) (usedRecoveryCode bool, err error) {
	row, err := m.row(tenantID, userID)
	if err != nil {
		return false, err
	}
	if !row.EnabledAt.Valid {
		return false, ErrNotEnabled
	}

	if IsRecoveryCode(code) {
		var left int
		if err := m.q.UseRecoveryCode.Get(&left, tenantID, userID, HashRecoveryCode(code)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, ErrInvalidCode
			}
			return false, err
		}
		return true, nil
	}

	secret, err := crypto.Decrypt(row.Secret, m.encryptionKey)
	if err != nil {
		m.lo.Error("error decrypting two-factor secret", "user_id", userID, "error", err)
		return false, err
	}
	step, ok := totp.Verify(secret, code, m.now())
	if !ok {
		return false, ErrInvalidCode
	}
	var id int
	if err := m.q.UseTOTPStep.Get(&id, tenantID, userID, int64(step)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrInvalidCode // Already used.
		}
		return false, err
	}
	return false, nil
}

// NewRecoveryCodes replaces the agent's recovery codes and returns the new ones.
func (m *Manager) NewRecoveryCodes(tenantID, userID int) ([]string, error) {
	codes, hashes, err := NewRecoveryCodeSet()
	if err != nil {
		return nil, err
	}
	res, err := m.q.SetRecoveryCodes.Exec(tenantID, userID, pq.Array(hashes))
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotEnabled
	}
	return codes, nil
}

func (m *Manager) row(tenantID, userID int) (totpRow, error) {
	var row totpRow
	if err := m.q.GetTOTP.Get(&row, tenantID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return row, ErrNotEnabled
		}
		m.lo.Error("error fetching two-factor login", "user_id", userID, "error", err)
		return row, err
	}
	return row, nil
}

var recoveryEnc = base32.NewEncoding("abcdefghijkmnpqrstuvwxyz23456789").WithPadding(base32.NoPadding)

// NewRecoveryCodeSet returns fresh recovery codes like "k7m2p-9xq4w" and the
// hashes they are stored as.
func NewRecoveryCodeSet() ([]string, []string, error) {
	codes := make([]string, RecoveryCodeCount)
	hashes := make([]string, RecoveryCodeCount)
	for i := range codes {
		b := make([]byte, 7)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, err
		}
		s := recoveryEnc.EncodeToString(b)[:10]
		codes[i] = s[:5] + "-" + s[5:]
		hashes[i] = HashRecoveryCode(codes[i])
	}
	return codes, hashes, nil
}

func normalizeRecoveryCode(code string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
}

// IsRecoveryCode tells a recovery code from an app's six digits.
func IsRecoveryCode(code string) bool {
	return len(normalizeRecoveryCode(code)) == 10
}

// HashRecoveryCode returns the hash a recovery code is stored as.
func HashRecoveryCode(code string) string {
	h := sha256.Sum256([]byte(normalizeRecoveryCode(code)))
	return hex.EncodeToString(h[:])
}
