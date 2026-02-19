package tui

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"dooh/internal/db"
)

type row struct {
	ID          string
	Title       string
	Status      string
	Priority    string
	DueAt       string
	Scheduled   string
	UpdatedAt   string
	Collection  string
	Tags        string
	Assignees   string
	ProjectIDs  string
	GoalIDs     string
	AssigneeIDs string
	Projects    string
	Goals       string
	Areas       string
	Groups      string
}

type progressRow struct {
	ID        string
	Name      string
	ColorHex  string
	Completed int
	Remaining int
	Total     int
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
	tagFilter      string
	assigneeFilter string
	selected       int
	limit          int
	loc            *time.Location
	plain          bool
	view           string
	rng            *rand.Rand

	scopeKind  string
	scopeID    string
	scopeName  string
	scopeColor string

	editFilter   bool
	filterDraft  string
	editCommand  bool
	commandDraft string
	commandMsg   string
	expandedID   string
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
	seed := time.Now().UnixNano()
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
		rng:            rand.New(rand.NewSource(seed)),
	}
}

func (m *model) handleKey(key string) bool {
	if m.editCommand {
		switch key {
		case "enter":
			m.executeCommand(strings.TrimSpace(m.commandDraft))
			m.editCommand = false
		case "esc":
			m.editCommand = false
		case "backspace":
			if len(m.commandDraft) > 0 {
				m.commandDraft = m.commandDraft[:len(m.commandDraft)-1]
			}
		default:
			if len(key) == 1 {
				m.commandDraft += key
			}
		}
		return false
	}

	if m.editFilter {
		switch key {
		case "enter", "esc":
			m.editFilter = false
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
	case "enter":
		m.enterSelected()
	case "/":
		m.editFilter = true
		m.filterDraft = m.filter
	case ":":
		m.editCommand = true
		m.commandDraft = ""
	case "s":
		m.statusFilter = cycle([]string{"open", "all", "completed", "archived"}, m.statusFilter)
		m.selected = 0
		m.expandedID = ""
	case "p":
		m.priorityFilter = cycle([]string{"all", "now", "soon", "later"}, m.priorityFilter)
		m.selected = 0
		m.expandedID = ""
	case "c":
		m.clearFiltersAndScope()
	case "t":
		m.randomizeTheme()
	case "1":
		m.switchView("tasks")
	case "2":
		m.switchView("projects")
	case "3":
		m.switchView("goals")
	case "4":
		m.switchView("today")
	case "5":
		m.switchView("assignees")
	}
	if m.selected < 0 {
		m.selected = 0
	}
	return false
}

func (m *model) enterSelected() {
	switch m.view {
	case "projects":
		rows, err := m.loadProgressRows("project")
		if err != nil || len(rows) == 0 {
			return
		}
		rows = m.applyProgressFilter(rows)
		if len(rows) == 0 {
			return
		}
		m.selected = clampIndex(m.selected, len(rows))
		r := rows[m.selected]
		m.scopeKind = "project"
		m.scopeID = r.ID
		m.scopeName = r.Name
		m.scopeColor = r.ColorHex
		m.switchView("tasks")
		m.commandMsg = "scope project: " + r.Name
	case "goals":
		rows, err := m.loadProgressRows("goal")
		if err != nil || len(rows) == 0 {
			return
		}
		rows = m.applyProgressFilter(rows)
		if len(rows) == 0 {
			return
		}
		m.selected = clampIndex(m.selected, len(rows))
		r := rows[m.selected]
		m.scopeKind = "goal"
		m.scopeID = r.ID
		m.scopeName = r.Name
		m.scopeColor = r.ColorHex
		m.switchView("tasks")
		m.commandMsg = "scope goal: " + r.Name
	case "assignees":
		rows, err := m.loadAssigneeRows()
		if err != nil || len(rows) == 0 {
			return
		}
		rows = m.applyProgressFilter(rows)
		if len(rows) == 0 {
			return
		}
		m.selected = clampIndex(m.selected, len(rows))
		r := rows[m.selected]
		m.scopeKind = "assignee"
		m.scopeID = r.ID
		m.scopeName = r.Name
		m.scopeColor = ""
		m.switchView("tasks")
		m.commandMsg = "scope assignee: " + r.Name
	default:
		m.toggleExpand()
	}
}

func (m *model) executeCommand(cmd string) {
	if cmd == "" {
		m.commandMsg = ""
		return
	}
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}
	switch strings.ToLower(parts[0]) {
	case "view":
		if len(parts) < 2 {
			m.commandMsg = "usage: view tasks|projects|goals|today|assignees"
			return
		}
		v := strings.ToLower(parts[1])
		if v == "task" {
			v = "tasks"
		}
		if v == "project" {
			v = "projects"
		}
		if v == "goal" {
			v = "goals"
		}
		if v == "assignee" {
			v = "assignees"
		}
		switch v {
		case "tasks", "projects", "goals", "today", "assignees":
			m.switchView(v)
			m.commandMsg = "view: " + v
		default:
			m.commandMsg = "unknown view"
		}
	case "tag":
		m.tagFilter = strings.TrimSpace(strings.TrimPrefix(cmd, parts[0]))
		m.selected = 0
		m.commandMsg = "tag filter: " + fallbackDash(m.tagFilter)
	case "assignee":
		m.assigneeFilter = strings.TrimSpace(strings.TrimPrefix(cmd, parts[0]))
		m.selected = 0
		m.commandMsg = "assignee filter: " + fallbackDash(m.assigneeFilter)
	case "status":
		if len(parts) < 2 {
			m.commandMsg = "usage: status open|all|completed|archived"
			return
		}
		v := strings.ToLower(parts[1])
		if v != "open" && v != "all" && v != "completed" && v != "archived" {
			m.commandMsg = "invalid status"
			return
		}
		m.statusFilter = v
		m.selected = 0
		m.commandMsg = "status: " + v
	case "priority":
		if len(parts) < 2 {
			m.commandMsg = "usage: priority all|now|soon|later"
			return
		}
		v := strings.ToLower(parts[1])
		if v != "all" && v != "now" && v != "soon" && v != "later" {
			m.commandMsg = "invalid priority"
			return
		}
		m.priorityFilter = v
		m.selected = 0
		m.commandMsg = "priority: " + v
	case "scope":
		if len(parts) >= 2 && strings.ToLower(parts[1]) == "clear" {
			m.scopeKind, m.scopeID, m.scopeName = "", "", ""
			m.scopeColor = ""
			m.commandMsg = "scope cleared"
			return
		}
		m.commandMsg = "usage: scope clear"
	case "clear":
		m.clearFiltersAndScope()
		m.commandMsg = "cleared"
	case "help":
		m.commandMsg = "cmd: view, tag, assignee, status, priority, scope clear, clear"
	default:
		m.commandMsg = "unknown command"
	}
}

