package main

import (
	"context"
	"time"

	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/voice"
)

// callReconcileInterval is how often Exotel's call list is checked for
// inbound calls the flow didn't report: voice.reconcile_interval, 5 minutes
// by default, and "0" switches the check off.
func callReconcileInterval() time.Duration {
	if ko.Exists("voice.reconcile_interval") && ko.Duration("voice.reconcile_interval") == 0 {
		return 0
	}
	if d := ko.Duration("voice.reconcile_interval"); d >= time.Minute {
		return d
	}
	return 5 * time.Minute
}

// callReconcileLookback is how far back the check looks:
// voice.reconcile_lookback, 2 hours by default.
func callReconcileLookback() time.Duration {
	if d := ko.Duration("voice.reconcile_lookback"); d >= 30*time.Minute {
		return d
	}
	return 2 * time.Hour
}

// callReconciler is the safety net for inbound calls. Exotel only tells
// ConectDesk about a call through the Passthru applets of the ExoPhone's
// flow, so a flow without them, a branch without a Passthru, or a webhook
// that failed leaves a call unlogged, or logged as ringing for good. The
// reconciler reads each phone inbox's calls from Exotel's Calls API, logs the
// inbound ones it doesn't know with their outcome and recording, and
// finishes the ones still ringing.
type callReconciler struct {
	app      *App
	lookback time.Duration
	// seen is when each ended call was first listed. A call is only acted on
	// a run later, so the flow's own webhooks, which come as the call ends,
	// go first.
	seen map[string]time.Time
	// done is the calls already logged and finished.
	done map[string]time.Time
}

func newCallReconciler(app *App, lookback time.Duration) *callReconciler {
	return &callReconciler{app: app, lookback: lookback, seen: map[string]time.Time{}, done: map[string]time.Time{}}
}

// runCallReconciler checks every running phone inbox each interval.
func runCallReconciler(ctx context.Context, app *App, interval, lookback time.Duration) {
	r := newCallReconciler(app, lookback)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.run(ctx, time.Now())
		}
	}
}

func (r *callReconciler) run(ctx context.Context, now time.Time) {
	for _, inb := range r.app.inbox.Running(inbox.ChannelVoice) {
		if ctx.Err() != nil {
			return
		}
		if v, ok := inb.(*voice.Voice); ok {
			r.reconcileInbox(ctx, v, now)
		}
	}
	r.forget(now)
}

// forget drops calls older than any list can return.
func (r *callReconciler) forget(now time.Time) {
	for _, m := range []map[string]time.Time{r.seen, r.done} {
		for sid, at := range m {
			if now.Sub(at) > r.lookback+time.Hour {
				delete(m, sid)
			}
		}
	}
}

func (r *callReconciler) reconcileInbox(ctx context.Context, v *voice.Voice, now time.Time) {
	c, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	calls, err := v.Exotel().ListCalls(c, now.Add(-r.lookback))
	if err != nil {
		r.app.lo.Warn("error listing exotel calls", "inbox_id", v.Identifier(), "error", err)
		return
	}
	for _, call := range r.ready(calls, v.Config().CallerID, now) {
		if r.reconcile(c, v, call) {
			r.done[call.Sid] = now
			delete(r.seen, call.Sid)
		}
	}
}

// ready returns the listed calls to settle now: ended inbound calls to the
// ExoPhone, not settled before, and listed as ended on an earlier run too.
func (r *callReconciler) ready(calls []voice.ExotelCall, exophone string, now time.Time) []voice.ExotelCall {
	var out []voice.ExotelCall
	for _, call := range calls {
		if _, ok := r.done[call.Sid]; ok || !call.IsInboundTo(exophone) {
			continue
		}
		if _, ok := r.seen[call.Sid]; !ok {
			r.seen[call.Sid] = now
			continue
		}
		out = append(out, call)
	}
	return out
}

// reconcile logs and finishes one ended inbound call, if it needs it, and
// reports whether it's settled.
func (r *callReconciler) reconcile(ctx context.Context, v *voice.Voice, call voice.ExotelCall) bool {
	app, tid := r.app, v.TenantID()
	logged, found, err := app.conversation.GetCall(tid, callSourceID(call.Sid))
	if err != nil {
		app.lo.Error("error fetching call", "call_sid", call.Sid, "error", err)
		return false
	}
	if found && loggedStatus(logged.Meta) != voice.StatusRinging {
		return true
	}

	// The list doesn't say whether anyone answered; the call's details do.
	details, err := v.Exotel().CallDetails(ctx, call.Sid)
	if err != nil {
		app.lo.Warn("error fetching exotel call details", "call_sid", call.Sid, "error", err)
		return false
	}
	call.Details = details.Details
	if call.RecordingURL == "" {
		call.RecordingURL = details.RecordingURL
	}
	ev := call.MissedEvent()

	if !found {
		if _, _, err := logIncomingCall(app, v, ev); err != nil {
			app.lo.Error("error logging recovered call", "inbox_id", v.Identifier(), "call_sid", call.Sid, "error", err)
			return false
		}
	}
	finishCall(app, v, tid, ev)
	app.lo.Info("recovered inbound call from exotel", "inbox_id", v.Identifier(), "call_sid", call.Sid,
		"status", ev.Status, "was_logged", found)
	return true
}

// loggedStatus is the status on a call's message meta.
func loggedStatus(meta map[string]any) string {
	if call, ok := meta[voice.MetaCallKey].(map[string]any); ok {
		if s, ok := call["status"].(string); ok {
			return s
		}
	}
	return ""
}
