// Package user managers all users in conectdesk - agents and contacts.
package user

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"log"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	rmodels "github.com/conectdesk/conectdesk/internal/role/models"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
	"golang.org/x/crypto/bcrypt"
)

var (
	//go:embed queries.sql
	efs embed.FS

	minPassword     = 10
	maxPassword     = 72
	maxListPageSize = 500

	// ErrPasswordTooLong is returned when the password passed to
	// GenerateFromPassword is too long (i.e. > 72 bytes).
	ErrPasswordTooLong = errors.New("password length exceeds 72 bytes")

	PasswordHint = fmt.Sprintf("Password must be %d-%d characters long should contain at least one uppercase letter, one lowercase letter, one number, and one special character.", minPassword, maxPassword)
)

const (
	lastActiveFlushDebounce = 30 * time.Second
	agentCacheTTL           = 10 * time.Minute
)

// Manager handles user-related operations.
type Manager struct {
	lo           *logf.Logger
	i18n         *i18ncat.Catalog
	q            queries
	db           *sqlx.DB
	agentCache   map[int]cachedAgent
	agentCacheMu sync.RWMutex

	lastActiveFlushAt   map[int]time.Time
	lastActiveFlushAtMu sync.Mutex
}

type cachedAgent struct {
	user      models.User
	expiresAt time.Time
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB *sqlx.DB
	Lo *logf.Logger
}

// queries contains prepared SQL queries.
type queries struct {
	GetUser                       *sqlx.Stmt `query:"get-user"`
	GetNotes                      *sqlx.Stmt `query:"get-notes"`
	GetNote                       *sqlx.Stmt `query:"get-note"`
	GetUserIDsByRole              *sqlx.Stmt `query:"get-user-ids-by-role"`
	GetUserByExternalID           *sqlx.Stmt `query:"get-user-by-external-id"`
	GetUsersCompact               string     `query:"get-users-compact"`
	UpdateContact                 *sqlx.Stmt `query:"update-contact"`
	UpdateContactBasicInfo        *sqlx.Stmt `query:"update-contact-basic-info"`
	UpdateAgent                   *sqlx.Stmt `query:"update-agent"`
	UpdateCustomAttributes        *sqlx.Stmt `query:"update-custom-attributes"`
	GetCustomAttributes           *sqlx.Stmt `query:"get-custom-attributes"`
	FillEmptyCustomAttributes     *sqlx.Stmt `query:"fill-empty-custom-attributes"`
	UpsertCustomAttributes        *sqlx.Stmt `query:"upsert-custom-attributes"`
	UpdateAvatar                  *sqlx.Stmt `query:"update-avatar"`
	UpdateAvailability            *sqlx.Stmt `query:"update-availability"`
	UpdateLastActiveAt            *sqlx.Stmt `query:"update-last-active-at"`
	UpdateInactiveOffline         *sqlx.Stmt `query:"update-inactive-offline"`
	GetAvailabilityStatus         *sqlx.Stmt `query:"get-availability-status"`
	UpdateLastLoginAt             *sqlx.Stmt `query:"update-last-login-at"`
	UpdateOwnName                 *sqlx.Stmt `query:"update-own-name"`
	UpdateOwnPhone                *sqlx.Stmt `query:"update-own-phone"`
	GetAgentsWithPhone            *sqlx.Stmt `query:"get-agents-with-phone"`
	SoftDeleteAgent               *sqlx.Stmt `query:"soft-delete-agent"`
	SetUserPassword               *sqlx.Stmt `query:"set-user-password"`
	SetResetPasswordToken         *sqlx.Stmt `query:"set-reset-password-token"`
	SetPassword                   *sqlx.Stmt `query:"set-password"`
	DeleteNote                    *sqlx.Stmt `query:"delete-note"`
	InsertAgent                   *sqlx.Stmt `query:"insert-agent"`
	InsertContactWithExtID        *sqlx.Stmt `query:"insert-contact-with-external-id"`
	InsertContactNoExtID          *sqlx.Stmt `query:"insert-contact-without-external-id"`
	InsertContactIfAbsent         *sqlx.Stmt `query:"insert-contact-if-absent"`
	GetContactByEmail             *sqlx.Stmt `query:"get-contact-by-email"`
	GetContactsByPhoneKey         *sqlx.Stmt `query:"get-contacts-by-phone-key"`
	GetContactByEmailWithoutExtID *sqlx.Stmt `query:"get-contact-by-email-without-ext-id"`
	IsEmailBlocked                *sqlx.Stmt `query:"is-email-blocked"`
	SetExternalUserID             *sqlx.Stmt `query:"set-external-user-id"`
	InsertNote                    *sqlx.Stmt `query:"insert-note"`
	InsertVisitor                 *sqlx.Stmt `query:"insert-visitor"`
	GetVisitorByEmail             *sqlx.Stmt `query:"get-visitor-by-email"`
	UpgradeVisitorToContact       *sqlx.Stmt `query:"upgrade-visitor-to-contact"`
	ToggleEnable                  *sqlx.Stmt `query:"toggle-enable"`

	// API key queries
	GetUserByAPIKey      *sqlx.Stmt `query:"get-user-by-api-key"`
	SetAPIKey            *sqlx.Stmt `query:"set-api-key"`
	RevokeAPIKey         *sqlx.Stmt `query:"revoke-api-key"`
	UpdateAPIKeyLastUsed *sqlx.Stmt `query:"update-api-key-last-used"`

	MergeVisitorToContact *sqlx.Stmt `query:"merge-visitor-to-contact"`
	DeleteContact         *sqlx.Stmt `query:"delete-contact"`
	ExportContactData     *sqlx.Stmt `query:"export-contact-data"`
}