func (m *model) clearFiltersAndScope() {
	m.filter = ""
	m.filterDraft = ""
	m.statusFilter = "open"
	m.priorityFilter = "all"
	m.tagFilter = ""
	m.assigneeFilter = ""
	m.scopeKind = ""
	m.scopeID = ""
	m.scopeName = ""
	m.scopeColor = ""
	m.selected = 0
	m.expandedID = ""
}

func (m *model) randomizeTheme() {
	if len(m.themes) <= 1 {
		return
	}
	next := m.themeIndex
	for next == m.themeIndex {
		next = m.rng.Intn(len(m.themes))
	}
	m.themeIndex = next
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

	headerLines := 8
	footerLines := 2
	bodyBudget := lines - headerLines - footerLines
	if bodyBudget < 4 {
		bodyBudget = 4
	}
	if bodyBudget > m.limit {
		bodyBudget = m.limit
	}

	scope := ""
	if m.scopeKind != "" {
		scope = " scope=" + m.scopeKind + ":" + m.scopeName
	}
	titleLine := fmt.Sprintf("dooh interactive  theme=%s  view=%s%s  filter=/%s", m.themes[m.themeIndex].Name, m.view, scope, m.activeFilter())
	filterLine := fmt.Sprintf("status=%s priority=%s tag=%s assignee=%s", m.statusFilter, m.priorityFilter, fallbackDash(m.tagFilter), fallbackDash(m.assigneeFilter))
	banner := bannerText(m.view, m.scopeKind, m.scopeName)
	bannerLine := centerText("["+banner+"]", cols)

	frame := make([]string, 0, lines)
	frame = append(frame, m.paintAccent(clampLine(titleLine, cols), p.Accent))
	frame = append(frame, m.paintBanner(clampLine(bannerLine, cols), p))
	frame = append(frame, m.paintMuted(clampLine(filterLine, cols), p))
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
	if m.editCommand {
		frame = append(frame, clampLine(":"+m.commandDraft, cols))
	} else if m.editFilter {
		frame = append(frame, clampLine("filter> "+m.filterDraft+" (live fuzzy; Enter/Esc close)", cols))
	} else if m.commandMsg != "" {
		frame = append(frame, clampLine(selectedLine+" | "+m.commandMsg, cols))
	} else {
		frame = append(frame, clampLine(selectedLine, cols))
	}
	frame = append(frame, clampLine("keys: arrows, Enter open/expand, / filter, : command, s status, p priority, c clear, t random theme, 1-5 views, q quit", cols))

	if len(frame) > lines {
		frame = frame[:lines]
	}
	for len(frame) < lines {
		frame = append(frame, "")
	}
	return joinFrame(frame), nil
}

