// Package team handles the management of teams and their members.
package team

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/conectdesk/conectdesk/internal/team/models"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS
)

// Manager handles team-related operations.
type Manager struct {
	lo   *logf.Logger
	i18n *i18ncat.Catalog
	q    queries
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB   *sqlx.DB
	Lo   *logf.Logger
	I18n *i18ncat.Catalog
}

// queries contains prepared SQL queries.
type queries struct {
	GetTeams          *sqlx.Stmt `query:"get-teams"`
	GetUserTeams      *sqlx.Stmt `query:"get-user-teams"`
	GetTeamsCompact   *sqlx.Stmt `query:"get-teams-compact"`
	GetTeam           *sqlx.Stmt `query:"get-team"`
	InsertTeam        *sqlx.Stmt `query:"insert-team"`
	UpdateTeam        *sqlx.Stmt `query:"update-team"`
	DeleteTeam        *sqlx.Stmt `query:"delete-team"`
	GetTeamMembers    *sqlx.Stmt `query:"get-team-members"`
	UpsertUserTeams   *sqlx.Stmt `query:"upsert-user-teams"`
	UserBelongsToTeam *sqlx.Stmt `query:"user-belongs-to-team"`
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
	}, nil
}

// GetAll retrieves all teams.
func (u *Manager) GetAll(tenantID int) ([]models.Team, error) {
	var teams = make([]models.Team, 0)
	if err := u.q.GetTeams.Select(&teams, tenantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return teams, nil
		}
		u.lo.Error("error fetching teams", "error", err)
		return teams, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return teams, nil
}

// GetAllCompact retrieves all teams with limited fields.
func (u *Manager) GetAllCompact(tenantID int) ([]models.TeamCompact, error) {
	var teams = make([]models.TeamCompact, 0)
	if err := u.q.GetTeamsCompact.Select(&teams, tenantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return teams, nil
		}
		u.lo.Error("error fetching teams", "error", err)
		return teams, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return teams, nil
}

// Get retrieves a team by ID.
func (u *Manager) Get(tenantID, id int) (models.Team, error) {
	var team models.Team
	if err := u.q.GetTeam.Get(&team, tenantID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			u.lo.Error("team not found", "id", id, "error", err)
			return team, envelope.NewError(envelope.InputError, u.i18n.For(tenantID).T("validation.notFoundTeam"), nil)
		}
		u.lo.Error("error fetching team", "id", id, "error", err)
		return team, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return team, nil
}

// Create creates a new team.
func (u *Manager) Create(tenantID int, name, timezone, conversationAssignmentType string, businessHrsID, slaPolicyID null.Int, emoji string, maxAutoAssignedConversations int) (models.Team, error) {
	var team models.Team
	if err := u.q.InsertTeam.Get(&team, tenantID, name, timezone, conversationAssignmentType, businessHrsID, slaPolicyID, emoji, maxAutoAssignedConversations); err != nil {
		if dbutil.IsUniqueViolationError(err) {
			return team, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("errors.alreadyExistsTeam"), nil)
		}
		u.lo.Error("error inserting team", "error", err)
		return team, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return team, nil
}

// Update updates an existing team.
func (u *Manager) Update(tenantID, id int, name, timezone, conversationAssignmentType string, businessHrsID, slaPolicyID null.Int, emoji string, maxAutoAssignedConversations int) (models.Team, error) {
	var team models.Team
	if err := u.q.UpdateTeam.Get(&team, tenantID, id, name, timezone, conversationAssignmentType, businessHrsID, slaPolicyID, emoji, maxAutoAssignedConversations); err != nil {
		u.lo.Error("error updating team", "error", err)
		return team, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return team, nil
}

// Delete deletes a team and returns the affected member IDs so callers can invalidate per-user state.
func (u *Manager) Delete(tenantID, id int) ([]int, error) {
	members, err := u.GetMembers(tenantID, id)
	if err != nil {
		return nil, err
	}
	if _, err := u.q.DeleteTeam.Exec(tenantID, id); err != nil {
		u.lo.Error("error deleting team", "error", err)
		return nil, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	ids := make([]int, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// GetUserTeams retrieves teams of a user by user ID.
func (u *Manager) GetUserTeams(tenantID, userID int) ([]models.Team, error) {
	var teams = make([]models.Team, 0)
	if err := u.q.GetUserTeams.Select(&teams, tenantID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return teams, nil
		}
		u.lo.Error("error fetching teams", "user_id", userID, "error", err)
		return teams, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return teams, nil
}

// UpsertUserTeams updates/inserts exists user teams
func (u *Manager) UpsertUserTeams(tenantID, id int, teamNames []string) error {
	if _, err := u.q.UpsertUserTeams.Exec(tenantID, id, pq.Array(teamNames)); err != nil {
		u.lo.Error("error updating user teams", "error", err)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// UserBelongsToTeam returns true if the user belongs to the team.
func (u *Manager) UserBelongsToTeam(tenantID, teamID, userID int) (bool, error) {
	var exists bool
	if err := u.q.UserBelongsToTeam.Get(&exists, tenantID, teamID, userID); err != nil {
		u.lo.Error("error fetching team members", "team_id", teamID, "user_id", userID, "error", err)
		return false, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return exists, nil
}

// GetMembers retrieves members of a team.
func (u *Manager) GetMembers(tenantID, id int) ([]models.TeamMember, error) {
	var members = make([]models.TeamMember, 0)
	if err := u.q.GetTeamMembers.Select(&members, tenantID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return members, nil
		}
		u.lo.Error("error fetching team members", "team_id", id, "error", err)
		return members, fmt.Errorf("fetching team members: %w", err)
	}
	return members, nil
}
