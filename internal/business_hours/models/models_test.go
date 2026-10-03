package models

import (
	"testing"
	"time"
)

func TestIsOpen(t *testing.T) {
	b := BusinessHours{
		Hours:    []byte(`{"Monday": {"open": "09:00", "close": "18:00"}, "Tuesday": {"open": "09:00", "close": "18:00"}}`),
		Holidays: []byte(`[{"name": "Diwali", "date": "2026-11-10"}]`),
	}
	ist := time.FixedZone("IST", 5*3600+1800)
	for at, want := range map[time.Time]bool{
		time.Date(2026, 9, 28, 10, 0, 0, 0, ist):     true,  // Monday morning
		time.Date(2026, 9, 28, 8, 59, 0, 0, ist):     false, // before opening
		time.Date(2026, 9, 28, 18, 0, 0, 0, ist):     false, // at closing
		time.Date(2026, 10, 4, 12, 0, 0, 0, ist):     false, // Sunday
		time.Date(2026, 11, 10, 12, 0, 0, 0, ist):    false, // a holiday, on a Tuesday
		time.Date(2026, 9, 28, 4, 0, 0, 0, time.UTC): true,  // 09:30 in Kolkata
	} {
		if got := b.IsOpen(at, "Asia/Kolkata"); got != want {
			t.Errorf("IsOpen(%v) = %v, want %v", at, got, want)
		}
	}
	if !(BusinessHours{IsAlwaysOpen: true}).IsOpen(time.Now(), "Asia/Kolkata") {
		t.Error("always open")
	}
}
