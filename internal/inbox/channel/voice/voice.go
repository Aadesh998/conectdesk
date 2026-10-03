// Package voice implements a phone inbox on Exotel.
//
// Calls are logged as messages: a caller becomes a conversation in the voice
// inbox, and an agent can call any contact with a phone number from any
// conversation. Exotel does the calling: for an outbound call it rings the
// agent's own phone first and then the customer (click-to-call); inbound
// calls reach agents through a flow set up in Exotel's App Bazaar, which asks
// ConectDesk who to ring and reports how the call went.
package voice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/conectdesk/conectdesk/internal/conversation/models"
	"github.com/conectdesk/conectdesk/internal/phone"
	"github.com/zerodha/logf"
)

const (
	ChannelVoice   = "voice"
	ProviderExotel = "exotel"

	// ExternalIDPrefix namespaces callers in users.external_user_id. The rest
	// is their number in international form without the plus.
	ExternalIDPrefix = "voice:"

	// DefaultMaxRecordingMB caps a downloaded call recording.
	DefaultMaxRecordingMB = 50
)

// Call directions and outcomes, kept on a call message's meta.
const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"

	StatusRinging   = "ringing"
	StatusCompleted = "completed"
	StatusBusy      = "busy"
	StatusNoAnswer  = "no-answer"
	StatusFailed    = "failed"
	// StatusVoicemail is an unanswered inbound call where the caller left a
	// message, attached as the call's recording.
	StatusVoicemail = "voicemail"
)

// MetaCallKey is the message meta key a call's details are kept under.
const MetaCallKey = "call"

// ErrNoWrittenReplies is returned for a written reply to a phone conversation.
var ErrNoWrittenReplies = errors.New("a phone conversation can't take a written reply; call the customer back")

// Config is the voice inbox configuration, stored as the inbox's JSON config.
type Config struct {
	Provider string `json:"provider"`

	// AccountID is the ExoPhone's digits: a number can serve one workspace.
	AccountID string `json:"account_id"`

	// Exotel credentials, from the dashboard's API settings. AccessToken is
	// the API token, encrypted at rest.
	AccountSID  string `json:"account_sid"`
	APIKey      string `json:"api_key"`
	AccessToken string `json:"access_token"`
	// Subdomain is the region's API host: api.exotel.com (Singapore) or
	// api.in.exotel.com (Mumbai).
	Subdomain string `json:"subdomain"`

	// CallerID is the ExoPhone calls come from and customers call.
	CallerID string `json:"caller_id"`

	// Record records calls, and the recording is attached to the call.
	Record bool `json:"record"`

	// RingAgents rings agents who are online and have a phone number set, for
	// inbound calls. FallbackNumbers are rung when none are.
	RingAgents      bool     `json:"ring_agents"`
	FallbackNumbers []string `json:"fallback_numbers"`

	MaxRecordingMB int `json:"max_recording_mb"`

	// Menu routes an inbound call to a team by the digit the caller pressed
	// in the flow's Gather applet ("press 1 for sales"). Only that team's
	// online agents are rung, and the conversation is assigned to it.
	Menu []MenuOption `json:"menu"`

	// RingSeconds is how long agents' phones ring before the call counts as
	// unanswered and goes on to voicemail. Defaults to 30.
	RingSeconds int `json:"ring_seconds"`

	// MissedCalls is what an unanswered inbound call leads to.
	MissedCalls MissedCallConfig `json:"missed_calls"`
}

// MissedCallConfig is what a phone line does about a call nobody answered: a
// call-back for the agents, and a WhatsApp message to the caller.
type MissedCallConfig struct {
	// NoCallBacks turns call-backs off for the line.
	NoCallBacks bool `json:"no_call_backs"`
	// CallBackMinutes is how long agents have to call back before the
	// call-back is overdue. 0 is the default, 15.
	CallBackMinutes int `json:"call_back_minutes"`

	// WhatsAppInboxID is the WhatsApp inbox the caller is messaged from, with
	// Template; 0 sends nothing. The business writes first, so it must be an
	// approved template.
	WhatsAppInboxID int         `json:"whatsapp_inbox_id"`
	Template        TemplateRef `json:"template"`
	// AfterHoursTemplate, when set, is sent instead outside the workspace's
	// business hours.
	AfterHoursTemplate TemplateRef `json:"after_hours_template"`
}

// TemplateRef names an approved WhatsApp template and its body values, which
// may use {{name}} for the caller's name, {{phone}} for their number and
// {{business}} for the workspace's name.
type TemplateRef struct {
	Name     string   `json:"name"`
	Language string   `json:"language"`
	Body     []string `json:"body"`
}

// Due returns how long agents have to call back.
func (c MissedCallConfig) Due() time.Duration {
	if c.CallBackMinutes <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(c.CallBackMinutes) * time.Minute
}

// Values fills a template's body values in for a caller.
func (t TemplateRef) Values(name, number, business string) []string {
	out := make([]string, len(t.Body))
	r := strings.NewReplacer("{{name}}", name, "{{phone}}", number, "{{business}}", business)
	for i, v := range t.Body {
		out[i] = strings.TrimSpace(r.Replace(v))
	}
	return out
}

// MenuOption is one choice of the phone menu.
type MenuOption struct {
	Digit  string `json:"digit"`
	TeamID int    `json:"team_id"`
	Label  string `json:"label"`
}

