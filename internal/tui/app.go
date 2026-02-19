package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"dooh/internal/db"
)

type row struct {
	ID         string
	Title      string
	Status     string
	Priority   string
	DueAt      string
	Scheduled  string
	UpdatedAt  string
	Collection string
}

type model struct {
	sqlite         db.SQLite
	themes         []Theme
	themeIndex     int
	filter         string
	statusFilter   string
	priorityFilter string
	selected       int
	limit          int
	loc            *time.Location
	editFilter     bool
	filterDraft    string
}

func RunInteractive(in io.Reader, out io.Writer, sqlite db.SQLite, catalog ThemeCatalog, themeID string, filter string, limit int, loc *time.Location) error {
	m := newModel(sqlite, catalog, themeID, filter, limit, loc)

	restoreTTY, _ := setRawTTY()
	defer restoreTTY()
	_, _ = fmt.Fprint(out, "\x1b[?1049h\x1b[?25l")
	defer func() { _, _ = fmt.Fprint(out, "\x1b[?25h\x1b[?1049l") }()

	r := bufio.NewReader(in)
	for {
		cols, rows := terminalSize()
		rendered, err := m.render(cols, rows)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(out, "\x1b[2J\x1b[H")
		_, _ = fmt.Fprint(out, rendered)

		key, err := readKey(r)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if m.handle(key) {
			_, _ = fmt.Fprint(out, "\x1b[2J\x1b[H")
			return nil
		}
	}
}

func RenderDashboard(sqlite db.SQLite, theme Theme, filter string, limit int, loc *time.Location) (string, error) {
	catalog := ThemeCatalog{Themes: []Theme{theme}, Default: theme.ID}
	m := newModel(sqlite, catalog, theme.ID, filter, limit, loc)
	return m.render(120, 36)
}

func newModel(sqlite db.SQLite, catalog ThemeCatalog, themeID string, filter string, limit int, loc *time.Location) model {
	if limit <= 0 {
		limit = 14
	}
	idx := 0
	for i, t := range catalog.Themes {
		if t.ID == themeID {
			idx = i
			break
		}
	}
	if loc == nil {
		loc = time.Local
	}
	return model{
		sqlite:         sqlite,
		themes:         catalog.Themes,
		themeIndex:     idx,
		filter:         strings.TrimSpace(filter),
		statusFilter:   "all",
		priorityFilter: "all",
		limit:          limit,
		loc:            loc,
	}
}

func (m *model) handle(key string) bool {
	if m.editFilter {
		switch key {
		case "enter":
			m.filter = strings.TrimSpace(m.filterDraft)
			m.editFilter = false
			m.selected = 0
		case "esc":
			m.editFilter = false
		case "backspace":
			if len(m.filterDraft) > 0 {
				m.filterDraft = m.filterDraft[:len(m.filterDraft)-1]
			}
		default:
			if len(key) == 1 {
				m.filterDraft += key
			}
		}
		return false
	}

	switch key {
	case "q":
		return true
	case "down", "j":
		m.selected++
	case "up", "k":
		m.selected--
	case "/":
		m.editFilter = true
		m.filterDraft = m.filter
	case "c":
		m.filter = ""
		m.selected = 0
	case "s":
		m.statusFilter = cycle([]string{"all", "open", "completed", "archived"}, m.statusFilter)
		m.selected = 0
	case "p":
		m.priorityFilter = cycle([]string{"all", "now", "soon", "later"}, m.priorityFilter)
		m.selected = 0
	case "t", "right", "left":
		if len(m.themes) > 0 {
			delta := 1
			if key == "left" {
				delta = -1
			}
			m.themeIndex = (m.themeIndex + len(m.themes) + delta) % len(m.themes)
		}
	}
	return false
}