// New creates and returns a new instance of the Manager.
func New(i18n *i18ncat.Catalog, opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, fmt.Errorf("error scanning SQL file: %w", err)
	}
	return &Manager{
		q:                 q,
		lo:                opts.Lo,
		i18n:              i18n,
		db:                opts.DB,
		agentCache:        make(map[int]cachedAgent),
		lastActiveFlushAt: make(map[int]time.Time),
	}, nil
}

// VerifyPassword authenticates an user by email and password, returning the user if successful.
func (u *Manager) VerifyPassword(tenantID int, email string, password []byte) (models.User, error) {
	var user models.User
	if err := u.q.GetUser.Get(&user, tenantID, 0, email, pq.Array([]string{models.UserTypeAgent})); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user, envelope.NewError(envelope.InputError, u.i18n.For(tenantID).T("user.invalidEmailPassword"), nil)
		}
		u.lo.Error("error fetching user from db", "error", err)
		return user, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if err := u.verifyPassword(password, user.Password.String); err != nil {
		return user, envelope.NewError(envelope.InputError, u.i18n.For(tenantID).T("user.invalidEmailPassword"), nil)
	}
	return user, nil
}

// GetAllUsers returns a list of all users.
func (u *Manager) GetAllUsers(tenantID, page, pageSize int, userTypes []string, order, orderBy string, filtersJSON, location string) ([]models.UserCompact, error) {
	query, qArgs, err := u.makeUserListQuery(tenantID, page, pageSize, userTypes, order, orderBy, filtersJSON, location)
	if err != nil {
		u.lo.Error("error creating user list query", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Start a read-only txn.
	tx, err := u.db.BeginTxx(context.Background(), &sql.TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		u.lo.Error("error starting read-only transaction", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	defer tx.Rollback()

	// Execute query
	var users = make([]models.UserCompact, 0)
	if err := tx.Select(&users, query, qArgs...); err != nil {
		u.lo.Error("error fetching users", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	return users, nil
}

// Get retrieves an user by ID or email or type. At least one of ID or email must be provided.
func (u *Manager) Get(tenantID, id int, email string, userType []string) (models.User, error) {
	if id == 0 && email == "" {
		return models.User{}, envelope.NewError(envelope.InputError, u.i18n.For(tenantID).T("validation.invalidUser"), nil)
	}

	var user models.User
	if err := u.q.GetUser.Get(&user, tenantID, id, email, pq.Array(userType)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user, envelope.NewError(envelope.NotFoundError, u.i18n.For(tenantID).T("validation.notFoundUser"), nil)
		}
		u.lo.Error("error fetching user from db", "error", err)
		return user, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return user, nil
}

// GetContactOrVisitor retrieves a user by ID or email that is either a contact or visitor.
func (u *Manager) GetContactOrVisitor(tenantID, id int, email string) (models.User, error) {
	return u.Get(tenantID, id, email, []string{models.UserTypeContact, models.UserTypeVisitor})
}

// GetSystemUser returns the tenant's system user.
func (u *Manager) GetSystemUser(tenantID int) (models.User, error) {
	return u.Get(tenantID, 0, models.SystemUserEmail, []string{models.UserTypeAgent})
}

// GetContactByExternalID retrieves a contact by external user ID.
func (u *Manager) GetContactByExternalID(tenantID int, externalUserID string) (models.User, error) {
	var user models.User
	if err := u.q.GetUserByExternalID.Get(&user, tenantID, externalUserID); err != nil {
		if err == sql.ErrNoRows {
			return user, envelope.NewError(envelope.NotFoundError, u.i18n.For(tenantID).T("validation.notFoundUser"), nil)
		}
		u.lo.Error("error fetching user by external ID", "external_user_id", externalUserID, "error", err)
		return user, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return user, nil
}

// GetContactByEmail retrieves a contact by email address regardless of external_user_id.
func (u *Manager) GetContactByEmail(tenantID int, email string) (models.User, error) {
	var user models.User
	if err := u.q.GetContactByEmail.Get(&user, tenantID, email); err != nil {
		if err == sql.ErrNoRows {
			return user, envelope.NewError(envelope.NotFoundError, u.i18n.For(tenantID).T("validation.notFoundUser"), nil)
		}
		u.lo.Error("error fetching contact by email", "email", email, "error", err)
		return user, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return user, nil
}

// GetContactByEmailWithoutExtID retrieves a contact by email that has no external_user_id set.
func (u *Manager) GetContactByEmailWithoutExtID(tenantID int, email string) (models.User, error) {
	var user models.User
	if err := u.q.GetContactByEmailWithoutExtID.Get(&user, tenantID, email); err != nil {
		if err == sql.ErrNoRows {
			return user, envelope.NewError(envelope.NotFoundError, u.i18n.For(tenantID).T("validation.notFoundUser"), nil)
		}
		u.lo.Error("error fetching contact by email without ext_id", "email", email, "error", err)
		return user, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return user, nil
}

// IsEmailBlocked checks if any contact or visitor with the given email is blocked.
func (u *Manager) IsEmailBlocked(tenantID int, email string) (bool, error) {
	var blocked bool
	if err := u.q.IsEmailBlocked.Get(&blocked, tenantID, email); err != nil {
		u.lo.Error("error checking if email is blocked", "email", email, "error", err)
		return false, fmt.Errorf("checking if email is blocked: %w", err)
	}
	return blocked, nil
}

// GetVisitorByEmail retrieves a visitor by email address.
func (u *Manager) GetVisitorByEmail(tenantID int, email string) (models.User, error) {
	var user models.User
	if err := u.q.GetVisitorByEmail.Get(&user, tenantID, email); err != nil {
		if err == sql.ErrNoRows {
			return user, envelope.NewError(envelope.NotFoundError, u.i18n.For(tenantID).T("validation.notFoundUser"), nil)
		}
		u.lo.Error("error fetching visitor by email", "email", email, "error", err)
		return user, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return user, nil
}

// UpgradeVisitorToContact changes a visitor's type to contact.
func (u *Manager) UpgradeVisitorToContact(tenantID, visitorID int) error {
	if _, err := u.q.UpgradeVisitorToContact.Exec(tenantID, visitorID); err != nil {
		u.lo.Error("error upgrading visitor to contact", "visitor_id", visitorID, "error", err)
		return fmt.Errorf("upgrading visitor to contact: %w", err)
	}
	return nil
}

// SetExternalUserID sets the external_user_id on an existing contact, reporting whether a row was updated.
func (u *Manager) SetExternalUserID(tenantID, id int, externalUserID string) (bool, error) {
	res, err := u.q.SetExternalUserID.Exec(tenantID, id, externalUserID)
	if err != nil {
		u.lo.Error("error setting external user ID", "id", id, "external_user_id", externalUserID, "error", err)
		return false, fmt.Errorf("setting external user ID: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("setting external user ID: %w", err)
	}
	return rows > 0, nil
}

// UpdateAvatar updates the user avatar.
func (u *Manager) UpdateAvatar(tenantID, id int, path string) error {
	if _, err := u.q.UpdateAvatar.Exec(tenantID, id, null.NewString(path, path != "")); err != nil {
		u.lo.Error("error updating user avatar", "error", err)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// UpdateLastLoginAt updates the last login timestamp of an user.
// UpdateName changes an agent's first and last name.
func (u *Manager) UpdateName(tenantID, id int, first, last string) error {
	first, last = strings.TrimSpace(first), strings.TrimSpace(last)
	if first == "" || len(first) > 140 || len(last) > 140 {
		return envelope.NewError(envelope.InputError, u.i18n.For(tenantID).Ts("globals.messages.empty", "name", u.i18n.For(tenantID).T("globals.terms.firstName")), nil)
	}
	if _, err := u.q.UpdateOwnName.Exec(tenantID, id, first, last); err != nil {
		u.lo.Error("error updating user name", "error", err)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	u.InvalidateAgentCache(id)
	return nil
}

// UpdatePhone sets the number an agent takes phone calls on, in international
// form. An empty number removes it.
func (u *Manager) UpdatePhone(tenantID, id int, phone string) error {
	phone = strings.TrimSpace(phone)
	if !ValidAgentPhone(phone) {
		return envelope.NewError(envelope.InputError, u.i18n.For(tenantID).T("voice.invalidAgentPhone"), nil)
	}
	if _, err := u.q.UpdateOwnPhone.Exec(tenantID, id, phone); err != nil {
		u.lo.Error("error updating user phone", "error", err)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	u.InvalidateAgentCache(id)
	return nil
}

// ValidAgentPhone reports whether phone is empty, clearing it, or a number
// with its country code: a plus and 8 to 15 digits, spaces, dashes and
// brackets allowed.
func ValidAgentPhone(phone string) bool {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return true
	}
	digits := 0
	for _, r := range phone {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '+' || r == ' ' || r == '-' || r == '(' || r == ')':
		default:
			return false
		}
	}
	return digits >= 8 && digits <= 15 && strings.HasPrefix(phone, "+")
}

// PhoneAgent is an agent who can take phone calls.
type PhoneAgent struct {
	ID           int    `db:"id"`
	FirstName    string `db:"first_name"`
	LastName     string `db:"last_name"`
	PhoneNumber  string `db:"phone_number"`
	Availability string `db:"availability_status"`
}

// AgentsWithPhone returns the enabled agents who have a phone number set.
func (u *Manager) AgentsWithPhone(tenantID int) ([]PhoneAgent, error) {
	var out []PhoneAgent
	if err := u.q.GetAgentsWithPhone.Select(&out, tenantID); err != nil {
		u.lo.Error("error fetching agents with phone numbers", "error", err)
		return nil, err
	}
	return out, nil
}

func (u *Manager) UpdateLastLoginAt(tenantID, id int) error {
	if _, err := u.q.UpdateLastLoginAt.Exec(tenantID, id); err != nil {
		u.lo.Error("error updating user last login at", "error", err)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// SetResetPasswordToken sets a reset password token for an user and returns the token.
func (u *Manager) SetResetPasswordToken(tenantID, id int) (string, error) {
	token, err := stringutil.RandomAlphanumeric(32)
	if err != nil {
		u.lo.Error("error generating reset password token", "error", err)
		return "", envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if _, err := u.q.SetResetPasswordToken.Exec(tenantID, id, token); err != nil {
		u.lo.Error("error setting reset password token", "error", err)
		return "", envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return token, nil
}

// ResetPassword sets a password for a given user's reset password token and returns the user ID.
func (u *Manager) ResetPassword(tenantID int, token, password string) (int, error) {
	if !IsStrongPassword(password) {
		return 0, envelope.NewError(envelope.InputError, "Password is not strong enough, "+PasswordHint, nil)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		u.lo.Error("error generating bcrypt password", "error", err)
		return 0, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	var id int
	if err := u.q.SetPassword.Get(&id, tenantID, passwordHash, token); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, envelope.NewError(envelope.InputError, u.i18n.For(tenantID).T("user.resetPasswordTokenExpired"), nil)
		}
		u.lo.Error("error setting new password", "error", err)
		return 0, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return id, nil
}

// UpdateAvailability updates the availability status of an user.
func (u *Manager) UpdateAvailability(tenantID, id int, status string) error {
	if _, err := u.q.UpdateAvailability.Exec(tenantID, id, status); err != nil {
		u.lo.Error("error updating user availability", "error", err)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// UpdateLastActive updates last_active_at and returns true if the user flipped from offline to online.
func (u *Manager) UpdateLastActive(tenantID, id int) (wasOffline bool, err error) {
	agent, cachedOK := u.GetAgentFromCache(tenantID, id)
	alreadyOnline := cachedOK && agent.AvailabilityStatus == models.Online

	// Already online and within the debounce window - nothing to do.
	if alreadyOnline && !u.reserveFlush(id) {
		return false, nil
	}

	if err := u.q.UpdateLastActiveAt.Get(&wasOffline, tenantID, id); err != nil {
		u.lo.Error("error updating user last active at", "error", err)
		return false, fmt.Errorf("updating user last active at: %w", err)
	}

	if wasOffline {
		u.InvalidateAgentCache(id)
	}
	return wasOffline, nil
}

// IsOffline returns true if the user's availability status is offline.
func (u *Manager) IsOffline(tenantID, id int) bool {
	var status string
	if err := u.q.GetAvailabilityStatus.Get(&status, tenantID, id); err != nil {
		return true
	}
	return status == "offline"
}

// SaveCustomAttributes sets or merges custom attributes for a user.
// If replace is true, existing attributes are overwritten. Otherwise, attributes are merged.
func (u *Manager) SaveCustomAttributes(tenantID, id int, customAttributes map[string]any, replace bool) error {
	jsonb, err := json.Marshal(customAttributes)
	if err != nil {
		u.lo.Error("error marshalling custom attributes", "error", err)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	var execErr error
	if replace {
		_, execErr = u.q.UpdateCustomAttributes.Exec(tenantID, id, jsonb)
	} else {
		_, execErr = u.q.UpsertCustomAttributes.Exec(tenantID, id, jsonb)
	}
	if execErr != nil {
		u.lo.Error("error saving custom attributes", "error", execErr)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// GetCustomAttributes returns a user's custom attributes.
func (u *Manager) GetCustomAttributes(tenantID, id int) (map[string]any, error) {
	var b json.RawMessage
	if err := u.q.GetCustomAttributes.Get(&b, tenantID, id); err != nil {
		u.lo.Error("error fetching custom attributes", "user_id", id, "error", err)
		return nil, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out, nil
}

// FillEmptyCustomAttributes sets the attributes a user doesn't have yet, or
// has empty, leaving every set value as it is, in one statement so a value
// saved meanwhile isn't overwritten. It returns all the user's attributes.
func (u *Manager) FillEmptyCustomAttributes(tenantID, id int, attrs map[string]any) (map[string]any, error) {
	b, err := json.Marshal(attrs)
	if err != nil {
		return nil, err
	}
	var merged json.RawMessage
	if err := u.q.FillEmptyCustomAttributes.Get(&merged, tenantID, id, b); err != nil {
		u.lo.Error("error filling custom attributes", "user_id", id, "error", err)
		return nil, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	out := map[string]any{}
	_ = json.Unmarshal(merged, &out)
	return out, nil
}

// ToggleEnabled toggles the enabled status of an user.
func (u *Manager) ToggleEnabled(tenantID, id int, typ string, enabled bool) error {
	if _, err := u.q.ToggleEnable.Exec(tenantID, id, typ, enabled); err != nil {
		u.lo.Error("error toggling user enabled status", "error", err)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// GenerateAPIKey generates a new API key and secret for a user
func (u *Manager) GenerateAPIKey(tenantID, userID int) (string, string, error) {
	// Generate API key (32 characters)
	apiKey, err := stringutil.RandomAlphanumeric(32)
	if err != nil {
		u.lo.Error("error generating API key", "error", err, "user_id", userID)
		return "", "", envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Generate API secret (64 characters)
	apiSecret, err := stringutil.RandomAlphanumeric(64)
	if err != nil {
		u.lo.Error("error generating API secret", "error", err, "user_id", userID)
		return "", "", envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Hash the API secret for storage
	secretHash, err := bcrypt.GenerateFromPassword([]byte(apiSecret), bcrypt.DefaultCost)
	if err != nil {
		u.lo.Error("error hashing API secret", "error", err, "user_id", userID)
		return "", "", envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Update user with API key.
	if _, err := u.q.SetAPIKey.Exec(tenantID, userID, apiKey, string(secretHash)); err != nil {
		u.lo.Error("error saving API key", "error", err, "user_id", userID)
		return "", "", envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	return apiKey, apiSecret, nil
}

// ValidateAPIKey validates API key and secret and returns the user
func (u *Manager) ValidateAPIKey(tenantID int, apiKey, apiSecret string) (models.User, error) {
	var user models.User

	// Find user by API key.
	if err := u.q.GetUserByAPIKey.Get(&user, tenantID, apiKey); err != nil {
		if err == sql.ErrNoRows {
			return user, envelope.NewError(envelope.UnauthorizedError, u.i18n.For(tenantID).T("validation.invalidCredential"), nil)
		}
		return user, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Verify API secret.
	if err := bcrypt.CompareHashAndPassword([]byte(user.APISecret.String), []byte(apiSecret)); err != nil {
		return user, envelope.NewError(envelope.UnauthorizedError, u.i18n.For(tenantID).T("validation.invalidCredential"), nil)
	}

	// Update last used timestamp.
	if _, err := u.q.UpdateAPIKeyLastUsed.Exec(tenantID, user.ID); err != nil {
		u.lo.Error("failed to update API key last used timestamp", "error", err, "user_id", user.ID)
	}

	return user, nil
}

// RevokeAPIKey deactivates the API key for a user
func (u *Manager) RevokeAPIKey(tenantID, userID int) error {
	if _, err := u.q.RevokeAPIKey.Exec(tenantID, userID); err != nil {
		u.lo.Error("error revoking API key", "error", err, "user_id", userID)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// MergeVisitorToContact transfers conversations from visitor to contact and deletes the visitor.
func (u *Manager) MergeVisitorToContact(tenantID, visitorID, contactID int) error {
	if _, err := u.q.MergeVisitorToContact.Exec(tenantID, visitorID, contactID); err != nil {
		u.lo.Error("error merging visitor to contact", "visitor_id", visitorID, "contact_id", contactID, "error", err)
		return fmt.Errorf("merging visitor to contact: %w", err)
	}
	return nil
}

func (u *Manager) GetUserIDsByRole(tenantID, roleID int) ([]int, error) {
	var ids []int
	if err := u.q.GetUserIDsByRole.Select(&ids, tenantID, roleID); err != nil {
		u.lo.Error("error fetching user ids by role", "role_id", roleID, "error", err)
		return nil, err
	}
	return ids, nil
}

// ChangeSystemUserPassword updates the tenant's system user password with a newly prompted one.
func ChangeSystemUserPassword(ctx context.Context, tenantID int, db *sqlx.DB) error {
	// Prompt for password and get hashed password
	hashedPassword, err := promptAndHashPassword(ctx)
	if err != nil {
		return err
	}

	// Update system user's password in the database.
	if err := updateSystemUserPassword(db, tenantID, hashedPassword); err != nil {
		return fmt.Errorf("error updating system user password: %v", err)
	}
	fmt.Println("password updated successfully. Login with email 'System' and the new password.")
	return nil
}

// CreateSystemUser creates the tenant's system user with the provided password or a random one.
func CreateSystemUser(ctx context.Context, tenantID int, password string, db *sqlx.DB) error {
	var err error

	// Set random password if not provided.
	if password == "" {
		password, err = stringutil.RandomAlphanumeric(32)
		if err != nil {
			return fmt.Errorf("failed to generate system used password: %v", err)
		}
	} else {
		log.Print("using provided password for system user")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash system user password: %v", err)
	}

	_, err = db.Exec(`
		WITH sys_user AS (
			INSERT INTO users (tenant_id, email, type, first_name, last_name, password)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id
		)
		INSERT INTO user_roles (tenant_id, user_id, role_id)
		SELECT $1, sys_user.id, roles.id
		FROM sys_user, roles
		WHERE roles.tenant_id = $1 AND roles.name = $7`,
		tenantID, models.SystemUserEmail, models.UserTypeAgent, "System", "", hashedPassword, rmodels.RoleAdmin)
	if err != nil {
		return fmt.Errorf("failed to create system user: %v", err)
	}
	log.Print("system user created successfully. Use command 'conectdesk --set-system-user-password' to set the password and login with email 'System'.")
	return nil
}

// IsStrongPassword checks if the password meets the required strength for system user.
func IsStrongPassword(password string) bool {
	if len(password) < minPassword || len(password) > maxPassword {
		return false
	}
	hasUppercase := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLowercase := regexp.MustCompile(`[a-z]`).MatchString(password)
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(password)
	// Matches special characters
	hasSpecial := regexp.MustCompile(`[\W_]`).MatchString(password)
	return hasUppercase && hasLowercase && hasNumber && hasSpecial
}

// promptAndHashPassword handles password input and validation, and returns the hashed password.
func promptAndHashPassword(ctx context.Context) ([]byte, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			fmt.Printf("Please set System user password (%s): ", PasswordHint)
			buffer := make([]byte, 256)
			n, err := os.Stdin.Read(buffer)
			if err != nil {
				return nil, fmt.Errorf("error reading input: %v", err)
			}
			password := strings.TrimSpace(string(buffer[:n]))
			if IsStrongPassword(password) {
				// Hash the password using bcrypt.
				hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
				if err != nil {
					return nil, fmt.Errorf("failed to hash password: %v", err)
				}
				return hashedPassword, nil
			}
			fmt.Println("Password does not meet the strength requirements.")
		}
	}
}

// updateSystemUserPassword updates the password of the tenant's system user in the database.
func updateSystemUserPassword(db *sqlx.DB, tenantID int, hashedPassword []byte) error {
	_, err := db.Exec(`UPDATE users SET password = $1 WHERE tenant_id = $2 AND email = $3`, hashedPassword, tenantID, models.SystemUserEmail)
	if err != nil {
		return fmt.Errorf("failed to update system user password: %v", err)
	}
	return nil
}

// makeUserListQuery generates a query to fetch users based on the provided filters.
func (u *Manager) makeUserListQuery(tenantID, page, pageSize int, userTypes []string, order, orderBy, filtersJSON, location string) (string, []interface{}, error) {
	qArgs := []any{tenantID, pq.Array(userTypes)}
	return dbutil.BuildPaginatedQuery(u.q.GetUsersCompact, qArgs, dbutil.PaginationOptions{
		Order:    order,
		OrderBy:  orderBy,
		Page:     page,
		PageSize: pageSize,
		Location: location,
	}, filtersJSON, dbutil.AllowedFields{
		"users": {"email", "created_at", "updated_at", "first_name", "phone_number"},
	}, nil)
}

// verifyPassword compares the provided password with the stored password hash.
func (u *Manager) verifyPassword(pwd []byte, pwdHash string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(pwdHash), pwd); err != nil {
		u.lo.Error("error verifying password", "error", err)
		return fmt.Errorf("error verifying password: %w", err)
	}
	return nil
}

// generatePassword generates a random password and returns its bcrypt hash.
func (u *Manager) generatePassword() ([]byte, error) {
	password, _ := stringutil.RandomAlphanumeric(70)
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		u.lo.Error("error generating bcrypt password", "error", err)
		return nil, fmt.Errorf("generating bcrypt password: %w", err)
	}
	return bytes, nil
}

// reserveFlush atomically claims the flush slot, returning false if still inside the debounce window.
func (u *Manager) reserveFlush(id int) bool {
	u.lastActiveFlushAtMu.Lock()
	defer u.lastActiveFlushAtMu.Unlock()
	if last, ok := u.lastActiveFlushAt[id]; ok && time.Since(last) < lastActiveFlushDebounce {
		return false
	}
	// Stamp timestamp.
	u.lastActiveFlushAt[id] = time.Now()
	return true
}
