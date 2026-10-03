// Package models contains the data models for the businesshours package.
package models

import (
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx/types"
	"github.com/volatiletech/null/v9"
)

// BusinessHours represents the business in the database.
type BusinessHours struct {
	ID           int            `db:"id" json:"id"`
	CreatedAt    time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time      `db:"updated_at" json:"updated_at"`
	Name         string         `db:"name" json:"name"`
	Description  null.String    `db:"description" json:"description"`
	IsAlwaysOpen bool           `db:"is_always_open" json:"is_always_open"`
	Holidays     types.JSONText `db:"holidays" json:"holidays"`
	Hours        types.JSONText `db:"hours" json:"hours"`
}

// WorkingHours represents the working hours for a specific day.
type WorkingHours struct {
	Open  string `json:"open"`
	Close string `json:"close"`
}

// Holiday represents a holiday.
type Holiday struct {
	Name string `json:"name"`
	Date string `json:"date"`
}

// IsOpen reports whether the business is open at t, in the time zone tz: on a
// working day that isn't a holiday, between its open and close times. Hours it
// can't read count as open, so a misconfigured schedule never hides a caller.
func (b BusinessHours) IsOpen(t time.Time, tz string) bool {
	if b.IsAlwaysOpen {
		return true
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	local := t.In(loc)

	var holidays []Holiday
	if len(b.Holidays) > 0 && json.Unmarshal(b.Holidays, &holidays) == nil {
		for _, h := range holidays {
			if h.Date == local.Format(time.DateOnly) {
				return false
			}
		}
	}

	var week map[string]WorkingHours
	if json.Unmarshal(b.Hours, &week) != nil {
		return true
	}
	day, ok := week[local.Weekday().String()]
	if !ok {
		return false
	}
	open, err1 := time.Parse("15:04", day.Open)
	closing, err2 := time.Parse("15:04", day.Close)
	if err1 != nil || err2 != nil {
		return true
	}
	now := local.Hour()*60 + local.Minute()
	return now >= open.Hour()*60+open.Minute() && now < closing.Hour()*60+closing.Minute()
}
