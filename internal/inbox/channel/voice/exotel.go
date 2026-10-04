package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Exotel regions, the API host of each.
const (
	ExotelSingapore = "api.exotel.com"
	ExotelMumbai    = "api.in.exotel.com"
)

const (
	exotelTimeout   = 30 * time.Second
	recordingTimout = 2 * time.Minute
	maxAPIResponse  = 1 << 20
)

// Exotel calls the Exotel API for one account.
type Exotel struct {
	sid, key, token string
	base            string
	http            *http.Client
}

// NewExotel returns a client for an Exotel account in a region.
func NewExotel(accountSID, apiKey, apiToken, subdomain string) *Exotel {
	if subdomain != ExotelMumbai {
		subdomain = ExotelSingapore
	}
	return &Exotel{
		sid: accountSID, key: apiKey, token: apiToken,
		base: "https://" + subdomain + "/v1/Accounts/" + url.PathEscape(accountSID),
		http: &http.Client{Timeout: exotelTimeout},
	}
}

// SetBaseURL points the client at another server, for tests.
func (e *Exotel) SetBaseURL(u string) { e.base = strings.TrimRight(u, "/") }

// ExotelError is an error the Exotel API returned.
type ExotelError struct {
	Status  int
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *ExotelError) Error() string {
	return fmt.Sprintf("exotel: %s (code %d, status %d)", e.Message, e.Code, e.Status)
}

func (e *Exotel) do(req *http.Request, out any) error {
	req.SetBasicAuth(e.key, e.token)
	req.Header.Set("Accept", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponse))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var env struct {
			RestException *ExotelError `json:"RestException"`
		}
		if json.Unmarshal(body, &env) == nil && env.RestException != nil {
			env.RestException.Status = resp.StatusCode
			return env.RestException
		}
		return &ExotelError{Status: resp.StatusCode, Message: strings.TrimSpace(string(body))}
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// Check verifies the credentials by reading the account.
func (e *Exotel) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.base+".json", nil)
	if err != nil {
		return err
	}
	return e.do(req, nil)
}

// ConnectOpts is a click-to-call: Exotel rings the agent first and, once they
// answer, dials the customer, showing callerID to both.
type ConnectOpts struct {
	Agent, Customer, CallerID string
	StatusURL                 string
	Record                    bool
	// TimeLimit caps the call in seconds; 0 leaves Exotel's default.
	TimeLimit int
	// CustomField comes back on the status callback, so it can be matched.
	CustomField string
}

// Connect starts a click-to-call and returns its call SID.
func (e *Exotel) Connect(ctx context.Context, o ConnectOpts) (string, error) {
	form := url.Values{
		"From":                      {o.Agent},
		"To":                        {o.Customer},
		"CallerId":                  {o.CallerID},
		"CallType":                  {"trans"},
		"StatusCallback":            {o.StatusURL},
		"StatusCallbackContentType": {"application/json"},
		"StatusCallbackEvents[0]":   {"terminal"},
		"Record":                    {strconv.FormatBool(o.Record)},
	}
	if o.TimeLimit > 0 {
		form.Set("TimeLimit", strconv.Itoa(o.TimeLimit))
	}
	if o.CustomField != "" {
		form.Set("CustomField", o.CustomField)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.base+"/Calls/connect.json", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		Call struct {
			Sid string `json:"Sid"`
		} `json:"Call"`
	}
	if err := e.do(req, &out); err != nil {
		return "", err
	}
	if out.Call.Sid == "" {
		return "", fmt.Errorf("exotel returned no call sid")
	}
	return out.Call.Sid, nil
}

// ExotelCall is a call as Exotel's Calls API lists it.
type ExotelCall struct {
	Sid          string `json:"Sid"`
	Direction    string `json:"Direction"`
	Status       string `json:"Status"`
	From         string `json:"From"`
	To           string `json:"To"`
	PhoneNumber  string `json:"PhoneNumber"`
	Duration     any    `json:"Duration"`
	RecordingURL string `json:"RecordingUrl"`
	DateCreated  string `json:"DateCreated"`
	// Details is only filled in by CallDetails.
	Details struct {
		ConversationDuration any `json:"ConversationDuration"`
	} `json:"Details"`
}

// ExotelZone is the time zone Exotel's call times and filters are in.
var ExotelZone = time.FixedZone("IST", 5*3600+1800)

// maxCallPages caps the pages ListCalls follows.
const maxCallPages = 20

// ListCalls returns the account's calls created since a time, every page of
// them up to maxCallPages.
func (e *Exotel) ListCalls(ctx context.Context, since time.Time) ([]ExotelCall, error) {
	q := url.Values{
		"DateCreated": {"gte:" + since.In(ExotelZone).Format(time.DateTime)},
		"PageSize":    {"100"},
	}
	next := e.base + "/Calls.json?" + q.Encode()
	var out []ExotelCall
	for range maxCallPages {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			Metadata struct {
				NextPageURI string `json:"NextPageUri"`
			} `json:"Metadata"`
			Calls []ExotelCall `json:"Calls"`
		}
		if err := e.do(req, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Calls...)
		if page.Metadata.NextPageURI == "" || len(page.Calls) == 0 {
			return out, nil
		}
		// The next page is a path on the same host.
		u, err := url.Parse(e.base)
		if err != nil {
			return nil, err
		}
		next = u.Scheme + "://" + u.Host + page.Metadata.NextPageURI
	}
	return out, nil
}

