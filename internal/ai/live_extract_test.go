package ai

import (
	"cmp"
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/conectdesk/conectdesk/internal/ai/models"
	"github.com/zerodha/logf"
)

// TestLiveExtract runs the lead-field prompt against a real provider when
// CONECTDESK_LIVE_OPENAI_KEY is set; it's skipped otherwise.
func TestLiveExtract(t *testing.T) {
	key := os.Getenv("CONECTDESK_LIVE_OPENAI_KEY")
	if key == "" {
		t.Skip("no live key")
	}
	lo := logf.New(logf.Opts{})
	c := NewOpenAIClient(models.ProviderConfig{APIKey: key, Model: cmp.Or(os.Getenv("CONECTDESK_LIVE_MODEL"), "gpt-4.1-nano")}, &lo, http.DefaultClient)
	fields := []LeadField{
		{Key: "student_name", Name: "Student's name", DataType: "text"},
		{Key: "class", Name: "Class", DataType: "text"},
		{Key: "course", Name: "Course", DataType: "text"},
		{Key: "batch", Name: "Preferred batch", DataType: "text"},
		{Key: "city", Name: "City", DataType: "text"},
	}
	for _, tr := range []string{
		"Customer: Hi, maths tuition for class 8 hai kya? Near Sector 21.",
		"Customer: Good evening. Is there a demo class for NEET this week?",
	} {
		b := "Fields:\n"
		for _, f := range fields {
			b += "- " + f.Key + " (" + f.DataType + "): " + f.Name + "\n"
		}
		resp, err := c.SendPrompt(context.Background(), models.PromptPayload{SystemPrompt: leadFieldsSystemPrompt, UserPrompt: b + "\nConversation:\n" + tr})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s\n=> %v", tr, ParseLeadFields(resp, fields))
	}
}
