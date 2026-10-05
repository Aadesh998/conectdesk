package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTranscriptIncludesCall(t *testing.T) {
	call := Message{
		Type: MessageOutgoing, SenderType: SenderTypeAgent, Private: true,
		TextContent: "Call ended · 0:14",
		Meta:        json.RawMessage(`{"call_ai":{"transcript":"हेलो, फीस कितनी है?"}}`),
	}
	note := Message{Type: MessageOutgoing, SenderType: SenderTypeAgent, Private: true, TextContent: "internal note"}
	if !call.IsCall() {
		t.Fatal("call with a transcript should be a call")
	}
	if note.IsCall() {
		t.Fatal("plain private note should not be a call")
	}
	got := Transcript([]Message{call}, 10)
	if !strings.Contains(got, "Phone call") || !strings.Contains(got, "फीस कितनी है") {
		t.Fatalf("transcript missing call text: %q", got)
	}
}
