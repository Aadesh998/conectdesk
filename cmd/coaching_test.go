package main

import (
	"encoding/json"
	"testing"

	amodels "github.com/conectdesk/conectdesk/internal/automation/models"
)

// TestAdmissionFieldsOnCards checks the admission fields use the keys the
// leads board shows on its cards.
func TestAdmissionFieldsOnCards(t *testing.T) {
	want := map[string]bool{"class": true, "course": true, "batch": true, "city": true}
	for _, f := range admissionFields {
		delete(want, f.Key)
	}
	if len(want) != 0 {
		t.Fatalf("missing card fields %v", want)
	}
}

// TestAssistantRuleShape checks the rule the setup writes reads back as the
// automation engine expects: any of the inboxes, assigned to the assistant.
func TestAssistantRuleShape(t *testing.T) {
	body, _ := json.Marshal([]amodels.Rule{{
		GroupOperator: amodels.OperatorAnd,
		Groups: []amodels.RuleGroup{{LogicalOp: amodels.OperatorOR, Rules: []amodels.RuleDetail{
			{Field: amodels.ConversationInbox, FieldType: amodels.FieldTypeConversationField, Operator: amodels.RuleOperatorEquals, Value: "4"},
		}}},
		Actions: []amodels.RuleAction{{Type: amodels.ActionAssignUser, Value: []string{"9"}}},
	}})
	var rules []amodels.Rule
	if err := json.Unmarshal(body, &rules); err != nil || len(rules) != 1 || rules[0].Groups[0].Rules[0].Field != "inbox" || rules[0].Actions[0].Value[0] != "9" {
		t.Fatalf("rules %s: %v", body, err)
	}
}
