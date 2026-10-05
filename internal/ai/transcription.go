package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/conectdesk/conectdesk/internal/ai/models"
)

const (
	defaultTranscriptionModel = "whisper-1"
	// transcriptionTimeout bounds one recording's transcription: a long call
	// takes a while to upload and transcribe.
	transcriptionTimeout = 3 * time.Minute
	// MaxTranscriptionBytes is the largest recording the API accepts.
	MaxTranscriptionBytes = 25 << 20
)

// hinglishPrompt steers speech-to-text towards Hinglish, Hindi written in
// Latin letters, by showing it what such a transcript looks like.
const hinglishPrompt = "Haan ji, namaste. Main aapki kya madad kar sakta hoon? Mujhe admission ke baare mein jaanna tha, fees kitni hai aur classes kab se start hongi? Okay, theek hai, thank you."

// Transcribe turns a recording into text with the provider's speech-to-text,
// in whatever language was spoken (Hindi, English, a mix of both), written
// in Latin letters.
func (o *OpenAIClient) Transcribe(ctx context.Context, name, contentType string, audio []byte) (string, error) {
	if o.cfg.APIKey == "" {
		return "", ErrApiKeyNotSet
	}
	if len(audio) > MaxTranscriptionBytes {
		return "", fmt.Errorf("recording is %d MB, over the %d MB the provider takes", len(audio)>>20, MaxTranscriptionBytes>>20)
	}
	ctx, cancel := context.WithTimeout(ctx, transcriptionTimeout)
	defer cancel()
	return o.transcribe(ctx, name, audio, hinglishPrompt)
}

// mostlyNonLatin reports whether most of the letters in s are in a script
// other than Latin, e.g. Devanagari, or Urdu's Arabic script, which Whisper
// often picks for spoken Hindi.
func mostlyNonLatin(s string) bool {
	var letters, other int
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if !unicode.Is(unicode.Latin, r) {
			other++
		}
	}
	return letters > 0 && other*2 > letters
}

// transcribe sends one transcription request, with a prompt showing the
// style the transcript should be written in.
func (o *OpenAIClient) transcribe(ctx context.Context, name string, audio []byte, prompt string) (string, error) {
	model := o.cfg.TranscriptionModel
	if model == "" {
		model = defaultTranscriptionModel
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("model", model)
	_ = w.WriteField("response_format", "json")
	if prompt != "" {
		_ = w.WriteField("prompt", prompt)
	}
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.BaseURL+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+o.cfg.APIKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrProviderUnavailable, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderResponseBytes))
	if err != nil {
		return "", err
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return "", ErrInvalidAPIKey
	case resp.StatusCode == http.StatusTooManyRequests:
		return "", fmt.Errorf("%w: %s", ErrRateLimited, b)
	case resp.StatusCode >= 500:
		return "", fmt.Errorf("%w: status %d: %s", ErrProviderUnavailable, resp.StatusCode, b)
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("provider API error: status %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", fmt.Errorf("decoding transcription: %w", err)
	}
	return out.Text, nil
}

// Transcribe turns a call recording into text with the workspace's
// completion provider.
func (m *Manager) Transcribe(tenantID int, ctx context.Context, name, contentType string, audio []byte) (string, error) {
	cfg, err := m.getProviderConfig(tenantID, models.ProviderTypeCompletion)
	if err != nil {
		return "", err
	}
	if err := m.consumeRequest(tenantID); err != nil {
		return "", err
	}
	text, err := NewOpenAIClient(cfg, m.lo, m.providerHTTPClient).Transcribe(ctx, name, contentType, audio)
	if err != nil {
		return "", m.providerError(err)
	}
	// The prompt usually gets Hinglish, but not always. A transcript still in
	// Urdu or Devanagari script is never shown; it is put into English instead.
	if mostlyNonLatin(text) {
		en, err := m.CompletionRaw(tenantID, ctx, englishFallbackPrompt, text)
		en = strings.TrimSpace(stripCodeFence(en))
		if err != nil || en == "" || mostlyNonLatin(en) {
			m.lo.Warn("error putting transcript into english", "error", err)
			return "", fmt.Errorf("transcript came back in a non-latin script and could not be put into english: %v", err)
		}
		return en, nil
	}
	return text, nil
}

// englishFallbackPrompt asks for a transcript the speech recogniser wrote in
// Urdu or Devanagari script to be put into English.
const englishFallbackPrompt = `You are given a phone call transcript between a business (often an educational institute) and a caller. The speech recogniser wrote it in Urdu or Devanagari script, but the call was most likely in Hindi or Hinglish.

Translate it into plain English. Keep it faithful: the same sentences in the same order, nothing added or summarised. Keep names and numbers as they are. Reply with the English transcript only, no notes or quotes.`
