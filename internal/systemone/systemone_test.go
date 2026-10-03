package systemone

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestEvaluate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("unexpected request %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var req struct {
			Model     string              `json:"model"`
			State     any                 `json:"state"`
			Questions map[string]Question `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != DefaultModel || req.Questions["action"].Type != TypeChoice || req.Questions["human"].Type != TypeNoul {
			t.Errorf("unexpected body %+v", req)
		}
		w.Write([]byte(`{"model":"jev-1.13.0","answers":{
			"action":{"type":"choice","choice":"reply","probabilities":{"reply":0.9,"no_reply":0.1},"confidence":0.88},
			"human":{"type":"noul","noul":0.04}},"usage":{"input_tokens":120,"output_tokens":3}}`))
	}))
	defer srv.Close()

	c := New("key", "")
	c.SetBaseURL(srv.URL)
	res, err := c.Evaluate(context.Background(), "customer: what are the fees?", map[string]Question{
		"action": Choice("What should happen?", map[string]string{"reply": "answer", "no_reply": "nothing"}),
		"human":  Noul("Does the customer ask for a person?"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if a := res.Answers["action"]; a.Choice != "reply" || a.Confidence != 0.88 {
		t.Errorf("action = %+v", a)
	}
	if res.Answers["human"].Noul != 0.04 || res.Usage.InputTokens != 120 {
		t.Errorf("response = %+v", res)
	}
}

func TestEvaluateRetriesOverload(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(529)
			w.Write([]byte(`{"message":"overloaded"}`))
			return
		}
		w.Write([]byte(`{"answers":{"q":{"type":"noul","noul":1}}}`))
	}))
	defer srv.Close()

	c := New("key", "")
	c.SetBaseURL(srv.URL)
	res, err := c.Evaluate(context.Background(), "s", map[string]Question{"q": Noul("?")})
	if err != nil || res.Answers["q"].Noul != 1 || calls.Load() != 3 {
		t.Fatalf("res %+v err %v calls %d", res, err, calls.Load())
	}
}

func TestEvaluateErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"invalid api key"}`))
	}))
	defer srv.Close()

	c := New("bad", "")
	c.SetBaseURL(srv.URL)
	_, err := c.Evaluate(context.Background(), "s", map[string]Question{"q": Noul("?")})
	var se *Error
	if !errors.As(err, &se) || se.Status != 401 || se.Message != "invalid api key" || calls.Load() != 1 {
		t.Fatalf("err %v calls %d", err, calls.Load())
	}

	if _, err := New("", "").Evaluate(context.Background(), "s", nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("no key: %v", err)
	}
}