func (m *model) renderHeader(cols int, p palette) string {
	if m.view == "projects" || m.view == "goals" || m.view == "assignees" {
		nameW := cols - (2 + 2 + 18 + 5 + 9 + 9)
		if nameW < 18 {
			nameW = 18
		}
		h := fmt.Sprintf("%-2s %-*s  %-18s  %-5s  %-9s  %-9s", "", nameW, "Name", "Progress", "%", "Completed", "Remaining")
		return m.paintMuted(clampLine(h, cols), p)
	}
	priorityW := 8
	scheduledW := 17
	idW := 8
	separatorW := 4
	titleW := cols - (1 + 1 + 1 + separatorW + 1 + separatorW + priorityW + separatorW + scheduledW + separatorW + idW)
	if titleW < 16 {
		titleW = 16
	}
	h := fmt.Sprintf("%-1s %-1s %-1s %-*s  %-*s  %-*s  %-*s", " ", " ", "D", titleW, "Title", priorityW, "Priority", scheduledW, "Scheduled", idW, "ID")
	return m.paintMuted(clampLine(h, cols), p)
}

func (m *model) renderBodyByView(cols, lines int, p palette) ([]string, string, string, error) {
	now := time.Now()
	headerLines := 8
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
			return []string{"(no project rows)"}, "projects=0", "selected: none", nil
		}
		m.selected = clampIndex(m.selected, len(rows))
		linesOut := m.composeProgressBody(rows, bodyBudget, cols, p, "project")
		r := rows[m.selected]
		selected := fmt.Sprintf("selected project: %s | completion=%d%% (%d completed, %d remaining)", r.Name, pct(r.Completed, r.Total), r.Completed, r.Remaining)
		return linesOut, fmt.Sprintf("projects=%d", len(rows)), selected, nil
	case "goals":
		rows, err := m.loadProgressRows("goal")
		if err != nil {
			return nil, "", "", err
		}
		rows = m.applyProgressFilter(rows)
		if len(rows) == 0 {
			m.selected = 0
			return []string{"(no goal rows)"}, "goals=0", "selected: none", nil
		}
		m.selected = clampIndex(m.selected, len(rows))
		linesOut := m.composeProgressBody(rows, bodyBudget, cols, p, "goal")
		r := rows[m.selected]
		selected := fmt.Sprintf("selected goal: %s | completion=%d%% (%d completed, %d remaining)", r.Name, pct(r.Completed, r.Total), r.Completed, r.Remaining)
		return linesOut, fmt.Sprintf("goals=%d", len(rows)), selected, nil
	case "assignees":
		rows, err := m.loadAssigneeRows()
		if err != nil {
			return nil, "", "", err
		}
		rows = m.applyProgressFilter(rows)
		if len(rows) == 0 {
			m.selected = 0
			return []string{"(no assignee rows)"}, "assignees=0", "selected: none", nil
		}
		m.selected = clampIndex(m.selected, len(rows))
		linesOut := m.composeProgressBody(rows, bodyBudget, cols, p, "assignee")
		r := rows[m.selected]
		selected := fmt.Sprintf("selected assignee: %s | completion=%d%% (%d completed, %d remaining)", r.Name, pct(r.Completed, r.Total), r.Completed, r.Remaining)
		return linesOut, fmt.Sprintf("assignees=%d", len(rows)), selected, nil
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
			return []string{"(no tasks scheduled today)"}, "today scheduled=0", "selected: none", nil
		}
		m.selected = clampIndex(m.selected, len(todayRows))
		linesOut := m.composeTaskBody(todayRows, bodyBudget, cols, now, p)
		r := todayRows[m.selected]
		selected := fmt.Sprintf("selected: %s | due=%s | scheduled=%s | updated=%s | collections=%s", r.Title, NaturalDate(r.DueAt, m.loc, now), NaturalDate(r.Scheduled, m.loc, now), NaturalDate(r.UpdatedAt, m.loc, now), r.Collection)
		return linesOut, fmt.Sprintf("today scheduled=%d", len(todayRows)), selected, nil
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
		selected := fmt.Sprintf("selected: %s | due=%s | scheduled=%s | updated=%s | collections=%s", r.Title, NaturalDate(r.DueAt, m.loc, now), NaturalDate(r.Scheduled, m.loc, now), NaturalDate(r.UpdatedAt, m.loc, now), r.Collection)
		return linesOut, fmt.Sprintf("open %d  completed %d  archived %d", counts["open"], counts["completed"], counts["archived"]), selected, nil
	}
}

