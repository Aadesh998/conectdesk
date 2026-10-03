package phone

import "testing"

func TestNumbers(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"IN", "9876543210"}:     "+919876543210",
		{"IN", "09876543210"}:    "+919876543210",
		{"IN", "919876543210"}:   "+919876543210",
		{"US", "(415) 555-0100"}: "+14155550100",
		{"+44", "7700 900123"}:   "+447700900123",
		{"", "+971 50 123 4567"}: "+971501234567",
		{"", "919876543210"}:     "+919876543210",
		{"IN", ""}:               "",
	} {
		if got := FromContact(in[0], in[1]); got != want {
			t.Errorf("FromContact(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
	if E164("09876543210") != "+919876543210" || Digits("+91 98765 43210") != "919876543210" {
		t.Error("E164/Digits")
	}
	for in, want := range map[string]string{
		"+91 98765 43210": "9876543210",
		"919876543210":    "9876543210",
		"555-0100":        "5550100",
		"":                "",
	} {
		if got := Key(in); got != want {
			t.Errorf("Key(%q) = %q, want %q", in, got, want)
		}
	}
}
