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
	plain          bool

	editFilter  bool
	filterDraft string
	expandedID  string
}

func RunInteractive(in io.Reader, out io.Writer, sqlite db.SQLite, catalog ThemeCatalog, themeID string, filter string, limit int, loc *time.Location, plain bool) error {
	m := newModel(sqlite, catalog, themeID, filter, limit, loc, plain)

	cols, _ := terminalSize()
	if cols < 72 || !isTTY() {
		m.plain = true
	}

	restoreTTY, err := setCbreakTTY()
	if err != nil {
		m.plain = true
	}
	if restoreTTY != nil {
		defer restoreTTY()
	}

	if m.plain {
		frame, err := m.render(cols, 24)
		if err != nil {
			return err
		}
		_, _ = io.WriteString(out, frame)
		return nil
	}

	_, _ = io.WriteString(out, "\x1b[?1049h\x1b[?25l")
	defer func() {
		_, _ = io.WriteString(out, "\x1b[?25h\x1b[?1049l")
	}()

	r := bufio.NewReader(in)
	for {
		c, l := terminalSize()
		frame, err := m.render(c, l)
		if err != nil {
			return err
		}
		_, _ = io.WriteString(out, "\x1b[H\x1b[2J")
		_, _ = io.WriteString(out, frame)

		key, err := readKey(r)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if m.handleKey(key) {
			_, _ = io.WriteString(out, "\x1b[H\x1b[2J")
			return nil
		}
	}
}

func RenderDashboard(sqlite db.SQLite, theme Theme, filter string, limit int, loc *time.Location, plain bool) (string, error) {
	catalog := ThemeCatalog{Themes: []Theme{theme}, Default: theme.ID}
	m := newModel(sqlite, catalog, theme.ID, filter, limit, loc, plain)
	cols, lines := terminalSize()
	return m.render(cols, lines)
}

func newModel(sqlite db.SQLite, catalog ThemeCatalog, themeID string, filter string, limit int, loc *time.Location, plain bool) model {
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
		plain:          plain,
	}
}

func (m *model) handleKey(key string) bool {
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
	case "up":
		m.selected--
	case "down":
		m.selected++
	case "right":
		m.toggleExpand()
	case "left":
		m.expandedID = ""
	case "/":
		m.editFilter = true
		m.filterDraft = m.filter
	case "s":
		m.statusFilter = cycle([]string{"all", "open", "completed", "archived"}, m.statusFilter)
		m.selected = 0
		m.expandedID = ""
	case "p":
		m.priorityFilter = cycle([]string{"all", "now", "soon", "later"}, m.priorityFilter)
		m.selected = 0
		m.expandedID = ""
	case "c":
		m.filter = ""
		m.selected = 0
		m.expandedID = ""
	case "t":
		if len(m.themes) > 0 {
			m.themeIndex = (m.themeIndex + 1) % len(m.themes)
		}
	}
	return false
}

func (m *model) toggleExpand() {
	rows, err := m.filteredRows()
	if err != nil || len(rows) == 0 {
		m.expandedID = ""
		return
	}
	m.selected = clampIndex(m.selected, len(rows))
	id := rows[m.selected].ID
	if m.expandedID == id {
		m.expandedID = ""
		return
	}
	m.expandedID = id
}

func (m *model) filteredRows() ([]row, error) {
	rows, err := m.loadRows()
	if err != nil {
		return nil, err
	}
	return m.applyFilters(rows), nil
}

