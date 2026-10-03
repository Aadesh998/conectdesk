package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

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

// Transcribe turns a recording into text with the provider's speech-to-text,
// in whatever language was spoken (Hindi, English, a mix of both).
func (o *OpenAIClient) Transcribe(ctx context.Context, name, contentType string, audio []byte) (string, error) {
	if o.cfg.APIKey == "" {
		return "", ErrApiKeyNotSet
	}
	if len(audio) > MaxTranscriptionBytes {
		return "", fmt.Errorf("recording is %d MB, over the %d MB the provider takes", len(audio)>>20, MaxTranscriptionBytes>>20)
	}
	model := o.cfg.TranscriptionModel
	if model == "" {
		model = defaultTranscriptionModel
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("model", model)
	_ = w.WriteField("response_format", "json")
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

	ctx, cancel := context.WithTimeout(ctx, transcriptionTimeout)
	defer cancel()
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
	return text, nil
}
