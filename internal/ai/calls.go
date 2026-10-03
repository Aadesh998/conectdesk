package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const callSummarySystemPrompt = `You summarise phone calls between a business (often an educational institute) and a caller (a student, parent or customer) for the business's team. The transcript may be in Hindi, English or a mix of both (Hinglish), and speakers aren't labelled.

Write the summary in %s, as 2 to 5 short bullet points starting with "- ":
- what the caller wanted;
- key facts they gave (course, class, city, budget, dates, names);
- what was promised or agreed, and the next step with its time if one was set.
Only use what was said in the call. Don't guess or add advice. If the call has no real conversation (silence, a wrong number, a voicemail with no request), write a single bullet saying so.`

// SummarizeCall writes a short summary of a call's transcript for the team,
// in language (e.g. "English").
func (m *Manager) SummarizeCall(tenantID int, ctx context.Context, transcript, language string) (string, error) {
	if language == "" {
		language = "English"
	}
	resp, err := m.CompletionRaw(tenantID, ctx, fmt.Sprintf(callSummarySystemPrompt, language), "Call transcript:\n"+capText(transcript, 24000))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stripCodeFence(resp)), nil
}

// LeadField is a contact field the AI can fill in from a conversation.
type LeadField struct {
	Key         string
	Name        string
	Description string
	// DataType is the custom attribute's: text, number, checkbox, date, link
	// or list.
	DataType string
	// Values are a list's options.
	Values []string
}

const leadFieldsSystemPrompt = `You read a conversation or phone call between a business and a customer (often a student or a parent enquiring at an educational institute) and fill in facts about the customer. It may be in Hindi, English or Hinglish.

Return only a JSON object. Its keys are field keys from the list the user gives. Include a field when the customer stated it or asked about it for themselves, e.g. "is there a demo class for NEET?" means the course is NEET, and "maths tuition for class 8" means the course is maths tuition and the class is Class 8. Leave out anything guessed or not mentioned. Values follow each field's type:
- text: a short plain value in English, e.g. "Class 11" or "Pune";
- number: a number;
- checkbox: true or false;
- date: YYYY-MM-DD;
- link: a URL;
- list: exactly one of the listed options.
Return {} when nothing is known. Never add keys that aren't in the list.`

// ExtractLeadFields reads a conversation's transcript and returns the fields
// it clearly states, checked against each field's type.
func (m *Manager) ExtractLeadFields(tenantID int, ctx context.Context, transcript string, fields []LeadField) (map[string]any, error) {
	if len(fields) == 0 || strings.TrimSpace(transcript) == "" {
		return map[string]any{}, nil
	}
	var b strings.Builder
	b.WriteString("Fields:\n")
	for _, f := range fields {
		fmt.Fprintf(&b, "- %s (%s): %s", f.Key, f.DataType, f.Name)
		if f.Description != "" {
			b.WriteString(" — " + f.Description)
		}
		if f.DataType == "list" && len(f.Values) > 0 {
			b.WriteString(". Options: " + strings.Join(f.Values, " | "))
		}
		b.WriteString("\n")
	}
	b.WriteString("\nConversation:\n" + capText(transcript, 24000))
	resp, err := m.CompletionRaw(tenantID, ctx, leadFieldsSystemPrompt, b.String())
	if err != nil {
		return nil, err
	}
	return ParseLeadFields(resp, fields), nil
}

// ParseLeadFields reads the model's JSON reply, keeping only known fields
// whose values fit their type. A reply that isn't JSON yields nothing.
func ParseLeadFields(resp string, fields []LeadField) map[string]any {
	out := map[string]any{}
	raw := strings.TrimSpace(stripCodeFence(resp))
	if i, j := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); i >= 0 && j > i {
		raw = raw[i : j+1]
	}
	var got map[string]any
	if json.Unmarshal([]byte(raw), &got) != nil {
		return out
	}
	for _, f := range fields {
		v, ok := got[f.Key]
		if !ok || v == nil {
			continue
		}
		if clean, ok := fitType(v, f); ok {
			out[f.Key] = clean
		}
	}
	return out
}

func fitType(v any, f LeadField) (any, bool) {
	switch f.DataType {
	case "number":
		switch n := v.(type) {
		case float64:
			return n, true
		case string:
			x, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(n, ",", "")), 64)
			return x, err == nil
		}
	case "checkbox":
		b, ok := v.(bool)
		return b, ok
	case "date":
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		if _, err := time.Parse(time.DateOnly, strings.TrimSpace(s)); err != nil {
			return nil, false
		}
		return strings.TrimSpace(s), true
	case "list":
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		for _, opt := range f.Values {
			if strings.EqualFold(strings.TrimSpace(s), opt) {
				return opt, true
			}
		}
	case "link":
		s, ok := v.(string)
		s = strings.TrimSpace(s)
		return s, ok && (strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://"))
	default:
		s, ok := v.(string)
		if !ok {
			if n, isNum := v.(float64); isNum {
				s, ok = strconv.FormatFloat(n, 'f', -1, 64), true
			}
		}
		s = strings.TrimSpace(s)
		if len(s) > 200 {
			s = s[:200]
		}
		return s, ok && s != ""
	}
	return nil, false
}

// capText keeps the end of a long text, where a conversation's latest facts are.
func capText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}
