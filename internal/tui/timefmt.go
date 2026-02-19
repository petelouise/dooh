package tui

import (
	"strings"
	"time"
)

func NaturalDate(ts string, loc *time.Location, now time.Time) string {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return "-"
	}
	parsed, ok := parseTime(ts)
	if !ok {
		return ts
	}
	if loc == nil {
		loc = time.Local
	}
	t := parsed.In(loc)
	n := now.In(loc)
	tY, tM, tD := t.Date()
	nY, nM, nD := n.Date()
	if tY == nY && tM == nM && tD == nD {
		return "today"
	}
	y := n.AddDate(0, 0, -1)
	yY, yM, yD := y.Date()
	if tY == yY && tM == yM && tD == yD {
		return "yesterday"
	}
	tomorrow := n.AddDate(0, 0, 1)
	oY, oM, oD := tomorrow.Date()
	if tY == oY && tM == oM && tD == oD {
		return "tomorrow"
	}
	nt := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	tt := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	diffDays := int(tt.Sub(nt).Hours() / 24)
	if diffDays >= -6 && diffDays <= 6 {
		return strings.ToLower(t.Weekday().String())
	}
	return t.Format("02 Jan 2006")
}

func parseTime(v string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t, true
	}
	layouts := []string{
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
