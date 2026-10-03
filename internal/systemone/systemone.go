// Package systemone calls TypeSafe's System One API (the Jev models). Unlike
// an LLM it writes no text: it answers typed questions about a state - pick
// one option, score on a rubric, or a yes/no probability - each with a
// calibrated confidence, in well under a second. The AI agent uses it to
// decide what to do with a conversation before, and instead of, asking the
// LLM.
package systemone

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is TypeSafe's API.
	DefaultBaseURL = "https://api.typesafe.ai"

	// DefaultModel follows TypeSafe's stable release.
	DefaultModel = "jev-latest"

	// requestTimeout bounds one call. Jev answers in 70-500ms; anything near
	// this means the service is struggling, and the agent carries on without it.
	requestTimeout = 8 * time.Second

	maxResponseBytes = 1 << 20
	maxErrorLen      = 300
	retries          = 2
)

// Question types.
const (
	TypeNoul   = "noul"
	TypeChoice = "choice"
	TypeScore  = "score"
)

// Question is one typed question about the state.
type Question struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	// Criteria is map[string]string for a choice (option -> description) or a
	// noul ({"true": ..., "false": ...}), and []string of 2-10 levels for a
	// score.
	Criteria any `json:"criteria,omitempty"`
}

// Noul asks a yes/no question.
func Noul(instructions string) Question {
	return Question{Type: TypeNoul, Instructions: instructions}
}

// Choice asks to pick one of options, keyed by the value returned.
func Choice(instructions string, options map[string]string) Question {
	return Question{Type: TypeChoice, Instructions: instructions, Criteria: options}
}

// Score asks to rate on levels, lowest first.
func Score(instructions string, levels []string) Question {
	return Question{Type: TypeScore, Instructions: instructions, Criteria: levels}
}

// Answer is the answer to one question.
type Answer struct {
	Type string `json:"type"`
	// Noul is the probability the answer is yes.
	Noul float64 `json:"noul"`
	// Choice is the option picked.
	Choice string `json:"choice"`
	// Score is the level picked, counting from 1.
	Score         float64            `json:"score"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
}

// Usage is what a call consumed.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Response is the answer to every question of a call.
type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Error is an error answer from the API.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("system one api: %s (status %d)", e.Message, e.Status)
}

// ErrNotConfigured is returned by a client without an API key.
var ErrNotConfigured = errors.New("system one api key is not set")

// Client calls the API with one workspace's key.
type Client struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

// New returns a client. An empty model means DefaultModel.
func New(apiKey, model string) *Client {
	if strings.TrimSpace(model) == "" {
		model = DefaultModel
	}
	return &Client{
		apiKey:  strings.TrimSpace(apiKey),
		model:   strings.TrimSpace(model),
		baseURL: DefaultBaseURL,
		http:    &http.Client{Timeout: requestTimeout},
	}
}

// SetBaseURL points the client elsewhere, for tests.
func (c *Client) SetBaseURL(u string) {
	c.baseURL = strings.TrimRight(u, "/")
}

// Evaluate asks every question about state in one call; Jev answers them
// independently and in parallel. state is a string or JSON-encodable value.
// It retries a rate limit or overload twice with backoff.
func (c *Client) Evaluate(ctx context.Context, state any, questions map[string]Question) (Response, error) {
	if c.apiKey == "" {
		return Response{}, ErrNotConfigured
	}
	body, err := json.Marshal(map[string]any{"model": c.model, "state": state, "questions": questions})
	if err != nil {
		return Response{}, err
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return Response{}, ctx.Err()
			case <-time.After(time.Duration(attempt*attempt) * 300 * time.Millisecond):
			}
		}
		res, retry, err := c.do(ctx, body)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return Response{}, lastErr
}

func (c *Client) do(ctx context.Context, body []byte) (Response, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return Response{}, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// A network error may be passing; a cancelled context isn't.
		return Response{}, ctx.Err() == nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Response{}, true, err
	}

	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		var env struct {
			Error   any    `json:"error"`
			Message string `json:"message"`
			Detail  any    `json:"detail"`
		}
		if json.Unmarshal(b, &env) == nil {
			switch {
			case env.Message != "":
				msg = env.Message
			case env.Error != nil:
				msg = fmt.Sprint(env.Error)
			case env.Detail != nil:
				msg = fmt.Sprint(env.Detail)
			}
		}
		if len(msg) > maxErrorLen {
			msg = msg[:maxErrorLen]
		}
		retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 529 || resp.StatusCode >= 500
		return Response{}, retry, &Error{Status: resp.StatusCode, Message: msg}
	}

	var out Response
	if err := json.Unmarshal(b, &out); err != nil {
		return Response{}, false, fmt.Errorf("decoding system one response: %w", err)
	}
	if out.Answers == nil {
		out.Answers = map[string]Answer{}
	}
	return out, false, nil
}
