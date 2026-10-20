package domain

import (
	"testing"
	"time"

	"github.com/mrweez/iwmmd/internal/calendar"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		ace   string
		ok    bool
	}{
		{"Example.COM.", "example.com", true},
		{"Bücher.example", "xn--bcher-kva.example", true},
		{"example.com", "example.com", true},
		{"https://example.com", "", false},
		{"example", "", false},
		{"-bad.example", "", false},
		{"192.0.2.1", "", false},
		{"bad_example.com", "", false},
		{"", "", false},
	}
	for _, test := range tests {
		got, err := Normalize(test.input)
		if test.ok && err != nil {
			t.Errorf("Normalize(%q) error: %v", test.input, err)
		}
		if !test.ok && err == nil {
			t.Errorf("Normalize(%q) unexpectedly succeeded", test.input)
		}
		if test.ok && got.ACE != test.ace {
			t.Errorf("Normalize(%q) = %q, want %q", test.input, got.ACE, test.ace)
		}
	}
}

func TestDueCalendarMath(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Date(2028, time.March, 31, 12, 0, 0, 0, time.UTC)

	oneMonthBefore, err := calendar.Due(expiry, 1, "months", "before", "09:00", loc)
	if err != nil {
		t.Fatal(err)
	}
	if got := oneMonthBefore.LocalDate.Format("2006-01-02"); got != "2028-02-29" {
		t.Fatalf("one month before = %s, want 2028-02-29", got)
	}

	after, err := calendar.Due(expiry, 1, "months", "after", "09:00", loc)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.LocalDate.Format("2006-01-02"); got != "2028-04-30" {
		t.Fatalf("one month after = %s, want 2028-04-30", got)
	}
}

func TestDueHandlesDSTGap(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// 2026-03-08 02:30 does not exist in New York.
	expiry := time.Date(2026, time.March, 8, 12, 0, 0, 0, time.UTC)
	schedule, err := calendar.Due(expiry, 0, "days", "before", "02:30", loc)
	if err != nil {
		t.Fatal(err)
	}
	local := schedule.DueAt.In(loc)
	if local.Hour() != 3 || local.Minute() != 0 {
		t.Fatalf("gap resolved to %s, want 03:00", local.Format(time.RFC3339))
	}
}

func TestParseSendTime(t *testing.T) {
	for _, value := range []string{"00:00", "09:05", "23:59"} {
		if _, _, err := calendar.ParseSendTime(value); err != nil {
			t.Errorf("ParseSendTime(%q): %v", value, err)
		}
	}
	for _, value := range []string{"9am", "24:00", "12:60", "12"} {
		if _, _, err := calendar.ParseSendTime(value); err == nil {
			t.Errorf("ParseSendTime(%q) unexpectedly succeeded", value)
		}
	}
}
