package main

import "testing"

// TestParseContactImportRow checks each way a CSV can give a number: with a
// +, with an ISO country, or with a calling code, and the rows it rejects.
func TestParseContactImportRow(t *testing.T) {
	header := contactImportHeader([]string{"\ufeffFirst Name", "Last Name", "Email", "Mobile", "Country Code"})
	cases := []struct {
		row          []string
		phone, email string
		problem      bool
	}{
		{row: []string{"Ravi", "K", "", "+91 98765 43210", ""}, phone: "+919876543210"},
		{row: []string{"Ravi", "K", "", "098765 43210", "IN"}, phone: "+919876543210"},
		{row: []string{"Ravi", "K", "", "9876543210", "91"}, phone: "+919876543210"},
		{row: []string{"Ravi", "K", "", "9876543210", "+91"}, phone: "+919876543210"},
		{row: []string{"Ann", "", "Ann@Example.com", "", ""}, email: "ann@example.com"},
		{row: []string{"Ravi", "", "", "9876543210", ""}, problem: true},
		{row: []string{"", "K", "", "+919876543210", ""}, problem: true},
		{row: []string{"Ravi", "", "", "", ""}, problem: true},
		{row: []string{"Ravi", "", "not-an-email", "", ""}, problem: true},
		{row: []string{"Ravi", "", "", "+12", ""}, problem: true},
	}
	for _, c := range cases {
		got, problem := parseContactImportRow(c.row, header)
		if (problem != "") != c.problem {
			t.Errorf("%v: problem %q, want problem %v", c.row, problem, c.problem)
			continue
		}
		if !c.problem && (got.Phone != c.phone || got.Email != c.email) {
			t.Errorf("%v: got phone %q email %q, want %q %q", c.row, got.Phone, got.Email, c.phone, c.email)
		}
	}
}

// TestDialNumber checks what the dialer accepts as a number to call.
func TestDialNumber(t *testing.T) {
	for typed, want := range map[string]string{
		"+91 98765 43210":   "+919876543210",
		"9876543210":        "+919876543210",
		"0091 9876543210":   "+919876543210",
		"+1 (415) 555-0100": "+14155550100",
		"12345":             "",
		"call me":           "",
		"":                  "",
	} {
		if got := dialNumber(typed); got != want {
			t.Errorf("dialNumber(%q) = %q, want %q", typed, got, want)
		}
	}
}