// MenuChoice returns the menu option for the digits a caller pressed.
func (c Config) MenuChoice(pressed string) (MenuOption, bool) {
	pressed = strings.Trim(strings.TrimSpace(pressed), `"`)
	if pressed == "" {
		return MenuOption{}, false
	}
	for _, o := range c.Menu {
		if o.Digit == pressed {
			return o, true
		}
	}
	return MenuOption{}, false
}

// CallEvent is what a provider reports about a call.
type CallEvent struct {
	SID          string
	Direction    string
	Status       string
	From, To     string
	Duration     int
	RecordingURL string
	// AnsweredBy is the number that picked up an inbound call.
	AnsweredBy  string
	CustomField string
	// Pressed is the digits the caller pressed in a Gather applet.
	Pressed string
}

// Call is a call's details on its message's meta.
type Call struct {
	SID       string `json:"sid"`
	Provider  string `json:"provider"`
	Direction string `json:"direction"`
	Status    string `json:"status"`
	From      string `json:"from"`
	To        string `json:"to"`
	Duration  int    `json:"duration"`
	AgentID   int    `json:"agent_id,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
	// TeamID and Menu are the team the phone menu routed the call to.
	TeamID int    `json:"team_id,omitempty"`
	Menu   string `json:"menu,omitempty"`
	// TransferredFrom is the agent who handed the customer over.
	TransferredFrom string    `json:"transferred_from,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at,omitzero"`
	// RecordingError is why the recording couldn't be kept, if it couldn't.
	RecordingError string `json:"recording_error,omitempty"`
}

// Voice is a phone inbox.
type Voice struct {
	id       int
	tenantID int
	name     string
	from     string
	config   Config
	exotel   *Exotel
	lo       *logf.Logger
}

// Opts holds the options required for the inbox.
type Opts struct {
	ID, TenantID int
	Name, From   string
	Config       Config
	Lo           *logf.Logger
}

// New returns a new voice inbox.
func New(opts Opts) (*Voice, error) {
	cfg := opts.Config
	if cfg.Provider == "" {
		cfg.Provider = ProviderExotel
	}
	if cfg.Provider != ProviderExotel {
		return nil, fmt.Errorf("unknown voice provider %q", cfg.Provider)
	}
	if cfg.AccountSID == "" || cfg.APIKey == "" || cfg.AccessToken == "" || cfg.CallerID == "" {
		return nil, fmt.Errorf("voice inbox %d is missing its exotel settings", opts.ID)
	}
	if cfg.MaxRecordingMB <= 0 {
		cfg.MaxRecordingMB = DefaultMaxRecordingMB
	}
	return &Voice{
		id: opts.ID, tenantID: opts.TenantID, name: opts.Name, from: opts.From,
		config: cfg,
		exotel: NewExotel(cfg.AccountSID, cfg.APIKey, cfg.AccessToken, cfg.Subdomain),
		lo:     opts.Lo,
	}, nil
}

func (v *Voice) Identifier() int               { return v.id }
func (v *Voice) Channel() string               { return ChannelVoice }
func (v *Voice) Name() string                  { return v.name }
func (v *Voice) FromAddress() string           { return v.from }
func (v *Voice) ReplyToAddress() string        { return "" }
func (v *Voice) FromNameTemplate() string      { return "" }
func (v *Voice) Receive(context.Context) error { return nil }
func (v *Voice) Close() error                  { return nil }

// TenantID returns the tenant owning the inbox.
func (v *Voice) TenantID() int { return v.tenantID }

// Config returns the inbox's resolved configuration.
func (v *Voice) Config() Config { return v.config }

// Exotel returns the provider client.
func (v *Voice) Exotel() *Exotel { return v.exotel }

// Send refuses written replies: a phone conversation is answered by calling.
func (v *Voice) Send(models.OutboundMessage) error {
	return ErrNoWrittenReplies
}

// Call starts a click-to-call from the agent's phone to the customer's.
func (v *Voice) Call(ctx context.Context, agentPhone, customerPhone, statusURL, ref string) (string, error) {
	return v.exotel.Connect(ctx, ConnectOpts{
		Agent:       agentPhone,
		Customer:    customerPhone,
		CallerID:    v.config.CallerID,
		StatusURL:   statusURL,
		Record:      v.config.Record,
		CustomField: ref,
	})
}

// E164 normalises a phone number to +<digits>; see phone.E164.
func E164(p string) string { return phone.E164(p) }

// ContactE164 builds a contact's international number from the contact
// form's country and number fields; see phone.FromContact.
func ContactE164(country, number string) string { return phone.FromContact(country, number) }

// Digits returns a number's digits in international form, for matching.
func Digits(p string) string { return phone.Digits(p) }

// StatusLabel is a call outcome in words, for a call message's content.
func StatusLabel(c Call) string {
	switch c.Status {
	case StatusRinging:
		if c.Direction == DirectionInbound {
			return "Incoming call"
		}
		return "Calling"
	case StatusCompleted:
		return "Call ended · " + FormatDuration(c.Duration)
	case StatusBusy:
		return "Busy"
	case StatusVoicemail:
		return "Voicemail"
	case StatusNoAnswer:
		if c.Direction == DirectionInbound {
			return "Missed call"
		}
		return "No answer"
	}
	return "Call failed"
}

// FormatDuration writes seconds as m:ss or h:mm:ss.
func FormatDuration(s int) string {
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
