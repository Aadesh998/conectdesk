// Package models holds the call-back task a missed call becomes.
package models

import (
	"time"

	"github.com/volatiletech/null/v9"
)

// Call-back statuses, and why one was closed.
const (
	StatusPending = "pending"
	StatusDone    = "done"

	// ReasonCalledBack is an agent's call to the contact that connected.
	ReasonCalledBack = "called_back"
	// ReasonAnswered is the contact calling again and getting through.
	ReasonAnswered = "answered"
	// ReasonManual is an agent marking it done.
	ReasonManual = "manual"
)

// CallBack is a missed call someone has to return. Who it belongs to is the
// phone conversation's assignee, so reassigning the conversation moves it.
type CallBack struct {
	ID                int64       `db:"id" json:"id"`
	CreatedAt         time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time   `db:"updated_at" json:"updated_at"`
	ContactID         int         `db:"contact_id" json:"contact_id"`
	ConversationID    int         `db:"conversation_id" json:"conversation_id"`
	InboxID           int         `db:"inbox_id" json:"inbox_id"`
	Phone             string      `db:"phone" json:"phone"`
	Status            string      `db:"status" json:"status"`
	MissedCalls       int         `db:"missed_calls" json:"missed_calls"`
	Voicemail         bool        `db:"voicemail" json:"voicemail"`
	Attempts          int         `db:"attempts" json:"attempts"`
	LastCallSID       string      `db:"last_call_sid" json:"-"`
	LastMissedAt      time.Time   `db:"last_missed_at" json:"last_missed_at"`
	DueAt             time.Time   `db:"due_at" json:"due_at"`
	EscalatedAt       null.Time   `db:"escalated_at" json:"escalated_at"`
	DoneAt            null.Time   `db:"done_at" json:"done_at"`
	DoneByID          null.Int    `db:"done_by_id" json:"done_by_id"`
	DoneReason        null.String `db:"done_reason" json:"done_reason"`
	WhatsAppMessageID null.Int    `db:"whatsapp_message_id" json:"-"`
	WhatsAppSentAt    null.Time   `db:"whatsapp_sent_at" json:"whatsapp_sent_at"`
	WhatsAppError     null.String `db:"whatsapp_error" json:"whatsapp_error"`

	// From the contact, the phone conversation and its assignee.
	ContactFirstName string      `db:"contact_first_name" json:"contact_first_name"`
	ContactLastName  string      `db:"contact_last_name" json:"contact_last_name"`
	ConversationUUID string      `db:"conversation_uuid" json:"conversation_uuid"`
	ReferenceNumber  string      `db:"reference_number" json:"reference_number"`
	AssignedUserID   null.Int    `db:"assigned_user_id" json:"assigned_user_id"`
	AssignedTeamID   null.Int    `db:"assigned_team_id" json:"assigned_team_id"`
	AssigneeName     null.String `db:"assignee_name" json:"assignee_name"`
	TeamName         null.String `db:"team_name" json:"team_name"`
	InboxName        string      `db:"inbox_name" json:"inbox_name"`
	// WhatsAppStatus is the WhatsApp message's latest status: pending, sent,
	// delivered, read or failed.
	WhatsAppStatus null.String `db:"whatsapp_status" json:"whatsapp_status"`
	// WhatsAppConversationUUID is where the WhatsApp message and any reply are.
	WhatsAppConversationUUID null.String `db:"whatsapp_conversation_uuid" json:"whatsapp_conversation_uuid"`

	Total int `db:"total" json:"-"`
}

// ContactName is the contact's full name.
func (c CallBack) ContactName() string {
	if c.ContactLastName == "" {
		return c.ContactFirstName
	}
	return c.ContactFirstName + " " + c.ContactLastName
}

// Missed is a missed call to record.
type Missed struct {
	ContactID      int
	ConversationID int
	InboxID        int
	Phone          string
	CallSID        string
	Voicemail      bool
	// DueIn is how long there is to call back before it's overdue.
	DueIn time.Duration
}

// Scope is which call-backs an agent may see, the same as which phone
// conversations they may read.
type Scope struct {
	All bool
	// UserID's own, when Assigned.
	UserID   int
	Assigned bool
	// TeamIDs' conversations: all of them with TeamAll, the unassigned ones
	// with TeamInbox.
	TeamIDs   []int
	TeamAll   bool
	TeamInbox bool
	// Unassigned are those neither assigned to an agent nor a team.
	Unassigned bool
}

// ListOpts filters a call-back list.
type ListOpts struct {
	Status   string
	MineOnly bool
	Page     int
	PerPage  int
}

// Counts is how many call-backs an agent has waiting.
type Counts struct {
	Pending int `db:"pending" json:"pending"`
	Overdue int `db:"overdue" json:"overdue"`
}
