package tui

import (
	"errors"
	"io"
	"time"

	"dooh/internal/db"
)

var errTeaUnavailable = errors.New("tea renderer unavailable in this build; use --renderer legacy")

// RunInteractiveTea is the phase-1 task-view entrypoint.
// In this offline build it delegates to legacy rendering so UX stays functional.
func RunInteractiveTea(in io.Reader, out io.Writer, sqlite db.SQLite, catalog ThemeCatalog, themeID string, filter string, limit int, loc *time.Location, identity Identity, plain bool) error {
	// Placeholder phase while Bubble Tea deps are unavailable in this environment.
	return RunInteractive(in, out, sqlite, catalog, themeID, filter, limit, loc, identity, plain)
}
