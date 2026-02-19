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
	theme          Theme
	filter         string
	statusFilter   string
	priorityFilter string
	selected       int
	limit          int
	loc            *time.Location
	editFilter     bool
	filterDraft    string
}

func RenderDashboard(sqlite db.SQLite, theme Theme, filter string, limit int, loc *time.Location) (string, error) {
	m := model{sqlite: sqlite, theme: theme, filter: strings.TrimSpace(filter), statusFilter: "all", priorityFilter: "all", limit: limit, loc: loc}
	if m.limit <= 0 {
		m.limit = 12
	}
	return m.render(120, 40)
}

func RunInteractive(in io.Reader, out io.Writer, sqlite db.SQLite, catalog ThemeCatalog, themeID string, filter string, limit int, loc *time.Location) error {
	th := catalog.Themes[0]
	for _, t := range catalog.Themes {
		if t.ID == themeID {
			th = t
			break
		}
	}
	m := model{sqlite: sqlite, theme: th, filter: strings.TrimSpace(filter), statusFilter: "all", priorityFilter: "all", limit: limit, loc: loc}
	if m.limit <= 0 {
		m.limit = 12
	}

	restoreTTY, _ := setRawTTY()
	defer restoreTTY()
	_, _ = fmt.Fprint(out, "\x1b[?1049h")
	defer func() { _, _ = fmt.Fprint(out, "\x1b[?1049l") }()

	r := bufio.NewReader(in)
	for {
		cols, rows := terminalSize()
		ui, err := m.render(cols, rows)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(out, "\x1b[2J\x1b[H")
		_, _ = fmt.Fprint(out, ui)

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

func (m *model) render(cols int, rows int) (string, error) {
	if cols < 80 {
		cols = 80
	}
	if rows < 20 {
		rows = 20
	}

	all, err := m.loadRows()
	if err != nil {
		return "", err
	}
	filtered := m.applyFilters(all)
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(filtered) && len(filtered) > 0 {
		m.selected = len(filtered) - 1
	}
	if len(filtered) == 0 {
		m.selected = 0
	}

	now := time.Now()
	counts := countStatus(all)
	header := fmt.Sprintf("dooh tui  theme=%s  filter=/%s  status=%s  priority=%s", m.theme.Name, m.filter, m.statusFilter, m.priorityFilter)

	var b strings.Builder
	b.WriteString(trimTo(header, cols) + "\n")
	b.WriteString(strings.Repeat("-", cols) + "\n")
	b.WriteString(fmt.Sprintf("open=%d  completed=%d  archived=%d\n", counts["open"], counts["completed"], counts["archived"]))
	b.WriteString(strings.Repeat("-", cols) + "\n")

	titleW := cols - 52
	if titleW < 20 {
		titleW = 20
	}
	b.WriteString(fmt.Sprintf("%-2s %-*s %-10s %-8s %-19s %-8s\n", "", titleW, "Title", "Status", "Priority", "Updated", "ID"))
	b.WriteString(strings.Repeat("-", cols) + "\n")

	visibleRows := rows - 14
	if visibleRows < 5 {
		visibleRows = 5
	}
	start := 0
	if m.selected >= visibleRows {
		start = m.selected - visibleRows + 1
	}
	end := start + visibleRows
	if end > len(filtered) {
		end = len(filtered)
	}

	for i := start; i < end; i++ {
		r := filtered[i]
		mark := " "
		if i == m.selected {
			mark = ">"
		}
		line := fmt.Sprintf("%-2s %-*s %-10s %-8s %-19s %-8s", mark, titleW, trimTo(r.Title, titleW), r.Status, r.Priority, NaturalDate(r.UpdatedAt, m.loc, now), r.ID)
		b.WriteString(trimTo(line, cols) + "\n")
	}
	for i := end; i < start+visibleRows; i++ {
		b.WriteString("\n")
	}

	b.WriteString(strings.Repeat("-", cols) + "\n")
	if len(filtered) > 0 {
		r := filtered[m.selected]
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
		b.WriteString(trimTo("filter> "+m.filterDraft+"  (Enter apply, Esc cancel)", cols) + "\n")
	} else {
		b.WriteString(trimTo("keys: up/down or j/k, / filter, s status, p priority, c clear, q quit", cols) + "\n")
	}
	return b.String(), nil
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
	case "s":
		m.statusFilter = cycle([]string{"all", "open", "completed", "archived"}, m.statusFilter)
		m.selected = 0
	case "p":
		m.priorityFilter = cycle([]string{"all", "now", "soon", "later"}, m.priorityFilter)
		m.selected = 0
	case "/":
		m.editFilter = true
		m.filterDraft = m.filter
	case "c":
		m.filter = ""
		m.selected = 0
	}
	return false
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

func (m *model) applyFilters(rows []row) []row {
	f := strings.ToLower(strings.TrimSpace(m.filter))
	out := make([]row, 0, len(rows))
	for _, r := range rows {
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

func countStatus(rows []row) map[string]int {
	m := map[string]int{"open": 0, "completed": 0, "archived": 0}
	for _, r := range rows {
		m[r.Status]++
	}
	return m
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
	rows := 40
	if c := strings.TrimSpace(os.Getenv("COLUMNS")); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n > 0 {
			cols = n
		}
	}
	if r := strings.TrimSpace(os.Getenv("LINES")); r != "" {
		if n, err := strconv.Atoi(r); err == nil && n > 0 {
			rows = n
		}
	}
	out, err := exec.Command("sh", "-c", "stty size </dev/tty").Output()
	if err == nil {
		parts := strings.Fields(strings.TrimSpace(string(out)))
		if len(parts) == 2 {
			if n, e := strconv.Atoi(parts[0]); e == nil && n > 0 {
				rows = n
			}
			if n, e := strconv.Atoi(parts[1]); e == nil && n > 0 {
				cols = n
			}
		}
	}
	return cols, rows
}
