package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
)

// FAQ suggestion review states.
const (
	FAQStatusPending  = "pending"
	FAQStatusApproved = "approved"
	FAQStatusRejected = "rejected"
)

// Tone and response-length presets shape the customer-facing voice.
var (
	Tones           = []string{"friendly", "professional", "neutral", "casual"}
	ResponseLengths = []string{"concise", "balanced", "detailed"}
)

// Assistant is one AI assistant: a persona plus the ai_assistant user that carries its identity.
type Assistant struct {
	ID             int            `db:"id" json:"id"`
	CreatedAt      time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at" json:"updated_at"`
	UserID         int            `db:"user_id" json:"user_id"`
	Name           string         `db:"name" json:"name"`
	AvatarURL      null.String    `db:"avatar_url" json:"avatar_url"`
	Description    string         `db:"description" json:"description"`
	Instructions   string         `db:"instructions" json:"instructions"`
	Guardrails     string         `db:"guardrails" json:"guardrails"`
	Expectation    string         `db:"expectation" json:"expectation"`
	Tone           string         `db:"tone" json:"tone"`
	ResponseLength string         `db:"response_length" json:"response_length"`
	MaxTurns       int            `db:"max_turns" json:"max_turns"`
	FallbackTeamID null.Int       `db:"fallback_team_id" json:"fallback_team_id"`
	HandoffEnabled bool           `db:"handoff_enabled" json:"handoff_enabled"`
	Languages      pq.StringArray `db:"languages" json:"languages"`
	Enabled        bool           `db:"enabled" json:"enabled"`
	ToolIDs        []int          `db:"-" json:"tool_ids"`

	// The Jev decision layer. JevGate lets Jev decide whether a customer
	// message is answered by the LLM, skipped, or handed to a person; JevGuard
	// has Jev check each reply before it is sent; JevLeadStage has Jev move the
	// contact's lead stage. Answers less confident than JevConfidence are
	// ignored, and the agent behaves as it would without Jev.
	JevGate       bool    `db:"jev_gate" json:"jev_gate"`
	JevGuard      bool    `db:"jev_guard" json:"jev_guard"`
	JevLeadStage  bool    `db:"jev_lead_stage" json:"jev_lead_stage"`
	JevConfidence float64 `db:"jev_confidence" json:"jev_confidence"`

	// FollowupsEnabled has the assistant follow up when the customer stops
	// replying, FollowupHours after its last message (one entry per
	// follow-up). Past WhatsApp's 24-hour window it can only send one of
	// FollowupTemplates, which Jev picks between.
	FollowupsEnabled  bool          `db:"followups_enabled" json:"followups_enabled"`
	FollowupHours     pq.Int64Array `db:"followup_hours" json:"followup_hours"`
	FollowupTemplates TemplateRefs  `db:"followup_templates" json:"followup_templates"`

	// RemoveAvatar, when set on a save request, clears the assistant's current avatar.
	RemoveAvatar bool `db:"-" json:"remove_avatar"`
}

// TemplateRef names an approved WhatsApp template and its body values, which
// may use {{name}}, {{phone}} and {{business}} for the contact's name and
// number and the workspace's name.
type TemplateRef struct {
	Name     string   `json:"name"`
	Language string   `json:"language"`
	Body     []string `json:"body"`
}

// TemplateRefs is a JSONB list of templates.
type TemplateRefs []TemplateRef

// Value stores the list as JSON.
func (t TemplateRefs) Value() (driver.Value, error) {
	if t == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(t)
}

// Scan reads the list from JSON.
func (t *TemplateRefs) Scan(src any) error {
	var b []byte
	switch v := src.(type) {
	case nil:
		*t = TemplateRefs{}
		return nil
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return errors.New("unsupported type for template list")
	}
	return json.Unmarshal(b, t)
}

// PreviewSource is one knowledge item a preview reply was grounded in.
type PreviewSource struct {
	ID    int     `json:"id"`
	Type  string  `json:"type"`
	Title string  `json:"title"`
	Score float64 `json:"score"`
}

// RecentConversation is a summary row of a contact's past conversation, fed to the assistant as context.
type RecentConversation struct {
	UUID            string    `db:"uuid"`
	ReferenceNumber string    `db:"reference_number"`
	CreatedAt       time.Time `db:"created_at"`
	Subject         string    `db:"subject"`
	Status          string    `db:"status"`
}

// StatWindow holds the raw counts scanned for one time window.
type StatWindow struct {
	Conversations int     `db:"conversations"`
	Replies       int     `db:"replies"`
	Handoffs      int     `db:"handoffs"`
	Resolves      int     `db:"resolves"`
	Reopened      int     `db:"reopened"`
	CSATCount     int     `db:"csat_count"`
	CSATAvg       float64 `db:"csat_avg"`
	CSATPositive  float64 `db:"csat_positive"`
	Skipped       int     `db:"skipped"`
	Blocked       int     `db:"blocked"`
	Followups     int     `db:"followups"`
}

// AssistantStats is the computed window summary with derived rates and trend deltas vs the previous equal window.
type AssistantStats struct {
	RangeDays      int     `json:"range_days"`
	Conversations  int     `json:"conversations"`
	Replies        int     `json:"replies"`
	Handoffs       int     `json:"handoffs"`
	Resolved       int     `json:"resolved"`
	Reopened       int     `json:"reopened"`
	ResolutionRate float64 `json:"resolution_rate"`
	HandoffRate    float64 `json:"handoff_rate"`
	ReopenRate     float64 `json:"reopen_rate"`
	Depth          float64 `json:"depth"`
	CSATCount      int     `json:"csat_count"`
	CSATAvg        float64 `json:"csat_avg"`
	CSATPositive   float64 `json:"csat_positive"`
	// What the Jev layer did: messages it decided needed no LLM reply,
	// replies it stopped, and follow-ups sent.
	Skipped   int                `json:"skipped"`
	Blocked   int                `json:"blocked"`
	Followups int                `json:"followups"`
	Trends    map[string]float64 `json:"trends"`
}

// FAQSuggestion is a candidate Q&A mined from a resolved conversation, awaiting review.
type FAQSuggestion struct {
	ID                          int       `db:"id" json:"id"`
	CreatedAt                   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt                   time.Time `db:"updated_at" json:"updated_at"`
	ConversationID              int       `db:"conversation_id" json:"conversation_id"`
	Question                    string    `db:"question" json:"question"`
	Answer                      string    `db:"answer" json:"answer"`
	Status                      string    `db:"status" json:"status"`
	ReviewedByID                null.Int  `db:"reviewed_by_id" json:"reviewed_by_id"`
	ReviewedAt                  null.Time `db:"reviewed_at" json:"reviewed_at"`
	ConversationUUID            string    `db:"conversation_uuid" json:"conversation_uuid"`
	ConversationReferenceNumber string    `db:"reference_number" json:"conversation_reference_number"`
}