func (m *model) render(cols int, lines int) (string, error) {
	if cols < 72 {
		cols = 72
	}
	if lines < 18 {
		lines = 18
	}

	rows, err := m.filteredRows()
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		m.selected = 0
		m.expandedID = ""
	} else {
		m.selected = clampIndex(m.selected, len(rows))
	}

	counts := countStatus(rows)
	now := time.Now()

	statusW := 9
	priorityW := 8
	updatedW := 17
	idW := 8
	separatorW := 4
	titleW := cols - (2 + separatorW + statusW + separatorW + priorityW + separatorW + updatedW + separatorW + idW)
	if titleW < 16 {
		titleW = 16
	}

	headerLines := 6
	footerLines := 3
	bodyBudget := lines - headerLines - footerLines
	if bodyBudget < 4 {
		bodyBudget = 4
	}
	if bodyBudget > m.limit {
		bodyBudget = m.limit
	}

	bodyLines := m.composeBody(rows, bodyBudget, titleW, statusW, priorityW, updatedW, idW, cols, now)

	frame := make([]string, 0, lines)
	titleLine := fmt.Sprintf("dooh interactive  theme=%s  filter=/%s  status=%s  priority=%s", m.themes[m.themeIndex].Name, m.filter, m.statusFilter, m.priorityFilter)
	frame = append(frame, m.paintHeader(clampLine(titleLine, cols)))
	frame = append(frame, strings.Repeat("-", cols))
	frame = append(frame, clampLine(fmt.Sprintf("open %d  completed %d  archived %d", counts["open"], counts["completed"], counts["archived"]), cols))
	frame = append(frame, strings.Repeat("-", cols))
	frame = append(frame, clampLine(fmt.Sprintf("%-2s %-*s  %-*s  %-*s  %-*s  %-*s", "", titleW, "Title", statusW, "Status", priorityW, "Priority", updatedW, "Updated", idW, "ID"), cols))
	frame = append(frame, strings.Repeat("-", cols))

	frame = append(frame, bodyLines...)
	for len(frame) < headerLines+bodyBudget {
		frame = append(frame, "")
	}

	frame = append(frame, strings.Repeat("-", cols))
	if len(rows) > 0 {
		r := rows[m.selected]
		footer := fmt.Sprintf("selected: %s | due=%s | scheduled=%s | collections=%s", r.Title, NaturalDate(r.DueAt, m.loc, now), NaturalDate(r.Scheduled, m.loc, now), r.Collection)
		frame = append(frame, clampLine(footer, cols))
	} else {
		frame = append(frame, "selected: none")
	}
	if m.editFilter {
		frame = append(frame, clampLine("filter> "+m.filterDraft+" (Enter apply, Esc cancel)", cols))
	} else {
		frame = append(frame, clampLine("keys: arrows navigate, right expand, left collapse, / filter, s status, p priority, t theme, c clear, q quit", cols))
	}

	if len(frame) > lines {
		frame = frame[:lines]
	}
	for len(frame) < lines {
		frame = append(frame, "")
	}
	return joinFrame(frame), nil
}

func (m *model) composeBody(rows []row, budget int, titleW, statusW, priorityW, updatedW, idW, cols int, now time.Time) []string {
	if len(rows) == 0 {
		return []string{"(no tasks)"}
	}

	start := 0
	if m.selected >= budget {
		start = m.selected - budget + 1
	}
	if start < 0 {
		start = 0
	}

	lines := make([]string, 0, budget)
	for i := start; i < len(rows) && len(lines) < budget; i++ {
		r := rows[i]
		mark := " "
		if i == m.selected {
			mark = ">"
		}
		rowLine := fmt.Sprintf("%-2s %-*s  %-*s  %-*s  %-*s  %-*s",
			mark,
			titleW, clampLine(r.Title, titleW),
			statusW, r.Status,
			priorityW, r.Priority,
			updatedW, NaturalDate(r.UpdatedAt, m.loc, now),
			idW, r.ID,
		)
		line := clampLine(rowLine, cols)
		if i == m.selected {
			line = m.paintSelected(line)
		}
		lines = append(lines, line)

		if m.expandedID == r.ID && len(lines) < budget {
			detail := []string{
				"title: " + r.Title,
				"due: " + NaturalDate(r.DueAt, m.loc, now),
				"scheduled: " + NaturalDate(r.Scheduled, m.loc, now),
				"updated: " + NaturalDate(r.UpdatedAt, m.loc, now),
				"collections: " + strings.TrimSpace(r.Collection),
			}
			for _, d := range detail {
				for _, wrapped := range wrapText(d, cols-4) {
					if len(lines) >= budget {
						break
					}
					lines = append(lines, clampLine("    "+wrapped, cols))
				}
				if len(lines) >= budget {
					break
				}
			}
		}
	}
	return lines
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

func wrapText(s string, width int) []string {
	if width <= 4 {
		return []string{clampLine(s, width)}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	out := make([]string, 0, len(words))
	line := words[0]
	for i := 1; i < len(words); i++ {
		w := words[i]
		if len(line)+1+len(w) <= width {
			line += " " + w
			continue
		}
		out = append(out, line)
		line = w
	}
	out = append(out, line)
	return out
}

func clampLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

func joinFrame(lines []string) string {
	if len(lines) == 0 {
		return "\r\n"
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

func clampIndex(v int, count int) int {
	if count <= 0 {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v >= count {
		return count - 1
	}
	return v
}

func (m *model) paintHeader(s string) string {
	if m.plain || strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return s
	}
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", colorForTheme(m.themes[m.themeIndex].ID), s)
}

func (m *model) paintSelected(s string) string {
	if m.plain || strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return s
	}
	return strings.Replace(s, ">", "\x1b[38;5;220m>\x1b[0m", 1)
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

func isTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func setCbreakTTY() (func(), error) {
	if !isTTY() {
		return func() {}, nil
	}
	stateOut, err := exec.Command("sh", "-c", "stty -g </dev/tty").Output()
	if err != nil {
		return nil, err
	}
	state := strings.TrimSpace(string(stateOut))
	if err := exec.Command("sh", "-c", "stty -icanon -echo min 1 time 0 </dev/tty").Run(); err != nil {
		return nil, err
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
