package voice

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNumbers(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"IN", "9876543210"}:     "+919876543210",
		{"IN", "09876543210"}:    "+919876543210",
		{"IN", "9123456789"}:     "+919123456789",
		{"IN", "919876543210"}:   "+919876543210",
		{"US", "(415) 555-0100"}: "+14155550100",
		{"+44", "7700 900123"}:   "+447700900123",
		{"", "+971 50 123 4567"}: "+971501234567",
		{"IN", ""}:               "",
	} {
		if got := ContactE164(in[0], in[1]); got != want {
			t.Errorf("ContactE164(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
	// Exotel reports Indian callers as 0-prefixed national numbers.
	if E164("09876543210") != "+919876543210" || Digits("+91 98765 43210") != "919876543210" {
		t.Error("E164/Digits")
	}
}

func TestParsePassthru(t *testing.T) {
	ev, err := ParsePassthru(url.Values{"CallSid": {"abc"}, "CallFrom": {"09876543210"}, "CallTo": {"08047112233"}})
	if err != nil || ev.SID != "abc" || ev.Status != StatusRinging || ev.Direction != DirectionInbound || ev.From != "09876543210" {
		t.Fatalf("incoming: %+v %v", ev, err)
	}
	ev, _ = ParsePassthru(url.Values{"CallSid": {"abc"}, "DialCallStatus": {"completed"}, "DialCallDuration": {"75"},
		"DialWhomNumber": {"09800000001"}, "RecordingUrl": {"https://recordings.exotel.com/x.mp3"}})
	if ev.Status != StatusCompleted || ev.Duration != 75 || ev.AnsweredBy != "09800000001" || ev.RecordingURL == "" {
		t.Fatalf("completed: %+v", ev)
	}
	if ev, _ := ParsePassthru(url.Values{"CallSid": {"abc"}, "DialCallStatus": {"no-answer"}}); ev.Status != StatusNoAnswer {
		t.Errorf("missed: %+v", ev)
	}
	if _, err := ParsePassthru(url.Values{}); err == nil {
		t.Error("no CallSid accepted")
	}
}

func TestParseStatusCallback(t *testing.T) {
	ev, err := ParseStatusCallback([]byte(`{"CallSid": "s1", "Status": "busy", "ConversationDuration": 0, "CustomField": "conv-1"}`), "application/json")
	if err != nil || ev.Status != StatusBusy || ev.CustomField != "conv-1" || ev.Direction != DirectionOutbound {
		t.Fatalf("json: %+v %v", ev, err)
	}
	ev, err = ParseStatusCallback([]byte(`CallSid=s2&Status=completed&ConversationDuration=42&RecordingUrl=https%3A%2F%2Frecordings.exotel.com%2Fa.mp3`), "application/x-www-form-urlencoded")
	if err != nil || ev.Status != StatusCompleted || ev.Duration != 42 || ev.RecordingURL != "https://recordings.exotel.com/a.mp3" {
		t.Fatalf("form: %+v %v", ev, err)
	}
}

func TestConnect(t *testing.T) {
	var got url.Values
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		auth = r.Header.Get("Authorization")
		if !strings.HasSuffix(r.URL.Path, "/Calls/connect.json") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"Call": {"Sid": "CALL1", "Status": "in-progress"}}`))
	}))
	defer srv.Close()

	e := NewExotel("acme1", "key", "token", ExotelMumbai)
	e.SetBaseURL(srv.URL + "/v1/Accounts/acme1")
	sid, err := e.Connect(context.Background(), ConnectOpts{Agent: "+919800000001", Customer: "+919876543210", CallerID: "+918047112233", StatusURL: "https://acme.example/s", Record: true})
	if err != nil || sid != "CALL1" {
		t.Fatalf("got %q %v", sid, err)
	}
	if got.Get("From") != "+919800000001" || got.Get("To") != "+919876543210" || got.Get("CallerId") != "+918047112233" || got.Get("Record") != "true" {
		t.Errorf("form: %v", got)
	}
	if !strings.HasPrefix(auth, "Basic ") {
		t.Errorf("auth: %q", auth)
	}
}

func TestRecordingHostPinned(t *testing.T) {
	e := NewExotel("a", "k", "t", ExotelSingapore)
	for _, u := range []string{"http://recordings.exotel.com/a.mp3", "https://169.254.169.254/a", "https://exotel.com.evil.io/a"} {
		if _, _, err := e.DownloadRecording(context.Background(), u, 1<<20); err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("%s: %v", u, err)
		}
	}
}

func TestStatusLabel(t *testing.T) {
	if StatusLabel(Call{Direction: DirectionInbound, Status: StatusNoAnswer}) != "Missed call" ||
		StatusLabel(Call{Status: StatusCompleted, Duration: 125}) != "Call ended · 2:05" ||
		FormatDuration(3725) != "1:02:05" {
		t.Error("labels")
	}
}

func TestMenuAndVoicemail(t *testing.T) {
	cfg := Config{Menu: []MenuOption{{Digit: "1", TeamID: 7, Label: "Sales"}, {Digit: "2", TeamID: 8, Label: "Support"}}}
	// Exotel's Gather sends the digits quoted.
	if o, ok := cfg.MenuChoice(`"2"`); !ok || o.TeamID != 8 {
		t.Errorf("choice: %+v %v", o, ok)
	}
	if _, ok := cfg.MenuChoice("9"); ok {
		t.Error("unknown key routed")
	}
	if _, ok := cfg.MenuChoice(""); ok {
		t.Error("no key routed")
	}
	ev, _ := ParsePassthru(url.Values{"CallSid": {"a"}, "digits": {`"1"`}})
	if ev.Pressed != "1" {
		t.Errorf("pressed: %q", ev.Pressed)
	}
	if StatusLabel(Call{Direction: DirectionInbound, Status: StatusVoicemail}) != "Voicemail" {
		t.Error("voicemail label")
	}
}

func TestSIP(t *testing.T) {
	if SIPLeg("asha") != "sip:asha" {
		t.Error("SIPLeg")
	}
	for in, want := range map[string]string{
		"sip:Asha@acme.voip.exotel.com": "asha",
		"SIP:asha":                      "asha",
		"asha@acme.voip.exotel.com":     "asha",
		" asha ":                        "asha",
		"":                              "",
	} {
		if got := SIPUser(in); got != want {
			t.Errorf("SIPUser(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListCalls(t *testing.T) {
	var filters []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/Accounts/acme1/Calls.json" && r.URL.Query().Get("Page") == "":
			filters = append(filters, r.URL.Query().Get("DateCreated"))
			w.Write([]byte(`{"Metadata": {"NextPageUri": "/v1/Accounts/acme1/Calls.json?Page=2"}, "Calls": [{"Sid": "c1", "Direction": "inbound"}]}`))
		case r.URL.Path == "/v1/Accounts/acme1/Calls.json":
			w.Write([]byte(`{"Metadata": {"NextPageUri": null}, "Calls": [{"Sid": "c2", "Direction": "outbound-api"}]}`))
		case r.URL.Path == "/v1/Accounts/acme1/Calls/c1.json" && r.URL.Query().Get("details") == "true":
			w.Write([]byte(`{"Call": {"Sid": "c1", "Details": {"ConversationDuration": 42}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	e := NewExotel("acme1", "key", "token", ExotelMumbai)
	e.SetBaseURL(srv.URL + "/v1/Accounts/acme1")
	since := time.Date(2026, 10, 4, 4, 30, 0, 0, time.UTC)
	calls, err := e.ListCalls(context.Background(), since)
	if err != nil || len(calls) != 2 || calls[0].Sid != "c1" || calls[1].Sid != "c2" {
		t.Fatalf("got %+v, %v", calls, err)
	}
	// Exotel filters on its own time zone, India's.
	if len(filters) != 1 || filters[0] != "gte:2026-10-04 10:00:00" {
		t.Errorf("filters: %v", filters)
	}
	c, err := e.CallDetails(context.Background(), "c1")
	if err != nil || toInt(c.Details.ConversationDuration) != 42 {
		t.Fatalf("details: %+v, %v", c, err)
	}
}

func TestRecoveredCalls(t *testing.T) {
	in := ExotelCall{Sid: "c1", Direction: "inbound", Status: "completed", From: "09876543210", To: "08047112233"}
	if !in.IsInboundTo("+918047112233") {
		t.Error("an inbound call to the ExoPhone wasn't matched")
	}
	for _, c := range []ExotelCall{
		{Direction: "inbound", Status: "in-progress", To: "08047112233"},
		{Direction: "outbound-api", Status: "completed", To: "08047112233"},
		{Direction: "inbound", Status: "completed", To: "08047112299"},
	} {
		if c.IsInboundTo("+918047112233") {
			t.Errorf("matched %+v", c)
		}
	}

	if ev := in.MissedEvent(); ev.Status != StatusNoAnswer || !ev.Recovered || ev.Direction != DirectionInbound {
		t.Errorf("missed: %+v", ev)
	}
	in.Details.ConversationDuration = "65"
	if ev := in.MissedEvent(); ev.Status != StatusCompleted || ev.Duration != 65 {
		t.Errorf("answered: %+v", ev)
	}
	in.Details.ConversationDuration = 0.0
	in.RecordingURL = "https://recordings.exotel.com/c1.mp3"
	if ev := in.MissedEvent(); ev.Status != StatusVoicemail || ev.RecordingURL == "" {
		t.Errorf("voicemail: %+v", ev)
	}
}