// CallDetails returns one call with its Details, which say how long the
// caller and an agent were connected.
func (e *Exotel) CallDetails(ctx context.Context, sid string) (ExotelCall, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.base+"/Calls/"+url.PathEscape(sid)+".json?details=true", nil)
	if err != nil {
		return ExotelCall{}, err
	}
	var out struct {
		Call ExotelCall `json:"Call"`
	}
	if err := e.do(req, &out); err != nil {
		return ExotelCall{}, err
	}
	return out.Call, nil
}

// IsInboundTo reports whether a listed call is a finished call into the
// given ExoPhone. Calls still going on are left until they end.
func (c ExotelCall) IsInboundTo(exophone string) bool {
	if !strings.EqualFold(c.Direction, "inbound") || Digits(c.To) == "" || Digits(c.To) != Digits(exophone) {
		return false
	}
	switch strings.ToLower(c.Status) {
	case "completed", "busy", "no-answer", "failed", "canceled", "cancelled":
		return true
	}
	return false
}

// MissedEvent is how an inbound call went, from its Details: answered when
// the caller was connected to someone, else voicemail when they left a
// recording, else missed.
func (c ExotelCall) MissedEvent() CallEvent {
	ev := CallEvent{
		SID:          c.Sid,
		Direction:    DirectionInbound,
		From:         c.From,
		To:           c.To,
		RecordingURL: c.RecordingURL,
		Status:       StatusNoAnswer,
		Recovered:    true,
	}
	switch talked := toInt(c.Details.ConversationDuration); {
	case talked > 0:
		ev.Status, ev.Duration = StatusCompleted, talked
	case c.RecordingURL != "":
		ev.Status = StatusVoicemail
	}
	return ev
}

// DownloadRecording fetches a call recording, refusing anything off
// Exotel's hosts or larger than maxBytes.
func (e *Exotel) DownloadRecording(ctx context.Context, rawURL string, maxBytes int64) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", err
	}
	if u.Scheme != "https" || !isExotelHost(u.Hostname()) {
		return nil, "", fmt.Errorf("refusing recording on host %q", u.Hostname())
	}
	ctx, cancel := context.WithTimeout(ctx, recordingTimout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	// Recordings are behind the account's credentials unless made public.
	req.SetBasicAuth(e.key, e.token)
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("recording download returned status %d", resp.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(content)) > maxBytes {
		return nil, "", fmt.Errorf("recording is over the %d byte limit", maxBytes)
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" || ct == "application/octet-stream" || ct == "binary/octet-stream" {
		ct = "audio/mpeg"
	}
	return content, ct, nil
}

func isExotelHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, h := range []string{"exotel.com", "exotel.in"} {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// ParseStatusCallback reads the status callback Exotel posts when a
// click-to-call ends, as JSON or as a form.
func ParseStatusCallback(body []byte, contentType string) (CallEvent, error) {
	var v struct {
		CallSid              string `json:"CallSid"`
		Status               string `json:"Status"`
		RecordingURL         string `json:"RecordingUrl"`
		ConversationDuration any    `json:"ConversationDuration"`
		CustomField          string `json:"CustomField"`
		From                 string `json:"From"`
		To                   string `json:"To"`
	}
	if strings.Contains(contentType, "json") {
		if err := json.Unmarshal(body, &v); err != nil {
			return CallEvent{}, fmt.Errorf("decoding status callback: %w", err)
		}
	} else {
		q, err := url.ParseQuery(string(body))
		if err != nil {
			return CallEvent{}, fmt.Errorf("decoding status callback: %w", err)
		}
		v.CallSid, v.Status, v.RecordingURL = q.Get("CallSid"), q.Get("Status"), q.Get("RecordingUrl")
		v.ConversationDuration, v.CustomField = q.Get("ConversationDuration"), q.Get("CustomField")
		v.From, v.To = q.Get("From"), q.Get("To")
	}
	if v.CallSid == "" {
		return CallEvent{}, fmt.Errorf("status callback has no CallSid")
	}
	return CallEvent{
		SID:          v.CallSid,
		Direction:    DirectionOutbound,
		Status:       normalizeStatus(v.Status),
		Duration:     toInt(v.ConversationDuration),
		RecordingURL: v.RecordingURL,
		From:         v.From,
		To:           v.To,
		CustomField:  v.CustomField,
	}, nil
}

// ParsePassthru reads the query of a Passthru applet request in an inbound
// call's flow. Exotel names some parameters two ways across flows, hence the
// fallbacks.
func ParsePassthru(q url.Values) (CallEvent, error) {
	sid := q.Get("CallSid")
	if sid == "" {
		return CallEvent{}, fmt.Errorf("passthru has no CallSid")
	}
	first := func(keys ...string) string {
		for _, k := range keys {
			if v := strings.TrimSpace(q.Get(k)); v != "" {
				return v
			}
		}
		return ""
	}
	ev := CallEvent{
		SID:          sid,
		Direction:    DirectionInbound,
		From:         first("CallFrom", "From"),
		To:           first("CallTo", "To"),
		RecordingURL: first("RecordingUrl"),
		Duration:     toInt(first("DialCallDuration", "ConversationDuration")),
		AnsweredBy:   first("DialWhomNumber"),
		Pressed:      strings.Trim(first("digits", "Digits"), `"`),
		Status:       StatusRinging,
	}
	if s := first("DialCallStatus"); s != "" {
		ev.Status = normalizeStatus(s)
	}
	return ev, nil
}

func normalizeStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "completed", "answered":
		return StatusCompleted
	case "busy":
		return StatusBusy
	case "no-answer", "noanswer", "no_answer", "canceled", "cancelled":
		return StatusNoAnswer
	case "failed":
		return StatusFailed
	case "in-progress", "ringing", "queued":
		return StatusRinging
	}
	return StatusFailed
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	}
	return 0
}
