package ai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/conectdesk/conectdesk/internal/ai/models"
	"github.com/zerodha/logf"
)

func TestParseLeadFields(t *testing.T) {
	fields := []LeadField{
		{Key: "course", DataType: "list", Values: []string{"JEE", "NEET", "Foundation"}},
		{Key: "class", DataType: "text"},
		{Key: "budget", DataType: "number"},
		{Key: "visit_date", DataType: "date"},
		{Key: "hostel", DataType: "checkbox"},
		{Key: "site", DataType: "link"},
	}
	got := ParseLeadFields("```json\n"+`{"course": "neet", "class": " Class 12 ", "budget": "1,20,000", "visit_date": "2026-10-03",
		"hostel": true, "site": "ftp://x", "city": "Pune"}`+"\n```", fields)
	want := map[string]any{"course": "NEET", "class": "Class 12", "budget": float64(120000), "visit_date": "2026-10-03", "hostel": true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v (%T), want %v", k, got[k], got[k], v)
		}
	}
	// A value outside a list, a bad date, and a reply that isn't JSON.
	if got := ParseLeadFields(`{"course": "MBA", "visit_date": "next week"}`, fields); len(got) != 0 {
		t.Errorf("kept %v", got)
	}
	if got := ParseLeadFields("I couldn't find anything.", fields); len(got) != 0 {
		t.Errorf("kept %v", got)
	}
}

func TestTranscribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" || r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil || r.FormValue("model") != "whisper-large-v3" {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "no file", http.StatusBadRequest)
			return
		}
		b, _ := io.ReadAll(f)
		w.Write([]byte(`{"text": "namaste, ` + string(b) + `"}`))
	}))
	defer srv.Close()

	lo := logf.New(logf.Opts{})
	c := NewOpenAIClient(models.ProviderConfig{BaseURL: srv.URL, APIKey: "k", TranscriptionModel: "whisper-large-v3"}, &lo, srv.Client())
	text, err := c.Transcribe(context.Background(), "call.mp3", "audio/mpeg", []byte("fees kitni hai"))
	if err != nil || text != "namaste, fees kitni hai" {
		t.Fatalf("Transcribe = %q, %v", text, err)
	}
	c = NewOpenAIClient(models.ProviderConfig{BaseURL: srv.URL}, &lo, srv.Client())
	if _, err := c.Transcribe(context.Background(), "call.mp3", "audio/mpeg", []byte("x")); err != ErrApiKeyNotSet {
		t.Fatalf("without a key: %v", err)
	}
	if _, err := NewOpenAIClient(models.ProviderConfig{BaseURL: srv.URL, APIKey: "k"}, &lo, srv.Client()).
		Transcribe(context.Background(), "call.mp3", "audio/mpeg", []byte(strings.Repeat("x", MaxTranscriptionBytes+1))); err == nil {
		t.Fatal("a recording over the limit went up")
	}
}
