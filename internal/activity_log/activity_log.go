// Package activity manages activity logs for all users.
package activitylog

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/conectdesk/conectdesk/internal/activity_log/models"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
)

type Manager struct {
	q    queries
	lo   *logf.Logger
	i18n *i18ncat.Catalog
	db   *sqlx.DB
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18ncat.Catalog
}

// queries contains prepared SQL queries.
type queries struct {
	GetAllActivities string     `query:"get-all-activities"`
	InsertActivity   *sqlx.Stmt `query:"insert-activity"`
}

// New creates and returns a new instance of the Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	return &Manager{
		q:    q,
		lo:   opts.Lo,
		i18n: opts.I18n,
		db:   opts.DB,
	}, nil
}

// GetAll retrieves all activity logs.
func (m *Manager) GetAll(tenantID int, order, orderBy, filtersJSON string, page, pageSize int, location string) ([]models.ActivityLog, error) {
	query, qArgs, err := m.makeQuery(tenantID, page, pageSize, order, orderBy, filtersJSON, location)
	if err != nil {
		m.lo.Error("error creating activity log list query", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}

	// Start a read-only txn.
	tx, err := m.db.BeginTxx(context.Background(), &sql.TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		m.lo.Error("error starting read-only transaction", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	defer tx.Rollback()

	// Execute query
	var activityLogs = make([]models.ActivityLog, 0)
	if err := tx.Select(&activityLogs, query, qArgs...); err != nil {
		m.lo.Error("error fetching activity logs", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return activityLogs, nil
}

// Login records a login event for the given user.
func (al *Manager) Login(tenantID, userID int, email, ip string) error {
	description := al.i18n.For(tenantID).Ts("activityLog.agentLogin",
		"email", email,
		"userId", fmt.Sprintf("#%d", userID))
	return al.create(tenantID,
		models.AgentLogin,
		description,
		userID,
		umodels.UserModel,
		userID,
		ip,
	)
}

// PlatformImpersonation records a platform admin logging in as the user.
func (al *Manager) PlatformImpersonation(tenantID, userID int, email, adminEmail, ip string) error {
	description := al.i18n.For(tenantID).Ts("activityLog.platformImpersonation",
		"adminEmail", adminEmail,
		"email", email,
		"userId", fmt.Sprintf("#%d", userID))
	return al.create(tenantID,
		models.PlatformImpersonation,
		description,
		userID,
		umodels.UserModel,
		userID,
		ip,
	)
}

// TwoFactorChanged records an agent's two-factor login switched on or off,
// by the agent or, for a reset, by an admin.
func (al *Manager) TwoFactorChanged(tenantID, actorID int, actorEmail string, targetID int, targetEmail string, enabled bool, ip string) error {
	typ, key := models.Agent2FADisabled, "activityLog.agent2FADisabled"
	if enabled {
		typ, key = models.Agent2FAEnabled, "activityLog.agent2FAEnabled"
	}
	description := al.i18n.For(tenantID).Ts(key,
		"actorEmail", actorEmail,
		"actorId", fmt.Sprintf("#%d", actorID),
		"email", targetEmail,
		"userId", fmt.Sprintf("#%d", targetID))
	return al.create(tenantID, typ, description, actorID, umodels.UserModel, targetID, ip)
}

// DataExported records an admin exporting all of the workspace's data.
func (al *Manager) DataExported(tenantID, actorID int, actorEmail, ip string) error {
	description := al.i18n.For(tenantID).Ts("activityLog.dataExported",
		"actorEmail", actorEmail,
		"actorId", fmt.Sprintf("#%d", actorID))
	return al.create(tenantID, models.DataExported, description, actorID, umodels.UserModel, actorID, ip)
}

// RetentionPolicyChanged records an admin changing the workspace's data
// retention: days 0 keeps everything.
func (al *Manager) RetentionPolicyChanged(tenantID, actorID int, actorEmail, ip string, days int, deleteContacts bool) error {
	key := "activityLog.retentionPolicyOff"
	switch {
	case days > 0 && deleteContacts:
		key = "activityLog.retentionPolicyOnContacts"
	case days > 0:
		key = "activityLog.retentionPolicyOn"
	}
	description := al.i18n.For(tenantID).Ts(key,
		"actorEmail", actorEmail,
		"actorId", fmt.Sprintf("#%d", actorID),
		"days", strconv.Itoa(days))
	return al.create(tenantID, models.RetentionPolicyChanged, description, actorID, umodels.UserModel, actorID, ip)
}

// RetentionApplied records what the workspace's data retention deleted. The
// actor is the workspace's system user.
func (al *Manager) RetentionApplied(tenantID, systemUserID, conversations, contacts int) error {
	description := al.i18n.For(tenantID).Ts("activityLog.retentionApplied",
		"conversations", strconv.Itoa(conversations),
		"contacts", strconv.Itoa(contacts))
	return al.create(tenantID, models.RetentionApplied, description, systemUserID, umodels.UserModel, systemUserID, "")
}

// Logout records a logout event for the given user.
func (al *Manager) Logout(tenantID, userID int, email, ip string) error {
	description := al.i18n.For(tenantID).Ts("activityLog.agentLogout",
		"email", email,
		"userId", fmt.Sprintf("#%d", userID))
	return al.create(tenantID,
		models.AgentLogout,
		description,
		userID,
		umodels.UserModel,
		userID,
		ip,
	)
}

// Away records an away event for the given user.
func (al *Manager) Away(tenantID, actorID int, actorEmail, ip string, targetID int, targetEmail string) error {
	var description string
	if targetID != 0 && targetEmail != "" && (targetID != actorID || targetEmail != actorEmail) {
		description = al.i18n.For(tenantID).Ts("activityLog.agentAway",
			"actorEmail", actorEmail,
			"actorId", fmt.Sprintf("#%d", actorID),
			"targetEmail", targetEmail,
			"targetId", fmt.Sprintf("#%d", targetID))
	} else {
		description = al.i18n.For(tenantID).Ts("activityLog.agentAwaySelf",
			"actorEmail", actorEmail,
			"actorId", fmt.Sprintf("#%d", actorID))
	}
	return al.create(tenantID,
		models.AgentAway, /* activity type*/
		description,
		actorID,           /*actor_id*/
		umodels.UserModel, /*target_model_type*/
		actorID,           /*target_model_id*/
		ip,
	)
}

// AwayReassigned records an away and reassigned event for the given user.
func (al *Manager) AwayReassigned(tenantID, actorID int, actorEmail, ip string, targetID int, targetEmail string) error {
	var description string
	if targetID != 0 && targetEmail != "" && (targetID != actorID || targetEmail != actorEmail) {
		description = al.i18n.For(tenantID).Ts("activityLog.agentAwayReassign",
			"actorEmail", actorEmail,
			"actorId", fmt.Sprintf("#%d", actorID),
			"targetEmail", targetEmail,
			"targetId", fmt.Sprintf("#%d", targetID))
	} else {
		description = al.i18n.For(tenantID).Ts("activityLog.agentAwayReassignSelf",
			"actorEmail", actorEmail,
			"actorId", fmt.Sprintf("#%d", actorID))
	}
	return al.create(tenantID,
		models.AgentAwayReassigned, /* activity type*/
		description,
		actorID,           /*actor_id*/
		umodels.UserModel, /*target_model_type*/
		actorID,           /*target_model_id*/
		ip,
	)
}

// Online records an online event for the given user.
func (al *Manager) Online(tenantID, actorID int, actorEmail, ip string, targetID int, targetEmail string) error {
	var description string
	if targetID != 0 && targetEmail != "" && (targetID != actorID || targetEmail != actorEmail) {
		description = al.i18n.For(tenantID).Ts("activityLog.agentOnline",
			"actorEmail", actorEmail,
			"actorId", fmt.Sprintf("#%d", actorID),
			"targetEmail", targetEmail,
			"targetId", fmt.Sprintf("#%d", targetID))
	} else {
		description = al.i18n.For(tenantID).Ts("activityLog.agentOnlineSelf",
			"actorEmail", actorEmail,
			"actorId", fmt.Sprintf("#%d", actorID))
	}
	return al.create(tenantID,
		models.AgentOnline, /* activity type*/
		description,
		actorID,           /*actor_id*/
		umodels.UserModel, /*target_model_type*/
		actorID,           /*target_model_id*/
		ip,
	)
}

// UserAvailability records a user availability event for the given user.
func (al *Manager) UserAvailability(tenantID, actorID int, actorEmail, status, ip, targetEmail string, targetID int) error {
	switch status {
	case umodels.Online:
		if err := al.Online(tenantID, actorID, actorEmail, ip, targetID, targetEmail); err != nil {
			return err
		}
	case umodels.AwayManual:
		if err := al.Away(tenantID, actorID, actorEmail, ip, targetID, targetEmail); err != nil {
			al.lo.Error("error logging away activity", "error", err)
			return err
		}
	case umodels.AwayAndReassigning:
		if err := al.AwayReassigned(tenantID, actorID, actorEmail, ip, targetID, targetEmail); err != nil {
			al.lo.Error("error logging away and reassigning activity", "error", err)
			return err
		}
	}
	return nil
}

// PasswordSet records a password set event.
func (al *Manager) PasswordSet(tenantID, actorID int, actorEmail, ip string, targetID int, targetEmail string) error {
	description := al.i18n.For(tenantID).Ts("activityLog.agentPasswordSet",
		"actorEmail", actorEmail,
		"actorId", fmt.Sprintf("#%d", actorID),
		"targetEmail", targetEmail,
		"targetId", fmt.Sprintf("#%d", targetID))
	return al.create(tenantID,
		models.AgentPasswordSet,
		description,
		actorID,
		umodels.UserModel,
		targetID,
		ip,
	)
}

// RolePermissionsChanged records a role permissions change event.
func (al *Manager) RolePermissionsChanged(tenantID, actorID int, actorEmail, ip string, roleID int, roleName string, added, removed []string) error {
	var description string
	if len(removed) > 0 && len(added) > 0 {
		description = al.i18n.For(tenantID).Ts("activityLog.rolePermissionsChanged",
			"actorEmail", actorEmail,
			"actorId", fmt.Sprintf("#%d", actorID),
			"removed", strings.Join(removed, ", "),
			"added", strings.Join(added, ", "),
			"roleName", roleName,
			"roleId", fmt.Sprintf("#%d", roleID))
	} else if len(removed) > 0 {
		description = al.i18n.For(tenantID).Ts("activityLog.rolePermissionsRemoved",
			"actorEmail", actorEmail,
			"actorId", fmt.Sprintf("#%d", actorID),
			"permissions", strings.Join(removed, ", "),
			"roleName", roleName,
			"roleId", fmt.Sprintf("#%d", roleID))
	} else if len(added) > 0 {
		description = al.i18n.For(tenantID).Ts("activityLog.rolePermissionsAdded",
			"actorEmail", actorEmail,
			"actorId", fmt.Sprintf("#%d", actorID),
			"permissions", strings.Join(added, ", "),
			"roleName", roleName,
			"roleId", fmt.Sprintf("#%d", roleID))
	} else {
		return nil // No changes
	}
	return al.create(tenantID,
		models.AgentRolePermissionsChanged,
		description,
		actorID,
		"role",
		roleID,
		ip,
	)
}

// ContactDeleted records permanent deletion of a contact and their data.
func (al *Manager) ContactDeleted(tenantID, actorID int, actorEmail, ip string, contactID int, contactEmail string) error {
	description := al.i18n.For(tenantID).Ts("activityLog.contactDeleted",
		"actorEmail", actorEmail,
		"actorId", fmt.Sprintf("#%d", actorID),
		"contactEmail", contactEmail,
		"contactId", fmt.Sprintf("#%d", contactID))
	return al.create(tenantID,
		models.ContactDeleted,
		description,
		actorID,
		umodels.UserModel,
		contactID,
		ip,
	)
}

// ContactDataExported records an export of a contact's stored data.
func (al *Manager) ContactDataExported(tenantID, actorID int, actorEmail, ip string, contactID int, contactEmail string) error {
	description := al.i18n.For(tenantID).Ts("activityLog.contactDataExported",
		"actorEmail", actorEmail,
		"actorId", fmt.Sprintf("#%d", actorID),
		"contactEmail", contactEmail,
		"contactId", fmt.Sprintf("#%d", contactID))
	return al.create(tenantID,
		models.ContactDataExported,
		description,
		actorID,
		umodels.UserModel,
		contactID,
		ip,
	)
}

// create creates a new activity log in DB.
func (m *Manager) create(tenantID int, activityType, activityDescription string, actorID int, targetModelType string, targetModelID int, ip string) error {
	if _, err := m.q.InsertActivity.Exec(tenantID, activityType, activityDescription, actorID, targetModelType, targetModelID, ip); err != nil {
		m.lo.Error("error inserting activity log", "error", err)
		return envelope.NewError(envelope.GeneralError, m.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// makeQuery constructs the SQL query for fetching activity logs with filters and pagination.
func (m *Manager) makeQuery(tenantID, page, pageSize int, order, orderBy, filtersJSON, location string) (string, []any, error) {
	var (
		baseQuery = m.q.GetAllActivities
		qArgs     = []any{tenantID}
	)
	return dbutil.BuildPaginatedQuery(baseQuery, qArgs, dbutil.PaginationOptions{
		Order:    order,
		OrderBy:  orderBy,
		Page:     page,
		PageSize: pageSize,
		Location: location,
	}, filtersJSON, dbutil.AllowedFields{
		"activity_logs": {"activity_type", "actor_id", "ip", "created_at"},
	}, nil)
}
