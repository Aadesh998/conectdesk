package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	aimodels "github.com/conectdesk/conectdesk/internal/ai/models"
	agentmodels "github.com/conectdesk/conectdesk/internal/aiagent/models"
	amodels "github.com/conectdesk/conectdesk/internal/automation/models"
	camodels "github.com/conectdesk/conectdesk/internal/custom_attribute/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
)

// The admissions setup gives an institute what it needs to answer enquiries
// with AI in one step: contact fields for admission details, knowledge base
// drafts to fill in, an AI assistant that replies in Hindi, English or
// Hinglish and hands over to a counsellor, and a rule sending new WhatsApp
// chats to it. Only what's missing is created, so running it again is safe.

// admissionFields are the contact fields an admissions enquiry fills in. The
// leads board shows the ones it knows on each card.
var admissionFields = []camodels.CustomAttribute{
	{Key: "student_name", Name: "Student's name", Description: "Who the enquiry is for, when a parent writes"},
	{Key: "class", Name: "Class", Description: "The student's current class or grade, e.g. Class 11"},
	{Key: "course", Name: "Course", Description: "The course or exam they're interested in, e.g. JEE, NEET, IELTS"},
	{Key: "batch", Name: "Preferred batch", Description: "The batch, timing or mode they want, e.g. weekend, evening, online"},
	{Key: "city", Name: "City", Description: "Where the student lives or wants to attend"},
}

// admissionSnippets are knowledge base drafts the assistant answers from once
// the institute fills them in and switches them on.
var admissionSnippets = []struct{ Title, Content string }{
	{"Courses and batches", "Courses we offer: [list each course, who it's for, and its duration]\n\nBatches: [list batches with days, timings and mode: classroom or online]\n\nNext batch starts on: [date]"},
	{"Fees and payment", "Fees: [course: fee, for each course]\n\nInstalments: [how fees can be paid in parts]\n\nWhat the fee includes: [study material, tests, etc.]"},
	{"Timings and location", "Address: [full address with a landmark]\nGoogle Maps: [link]\n\nOffice hours: [days and times]\nClass timings: [times]"},
	{"Admission process", "How to join: [steps, e.g. visit, counselling, admission test, fee payment]\n\nDocuments needed: [list]\n\nFree demo class: [yes/no, and how to book]"},
	{"Scholarships and discounts", "Scholarships: [who qualifies and how, e.g. a scholarship test]\n\nDiscounts: [sibling, early-bird, etc.]\n\nFor anything else, a counsellor will help."},
	{"Results and faculty", "Results: [recent selections and toppers]\n\nFaculty: [experience of the teachers]"},
}

const admissionsAssistantInstructions = `You answer admission enquiries for %s, an educational institute, from students and parents on WhatsApp.

- Students and parents often write in Hindi, English or Hinglish (Hindi typed in English letters). Always reply in the same language and script as their last message: Hinglish to Hinglish, Hindi to Hindi, English to English. For example, to "fees kitni hai?" reply like "JEE ki fees Rs 1,20,000 hai, 4 instalments mein de sakte hain.", not in English.
- Help with courses, batches, timings, fees, location, the admission process and demo classes, only from what you know about the institute.
- When someone is interested, ask for the student's name, class and the course they want, one question at a time, and offer a free counselling call or a visit.
- Hand over to a counsellor when they ask for a call or a person, want to book a visit, or ask something you can't answer, such as a discount or a special case.`

const admissionsAssistantGuardrails = `Never promise admission, a scholarship, a discount or results. Never ask for payment details or share anything about other students.`

// admissionsSetupRequest says what the admissions setup creates.
type admissionsSetupRequest struct {
	Fields    bool `json:"fields"`
	Snippets  bool `json:"snippets"`
	Assistant bool `json:"assistant"`
	// AssistantName is the new assistant's name.
	AssistantName string `json:"assistant_name"`
	// FallbackTeamID is the team the assistant hands over to; 0 for none.
	FallbackTeamID int `json:"fallback_team_id"`
	// WhatsAppInboxIDs are the inboxes whose new chats go to the assistant.
	WhatsAppInboxIDs []int `json:"whatsapp_inbox_ids"`
}

// admissionsSetupResult says what the setup created, and what already existed.
type admissionsSetupResult struct {
	FieldsAdded    []string `json:"fields_added"`
	SnippetsAdded  []string `json:"snippets_added"`
	AssistantID    int      `json:"assistant_id,omitempty"`
	AssistantAdded bool     `json:"assistant_added"`
	RuleAdded      bool     `json:"rule_added"`
}

