package main

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/user"
)

// TestAnsweredBy matches the leg Exotel says answered a call to an agent, by
// phone number or by the SIP user of their browser.
func TestAnsweredBy(t *testing.T) {
	phone := user.PhoneAgent{PhoneNumber: "+91 98765 43210"}
	browser := user.PhoneAgent{SIPUsername: "Asha01"}
	for _, tc := range []struct {
		a    user.PhoneAgent
		leg  string
		want bool
	}{
		{phone, "09876543210", true},
		{phone, "+919876543211", false},
		{browser, "sip:asha01@acme.voip.exotel.com", true},
		{browser, "asha01", true},
		{browser, "sip:asha02", false},
		// A browser agent without a phone doesn't match a leg without digits.
		{user.PhoneAgent{SIPUsername: "asha01"}, "sip:ravi", false},
		{phone, "sip:asha01", false},
	} {
		if got := answeredBy(tc.a, tc.leg); got != tc.want {
			t.Errorf("answeredBy(%+v, %q) = %v, want %v", tc.a, tc.leg, got, tc.want)
		}
	}
}