func (m *model) render(cols int, lines int) (string, error) {
	if cols < 72 {
		cols = 72
	}
	if lines < 18 {
		lines = 18
	}
	rows, err := m.loadRows()
	if err != nil {
		return "", err
	}
	rows = m.applyFilters(rows)
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(rows) && len(rows) > 0 {
		m.selected = len(rows) - 1
	}
	if len(rows) == 0 {
		m.selected = 0
	}

	t := m.themes[m.themeIndex]
	now := time.Now()
	counts := countStatus(rows)

	statusW := 10
	priorityW := 8
	updatedW := 17
	idW := 8
	// Keep generous safety margin to prevent wrapping in terminals with
	// ambiguous width handling.
	titleW := cols - 60
	if titleW < 16 {
		titleW = 16
	}

	visible := lines - 8
	if visible < 6 {
		visible = 6
	}
	if m.limit < visible {
		visible = m.limit
	}

	start := 0
	if m.selected >= visible {
		start = m.selected - visible + 1
	}
	end := start + visible
	if end > len(rows) {
		end = len(rows)
	}

	var b strings.Builder
	titleColor := colorForTheme(t.ID)
	b.WriteString(color(titleColor, trimTo(fmt.Sprintf("dooh tui | theme=%s | filter=/%s | status=%s | priority=%s", t.Name, m.filter, m.statusFilter, m.priorityFilter), cols)) + "\n")
	b.WriteString(strings.Repeat("-", cols) + "\n")
	b.WriteString(trimTo(fmt.Sprintf("open=%d  completed=%d  archived=%d", counts["open"], counts["completed"], counts["archived"]), cols) + "\n")
	b.WriteString(strings.Repeat("-", cols) + "\n")
	b.WriteString(fmt.Sprintf("%-2s %-*s %-*s %-*s %-*s %-*s\n", "", titleW, "Title", statusW, "Status", priorityW, "Priority", updatedW, "Updated", idW, "ID"))
	b.WriteString(strings.Repeat("-", cols) + "\n")

	for i := start; i < end; i++ {
		r := rows[i]
		mark := " "
		if i == m.selected {
			mark = ">"
		}
		line := fmt.Sprintf("%-2s %-*s %-*s %-*s %-*s %-*s",
			mark,
			titleW, trimTo(r.Title, titleW),
			statusW, r.Status,
			priorityW, r.Priority,
			updatedW, NaturalDate(r.UpdatedAt, m.loc, now),
			idW, r.ID,
		)
		if i == m.selected {
			b.WriteString(strings.Replace(line, ">", color(220, ">"), 1) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
	for i := end; i < start+visible; i++ {
		b.WriteString("\n")
	}

	b.WriteString(strings.Repeat("-", cols) + "\n")
	if len(rows) > 0 {
		r := rows[m.selected]
		detail := fmt.Sprintf("selected: %s | due=%s | scheduled=%s | collections=%s",
			r.Title,
			NaturalDate(r.DueAt, m.loc, now),
			NaturalDate(r.Scheduled, m.loc, now),
			r.Collection,
		)
		b.WriteString(trimTo(detail, cols) + "\n")
	} else {
		b.WriteString("selected: none\n")
	}
	if m.editFilter {
		b.WriteString(trimTo("filter> "+m.filterDraft+" (Enter apply, Esc cancel)", cols) + "\n")
	} else {
		b.WriteString(trimTo("keys: up/down or j/k, / filter, s status, p priority, t theme, c clear, q quit", cols) + "\n")
	}
	return b.String(), nil
}

func (m *model) loadRows() ([]row, error) {
	rows, err := m.sqlite.QueryTSV(`
SELECT
  t.short_id,
  t.title,
  t.status,
  t.priority,
  COALESCE(t.due_at,''),
  COALESCE(t.scheduled_at,''),
  COALESCE(t.updated_at,''),
  COALESCE(group_concat(c.name, ', '), '')
FROM tasks t
LEFT JOIN task_collections tc ON tc.task_id=t.id
LEFT JOIN collections c ON c.id=tc.collection_id
WHERE t.deleted_at IS NULL
GROUP BY t.id
ORDER BY t.updated_at DESC;`)
	if err != nil {
		return nil, err
	}
	out := make([]row, 0, len(rows))
	for _, r := range rows {
		if len(r) < 8 {
			continue
		}
		out = append(out, row{ID: r[0], Title: r[1], Status: r[2], Priority: r[3], DueAt: r[4], Scheduled: r[5], UpdatedAt: r[6], Collection: r[7]})
	}
	return out, nil
}

func (m *model) applyFilters(in []row) []row {
	f := strings.ToLower(strings.TrimSpace(m.filter))
	out := make([]row, 0, len(in))
	for _, r := range in {
		if m.statusFilter != "all" && r.Status != m.statusFilter {
			continue
		}
		if m.priorityFilter != "all" && r.Priority != m.priorityFilter {
			continue
		}
		if f != "" {
			h := strings.ToLower(strings.Join([]string{r.Title, r.ID, r.Status, r.Priority, r.Collection}, " "))
			if !strings.Contains(h, f) {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

func countStatus(rows []row) map[string]int {
	m := map[string]int{"open": 0, "completed": 0, "archived": 0}
	for _, r := range rows {
		m[r.Status]++
	}
	return m
}

func cycle(values []string, current string) string {
	for i, v := range values {
		if v == current {
			return values[(i+1)%len(values)]
		}
	}
	if len(values) == 0 {
		return current
	}
	return values[0]
}

func trimTo(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func color(code int, s string) string {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return s
	}
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", code, s)
}

func colorForTheme(id string) int {
	switch id {
	case "sunset-pop":
		return 209
	case "mint-circuit":
		return 79
	case "paper-fruit":
		return 167
	case "midnight-arcade":
		return 45
	default:
		return 81
	}
}

func readKey(r *bufio.Reader) (string, error) {
	b, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	if b == 0x1b {
		b2, err := r.ReadByte()
		if err != nil {
			return "esc", nil
		}
		if b2 != '[' {
			return "esc", nil
		}
		b3, err := r.ReadByte()
		if err != nil {
			return "esc", nil
		}
		switch b3 {
		case 'A':
			return "up", nil
		case 'B':
			return "down", nil
		case 'C':
			return "right", nil
		case 'D':
			return "left", nil
		default:
			return "", nil
		}
	}
	switch b {
	case '\r', '\n':
		return "enter", nil
	case 127, 8:
		return "backspace", nil
	}
	if b >= 32 && b <= 126 {
		return string(b), nil
	}
	return "", nil
}

func setRawTTY() (func(), error) {
	if fi, err := os.Stdin.Stat(); err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return func() {}, nil
	}
	stateOut, err := exec.Command("sh", "-c", "stty -g </dev/tty").Output()
	if err != nil {
		return func() {}, nil
	}
	state := strings.TrimSpace(string(stateOut))
	if err := exec.Command("sh", "-c", "stty raw -echo </dev/tty").Run(); err != nil {
		return func() {}, nil
	}
	return func() {
		_ = exec.Command("sh", "-c", "stty "+state+" </dev/tty").Run()
	}, nil
}

func terminalSize() (int, int) {
	cols := 120
	lines := 36
	if c := strings.TrimSpace(os.Getenv("COLUMNS")); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n > 0 {
			cols = n
		}
	}
	if l := strings.TrimSpace(os.Getenv("LINES")); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			lines = n
		}
	}
	out, err := exec.Command("sh", "-c", "stty size </dev/tty").Output()
	if err == nil {
		parts := strings.Fields(strings.TrimSpace(string(out)))
		if len(parts) == 2 {
			if n, e := strconv.Atoi(parts[0]); e == nil && n > 0 {
				lines = n
			}
			if n, e := strconv.Atoi(parts[1]); e == nil && n > 0 {
				cols = n
			}
		}
	}
	return cols, lines
}
