package calendar

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a resolved alert send instant.
type Schedule struct {
	DueAt       time.Time
	LocalDate   time.Time
	ExpiryUTC   string
	CalendarDay string
}

// Due resolves a before/after offset against an expiry instant and local send
// time. Calendar math is done in the configured location.
func Due(expiresAt time.Time, offset int, unit, direction, sendTime string, loc *time.Location) (Schedule, error) {
	if expiresAt.IsZero() {
		return Schedule{}, errors.New("expiry date is required")
	}
	if offset < 0 {
		return Schedule{}, errors.New("offset cannot be negative")
	}
	if direction != "before" && direction != "after" {
		return Schedule{}, fmt.Errorf("direction must be before or after, got %q", direction)
	}
	if unit != "days" && unit != "months" {
		return Schedule{}, fmt.Errorf("offset unit must be days or months, got %q", unit)
	}
	if loc == nil {
		loc = time.UTC
	}
	hour, minute, err := ParseSendTime(sendTime)
	if err != nil {
		return Schedule{}, err
	}

	expiryLocal := expiresAt.In(loc)
	day := time.Date(expiryLocal.Year(), expiryLocal.Month(), expiryLocal.Day(), 0, 0, 0, 0, loc)
	delta := offset
	if direction == "before" {
		delta = -offset
	}
	if unit == "days" {
		day = day.AddDate(0, 0, delta)
	} else {
		day = addMonthsClamped(day, delta)
	}

	due := resolveLocal(day.Year(), day.Month(), day.Day(), hour, minute, loc)
	return Schedule{
		DueAt:       due.UTC(),
		LocalDate:   day,
		ExpiryUTC:   ExpiryDate(expiresAt),
		CalendarDay: day.Format("2006-01-02"),
	}, nil
}

// ParseSendTime parses HH:MM in 24-hour form.
func ParseSendTime(value string) (int, int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, errors.New("send time must use HH:MM")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, errors.New("send time must use HH:MM")
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, errors.New("send time must use HH:MM")
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, errors.New("send time must be between 00:00 and 23:59")
	}
	return hour, minute, nil
}

// ExpiryDate returns the UTC calendar date used for dedupe.
func ExpiryDate(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// CalendarDaysBetween returns whole calendar days from a to b in UTC.
func CalendarDaysBetween(a, b time.Time) int {
	start := time.Date(a.UTC().Year(), a.UTC().Month(), a.UTC().Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(b.UTC().Year(), b.UTC().Month(), b.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return int(end.Sub(start).Hours() / 24)
}

func addMonthsClamped(day time.Time, delta int) time.Time {
	target := time.Date(day.Year(), day.Month()+time.Month(delta), 1, 0, 0, 0, 0, day.Location())
	last := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, day.Location()).Day()
	date := day.Day()
	if date > last {
		date = last
	}
	return time.Date(target.Year(), target.Month(), date, 0, 0, 0, 0, day.Location())
}

// resolveLocal returns the first valid instant at or after the requested wall
// time. During a spring-forward gap this moves to the next valid minute.
func resolveLocal(year int, month time.Month, day, hour, minute int, loc *time.Location) time.Time {
	start := time.Date(year, month, day, 0, 0, 0, 0, loc)
	target := hour*60 + minute
	var best time.Time
	for i := 0; i < 26*60; i++ {
		candidate := start.Add(time.Duration(i) * time.Minute)
		wall := candidate.Hour()*60 + candidate.Minute()
		if wall >= target && candidate.Day() == day {
			if best.IsZero() {
				best = candidate
			}
			if wall == target {
				return candidate
			}
		}
	}
	if best.IsZero() {
		return time.Date(year, month, day, hour, minute, 0, 0, loc)
	}
	return best
}
