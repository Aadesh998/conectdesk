// Package models holds lead stages and the contacts on them.
package models

import (
	"encoding/json"
	"time"

	"github.com/volatiletech/null/v9"
)

// Stage kinds: a contact still being worked on, one who enrolled, and one
// who won't.
const (
	KindOpen = "open"
	KindWon  = "won"
	KindLost = "lost"
)

// Stage is one step of the admissions pipeline, from the leads.stages setting.
// Key is what contacts store; Label can be renamed freely.
type Stage struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

// Settings are the workspace's lead settings.
type Settings struct {
	Stages []Stage `json:"leads.stages"`
	// AutoNew puts a contact on the first stage when they first get in touch.
	AutoNew bool `json:"leads.auto_new"`
}

// Card is a contact on the leads board.
type Card struct {
	ID               int             `db:"id" json:"id"`
	FirstName        string          `db:"first_name" json:"first_name"`
	LastName         string          `db:"last_name" json:"last_name"`
	Email            null.String     `db:"email" json:"email"`
	PhoneNumber      null.String     `db:"phone_number" json:"phone_number"`
	AvatarURL        null.String     `db:"avatar_url" json:"avatar_url"`
	CustomAttributes json.RawMessage `db:"custom_attributes" json:"custom_attributes"`
	Stage            string          `db:"lead_stage" json:"stage"`
	StageChangedAt   null.Time       `db:"lead_stage_changed_at" json:"stage_changed_at"`
	LastConversation null.String     `db:"last_conversation_uuid" json:"last_conversation_uuid"`
	LastMessageAt    null.Time       `db:"last_message_at" json:"last_message_at"`
	PendingCallBack  bool            `db:"pending_call_back" json:"pending_call_back"`
	StageCount       int             `db:"stage_count" json:"-"`
}

// Column is a stage's cards on the board, and how many contacts it has in all.
type Column struct {
	Stage Stage  `json:"stage"`
	Total int    `json:"total"`
	Cards []Card `json:"cards"`
}

// Event is a contact's move from one stage to another.
type Event struct {
	ID        int         `db:"id" json:"id"`
	CreatedAt time.Time   `db:"created_at" json:"created_at"`
	FromStage null.String `db:"from_stage" json:"from_stage"`
	ToStage   string      `db:"to_stage" json:"to_stage"`
	ActorID   null.Int    `db:"actor_id" json:"actor_id"`
	ActorName null.String `db:"actor_name" json:"actor_name"`
}

// ContactLead is a contact's stage and how it got there.
type ContactLead struct {
	Stage          null.String `db:"lead_stage" json:"stage"`
	StageChangedAt null.Time   `db:"lead_stage_changed_at" json:"stage_changed_at"`
	History        []Event     `db:"-" json:"history"`
}
