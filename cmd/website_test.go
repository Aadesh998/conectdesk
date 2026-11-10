package main

import "testing"

func TestInstagramHandle(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.instagram.com/getconnectdesk":        "getconnectdesk",
		"https://www.instagram.com/getconnectdesk/":       "getconnectdesk",
		"https://www.instagram.com/getconnectdesk/?hl=en": "getconnectdesk",
		"https://www.instagram.com":                       "",
		"https://www.instagram.com/":                      "",
	} {
		if got := instagramHandle(in); got != want {
			t.Errorf("instagramHandle(%q) = %q, want %q", in, got, want)
		}
	}
}
