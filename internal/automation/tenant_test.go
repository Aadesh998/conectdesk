package automation

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/automation/models"
)

// TestRulesStayInTheirTenant checks a tenant's rules are never selected for
// another tenant's conversations.
func TestRulesStayInTheirTenant(t *testing.T) {
	e := &Engine{rules: []models.Rule{
		{TenantID: 2, Type: models.RuleTypeNewConversation},
		{TenantID: 2, Type: models.RuleTypeConversationUpdate, Events: []string{"conversation.user.assigned"}},
		{TenantID: 3, Type: models.RuleTypeNewConversation},
	}}

	for _, c := range []struct {
		tenant, want int
		typ, event   string
	}{
		{2, 1, models.RuleTypeNewConversation, ""},
		{3, 1, models.RuleTypeNewConversation, ""},
		{3, 0, models.RuleTypeConversationUpdate, "conversation.user.assigned"},
		{4, 0, models.RuleTypeNewConversation, ""},
	} {
		got := e.filterRulesByType(c.tenant, c.typ, c.event)
		if len(got) != c.want {
			t.Errorf("tenant %d %s: %d rules, want %d", c.tenant, c.typ, len(got), c.want)
		}
		for _, r := range got {
			if r.TenantID != c.tenant {
				t.Errorf("tenant %d got tenant %d's rule", c.tenant, r.TenantID)
			}
		}
	}

	systemUsers := map[int]int{2: 10, 3: 20}
	e.systemUserID = func(tenantID int) (int, error) { return systemUsers[tenantID], nil }
	if !e.isSystemUser(2, 10) || e.isSystemUser(3, 10) {
		t.Fatal("system user of one tenant treated as another's")
	}
}
