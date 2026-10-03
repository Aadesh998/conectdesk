package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/conectdesk/conectdesk/internal/aiagent/models"
	cmodels "github.com/conectdesk/conectdesk/internal/conversation/models"
	lmodels "github.com/conectdesk/conectdesk/internal/lead/models"
	"github.com/conectdesk/conectdesk/internal/systemone"
)

// The Jev decision layer. Jev (TypeSafe's System One model) can't write a
// reply, but it answers typed questions about a conversation in well under a
// second, with a calibrated confidence. The agent asks it:
//
//   - before the LLM runs (the gate): does the customer's message need an
//     LLM reply at all, should it go to a person instead, or can it be left?
//   - before a reply is sent (the guard): does the LLM's draft promise
//     something it mustn't, or state facts the knowledge base doesn't back?
//   - after a turn: which lead stage is the contact at now?
//   - for a quiet conversation: is a follow-up due, and past WhatsApp's
//     24-hour window, which approved template fits?
//
// An answer less confident than the assistant's threshold is ignored, and so
// is any Jev error: the agent then does what it would without Jev, which is
// to let the LLM reply. Jev never makes the agent go silent on its own say-so
// unless it is confident.

const (
	// jevTimeout bounds one Jev call. It answers in 70-500ms.
	jevTimeout = 5 * time.Second

	// jevTranscriptMessages is how many recent messages Jev sees, and
	// jevMessageChars how much of each: enough to judge the latest turn while
	// staying far inside its 32k-token state limit.
	jevTranscriptMessages = 12
	jevMessageChars       = 1200
	jevKnowledgeChars     = 1500

	// frustrationHandoff is the frustration level, of five, at which a
	// customer goes to a person.
	frustrationHandoff = 4

	// settledResolve is how sure Jev must be that nothing is left to answer
	// before a message that needs no reply resolves the conversation.
	settledResolve = 0.85

	// terminalStageConfidence is the confidence needed to move a contact into
	// a won or lost stage, which ends their journey.
	terminalStageConfidence = 0.9
)

// Gate verdicts.
const (
	verdictLLM     = "llm"
	verdictSkip    = "skip"
	verdictResolve = "resolve"
	verdictHandoff = "handoff"
)

// jevClient returns the workspace's Jev client, or nil when Jev is off or has
// no key. Read per call, so a key saved in settings applies at once.
func (m *Manager) jevClient(tenantID int) *systemone.Client {
	if m.jevFactory != nil {
		return m.jevFactory(tenantID)
	}
	var s struct {
		Enabled bool   `json:"jev.enabled"`
		APIKey  string `json:"jev.api_key"`
		Model   string `json:"jev.model"`
	}
	b, err := m.setting.GetByPrefix(tenantID, "jev.")
	if err != nil || json.Unmarshal(b, &s) != nil || !s.Enabled || strings.TrimSpace(s.APIKey) == "" {
		return nil
	}
	return systemone.New(s.APIKey, s.Model)
}

// transcriptLine is one message as Jev sees it.
type transcriptLine struct {
	From string `json:"from"`
	Text string `json:"text"`
}

