package main

import "testing"

func TestFormatToolDuration(t *testing.T) {
	for sec, want := range map[float64]string{
		-5:     "0s",
		42:     "42s",
		600:    "10m",
		3600:   "1h",
		8100:   "2h 15m",
		86400:  "1d",
		97200:  "1d 3h",
		599.6:  "10m",
		359999: "4d 3h",
	} {
		if got := formatToolDuration(sec); got != want {
			t.Errorf("formatToolDuration(%v) = %q, want %q", sec, got, want)
		}
	}
}

func TestCompliance(t *testing.T) {
	for _, c := range []struct {
		met, breached int
		want          string
	}{
		{0, 0, "no"},
		{1, 0, "100.0%"},
		{2, 1, "66.7%"},
		{1, 2, "33.3%"},
	} {
		if got := compliance(c.met, c.breached); got != c.want {
			t.Errorf("compliance(%d, %d) = %q, want %q", c.met, c.breached, got, c.want)
		}
	}
}

func TestSLAToolDays(t *testing.T) {
	base := slaToolBase{days: 30}
	ptr := func(v int) *int { return &v }
	for _, c := range []struct {
		in   *int
		want int
	}{
		{nil, 30},
		{ptr(0), 0},
		{ptr(7), 7},
		{ptr(-3), 0},
		{ptr(1000), maxSLAToolDays},
	} {
		if got := base.resolveDays(c.in); got != c.want {
			t.Errorf("resolveDays(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestSLAToolArgs(t *testing.T) {
	if _, ok := slaMetricArg("first_response"); !ok {
		t.Error("first_response rejected")
	}
	if m, ok := slaMetricArg(""); !ok || m != "" {
		t.Error("blank metric should mean all")
	}
	if _, ok := slaMetricArg("csat"); ok {
		t.Error("unknown metric accepted")
	}
	if slaListLimit(0) != defaultSLAListLimit || slaListLimit(500) != maxSLAListLimit || slaListLimit(5) != 5 {
		t.Error("list limit not bounded")
	}
	var in struct {
		Days *int `json:"days"`
	}
	if err := parseToolArgs("  ", &in); err != nil || in.Days != nil {
		t.Errorf("blank args = %v, %v", in.Days, err)
	}
	if err := parseToolArgs(`{"days": 3}`, &in); err != nil || in.Days == nil || *in.Days != 3 {
		t.Errorf("days args = %v, %v", in.Days, err)
	}
	if err := parseToolArgs(`not json`, &in); err == nil {
		t.Error("bad JSON accepted")
	}
}
