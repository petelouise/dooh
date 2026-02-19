package tui

import (
	"testing"
	"time"
)

func TestNaturalDateTodayYesterdayAbsolute(t *testing.T) {
	loc := time.FixedZone("Test", -8*3600)
	now := time.Date(2026, 2, 19, 13, 10, 0, 0, loc)

	todayUTC := time.Date(2026, 2, 19, 20, 35, 0, 0, time.UTC).Format(time.RFC3339)
	got := NaturalDate(todayUTC, loc, now)
	if got != "today 12:35" {
		t.Fatalf("today mismatch: %s", got)
	}

	yesterdayUTC := time.Date(2026, 2, 19, 6, 0, 0, 0, time.UTC).Format(time.RFC3339)
	got = NaturalDate(yesterdayUTC, loc, now)
	if got != "yesterday 22:00" {
		t.Fatalf("yesterday mismatch: %s", got)
	}

	absUTC := time.Date(2026, 2, 3, 13, 21, 0, 0, time.UTC).Format(time.RFC3339)
	got = NaturalDate(absUTC, loc, now)
	if got != "03 Feb 2026 05:21" {
		t.Fatalf("absolute mismatch: %s", got)
	}
}
