package main

import (
	"strconv"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/role"
	"github.com/conectdesk/conectdesk/internal/role/models"
	realip "github.com/ferluci/fast-realip"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// handleGetRoles returns all roles
func handleGetRoles(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
	)
	roles, err := app.role.GetAll(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(roles)
}

// handleGetRole returns a single role
func handleGetRole(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	role, err := app.role.Get(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(role)
}

// handleDeleteRole deletes a role
func handleDeleteRole(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	userIDs, _ := app.user.GetUserIDsByRole(tenantID(r), id)
	if err := app.role.Delete(tenantID(r), id); err != nil {
		return sendErrorEnvelope(r, err)
	}

	app.user.InvalidateAllAgentCache()

	for _, uid := range userIDs {
		app.wsHub.KickUser(uid)
	}

	return r.SendEnvelope(true)
}

// handleCreateRole creates a new role
func handleCreateRole(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req = models.Role{}
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	createdRole, err := app.role.Create(tenantID(r), req)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(createdRole)
}

// handleUpdateRole updates a role
func handleUpdateRole(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		ip    = realip.FromRequest(r.RequestCtx)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		req   = models.Role{}
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), nil, envelope.InputError)
	}

	// Get old role before update to compare permissions.
	oldRole, err := app.role.Get(tenantID(r), id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	updatedRole, err := app.role.Update(tenantID(r), id, req)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	added, removed := role.ComparePermissions(oldRole.Permissions, updatedRole.Permissions)
	if len(added) > 0 || len(removed) > 0 {
		app.user.InvalidateAllAgentCache()

		if len(removed) > 0 {
			userIDs, err := app.user.GetUserIDsByRole(tenantID(r), updatedRole.ID)
			if err == nil {
				for _, id := range userIDs {
					app.wsHub.KickUser(id)
				}
			}
		}

		if err := app.activityLog.RolePermissionsChanged(tenantID(r), auser.ID, auser.Email, ip, updatedRole.ID, updatedRole.Name, added, removed); err != nil {
			app.lo.Error("error creating activity log", "error", err)
		}
	}

	return r.SendEnvelope(updatedRole)
}