// handleAdmissionsSetup sets up the AI admissions assistant and what it needs.
func handleAdmissionsSetup(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
		req admissionsSetupRequest
		out = admissionsSetupResult{FieldsAdded: []string{}, SnippetsAdded: []string{}}
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	for _, id := range req.WhatsAppInboxIDs {
		if rec, err := app.inbox.GetDBRecord(tid, id); err != nil || rec.Channel != inbox.ChannelWhatsApp {
			return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("admissionsSetup.invalidInbox"), nil))
		}
	}

	if req.Fields {
		added, err := addAdmissionFields(app, tid)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		out.FieldsAdded = added
	}
	if req.Snippets {
		added, err := addAdmissionSnippets(app, tid)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		out.SnippetsAdded = added
	}
	if req.Assistant {
		a, created, err := admissionsAssistant(app, tid, req)
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		out.AssistantID, out.AssistantAdded = a.ID, created
		if len(req.WhatsAppInboxIDs) > 0 {
			if out.RuleAdded, err = addAssistantRule(app, tid, a, req.WhatsAppInboxIDs); err != nil {
				return sendErrorEnvelope(r, err)
			}
		}
	}
	return r.SendEnvelope(out)
}

func addAdmissionFields(app *App, tid int) ([]string, error) {
	existing, err := app.customAttribute.GetAll(tid, "contact")
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, a := range existing {
		have[a.Key] = true
	}
	added := []string{}
	for _, f := range admissionFields {
		if have[f.Key] {
			continue
		}
		f.AppliesTo, f.DataType = "contact", "text"
		if _, err := app.customAttribute.Create(tid, f); err != nil {
			return added, err
		}
		added = append(added, f.Name)
	}
	return added, nil
}

// addAdmissionSnippets adds the knowledge base drafts, switched off, since the
// assistant would otherwise answer with their placeholders.
func addAdmissionSnippets(app *App, tid int) ([]string, error) {
	items, err := app.ai.GetKnowledgeBaseItems(tid)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, it := range items {
		have[strings.ToLower(it.Title)] = true
	}
	added := []string{}
	for _, s := range admissionSnippets {
		if have[strings.ToLower(s.Title)] {
			continue
		}
		if _, err := app.ai.CreateKnowledgeBaseItem(tid, s.Title, s.Content, aimodels.KnowledgeSourceManual, "", false); err != nil {
			return added, err
		}
		added = append(added, s.Title)
	}
	return added, nil
}

// admissionsAssistant returns the workspace's assistant by the requested
// name, creating it with the admissions instructions if there is none.
func admissionsAssistant(app *App, tid int, req admissionsSetupRequest) (agentmodels.Assistant, bool, error) {
	name := strings.TrimSpace(req.AssistantName)
	if name == "" {
		name = "Admissions assistant"
	}
	existing, err := app.aiAgent.GetAssistants(tid)
	if err != nil {
		return agentmodels.Assistant{}, false, err
	}
	for _, a := range existing {
		if strings.EqualFold(a.Name, name) {
			return a, false, nil
		}
	}
	a := agentmodels.Assistant{
		Name:           name,
		Description:    "Answers admission enquiries on WhatsApp and hands over to a counsellor.",
		Instructions:   fmt.Sprintf(admissionsAssistantInstructions, tenantConsts(app, tid).SiteName),
		Guardrails:     admissionsAssistantGuardrails,
		Tone:           "friendly",
		ResponseLength: "concise",
		MaxTurns:       10,
		HandoffEnabled: true,
		FallbackTeamID: null.NewInt(req.FallbackTeamID, req.FallbackTeamID > 0),
		Enabled:        true,
	}
	created, err := app.aiAgent.CreateAssistant(tid, a)
	return created, err == nil, err
}

// assistantRuleName names the rule sending new WhatsApp chats to an
// assistant, which is how the setup finds it again.
func assistantRuleName(a agentmodels.Assistant) string {
	return "New WhatsApp chats to " + a.Name
}

// addAssistantRule adds a rule assigning new conversations in the inboxes to
// the assistant, unless the workspace already has it.
func addAssistantRule(app *App, tid int, a agentmodels.Assistant, inboxIDs []int) (bool, error) {
	rules, err := app.automation.GetAllRules(tid, []byte(amodels.RuleTypeNewConversation))
	if err != nil {
		return false, err
	}
	for _, rr := range rules {
		if rr.Name == assistantRuleName(a) {
			return false, nil
		}
	}
	conds := make([]amodels.RuleDetail, 0, len(inboxIDs))
	for _, id := range inboxIDs {
		conds = append(conds, amodels.RuleDetail{
			Field: amodels.ConversationInbox, FieldType: amodels.FieldTypeConversationField,
			Operator: amodels.RuleOperatorEquals, Value: strconv.Itoa(id),
		})
	}
	body, err := json.Marshal([]amodels.Rule{{
		GroupOperator: amodels.OperatorAnd,
		Groups:        []amodels.RuleGroup{{LogicalOp: amodels.OperatorOR, Rules: conds}},
		Actions:       []amodels.RuleAction{{Type: amodels.ActionAssignUser, Value: []string{strconv.Itoa(a.UserID)}, DisplayValue: []string{a.Name}}},
	}})
	if err != nil {
		return false, err
	}
	if _, err := app.automation.CreateRule(tid, amodels.RuleRecord{
		Name:        assistantRuleName(a),
		Description: "Added by the admissions setup.",
		Type:        amodels.RuleTypeNewConversation,
		Enabled:     true,
		Rules:       body,
	}); err != nil {
		return false, err
	}
	return true, nil
}
