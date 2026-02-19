package tui

import (
	"testing"
	"time"
)

func TestNaturalDateTodayYesterdayAbsolute(t *testing.T) {
	loc := time.FixedZone("Test", -8*3600)
	now := time.Date(2026, 2, 19, 13, 10, 0, 0, loc)

	todayUTC := time.Date(2026, 2, 19, 20, 35, 0, 0, time.UTC).Format(time.RFC3339)
	if got := NaturalDate(todayUTC, loc, now); got != "today" {
		t.Fatalf("today mismatch: %s", got)
	}

	yesterdayUTC := time.Date(2026, 2, 19, 6, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if got := NaturalDate(yesterdayUTC, loc, now); got != "yesterday" {
		t.Fatalf("yesterday mismatch: %s", got)
	}

	tomorrowUTC := time.Date(2026, 2, 20, 18, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if got := NaturalDate(tomorrowUTC, loc, now); got != "tomorrow" {
		t.Fatalf("tomorrow mismatch: %s", got)
	}

	withinWeekUTC := time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if got := NaturalDate(withinWeekUTC, loc, now); got != "sunday" {
		t.Fatalf("weekday mismatch: %s", got)
	}

	absUTC := time.Date(2026, 2, 3, 13, 21, 0, 0, time.UTC).Format(time.RFC3339)
	if got := NaturalDate(absUTC, loc, now); got != "03 Feb 2026" {
		t.Fatalf("absolute mismatch: %s", got)
	}
}
