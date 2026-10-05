package ai

import "testing"

func TestMostlyNonLatin(t *testing.T) {
	cases := map[string]bool{
		"ہیلو ہیلو باغ کو تیاری کرو":  true,
		"हेलो हेलो बाग को तैयारी करो": true,
		"hello, fees kitni hai?":      false,
		"ok ہیلو ہیلو باغ":            true,
		"admission कब है bhai":        false,
		"":                            false,
		"123 ...":                     false,
	}
	for in, want := range cases {
		if got := mostlyNonLatin(in); got != want {
			t.Errorf("mostlyNonLatin(%q) = %v, want %v", in, got, want)
		}
	}
}