// transcript turns the newest messages into Jev's state: who said what, the
// customer's own messages and public replies only.
func (m *Manager) transcript(msgs []cmodels.Message, contactID int) []transcriptLine {
	out := make([]transcriptLine, 0, jevTranscriptMessages)
	for _, msg := range msgs {
		if msg.Private || (msg.Type != cmodels.MessageIncoming && msg.Type != cmodels.MessageOutgoing) {
			continue
		}
		from := "business"
		switch {
		case msg.Type == cmodels.MessageIncoming && msg.SenderID == contactID:
			from = "customer"
		case msg.Type == cmodels.MessageIncoming:
			continue
		case m.isAssistantUser(msg.SenderID):
			from = "assistant"
		}
		text := m.messageText(msg)
		if text == "" {
			text = "[attachment]"
		}
		out = append(out, transcriptLine{From: from, Text: truncateRunes(text, jevMessageChars)})
	}
	if len(out) > jevTranscriptMessages {
		out = out[len(out)-jevTranscriptMessages:]
	}
	return out
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// gateDecision is what Jev said about the customer's latest message.
type gateDecision struct {
	Action           string
	ActionConfidence float64
	Frustration      float64
	FrustrationConf  float64
	Settled          float64
}

// Gate question IDs and the options of the action question.
const (
	actionReply   = "reply"
	actionNoReply = "no_reply"
	actionHuman   = "human"
)

var frustrationLevels = []string{
	"Calm, neutral or positive",
	"Slightly impatient",
	"Clearly annoyed",
	"Angry",
	"Furious, abusive or threatening",
}

// gate asks Jev what to do with the customer's latest message.
func (m *Manager) gate(ctx context.Context, jev *systemone.Client, a models.Assistant, lines []transcriptLine) (gateDecision, error) {
	state := map[string]any{
		"assistant":    a.Name,
		"role":         truncateRunes(a.Description, 500),
		"conversation": lines,
	}
	questions := map[string]systemone.Question{
		"action": systemone.Choice(
			"A business's automated assistant handles this chat. Decide what should happen with the customer's latest message.",
			map[string]string{
				actionReply:   "The latest customer message greets the business to start a conversation, asks a question, makes a request, or shares information the assistant should respond to.",
				actionNoReply: "The latest customer message needs no response: a plain acknowledgement such as ok, thanks or an emoji, a goodbye after the matter was settled, or spam and gibberish.",
				actionHuman:   "The customer asks to talk to a person, or raises something an automated assistant must not handle alone: a payment or refund complaint, a legal, medical or safety matter, or a dispute.",
			}),
		"frustration": systemone.Score("How frustrated or upset is the customer in their latest messages?", frustrationLevels),
		"settled":     systemone.Noul("Has everything the customer asked about been fully answered, so the conversation can be closed?"),
	}
	c, cancel := context.WithTimeout(ctx, jevTimeout)
	defer cancel()
	res, err := jev.Evaluate(c, state, questions)
	if err != nil {
		return gateDecision{}, err
	}
	action, frustration := res.Answers["action"], res.Answers["frustration"]
	return gateDecision{
		Action:           action.Choice,
		ActionConfidence: action.Confidence,
		Frustration:      frustration.Score,
		FrustrationConf:  frustration.Confidence,
		Settled:          res.Answers["settled"].Noul,
	}, nil
}

// decideGate turns Jev's answers into what the agent does, and why. Anything
// Jev isn't sure of goes to the LLM, which has its own handoff and resolve
// tools. hasReplied is whether the assistant has answered this customer
// before, without which a conversation is never resolved from a gate.
func decideGate(d gateDecision, threshold float64, handoffEnabled, hasReplied bool) (string, string) {
	if handoffEnabled && d.FrustrationConf >= threshold && d.Frustration >= frustrationHandoff {
		return verdictHandoff, "frustration"
	}
	if d.ActionConfidence < threshold {
		return verdictLLM, ""
	}
	switch d.Action {
	case actionHuman:
		if handoffEnabled {
			return verdictHandoff, "human"
		}
	case actionNoReply:
		// A customer the assistant hasn't answered yet always gets a reply,
		// even to a bare "hi".
		if !hasReplied {
			return verdictLLM, ""
		}
		if d.Settled >= settledResolve {
			return verdictResolve, ""
		}
		return verdictSkip, ""
	}
	return verdictLLM, ""
}

// guardResult is what Jev found wrong with a draft reply, if anything.
type guardResult struct {
	Reasons []string
}

func (g guardResult) Blocked() bool { return len(g.Reasons) > 0 }

// Guard reasons, which name i18n keys ai.jev.reason.<reason>.
const (
	reasonPolicy      = "policy"
	reasonUnsupported = "unsupported"
	reasonTone        = "tone"
)

// guard asks Jev whether the LLM's draft is safe to send. knowledge is what
// the knowledge base searches returned during the run; without any, Jev
// can't judge facts, so that question isn't asked.
func (m *Manager) guard(ctx context.Context, jev *systemone.Client, a models.Assistant, customerMessage, draft string, knowledge []string, threshold float64) (guardResult, error) {
	state := map[string]any{
		"guardrails":       cmpOr(truncateRunes(a.Guardrails, 3000), "No extra guardrails."),
		"customer_message": truncateRunes(customerMessage, jevMessageChars),
		"draft_reply":      truncateRunes(draft, 4000),
	}
	questions := map[string]systemone.Question{
		reasonPolicy: systemone.Noul("Does the draft reply promise, offer or commit to something the guardrails forbid or that an automated assistant can't grant on its own, such as a refund, discount, fee waiver, guaranteed admission or result, or a deadline the business hasn't confirmed?"),
		reasonTone:   systemone.Noul("Is the draft reply rude, dismissive, or unsafe to send to a customer?"),
	}
	if len(knowledge) > 0 {
		state["knowledge"] = knowledge
		questions[reasonUnsupported] = systemone.Noul("Does the draft reply state a specific fact, such as an amount, date, number, name or policy, that the knowledge doesn't support?")
	}
	c, cancel := context.WithTimeout(ctx, jevTimeout)
	defer cancel()
	res, err := jev.Evaluate(c, state, questions)
	if err != nil {
		return guardResult{}, err
	}
	var g guardResult
	for _, r := range []string{reasonPolicy, reasonUnsupported, reasonTone} {
		if ans, ok := res.Answers[r]; ok && ans.Noul >= threshold {
			g.Reasons = append(g.Reasons, r)
		}
	}
	return g, nil
}

// pickLeadStage asks Jev which stage the contact is at. It returns the stage
// to move to, or "" to leave them: when Jev isn't sure, agrees with the
// current stage, or the contact is already won or lost (a person decides
// those). A move into won or lost needs more confidence than one between
// open stages.
func (m *Manager) pickLeadStage(ctx context.Context, jev *systemone.Client, lines []transcriptLine, stages []lmodels.Stage, current string, threshold float64) (string, error) {
	if len(stages) < 2 {
		return "", nil
	}
	byKey := map[string]lmodels.Stage{}
	options := map[string]string{}
	for _, s := range stages {
		byKey[s.Key] = s
		options[s.Key] = s.Label + ": " + stageKindHint(s.Kind)
	}
	if cur, ok := byKey[current]; ok && cur.Kind != lmodels.KindOpen {
		return "", nil
	}
	state := map[string]any{"current_stage": byKey[current].Label, "conversation": lines}
	c, cancel := context.WithTimeout(ctx, jevTimeout)
	defer cancel()
	res, err := jev.Evaluate(c, state, map[string]systemone.Question{
		"stage": systemone.Choice("Where is this contact in the business's sales or admissions pipeline now, judging only by the conversation?", options),
	})
	if err != nil {
		return "", err
	}
	ans := res.Answers["stage"]
	next, ok := byKey[ans.Choice]
	if !ok || ans.Choice == current || ans.Confidence < threshold {
		return "", nil
	}
	if next.Kind != lmodels.KindOpen && ans.Confidence < max(threshold, terminalStageConfidence) {
		return "", nil
	}
	return next.Key, nil
}

func stageKindHint(kind string) string {
	switch kind {
	case lmodels.KindWon:
		return "the contact has committed: enrolled, paid, or confirmed they're joining."
	case lmodels.KindLost:
		return "the contact has said they aren't interested, chose elsewhere, or asked not to be contacted."
	default:
		return "the contact is still deciding."
	}
}

// updateLeadStage moves the contact to the stage Jev picks, if it's sure.
func (m *Manager) updateLeadStage(ctx context.Context, jev *systemone.Client, conv cmodels.Conversation, a models.Assistant, lines []transcriptLine) {
	if m.hooks.LeadStage == nil || m.hooks.SetLeadStage == nil {
		return
	}
	stages, current, err := m.hooks.LeadStage(conv.TenantID, conv.ContactID)
	if err != nil {
		m.lo.Error("error reading lead stage for jev", "contact_id", conv.ContactID, "error", err)
		return
	}
	next, err := m.pickLeadStage(ctx, jev, lines, stages, current, a.JevConfidence)
	if err != nil {
		m.lo.Warn("jev lead stage unavailable", "conversation_uuid", conv.UUID, "error", err)
		return
	}
	if next == "" {
		return
	}
	if err := m.hooks.SetLeadStage(conv.TenantID, conv.ContactID, next, a.UserID); err != nil {
		m.lo.Error("error moving lead stage", "contact_id", conv.ContactID, "stage", next, "error", err)
		return
	}
	m.lo.Info("jev moved lead stage", "contact_id", conv.ContactID, "from", current, "to", next)
	m.recordEvent(conv.TenantID, a.ID, conv.ID, "lead_stage")
}

// blockedNote is the private note a stopped reply leaves for the person who
// takes over: why Jev stopped it, and the draft to send or fix.
func (m *Manager) blockedNote(tenantID int, g guardResult, draft string) string {
	t := m.i18n.For(tenantID)
	reasons := make([]string, 0, len(g.Reasons))
	for _, r := range g.Reasons {
		reasons = append(reasons, t.T("ai.jev.reason."+r))
	}
	return t.Ts("ai.jev.blocked", "reasons", strings.Join(reasons, "; ")) + "\n\n" + draft
}

// followupDecision is Jev's call on a quiet conversation.
type followupDecision struct {
	FollowUp bool
	// Template is the index into the assistant's follow-up templates, or -1.
	Template int
}

const noTemplate = "none"

// decideFollowup asks Jev whether a quiet conversation should get a
// follow-up, and when templates are given (the reply window has closed),
// which fits. Without Jev, the caller follows up inside the window only.
func (m *Manager) decideFollowup(ctx context.Context, jev *systemone.Client, lines []transcriptLine, quietHours float64, sent int, templates []string, threshold float64) (followupDecision, error) {
	d := followupDecision{Template: -1}
	state := map[string]any{
		"conversation":            lines,
		"hours_since_last_reply":  fmt.Sprintf("%.0f", quietHours),
		"follow_ups_already_sent": sent,
	}
	questions := map[string]systemone.Question{
		"follow_up": systemone.Noul("The customer stopped replying after the business's last message. Is the conversation waiting on the customer, so one polite follow-up would help, for example they were asked a question, sent details, or offered a visit? Answer no if it reached a natural end, the customer declined, or asked not to be contacted."),
	}
	if len(templates) > 0 {
		options := map[string]string{noTemplate: "None of these messages fits this conversation."}
		for i, t := range templates {
			options[fmt.Sprintf("t%d", i)] = truncateRunes(t, 600)
		}
		questions["template"] = systemone.Choice("Which message is the best follow-up to send this customer now?", options)
	}
	c, cancel := context.WithTimeout(ctx, jevTimeout)
	defer cancel()
	res, err := jev.Evaluate(c, state, questions)
	if err != nil {
		return d, err
	}
	d.FollowUp = res.Answers["follow_up"].Noul >= threshold
	if t, ok := res.Answers["template"]; ok && t.Choice != noTemplate && t.Confidence >= threshold {
		var i int
		if _, err := fmt.Sscanf(t.Choice, "t%d", &i); err == nil && i >= 0 && i < len(templates) {
			d.Template = i
		}
	}
	return d, nil
}

func cmpOr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
