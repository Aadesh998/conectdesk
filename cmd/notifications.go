package main

import (
	cmodels "github.com/conectdesk/conectdesk/internal/conversation/models"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
)

func markAssignmentNotificationRead(app *App, tenantID int, conv *cmodels.Conversation, user umodels.User) {
	if conv == nil || conv.ID == 0 || conv.AssignedUserID.Int != user.ID {
		return
	}
	_ = app.userNotification.MarkAssignmentAsRead(tenantID, conv.ID, user.ID)
}
