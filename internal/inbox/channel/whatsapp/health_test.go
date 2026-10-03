package whatsapp

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestQualityAlert(t *testing.T) {
	number, a, ok := QualityAlert([]byte(`{"display_phone_number": "+91 98765 43210", "event": "DOWNGRADE", "current_limit": "TIER_250"}`))
	if !ok || number != "919876543210" || a.Severity != AlertCritical || !strings.Contains(a.Detail, "250 people a day") {
		t.Fatalf("downgrade: %q %+v %v", number, a, ok)
	}
	if _, a, _ := QualityAlert([]byte(`{"display_phone_number": "1", "event": "FLAGGED"}`)); a.Severity != AlertWarning {
		t.Errorf("flagged: %+v", a)
	}
	if _, a, _ := QualityAlert([]byte(`{"display_phone_number": "1", "event": "UNFLAGGED"}`)); a.Severity != AlertInfo {
		t.Errorf("unflagged: %+v", a)
	}
	if _, _, ok := QualityAlert([]byte(`{}`)); ok {
		t.Error("empty change taken for an alert")
	}
}

func TestAccountAlert(t *testing.T) {
	a, ok := AccountAlert([]byte(`{"event": "ACCOUNT_RESTRICTION", "restriction_info": [{"restriction_type": "RESTRICTED_BIZ_INITIATED_MESSAGING", "expiration": "2026-10-05"}]}`))
	if !ok || a.Severity != AlertCritical || !strings.Contains(a.Detail, "restricted biz initiated messaging until 2026-10-05") {
		t.Fatalf("restriction: %+v %v", a, ok)
	}
	if _, ok := AccountAlert([]byte(`{"event": "VERIFIED_ACCOUNT"}`)); ok {
		t.Error("unrelated event taken for an alert")
	}
}

func TestNameAlert(t *testing.T) {
	_, a, ok := NameAlert([]byte(`{"display_phone_number": "1", "decision": "REJECTED", "requested_verified_name": "Acme", "rejection_reason": "NAME_FORMAT_UNACCEPTABLE"}`))
	if !ok || a.Severity != AlertWarning || !strings.Contains(a.Detail, "name format unacceptable") {
		t.Fatalf("got %+v %v", a, ok)
	}
}

func TestHealth(t *testing.T) {
	f := &fakeGraph{handler: func(w http.ResponseWriter, r *http.Request, body []byte) {
		w.Write([]byte(`{"quality_rating": "YELLOW", "messaging_limit_tier": "TIER_1K", "status": "CONNECTED", "throughput": {"level": "STANDARD"}}`))
	}}
	w := newTestInbox(t, f)
	h, err := w.Health(context.Background())
	if err != nil || h.QualityRating != "YELLOW" || h.MessagingLimitTier != "TIER_1K" || h.Throughput.Level != "STANDARD" {
		t.Fatalf("got %+v %v", h, err)
	}
	if !strings.Contains(f.calls[0], "/PNID") {
		t.Errorf("call: %s", f.calls[0])
	}
}
