package tui

import (
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"dooh/internal/db"
)

type teaProgramModel struct {
	core   model
	width  int
	height int
	err    error
}

func newTeaProgramModel(sqlite db.SQLite, catalog ThemeCatalog, themeID string, filter string, limit int, loc *time.Location, identity Identity, plain bool) teaProgramModel {
	core := newModel(sqlite, catalog, themeID, filter, limit, loc, identity, plain)
	return teaProgramModel{
		core: core,
	}
}

func (m teaProgramModel) Init() tea.Cmd {
	return nil
}

func (m teaProgramModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		key := normalizeTeaKey(msg)
		if key != "" {
			if m.core.handleKey(key) {
				return m, tea.Quit
			}
		}
		return m, nil
	}
	return m, nil
}

func (m teaProgramModel) View() string {
	if m.err != nil {
		return "tui error: " + m.err.Error() + "\n"
	}
	width := m.width
	height := m.height
	// Wait for Bubble Tea's window size so full-width backgrounds are accurate.
	if width <= 0 || height <= 0 {
		return ""
	}
	frame, err := m.core.render(width, height)
	if err != nil {
		return "tui error: " + err.Error() + "\n"
	}
	// Bubble Tea expects LF boundaries.
	return strings.ReplaceAll(frame, "\r\n", "\n")
}

// RunInteractiveTea is the default interactive TUI path.
func RunInteractiveTea(in io.Reader, out io.Writer, sqlite db.SQLite, catalog ThemeCatalog, themeID string, filter string, limit int, loc *time.Location, identity Identity, plain bool) error {
	if plain {
		return RunInteractive(in, out, sqlite, catalog, themeID, filter, limit, loc, identity, true)
	}
	if !isTTY() {
		return RunInteractive(in, out, sqlite, catalog, themeID, filter, limit, loc, identity, true)
	}
	program := tea.NewProgram(
		newTeaProgramModel(sqlite, catalog, themeID, filter, limit, loc, identity, plain),
		tea.WithAltScreen(),
		tea.WithInput(in),
		tea.WithOutput(out),
	)
	_, err := program.Run()
	return err
}

func normalizeTeaKey(msg tea.KeyMsg) string {
	switch msg.Type {
	case tea.KeyUp:
		return "up"
	case tea.KeyDown:
		return "down"
	case tea.KeyRight:
		return "right"
	case tea.KeyLeft:
		return "left"
	case tea.KeyEnter:
		return "enter"
	case tea.KeyTab:
		return "tab"
	case tea.KeyShiftTab:
		return "shift_tab"
	case tea.KeyEsc:
		return "esc"
	case tea.KeyBackspace, tea.KeyCtrlH:
		return "backspace"
	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			return string(msg.Runes[0])
		}
	}
	s := msg.String()
	if s == "shift+tab" {
		return "shift_tab"
	}
	return ""
}