func (m *model) composeTaskBody(rows []row, budget int, cols int, now time.Time, p palette) []string {
	priorityW := 8
	scheduledW := 17
	idW := 8
	separatorW := 4
	titleW := cols - (1 + 1 + 1 + separatorW + 1 + separatorW + priorityW + separatorW + scheduledW + separatorW + idW)
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
		dueFlag := dueFlagIcon(r, now, m.loc)
		rowLine := fmt.Sprintf("%-1s %-1s %-1s %-*s  %-*s  %-*s  %-*s",
			icon,
			mark,
			dueFlag,
			titleW, clampLine(r.Title, titleW),
			priorityW, r.Priority,
			scheduledW, NaturalDate(r.Scheduled, m.loc, now),
			idW, r.ID,
		)
		line := clampLine(rowLine, cols)
		line = m.paintStatusMarker(line, icon, r.Status, p)
		line = m.paintDueMarker(line, dueFlag, r, now, p)
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
				"projects: " + strings.TrimSpace(r.Projects),
				"goals: " + strings.TrimSpace(r.Goals),
				"areas: " + strings.TrimSpace(r.Areas),
				"groups: " + strings.TrimSpace(r.Groups),
				"tags: " + strings.TrimSpace(r.Tags),
				"assignees: " + strings.TrimSpace(r.Assignees),
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

func (m *model) composeProgressBody(rows []progressRow, budget int, cols int, p palette, kind string) []string {
	nameW := cols - (2 + 2 + 18 + 5 + 9 + 9)
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
	barCode := p.Accent
	if kind == "goal" {
		barCode = 186
	}
	if kind == "assignee" {
		barCode = 110
	}
	for i := start; i < len(rows) && len(lines) < budget; i++ {
		r := rows[i]
		mark := " "
		if i == m.selected {
			mark = ">"
		}
		bar := m.paintAccent(progressBar(r.Completed, r.Total, 18), barCode)
		if strings.TrimSpace(r.ColorHex) != "" {
			bar = m.paintHex(bar, r.ColorHex)
		}
		name := clampLine(r.Name, nameW)
		if strings.TrimSpace(r.ColorHex) != "" {
			name = m.paintHex(name, r.ColorHex)
		}
		line := fmt.Sprintf("%-2s %-*s  %-18s  %3d%%  %9d  %9d", mark, nameW, name, bar, pct(r.Completed, r.Total), r.Completed, r.Remaining)
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
  COALESCE(group_concat(DISTINCT c.name), ''),
  COALESCE(group_concat(DISTINCT CASE WHEN c.kind='tag' THEN c.name END), ''),
  COALESCE(group_concat(DISTINCT u.name), ''),
  COALESCE(group_concat(DISTINCT CASE WHEN c.kind='project' THEN c.short_id END), ''),
  COALESCE(group_concat(DISTINCT CASE WHEN c.kind='goal' THEN c.short_id END), ''),
  COALESCE(group_concat(DISTINCT u.id), ''),
  COALESCE(group_concat(DISTINCT CASE WHEN c.kind='project' THEN c.name END), ''),
  COALESCE(group_concat(DISTINCT CASE WHEN c.kind='goal' THEN c.name END), ''),
  COALESCE(group_concat(DISTINCT CASE WHEN c.kind='area' THEN c.name END), ''),
  COALESCE(group_concat(DISTINCT CASE WHEN c.kind='class' THEN c.name END), '')
FROM tasks t
LEFT JOIN task_collections tc ON tc.task_id=t.id
LEFT JOIN collections c ON c.id=tc.collection_id
LEFT JOIN task_assignees ta ON ta.task_id=t.id
LEFT JOIN users u ON u.id=ta.user_id
WHERE t.deleted_at IS NULL
GROUP BY t.id
ORDER BY t.updated_at DESC;`)
	if err != nil {
		return nil, err
	}
	out := make([]row, 0, len(rows))
	for _, r := range rows {
		if len(r) < 13 {
			continue
		}
		col := func(i int) string {
			if i < 0 || i >= len(r) {
				return ""
			}
			return r[i]
		}
		out = append(out, row{
			ID:          col(0),
			Title:       col(1),
			Status:      col(2),
			Priority:    col(3),
			DueAt:       col(4),
			Scheduled:   col(5),
			UpdatedAt:   col(6),
			Collection:  col(7),
			Tags:        col(8),
			Assignees:   col(9),
			ProjectIDs:  col(10),
			GoalIDs:     col(11),
			AssigneeIDs: col(12),
			Projects:    col(13),
			Goals:       col(14),
			Areas:       col(15),
			Groups:      col(16),
		})
	}
	return out, nil
}

func (m *model) loadProgressRows(kind string) ([]progressRow, error) {
	rows, err := m.sqlite.QueryTSV(`
SELECT
  c.short_id,
  c.name,
  COALESCE(c.color_hex,''),
  COALESCE(SUM(CASE WHEN t.status='completed' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN t.status='open' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN t.id IS NOT NULL THEN 1 ELSE 0 END), 0)
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
		if len(r) < 6 {
			continue
		}
		completed, _ := strconv.Atoi(r[3])
		remaining, _ := strconv.Atoi(r[4])
		total, _ := strconv.Atoi(r[5])
		out = append(out, progressRow{ID: r[0], Name: r[1], ColorHex: r[2], Completed: completed, Remaining: remaining, Total: total})
	}
	return out, nil
}

func (m *model) loadAssigneeRows() ([]progressRow, error) {
	rows, err := m.sqlite.QueryTSV(`
SELECT
  u.id,
  u.name,
  COALESCE(SUM(CASE WHEN t.status='completed' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN t.status='open' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN t.id IS NOT NULL THEN 1 ELSE 0 END), 0)
FROM users u
LEFT JOIN task_assignees ta ON ta.user_id=u.id
LEFT JOIN tasks t ON t.id=ta.task_id AND t.deleted_at IS NULL
WHERE u.status='active'
GROUP BY u.id
ORDER BY u.name ASC;`)
	if err != nil {
		return nil, err
	}
	out := make([]progressRow, 0, len(rows))
	for _, r := range rows {
		if len(r) < 5 {
			continue
		}
		completed, _ := strconv.Atoi(r[2])
		remaining, _ := strconv.Atoi(r[3])
		total, _ := strconv.Atoi(r[4])
		out = append(out, progressRow{ID: r[0], Name: r[1], Completed: completed, Remaining: remaining, Total: total})
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
		if m.tagFilter != "" && !fuzzyMatch(strings.ToLower(r.Tags), strings.ToLower(m.tagFilter)) {
			continue
		}
		if m.assigneeFilter != "" && !fuzzyMatch(strings.ToLower(r.Assignees), strings.ToLower(m.assigneeFilter)) {
			continue
		}
		if m.scopeKind == "project" && !containsToken(r.ProjectIDs, m.scopeID) {
			continue
		}
		if m.scopeKind == "goal" && !containsToken(r.GoalIDs, m.scopeID) {
			continue
		}
		if m.scopeKind == "assignee" && !containsToken(r.AssigneeIDs, m.scopeID) {
			continue
		}
		if f != "" {
			h := strings.ToLower(strings.Join([]string{r.Title, r.ID, r.Priority, r.Collection, r.Tags, r.Assignees}, " "))
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
	t := []string{"[1 tasks]", "[2 projects]", "[3 goals]", "[4 today]", "[5 assignees]"}
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

func containsToken(csv string, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	for _, p := range strings.Split(csv, ",") {
		if strings.TrimSpace(p) == token {
			return true
		}
	}
	return false
}

func fallbackDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return strings.TrimSpace(v)
}

func bannerText(view string, scopeKind string, scopeName string) string {
	if scopeKind == "project" && strings.TrimSpace(scopeName) != "" {
		return "PROJECT: " + strings.ToUpper(scopeName)
	}
	if scopeKind == "goal" && strings.TrimSpace(scopeName) != "" {
		return "GOAL: " + strings.ToUpper(scopeName)
	}
	switch view {
	case "projects":
		return "PROJECTS"
	case "goals":
		return "GOALS"
	case "today":
		return "TODAY"
	case "assignees":
		return "ASSIGNEES"
	default:
		return "ALL TASKS"
	}
}

func centerText(s string, width int) string {
	if width <= len(s) {
		return s
	}
	pad := (width - len(s)) / 2
	return strings.Repeat(" ", pad) + s
}

func dueFlagIcon(r row, now time.Time, loc *time.Location) string {
	if strings.TrimSpace(r.DueAt) == "" {
		return " "
	}
	if isOverdue(r, now, loc) {
		return "!"
	}
	return "⚑"
}

func isOverdue(r row, now time.Time, loc *time.Location) bool {
	if r.Status != "open" || strings.TrimSpace(r.DueAt) == "" {
		return false
	}
	t, ok := parseTime(r.DueAt)
	if !ok {
		return false
	}
	if loc == nil {
		loc = time.Local
	}
	return t.In(loc).Before(now.In(loc))
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

func (m *model) paintBanner(s string, p palette) string {
	if strings.TrimSpace(m.scopeColor) != "" {
		return m.paintHex(s, m.scopeColor)
	}
	return m.paintAccent(s, p.Accent)
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

func (m *model) paintDueMarker(line string, marker string, r row, now time.Time, p palette) string {
	if m.plain || strings.TrimSpace(marker) == "" {
		return line
	}
	if marker == "!" || isOverdue(r, now, m.loc) {
		return strings.Replace(line, marker, m.colorize(marker, 203), 1)
	}
	return strings.Replace(line, marker, m.colorize(marker, p.Warn), 1)
}

func (m *model) paintHex(s string, hex string) string {
	if m.plain {
		return s
	}
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		return s
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", r, g, b, s)
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
		seq, ok, err := readEscapeSequence(r)
		if err != nil {
			return "", err
		}
		if !ok {
			return "esc", nil
		}
		switch seq {
		case "[A":
			return "up", nil
		case "[B":
			return "down", nil
		case "[C":
			return "right", nil
		case "[D":
			return "left", nil
		default:
			if strings.HasPrefix(seq, "[<64;") {
				return "up", nil
			}
			if strings.HasPrefix(seq, "[<65;") {
				return "down", nil
			}
			return "", nil
		}
	}
	switch b {
	case '\r', '\n':
		return "enter", nil
	case 127, 8:
		return "backspace", nil
	case ':', '/':
		return string(b), nil
	}
	if b >= 32 && b <= 126 {
		return string(b), nil
	}
	return "", nil
}

func readEscapeSequence(r *bufio.Reader) (string, bool, error) {
	b, err := r.ReadByte()
	if err != nil {
		return "", false, nil
	}
	if b != '[' {
		return "", false, nil
	}
	buf := []byte{'['}
	for i := 0; i < 32; i++ {
		next, err := r.ReadByte()
		if err != nil {
			return string(buf), true, nil
		}
		buf = append(buf, next)
		if (next >= 'A' && next <= 'Z') || (next >= 'a' && next <= 'z') || next == '~' {
			break
		}
	}
	return string(buf), true, nil
}

func parseHexColor(v string) (int, int, int, bool) {
	s := strings.TrimSpace(strings.TrimPrefix(v, "#"))
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	n, err := strconv.ParseInt(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	r := int((n >> 16) & 0xFF)
	g := int((n >> 8) & 0xFF)
	b := int(n & 0xFF)
	return r, g, b, true
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
