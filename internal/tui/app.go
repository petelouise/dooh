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

type progressRow struct {
	ID    string
	Name  string
	Total int
	Done  int
	Open  int
}

type palette struct {
	Accent  int
	Open    int
	Done    int
	Archive int
	Muted   int
	Warn    int
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
	view           string

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
		statusFilter:   "open",
		priorityFilter: "all",
		limit:          limit,
		loc:            loc,
		plain:          plain,
		view:           "tasks",
	}
}

func (m *model) handleKey(key string) bool {
	if m.editFilter {
		switch key {
		case "enter":
			m.editFilter = false
		case "esc":
			m.editFilter = false
			m.filterDraft = m.filter
		case "backspace":
			if len(m.filterDraft) > 0 {
				m.filterDraft = m.filterDraft[:len(m.filterDraft)-1]
				m.filter = strings.TrimSpace(m.filterDraft)
				m.selected = 0
			}
		default:
			if len(key) == 1 {
				m.filterDraft += key
				m.filter = strings.TrimSpace(m.filterDraft)
				m.selected = 0
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
		if m.view == "tasks" || m.view == "today" {
			m.toggleExpand()
		}
	case "left":
		m.expandedID = ""
	case "/":
		m.editFilter = true
		m.filterDraft = m.filter
	case "s":
		m.statusFilter = cycle([]string{"open", "all", "completed", "archived"}, m.statusFilter)
		m.selected = 0
		m.expandedID = ""
	case "p":
		m.priorityFilter = cycle([]string{"all", "now", "soon", "later"}, m.priorityFilter)
		m.selected = 0
		m.expandedID = ""
	case "c":
		m.filter = ""
		m.filterDraft = ""
		m.statusFilter = "open"
		m.priorityFilter = "all"
		m.selected = 0
		m.expandedID = ""
	case "t":
		if len(m.themes) > 0 {
			m.themeIndex = (m.themeIndex + 1) % len(m.themes)
		}
	case "1":
		m.switchView("tasks")
	case "2":
		m.switchView("projects")
	case "3":
		m.switchView("goals")
	case "4":
		m.switchView("today")
	}
	return false
}

func (m *model) switchView(v string) {
	m.view = v
	m.selected = 0
	m.expandedID = ""
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
	p := paletteForTheme(m.themes[m.themeIndex].ID)
	body, countLine, selectedLine, err := m.renderBodyByView(cols, lines, p)
	if err != nil {
		return "", err
	}

	headerLines := 6
	footerLines := 2
	bodyBudget := lines - headerLines - footerLines
	if bodyBudget < 4 {
		bodyBudget = 4
	}
	if bodyBudget > m.limit {
		bodyBudget = m.limit
	}

	frame := make([]string, 0, lines)
	titleLine := fmt.Sprintf("dooh interactive  theme=%s  view=%s  filter=/%s  status=%s  priority=%s", m.themes[m.themeIndex].Name, m.view, m.activeFilter(), m.statusFilter, m.priorityFilter)
	frame = append(frame, m.paintAccent(clampLine(titleLine, cols), p.Accent))
	frame = append(frame, m.paintMuted(strings.Repeat("-", cols), p))
	frame = append(frame, clampLine(renderTabs(cols, m.view), cols))
	frame = append(frame, clampLine(countLine, cols))
	frame = append(frame, m.paintMuted(strings.Repeat("-", cols), p))
	frame = append(frame, m.renderHeader(cols, p))

	frame = append(frame, body...)
	for len(frame) < headerLines+bodyBudget {
		frame = append(frame, "")
	}

	frame = append(frame, m.paintMuted(strings.Repeat("-", cols), p))
	frame = append(frame, clampLine(selectedLine, cols))
	if m.editFilter {
		frame = append(frame, clampLine("filter> "+m.filterDraft+" (live fuzzy; Enter close, Esc close)", cols))
	} else {
		frame = append(frame, clampLine("keys: arrows, / live filter, s status, p priority, c clear(all), t theme, 1 tasks, 2 projects, 3 goals, 4 today, q quit", cols))
	}

	if len(frame) > lines {
		frame = frame[:lines]
	}
	for len(frame) < lines {
		frame = append(frame, "")
	}
	return joinFrame(frame), nil
}

func (m *model) renderHeader(cols int, p palette) string {
	if m.view == "projects" || m.view == "goals" {
		nameW := cols - (2 + 2 + 18 + 5 + 4 + 4)
		if nameW < 18 {
			nameW = 18
		}
		h := fmt.Sprintf("%-2s %-*s  %-18s  %-5s  %-4s  %-4s", "", nameW, "Name", "Progress", "%", "Done", "Open")
		return m.paintMuted(clampLine(h, cols), p)
	}
	iconW := 1
	priorityW := 8
	updatedW := 17
	idW := 8
	separatorW := 4
	titleW := cols - (2 + separatorW + iconW + separatorW + separatorW + priorityW + separatorW + updatedW + separatorW + idW)
	if titleW < 16 {
		titleW = 16
	}
	h := fmt.Sprintf("%-2s %-*s  %-1s  %-*s  %-*s  %-*s", "", titleW, "Title", " ", priorityW, "Priority", updatedW, "Updated", idW, "ID")
	return m.paintMuted(clampLine(h, cols), p)
}

func (m *model) renderBodyByView(cols, lines int, p palette) ([]string, string, string, error) {
	now := time.Now()
	headerLines := 6
	footerLines := 2
	bodyBudget := lines - headerLines - footerLines
	if bodyBudget < 4 {
		bodyBudget = 4
	}
	if bodyBudget > m.limit {
		bodyBudget = m.limit
	}
	switch m.view {
	case "projects":
		rows, err := m.loadProgressRows("project")
		if err != nil {
			return nil, "", "", err
		}
		rows = m.applyProgressFilter(rows)
		if len(rows) == 0 {
			m.selected = 0
			return []string{"(no project rows)"}, "projects", "selected: none", nil
		}
		m.selected = clampIndex(m.selected, len(rows))
		linesOut := m.composeProgressBody(rows, bodyBudget, cols, p)
		r := rows[m.selected]
		selected := fmt.Sprintf("selected project: %s | completion=%d%% (%d/%d)", r.Name, pct(r.Done, r.Total), r.Done, r.Total)
		return linesOut, fmt.Sprintf("projects=%d", len(rows)), selected, nil
	case "goals":
		rows, err := m.loadProgressRows("goal")
		if err != nil {
			return nil, "", "", err
		}
		rows = m.applyProgressFilter(rows)
		if len(rows) == 0 {
			m.selected = 0
			return []string{"(no goal rows)"}, "goals", "selected: none", nil
		}
		m.selected = clampIndex(m.selected, len(rows))
		linesOut := m.composeProgressBody(rows, bodyBudget, cols, p)
		r := rows[m.selected]
		selected := fmt.Sprintf("selected goal: %s | completion=%d%% (%d/%d)", r.Name, pct(r.Done, r.Total), r.Done, r.Total)
		return linesOut, fmt.Sprintf("goals=%d", len(rows)), selected, nil
	case "today":
		rows, err := m.filteredRows()
		if err != nil {
			return nil, "", "", err
		}
		todayRows := make([]row, 0, len(rows))
		for _, r := range rows {
			if isTodayScheduled(r.Scheduled, m.loc, now) {
				todayRows = append(todayRows, r)
			}
		}
		if len(todayRows) == 0 {
			m.selected = 0
			m.expandedID = ""
			return []string{"(no tasks scheduled today)"}, "today: scheduled=0", "selected: none", nil
		}
		m.selected = clampIndex(m.selected, len(todayRows))
		linesOut := m.composeTaskBody(todayRows, bodyBudget, cols, now, p)
		r := todayRows[m.selected]
		selected := fmt.Sprintf("selected: %s | due=%s | scheduled=%s | collections=%s", r.Title, NaturalDate(r.DueAt, m.loc, now), NaturalDate(r.Scheduled, m.loc, now), r.Collection)
		return linesOut, fmt.Sprintf("today: scheduled=%d", len(todayRows)), selected, nil
	default:
		rows, err := m.filteredRows()
		if err != nil {
			return nil, "", "", err
		}
		if len(rows) == 0 {
			m.selected = 0
			m.expandedID = ""
			return []string{"(no tasks)"}, "open 0  completed 0  archived 0", "selected: none", nil
		}
		m.selected = clampIndex(m.selected, len(rows))
		counts := countStatus(rows)
		linesOut := m.composeTaskBody(rows, bodyBudget, cols, now, p)
		r := rows[m.selected]
		selected := fmt.Sprintf("selected: %s | due=%s | scheduled=%s | collections=%s", r.Title, NaturalDate(r.DueAt, m.loc, now), NaturalDate(r.Scheduled, m.loc, now), r.Collection)
		return linesOut, fmt.Sprintf("open %d  completed %d  archived %d", counts["open"], counts["completed"], counts["archived"]), selected, nil
	}
}

func (m *model) composeTaskBody(rows []row, budget int, cols int, now time.Time, p palette) []string {
	iconW := 1
	priorityW := 8
	updatedW := 17
	idW := 8
	separatorW := 4
	titleW := cols - (2 + separatorW + iconW + separatorW + separatorW + priorityW + separatorW + updatedW + separatorW + idW)
	if titleW < 16 {
		titleW = 16
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
		icon := statusIcon(r.Status)
		rowLine := fmt.Sprintf("%-2s %-*s  %-1s  %-*s  %-*s  %-*s",
			mark,
			titleW, clampLine(r.Title, titleW),
			icon,
			priorityW, r.Priority,
			updatedW, NaturalDate(r.UpdatedAt, m.loc, now),
			idW, r.ID,
		)
		line := clampLine(rowLine, cols)
		line = m.paintStatusMarker(line, icon, r.Status, p)
		if i == m.selected {
			line = m.paintSelected(line, p)
		}
		lines = append(lines, line)

		if m.expandedID == r.ID && len(lines) < budget && (m.view == "tasks" || m.view == "today") {
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

func (m *model) composeProgressBody(rows []progressRow, budget int, cols int, p palette) []string {
	nameW := cols - (2 + 2 + 18 + 5 + 4 + 4)
	if nameW < 18 {
		nameW = 18
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
		bar := progressBar(r.Done, r.Total, 18)
		bar = m.paintAccent(bar, p.Accent)
		line := fmt.Sprintf("%-2s %-*s  %-18s  %3d%%  %4d  %4d", mark, nameW, clampLine(r.Name, nameW), bar, pct(r.Done, r.Total), r.Done, r.Open)
		line = clampLine(line, cols)
		if i == m.selected {
			line = m.paintSelected(line, p)
		}
		lines = append(lines, line)
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

func (m *model) loadProgressRows(kind string) ([]progressRow, error) {
	rows, err := m.sqlite.QueryTSV(`
SELECT
  c.short_id,
  c.name,
  COALESCE(SUM(CASE WHEN t.id IS NOT NULL THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN t.status IN ('completed','archived') THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN t.status='open' THEN 1 ELSE 0 END), 0)
FROM collections c
LEFT JOIN task_collections tc ON tc.collection_id=c.id
LEFT JOIN tasks t ON t.id=tc.task_id AND t.deleted_at IS NULL
WHERE c.deleted_at IS NULL AND c.kind=` + db.Quote(kind) + `
GROUP BY c.id
ORDER BY c.updated_at DESC, c.name ASC;`)
	if err != nil {
		return nil, err
	}
	out := make([]progressRow, 0, len(rows))
	for _, r := range rows {
		if len(r) < 5 {
			continue
		}
		total, _ := strconv.Atoi(r[2])
		done, _ := strconv.Atoi(r[3])
		open, _ := strconv.Atoi(r[4])
		out = append(out, progressRow{ID: r[0], Name: r[1], Total: total, Done: done, Open: open})
	}
	return out, nil
}

func (m *model) activeFilter() string {
	if m.editFilter {
		return strings.TrimSpace(m.filterDraft)
	}
	return strings.TrimSpace(m.filter)
}

func (m *model) applyFilters(in []row) []row {
	f := strings.ToLower(m.activeFilter())
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
			if !fuzzyMatch(h, f) {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

func (m *model) applyProgressFilter(in []progressRow) []progressRow {
	f := strings.ToLower(m.activeFilter())
	if f == "" {
		return in
	}
	out := make([]progressRow, 0, len(in))
	for _, r := range in {
		if fuzzyMatch(strings.ToLower(strings.Join([]string{r.Name, r.ID}, " ")), f) {
			out = append(out, r)
		}
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

func statusIcon(status string) string {
	switch status {
	case "completed":
		return "✓"
	case "archived":
		return "✕"
	default:
		return "○"
	}
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

func renderTabs(cols int, active string) string {
	t := []string{"[1 tasks]", "[2 projects]", "[3 goals]", "[4 today]"}
	s := strings.Join(t, "  ") + "  (active: " + active + ")"
	return clampLine(s, cols)
}

func progressBar(done, total, width int) string {
	if width < 4 {
		width = 4
	}
	filled := 0
	if total > 0 {
		filled = (done * width) / total
	}
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}

func pct(done, total int) int {
	if total <= 0 {
		return 0
	}
	v := (done * 100) / total
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func isTodayScheduled(s string, loc *time.Location, now time.Time) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	t, ok := parseTime(s)
	if !ok {
		return false
	}
	if loc == nil {
		loc = time.Local
	}
	st := t.In(loc)
	n := now.In(loc)
	sy, sm, sd := st.Date()
	ny, nm, nd := n.Date()
	return sy == ny && sm == nm && sd == nd
}

func fuzzyMatch(hay, needle string) bool {
	hay = strings.TrimSpace(strings.ToLower(hay))
	needle = strings.TrimSpace(strings.ToLower(needle))
	if needle == "" {
		return true
	}
	h := []rune(hay)
	n := []rune(needle)
	j := 0
	for i := 0; i < len(h) && j < len(n); i++ {
		if h[i] == n[j] {
			j++
		}
	}
	return j == len(n)
}

func paletteForTheme(id string) palette {
	switch id {
	case "sunset-pop":
		return palette{Accent: 209, Open: 214, Done: 120, Archive: 208, Muted: 246, Warn: 220}
	case "mint-circuit":
		return palette{Accent: 79, Open: 50, Done: 84, Archive: 117, Muted: 245, Warn: 159}
	case "paper-fruit":
		return palette{Accent: 167, Open: 174, Done: 107, Archive: 131, Muted: 246, Warn: 180}
	case "midnight-arcade":
		return palette{Accent: 45, Open: 81, Done: 119, Archive: 39, Muted: 110, Warn: 228}
	default:
		return palette{Accent: 81, Open: 39, Done: 120, Archive: 208, Muted: 245, Warn: 220}
	}
}

func (m *model) colorize(s string, code int) string {
	if m.plain {
		return s
	}
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", code, s)
}

func (m *model) paintAccent(s string, code int) string {
	return m.colorize(s, code)
}

func (m *model) paintMuted(s string, p palette) string {
	return m.colorize(s, p.Muted)
}

func (m *model) paintStatus(s string, status string, p palette) string {
	switch status {
	case "completed":
		return m.colorize(s, p.Done)
	case "archived":
		return m.colorize(s, p.Archive)
	default:
		return m.colorize(s, p.Open)
	}
}

func (m *model) paintStatusMarker(line string, marker string, status string, p palette) string {
	if m.plain {
		return line
	}
	return strings.Replace(line, marker, m.paintStatus(marker, status, p), 1)
}

func (m *model) paintSelected(s string, p palette) string {
	if m.plain {
		return s
	}
	return strings.Replace(s, ">", fmt.Sprintf("\x1b[38;5;%dm>\x1b[0m", p.Warn), 1)
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
