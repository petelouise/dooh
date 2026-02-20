package tui

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"dooh/internal/db"
	"github.com/mattn/go-runewidth"
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

type Identity struct {
	Actor    string
	UserID   string
	UserName string
}

type FilterState struct {
	Text      string
	Status    string
	Priority  string
	Sort      string
	SortDir   string
	Tags      []string
	Assignee  string
	ScopeKind string
	ScopeID   string
	ScopeName string
	TodayMode string
}

type FacetOption struct {
	Name  string
	Count int
}

type palette struct {
	Accent   int
	BgAccent int
	Open     int
	Done     int
	Archive  int
	Muted    int
	Warn     int
}

const (
	filterFieldText = iota
	filterFieldStatus
	filterFieldPriority
	filterFieldTags
	filterFieldAssignee
	filterFieldTodayMode
	filterFieldCount
)

type model struct {
	sqlite          db.SQLite
	themes          []Theme
	themeIndex      int
	filters         FilterState
	selected        int
	limit           int
	loc             *time.Location
	plain           bool
	view            string
	rng             *rand.Rand
	currentUserHint string
	identity        Identity

	scopeColor string

	filterFocus    int
	filterBarFocus bool
	editFilter     bool
	editField      int
	fieldInput     string
	fieldDraftTags []string
	dropdown       []FacetOption
	dropdownIndex  int
	dropdownOpen   bool
	expandedID     string
}

func RunInteractive(in io.Reader, out io.Writer, sqlite db.SQLite, catalog ThemeCatalog, themeID string, filter string, limit int, loc *time.Location, identity Identity, plain bool) error {
	m := newModel(sqlite, catalog, themeID, filter, limit, loc, identity, plain)

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

func RenderDashboard(sqlite db.SQLite, theme Theme, filter string, limit int, loc *time.Location, identity Identity, plain bool) (string, error) {
	catalog := ThemeCatalog{Themes: []Theme{theme}, Default: theme.ID}
	m := newModel(sqlite, catalog, theme.ID, filter, limit, loc, identity, plain)
	cols, lines := terminalSize()
	return m.render(cols, lines)
}

func newModel(sqlite db.SQLite, catalog ThemeCatalog, themeID string, filter string, limit int, loc *time.Location, identity Identity, plain bool) model {
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
	userHint := strings.TrimSpace(identity.UserName)
	if userHint == "" {
		userHint = strings.TrimSpace(identity.UserID)
	}
	return model{
		sqlite:     sqlite,
		themes:     catalog.Themes,
		themeIndex: idx,
		filters: FilterState{
			Text:      strings.TrimSpace(filter),
			Status:    "open",
			Priority:  "all",
			Sort:      "updated",
			SortDir:   "desc",
			TodayMode: "mine",
		},
		limit:           limit,
		loc:             loc,
		plain:           plain,
		view:            "tasks",
		rng:             rand.New(rand.NewSource(seed)),
		currentUserHint: strings.ToLower(userHint),
		identity:        identity,
	}
}

func (m *model) handleKey(key string) bool {
	if m.editFilter {
		if key == "tab" || key == "shift_tab" {
			m.editFilter = false
			m.dropdownOpen = false
			if key == "tab" {
				m.filterFocus = m.nextFilterFocus(1)
			} else {
				m.filterFocus = m.nextFilterFocus(-1)
			}
			m.filterBarFocus = true
			return false
		}
		m.handleFilterEdit(key)
		return false
	}

	switch key {
	case "q":
		return true
	case "tab":
		m.filterFocus = m.nextFilterFocus(1)
		m.filterBarFocus = true
	case "shift_tab":
		m.filterFocus = m.nextFilterFocus(-1)
		m.filterBarFocus = true
	case "up":
		m.filterBarFocus = false
		m.selected--
	case "down":
		m.filterBarFocus = false
		m.selected++
	case "right":
		m.filterBarFocus = false
		if m.view == "tasks" || m.view == "today" {
			m.toggleExpand()
		}
	case "left":
		m.filterBarFocus = false
		m.expandedID = ""
	case "enter":
		if m.filterBarFocus {
			m.beginFilterEdit(m.filterFocus)
		} else {
			m.enterSelected()
		}
	case "/":
		m.filterFocus = filterFieldText
		m.filterBarFocus = true
		m.beginFilterEdit(filterFieldText)
	case "f":
		m.filterFocus = filterFieldText
		m.filterBarFocus = true
		m.beginFilterEdit(filterFieldText)
	case "g":
		m.filterFocus = filterFieldTags
		m.filterBarFocus = true
		m.beginFilterEdit(filterFieldTags)
	case "a":
		m.filterFocus = filterFieldAssignee
		m.filterBarFocus = true
		m.beginFilterEdit(filterFieldAssignee)
	case "s":
		m.filters.Status = cycle([]string{"open", "all", "completed", "archived"}, m.filters.Status)
		m.selected = 0
		m.expandedID = ""
	case "p":
		m.filters.Priority = cycle([]string{"all", "now", "soon", "later"}, m.filters.Priority)
		m.selected = 0
		m.expandedID = ""
	case "o":
		m.filters.Sort = cycle([]string{"updated", "priority", "scheduled"}, m.filters.Sort)
		m.filters.SortDir = defaultSortDirection(m.filters.Sort)
		m.selected = 0
		m.expandedID = ""
	case "O":
		m.filters.SortDir = toggleSortDirection(m.filters.SortDir)
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

func (m *model) nextFilterFocus(delta int) int {
	fields := []int{filterFieldText, filterFieldStatus, filterFieldPriority, filterFieldTags, filterFieldAssignee}
	if m.view == "today" {
		fields = append(fields, filterFieldTodayMode)
	}
	if len(fields) == 0 {
		return filterFieldText
	}
	idx := 0
	for i, f := range fields {
		if f == m.filterFocus {
			idx = i
			break
		}
	}
	next := idx + delta
	for next < 0 {
		next += len(fields)
	}
	next = next % len(fields)
	return fields[next]
}

func (m *model) beginFilterEdit(field int) {
	m.editFilter = true
	m.editField = field
	m.filterBarFocus = true
	m.dropdownOpen = false
	m.dropdown = nil
	m.dropdownIndex = 0
	switch field {
	case filterFieldText:
		m.fieldInput = m.filters.Text
	case filterFieldTags:
		m.fieldDraftTags = append([]string{}, m.filters.Tags...)
		m.fieldInput = ""
		m.updateFacetOptions(filterFieldTags)
	case filterFieldAssignee:
		m.fieldInput = m.filters.Assignee
		m.updateFacetOptions(filterFieldAssignee)
	case filterFieldStatus, filterFieldPriority, filterFieldTodayMode:
		m.updateFacetOptions(field)
	}
}

func (m *model) handleFilterEdit(key string) {
	switch m.editField {
	case filterFieldText:
		switch key {
		case "enter", "esc":
			m.editFilter = false
		case "backspace":
			if len(m.fieldInput) > 0 {
				m.fieldInput = m.fieldInput[:len(m.fieldInput)-1]
				m.filters.Text = strings.TrimSpace(m.fieldInput)
				m.selected = 0
			}
		default:
			if len(key) == 1 {
				m.fieldInput += key
				m.filters.Text = strings.TrimSpace(m.fieldInput)
				m.selected = 0
			}
		}
	case filterFieldTags:
		switch key {
		case "esc":
			m.editFilter = false
		case "enter":
			m.selectCurrentFacetOption()
		case "up":
			if len(m.dropdown) > 0 {
				m.dropdownIndex = (m.dropdownIndex + len(m.dropdown) - 1) % len(m.dropdown)
			}
		case "down":
			if len(m.dropdown) > 0 {
				m.dropdownIndex = (m.dropdownIndex + 1) % len(m.dropdown)
			}
		case "backspace":
			if len(m.fieldInput) > 0 {
				m.fieldInput = m.fieldInput[:len(m.fieldInput)-1]
			} else if len(m.fieldDraftTags) > 0 {
				m.fieldDraftTags = m.fieldDraftTags[:len(m.fieldDraftTags)-1]
				m.filters.Tags = append([]string{}, m.fieldDraftTags...)
				m.selected = 0
			}
			m.updateFacetOptions(filterFieldTags)
		default:
			if len(key) == 1 {
				m.fieldInput += key
				m.updateFacetOptions(filterFieldTags)
			}
		}
	case filterFieldAssignee:
		switch key {
		case "esc":
			m.editFilter = false
		case "enter":
			m.selectCurrentFacetOption()
		case "up":
			if len(m.dropdown) > 0 {
				m.dropdownIndex = (m.dropdownIndex + len(m.dropdown) - 1) % len(m.dropdown)
			}
		case "down":
			if len(m.dropdown) > 0 {
				m.dropdownIndex = (m.dropdownIndex + 1) % len(m.dropdown)
			}
		case "backspace":
			if len(m.fieldInput) > 0 {
				m.fieldInput = m.fieldInput[:len(m.fieldInput)-1]
				m.updateFacetOptions(filterFieldAssignee)
			}
		default:
			if len(key) == 1 {
				m.fieldInput += key
				m.updateFacetOptions(filterFieldAssignee)
			}
		}
	case filterFieldStatus:
		switch key {
		case "esc", "enter":
			m.editFilter = false
		case "up", "down":
			m.filters.Status = cycle([]string{"open", "all", "completed", "archived"}, m.filters.Status)
		}
	case filterFieldPriority:
		switch key {
		case "esc", "enter":
			m.editFilter = false
		case "up", "down":
			m.filters.Priority = cycle([]string{"all", "now", "soon", "later"}, m.filters.Priority)
		}
	case filterFieldTodayMode:
		switch key {
		case "esc", "enter":
			m.editFilter = false
		case "up", "down":
			m.filters.TodayMode = cycle([]string{"mine", "all"}, m.filters.TodayMode)
		}
	}
}

func (m *model) selectCurrentFacetOption() {
	if len(m.dropdown) == 0 {
		if m.editField == filterFieldAssignee {
			m.filters.Assignee = strings.TrimSpace(m.fieldInput)
			m.editFilter = false
		}
		return
	}
	opt := m.dropdown[m.dropdownIndex]
	switch m.editField {
	case filterFieldTags:
		if !containsExact(m.fieldDraftTags, opt.Name) {
			m.fieldDraftTags = append(m.fieldDraftTags, opt.Name)
			m.filters.Tags = append([]string{}, m.fieldDraftTags...)
			m.selected = 0
		}
		m.fieldInput = ""
		m.updateFacetOptions(filterFieldTags)
	case filterFieldAssignee:
		m.filters.Assignee = opt.Name
		m.selected = 0
		m.editFilter = false
	}
}

func (m *model) updateFacetOptions(field int) {
	options, err := m.computeFacetOptions(field)
	if err != nil {
		m.dropdown = nil
		m.dropdownOpen = false
		return
	}
	needle := strings.ToLower(strings.TrimSpace(m.fieldInput))
	filtered := make([]FacetOption, 0, len(options))
	for _, opt := range options {
		if needle == "" || fuzzyMatch(strings.ToLower(opt.Name), needle) {
			filtered = append(filtered, opt)
		}
	}
	m.dropdown = filtered
	m.dropdownOpen = len(filtered) > 0
	if m.dropdownIndex >= len(filtered) {
		m.dropdownIndex = 0
	}
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
		m.filters.ScopeKind = "project"
		m.filters.ScopeID = r.ID
		m.filters.ScopeName = r.Name
		m.scopeColor = r.ColorHex
		m.switchView("tasks")
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
		m.filters.ScopeKind = "goal"
		m.filters.ScopeID = r.ID
		m.filters.ScopeName = r.Name
		m.scopeColor = r.ColorHex
		m.switchView("tasks")
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
		m.filters.ScopeKind = "assignee"
		m.filters.ScopeID = r.ID
		m.filters.ScopeName = r.Name
		m.scopeColor = ""
		m.switchView("tasks")
	default:
		m.toggleExpand()
	}
}

func (m *model) clearFiltersAndScope() {
	m.filters.Text = ""
	m.fieldInput = ""
	m.filters.Status = "open"
	m.filters.Priority = "all"
	m.filters.Sort = "updated"
	m.filters.SortDir = "desc"
	m.filters.Tags = nil
	m.filters.Assignee = ""
	m.filters.ScopeKind = ""
	m.filters.ScopeID = ""
	m.filters.ScopeName = ""
	m.filters.TodayMode = "mine"
	m.scopeColor = ""
	m.selected = 0
	m.expandedID = ""
	m.filterBarFocus = false
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
	footerLines := 3
	bodyBudget := lines - headerLines - footerLines
	if m.editFilter && m.dropdownOpen {
		bodyBudget -= m.dropdownHeight()
	}
	if bodyBudget < 4 {
		bodyBudget = 4
	}

	scope := ""
	if m.filters.ScopeKind != "" {
		scope = m.filters.ScopeKind + ":" + m.filters.ScopeName
	}
	identityText := fmt.Sprintf("%s | %s", fallbackDash(m.identity.Actor), fallbackDash(m.identity.UserName))
	titleLine := fmt.Sprintf("dooh interactive  theme=%s  view=%s  identity=%s", m.themes[m.themeIndex].Name, m.view, identityText)
	filterLine := m.renderFilterBar(scope, cols)
	banner := bannerText(m.view, m.filters.ScopeKind, m.filters.ScopeName)
	bannerLine := centerText("["+banner+"]", cols)

	frame := make([]string, 0, lines)
	frame = append(frame, m.paintTitleBar(titleLine, cols, p))
	frame = append(frame, m.paintBanner(clampLine(bannerLine, cols), cols, p))
	frame = append(frame, m.paintFilterBarLine(filterLine, cols, p))
	frame = append(frame, m.paintMuted(strings.Repeat("-", cols), p))
	frame = append(frame, m.paintSubheaderLine(renderTabs(cols, m.view), cols, p))
	frame = append(frame, m.paintSubheaderLine(countLine, cols, p))
	frame = append(frame, m.paintMuted(strings.Repeat("-", cols), p))
	frame = append(frame, m.renderHeader(cols, p))
	if m.dropdownOpen && m.editFilter {
		for _, line := range m.renderDropdown(cols, p) {
			frame = append(frame, line)
		}
	}

	frame = append(frame, body...)
	for len(frame) < headerLines+bodyBudget {
		frame = append(frame, "")
	}

	frame = append(frame, m.paintMuted(strings.Repeat("-", cols), p))
	if m.editFilter {
		frame = append(frame, m.paintFooterLine(m.editPrompt(), cols, p))
	} else {
		frame = append(frame, m.paintFooterLine(selectedLine, cols, p))
	}
	if m.editFilter {
		frame = append(frame, m.paintFooterLine("filter: #[tag] ~[area] ^[goal] @[assignee] !due !todaydue !overdue !nodue | Enter select/add | Esc close", cols, p))
	} else {
		frame = append(frame, m.paintFooterLine("keys: arrows move, Enter action, Tab focus, / text, s status, p priority, o sort, O reverse", cols, p))
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
	if m.view == "projects" || m.view == "goals" || m.view == "assignees" {
		nameW := cols - (2 + 2 + 18 + 5 + 9 + 9)
		if nameW < 18 {
			nameW = 18
		}
		h := strings.Join([]string{
			padCell("", 2),
			padCell("Name", nameW),
			padCell("Progress", 18),
			padCell("%", 5),
			padCell("Completed", 9),
			padCell("Remaining", 9),
		}, "  ")
		return m.paintMuted(clampLine(h, cols), p)
	}
	priorityW := 8
	scheduledW := 17
	separatorW := 4
	assigneeW := 1
	titleW := cols - (1 + 1 + 1 + 1 + separatorW + assigneeW + separatorW + priorityW + separatorW + scheduledW)
	if titleW < 16 {
		titleW = 16
	}
	h := strings.Join([]string{
		padCell(">", 1),
		padCell("S", 1),
		padCell("Asg", assigneeW),
		padCell("Title", titleW),
		padCell("Priority", priorityW),
		padCell("Scheduled", scheduledW),
	}, "  ")
	return m.paintMuted(clampLine(h, cols), p)
}

func (m *model) renderBodyByView(cols, lines int, p palette) ([]string, string, string, error) {
	now := time.Now()
	headerLines := 8
	footerLines := 3
	bodyBudget := lines - headerLines - footerLines
	if m.editFilter && m.dropdownOpen {
		bodyBudget -= m.dropdownHeight()
	}
	if bodyBudget < 4 {
		bodyBudget = 4
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
			if !isTodayScheduled(r.Scheduled, m.loc, now) {
				continue
			}
			if m.filters.TodayMode == "mine" && m.filters.ScopeKind != "assignee" && strings.TrimSpace(m.filters.Assignee) == "" && !m.assignedToCurrent(r) {
				continue
			}
			todayRows = append(todayRows, r)
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
	separatorW := 4
	assigneeW := 1
	titleW := cols - (1 + 1 + 1 + 1 + separatorW + assigneeW + separatorW + priorityW + separatorW + scheduledW)
	if titleW < 16 {
		titleW = 16
	}
	selectedExtra := 0
	if m.selected >= 0 && m.selected < len(rows) && m.expandedID == rows[m.selected].ID && (m.view == "tasks" || m.view == "today") {
		selectedExtra = m.detailLineCount(rows[m.selected], cols, now)
	}
	start := 0
	maxSelectedPos := budget - 1 - selectedExtra
	if maxSelectedPos < 0 {
		maxSelectedPos = 0
	}
	if m.selected > maxSelectedPos {
		start = m.selected - maxSelectedPos
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
		title := r.Title + dueSuffix(r, now, m.loc)
		asg := assigneeInitials(r.Assignees)
		rowLine := strings.Join([]string{
			padCell(mark, 1),
			padCell(icon, 1),
			padCell(asg, assigneeW),
			padCell(title, titleW),
			padCell(r.Priority, priorityW),
			padCell(NaturalDate(r.Scheduled, m.loc, now), scheduledW),
		}, "  ")
		line := clampLine(rowLine, cols)
		line = m.paintStatusMarker(line, icon, r.Status, p)
		line = m.paintDueSuffix(line, title, r, now, p)
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
				"projects: " + fallbackDash(strings.TrimSpace(r.Projects)),
				"goals: " + fallbackDash(strings.TrimSpace(r.Goals)),
				"areas: " + fallbackDash(strings.TrimSpace(r.Areas)),
				"groups: " + fallbackDash(strings.TrimSpace(r.Groups)),
				"tags: " + fallbackDash(strings.TrimSpace(r.Tags)),
				"assignees: " + strings.TrimSpace(r.Assignees),
			}
			for _, d := range detail {
				for _, wrapped := range wrapText(d, cols-4) {
					if len(lines) >= budget {
						break
					}
					lines = append(lines, m.paintCollectionLine(clampLine("    "+wrapped, cols), p))
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
		line := strings.Join([]string{
			padCell(mark, 2),
			padCell(name, nameW),
			padCell(bar, 18),
			padCell(fmt.Sprintf("%3d%%", pct(r.Completed, r.Total)), 5),
			padCell(fmt.Sprintf("%d", r.Completed), 9),
			padCell(fmt.Sprintf("%d", r.Remaining), 9),
		}, "  ")
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
	if m.editFilter && m.editField == filterFieldText {
		return strings.TrimSpace(m.fieldInput)
	}
	return strings.TrimSpace(m.filters.Text)
}

func (m *model) applyFilters(in []row) []row {
	qf := parseQuickFilter(m.activeFilter())
	f := strings.ToLower(strings.TrimSpace(qf.Text))
	requiredTags := append([]string{}, m.filters.Tags...)
	requiredTags = append(requiredTags, qf.Tags...)
	now := time.Now()
	out := make([]row, 0, len(in))
	for _, r := range in {
		if m.filters.Status != "all" && r.Status != m.filters.Status {
			continue
		}
		if m.filters.Priority != "all" && r.Priority != m.filters.Priority {
			continue
		}
		if len(requiredTags) > 0 {
			rowTags := splitCSVLower(r.Tags)
			if !containsAllTags(rowTags, requiredTags) {
				continue
			}
		}
		if m.filters.Assignee != "" && !fuzzyMatch(strings.ToLower(r.Assignees), strings.ToLower(m.filters.Assignee)) {
			continue
		}
		if m.filters.ScopeKind == "project" && !containsToken(r.ProjectIDs, m.filters.ScopeID) {
			continue
		}
		if m.filters.ScopeKind == "goal" && !containsToken(r.GoalIDs, m.filters.ScopeID) {
			continue
		}
		if m.filters.ScopeKind == "assignee" && !containsToken(r.AssigneeIDs, m.filters.ScopeID) {
			continue
		}
		if len(qf.Areas) > 0 && !matchesAllInCSV(r.Areas, qf.Areas) {
			continue
		}
		if len(qf.Goals) > 0 && !matchesAllInCSV(r.Goals, qf.Goals) {
			continue
		}
		if len(qf.Assignees) > 0 && !matchesAllInCSV(r.Assignees, qf.Assignees) {
			continue
		}
		if qf.HasDue && strings.TrimSpace(r.DueAt) == "" {
			continue
		}
		if qf.NoDue && strings.TrimSpace(r.DueAt) != "" {
			continue
		}
		if qf.TodayDue && !isTodayDue(r, m.loc, now) {
			continue
		}
		if qf.Overdue && !isOverdue(r, now, m.loc) {
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
	sortRows(out, m.filters.Sort, m.filters.SortDir, m.loc)
	return out
}

func (m *model) applyProgressFilter(in []progressRow) []progressRow {
	f := strings.ToLower(parseQuickFilter(m.activeFilter()).Text)
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
	if runewidth.StringWidth(s) <= width {
		return s
	}
	if width <= 3 {
		return runewidth.Truncate(s, width, "")
	}
	return runewidth.Truncate(s, width, "...")
}

func fitLine(s string, width int) string {
	s = clampLine(s, width)
	if w := runewidth.StringWidth(s); w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}

func padCell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return fitLine(s, width)
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

func isTodayDue(r row, loc *time.Location, now time.Time) bool {
	if strings.TrimSpace(r.DueAt) == "" {
		return false
	}
	t, ok := parseTime(r.DueAt)
	if !ok {
		return false
	}
	if loc == nil {
		loc = time.Local
	}
	d := t.In(loc)
	n := now.In(loc)
	dy, dm, dd := d.Date()
	ny, nm, nd := n.Date()
	return dy == ny && dm == nm && dd == nd
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

func (m *model) renderFilterBar(scope string, cols int) string {
	p := paletteForTheme(m.themes[m.themeIndex].ID)
	qf := parseQuickFilter(m.activeFilter())
	textDisplay := m.renderTextChipValue(qf, p)
	tagsDisplay := fallbackDash(strings.Join(mergeTokenValues(m.filters.Tags, qf.Tags), ","))
	assigneeDisplay := fallbackDash(strings.Join(mergeTokenValues(singleValueList(m.filters.Assignee), qf.Assignees), ","))
	parts := []string{
		m.renderChip("text", textDisplay, m.filterFocus == filterFieldText && m.filterBarFocus && !m.editFilter, p),
		m.renderChip("status", m.filters.Status, m.filterFocus == filterFieldStatus && m.filterBarFocus && !m.editFilter, p),
		m.renderChip("priority", m.filters.Priority, m.filterFocus == filterFieldPriority && m.filterBarFocus && !m.editFilter, p),
		m.renderChip("sort", m.filters.Sort, false, p),
		m.renderChip("order", normalizeSortDirection(m.filters.SortDir), false, p),
		m.renderChip("tags", tagsDisplay, m.filterFocus == filterFieldTags && m.filterBarFocus && !m.editFilter, p),
		m.renderChip("assignee", assigneeDisplay, m.filterFocus == filterFieldAssignee && m.filterBarFocus && !m.editFilter, p),
	}
	if strings.TrimSpace(scope) != "" {
		parts = append(parts, m.renderChip("scope", scope, false, p))
	}
	if m.view == "today" {
		parts = append(parts, m.renderChip("today", m.filters.TodayMode, m.filterFocus == filterFieldTodayMode && m.filterBarFocus && !m.editFilter, p))
	}
	line := "FILTERS(status=open|all|completed|archived)  " + strings.Join(parts, " ")
	return clampLine(line, cols)
}

func (m *model) renderChip(k string, v string, focused bool, _ palette) string {
	raw := fmt.Sprintf("[%s:%s]", k, v)
	if m.plain {
		return raw
	}
	if focused {
		return "\x1b[1m" + raw + "\x1b[0m"
	}
	return raw
}

func singleValueList(v string) []string {
	t := strings.TrimSpace(v)
	if t == "" {
		return nil
	}
	return []string{t}
}

func mergeTokenValues(base []string, extra []string) []string {
	out := make([]string, 0, len(base)+len(extra))
	for _, v := range base {
		t := strings.TrimSpace(v)
		if t == "" || containsExact(out, t) {
			continue
		}
		out = append(out, t)
	}
	for _, v := range extra {
		t := strings.TrimSpace(v)
		if t == "" || containsExact(out, t) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (m *model) renderTextChipValue(qf quickFilter, p palette) string {
	parts := make([]string, 0, 8)
	if strings.TrimSpace(qf.Text) != "" {
		parts = append(parts, qf.Text)
	}
	for _, a := range qf.Areas {
		parts = append(parts, m.renderQuickTokenChip("~["+a+"]", "area", p))
	}
	for _, g := range qf.Goals {
		parts = append(parts, m.renderQuickTokenChip("^["+g+"]", "goal", p))
	}
	if qf.HasDue {
		parts = append(parts, m.renderQuickTokenChip("!due", "due", p))
	}
	if qf.TodayDue {
		parts = append(parts, m.renderQuickTokenChip("!todaydue", "due", p))
	}
	if qf.NoDue {
		parts = append(parts, m.renderQuickTokenChip("!nodue", "due", p))
	}
	if qf.Overdue {
		parts = append(parts, m.renderQuickTokenChip("!overdue", "overdue", p))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(limitTokens(parts, 6), " ")
}

func limitTokens(tokens []string, max int) []string {
	if len(tokens) <= max {
		return tokens
	}
	out := append([]string{}, tokens[:max-1]...)
	out = append(out, fmt.Sprintf("+%d", len(tokens)-(max-1)))
	return out
}

func (m *model) renderQuickTokenChip(token string, kind string, p palette) string {
	if m.plain {
		return token
	}
	theme := m.themes[m.themeIndex]
	bgHex := strings.TrimSpace(theme.Colors["panel"])
	switch kind {
	case "tag":
		bgHex = strings.TrimSpace(theme.Colors["chart1"])
	case "area":
		bgHex = strings.TrimSpace(theme.Colors["chart3"])
	case "goal":
		bgHex = strings.TrimSpace(theme.Colors["chart2"])
	case "assignee":
		bgHex = strings.TrimSpace(theme.Colors["accent2"])
	case "due":
		bgHex = strings.TrimSpace(theme.Colors["warning"])
	case "overdue":
		bgHex = strings.TrimSpace(theme.Colors["danger"])
	}
	if bgHex == "" {
		return token
	}
	fgHex := readableTextHex(bgHex)
	if rbg, gbg, bbg, ok := parseHexColor(bgHex); ok {
		if rfg, gfg, bfg, ok := parseHexColor(fgHex); ok {
			return fmt.Sprintf("\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm%s\x1b[0m", rfg, gfg, bfg, rbg, gbg, bbg, token)
		}
	}
	return m.colorize(token, p.Warn)
}

func (m *model) editPrompt() string {
	field := []string{"text", "status", "priority", "tags", "assignee", "today"}[m.editField]
	switch m.editField {
	case filterFieldTags:
		return fmt.Sprintf("edit %s: %s | input=%s (Enter selects highlighted, Backspace removes last when input empty, Esc done)", field, strings.Join(m.fieldDraftTags, ","), m.fieldInput)
	case filterFieldAssignee:
		return fmt.Sprintf("edit %s: %s (Enter select, Esc done)", field, m.fieldInput)
	case filterFieldStatus:
		return fmt.Sprintf("edit %s: %s (Up/Down cycle, Enter/Esc close)", field, m.filters.Status)
	case filterFieldPriority:
		return fmt.Sprintf("edit %s: %s (Up/Down cycle, Enter/Esc close)", field, m.filters.Priority)
	case filterFieldTodayMode:
		return fmt.Sprintf("edit %s: %s (Up/Down cycle, Enter/Esc close)", field, m.filters.TodayMode)
	default:
		return fmt.Sprintf("edit %s: %s (live fuzzy, Enter/Esc close)", field, m.fieldInput)
	}
}

func (m *model) renderDropdown(cols int, p palette) []string {
	max := m.dropdownHeight()
	if max <= 0 {
		return nil
	}
	out := make([]string, 0, max+2)
	total := len(m.dropdown)
	if total == 0 {
		return out
	}
	start := 0
	if m.dropdownIndex >= max {
		start = m.dropdownIndex - max + 1
	}
	if start < 0 {
		start = 0
	}
	end := start + max
	if end > total {
		end = total
	}
	if start > 0 {
		out = append(out, m.paintMuted(clampLine("  ↑ more", cols), p))
	}
	for i := start; i < end; i++ {
		prefix := "  "
		if i == m.dropdownIndex {
			prefix = "> "
		}
		line := fmt.Sprintf("%s%s (%d)", prefix, m.dropdown[i].Name, m.dropdown[i].Count)
		if i == m.dropdownIndex {
			line = m.paintSelected(line, p)
		} else {
			line = m.paintMuted(line, p)
		}
		out = append(out, clampLine(line, cols))
	}
	if end < total {
		out = append(out, m.paintMuted(clampLine("  ↓ more", cols), p))
	}
	return out
}

func (m *model) dropdownHeight() int {
	if !m.dropdownOpen {
		return 0
	}
	if len(m.dropdown) > 6 {
		return 6
	}
	return len(m.dropdown)
}

func (m *model) computeFacetOptions(field int) ([]FacetOption, error) {
	rows, err := m.loadRows()
	if err != nil {
		return nil, err
	}
	filtered := m.applyFiltersExcluding(rows, field)
	switch field {
	case filterFieldTags:
		counts := map[string]int{}
		for _, r := range filtered {
			seen := map[string]bool{}
			for _, tag := range splitCSV(r.Tags) {
				if tag == "" || seen[strings.ToLower(tag)] {
					continue
				}
				seen[strings.ToLower(tag)] = true
				counts[tag]++
			}
		}
		return mapToFacetOptions(counts), nil
	case filterFieldAssignee:
		counts := map[string]int{}
		for _, r := range filtered {
			seen := map[string]bool{}
			for _, name := range splitCSV(r.Assignees) {
				if name == "" || seen[strings.ToLower(name)] {
					continue
				}
				seen[strings.ToLower(name)] = true
				counts[name]++
			}
		}
		return mapToFacetOptions(counts), nil
	case filterFieldStatus:
		return []FacetOption{{Name: "open"}, {Name: "all"}, {Name: "completed"}, {Name: "archived"}}, nil
	case filterFieldPriority:
		return []FacetOption{{Name: "all"}, {Name: "now"}, {Name: "soon"}, {Name: "later"}}, nil
	case filterFieldTodayMode:
		return []FacetOption{{Name: "mine"}, {Name: "all"}}, nil
	default:
		return nil, nil
	}
}

func mapToFacetOptions(counts map[string]int) []FacetOption {
	out := make([]FacetOption, 0, len(counts))
	for k, v := range counts {
		out = append(out, FacetOption{Name: k, Count: v})
	}
	sortFacetOptions(out)
	return out
}

func sortFacetOptions(in []FacetOption) {
	for i := 0; i < len(in); i++ {
		for j := i + 1; j < len(in); j++ {
			if in[j].Count > in[i].Count || (in[j].Count == in[i].Count && strings.ToLower(in[j].Name) < strings.ToLower(in[i].Name)) {
				in[i], in[j] = in[j], in[i]
			}
		}
	}
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func splitCSVLower(v string) map[string]bool {
	out := map[string]bool{}
	for _, p := range splitCSV(v) {
		out[strings.ToLower(p)] = true
	}
	return out
}

func containsAllTags(rowTags map[string]bool, selected []string) bool {
	for _, t := range selected {
		if !rowTags[strings.ToLower(strings.TrimSpace(t))] {
			return false
		}
	}
	return true
}

func containsExact(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func (m *model) applyFiltersExcluding(in []row, excludeField int) []row {
	qf := parseQuickFilter(m.filters.Text)
	requiredTags := append([]string{}, m.filters.Tags...)
	requiredTags = append(requiredTags, qf.Tags...)
	now := time.Now()
	out := make([]row, 0, len(in))
	for _, r := range in {
		if excludeField != filterFieldStatus && m.filters.Status != "all" && r.Status != m.filters.Status {
			continue
		}
		if excludeField != filterFieldPriority && m.filters.Priority != "all" && r.Priority != m.filters.Priority {
			continue
		}
		if excludeField != filterFieldTags && len(requiredTags) > 0 {
			rowTags := splitCSVLower(r.Tags)
			if !containsAllTags(rowTags, requiredTags) {
				continue
			}
		}
		if excludeField != filterFieldAssignee && m.filters.Assignee != "" && !fuzzyMatch(strings.ToLower(r.Assignees), strings.ToLower(m.filters.Assignee)) {
			continue
		}
		if m.filters.ScopeKind == "project" && !containsToken(r.ProjectIDs, m.filters.ScopeID) {
			continue
		}
		if m.filters.ScopeKind == "goal" && !containsToken(r.GoalIDs, m.filters.ScopeID) {
			continue
		}
		if m.filters.ScopeKind == "assignee" && !containsToken(r.AssigneeIDs, m.filters.ScopeID) {
			continue
		}
		if excludeField != filterFieldText {
			if len(qf.Areas) > 0 && !matchesAllInCSV(r.Areas, qf.Areas) {
				continue
			}
			if len(qf.Goals) > 0 && !matchesAllInCSV(r.Goals, qf.Goals) {
				continue
			}
			if len(qf.Assignees) > 0 && !matchesAllInCSV(r.Assignees, qf.Assignees) {
				continue
			}
			if qf.HasDue && strings.TrimSpace(r.DueAt) == "" {
				continue
			}
			if qf.NoDue && strings.TrimSpace(r.DueAt) != "" {
				continue
			}
			if qf.TodayDue && !isTodayDue(r, m.loc, now) {
				continue
			}
			if qf.Overdue && !isOverdue(r, now, m.loc) {
				continue
			}
		}
		if excludeField != filterFieldText && m.filters.Text != "" {
			if strings.TrimSpace(qf.Text) != "" {
				h := strings.ToLower(strings.Join([]string{r.Title, r.ID, r.Priority, r.Collection, r.Tags, r.Assignees}, " "))
				if !fuzzyMatch(h, strings.ToLower(qf.Text)) {
					continue
				}
			}
		}
		out = append(out, r)
	}
	return out
}

type quickFilter struct {
	Text      string
	Tags      []string
	Areas     []string
	Goals     []string
	Assignees []string
	Overdue   bool
	HasDue    bool
	NoDue     bool
	TodayDue  bool
}

func parseQuickFilter(input string) quickFilter {
	rawTokens := splitFilterTokens(input)
	q := quickFilter{}
	textTerms := make([]string, 0, len(rawTokens))
	for _, tok := range rawTokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		switch {
		case strings.EqualFold(tok, "!overdue"):
			q.Overdue = true
		case strings.EqualFold(tok, "!due"):
			q.HasDue = true
		case strings.EqualFold(tok, "!nodue"):
			q.NoDue = true
		case strings.EqualFold(tok, "!todaydue"):
			q.TodayDue = true
		case strings.HasPrefix(tok, "#"):
			if v := parseTypedToken(tok); v != "" && !containsExact(q.Tags, v) {
				q.Tags = append(q.Tags, v)
			}
		case strings.HasPrefix(tok, "~"):
			if v := parseTypedToken(tok); v != "" && !containsExact(q.Areas, v) {
				q.Areas = append(q.Areas, v)
			}
		case strings.HasPrefix(tok, "^"):
			if v := parseTypedToken(tok); v != "" && !containsExact(q.Goals, v) {
				q.Goals = append(q.Goals, v)
			}
		case strings.HasPrefix(tok, "@"):
			if v := parseTypedToken(tok); v != "" && !containsExact(q.Assignees, v) {
				q.Assignees = append(q.Assignees, v)
			}
		default:
			textTerms = append(textTerms, tok)
		}
	}
	q.Text = strings.TrimSpace(strings.Join(textTerms, " "))
	return q
}

func normalizeQuickToken(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "\"")
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		v = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(v, "["), "]"))
	}
	return strings.TrimSpace(v)
}

func parseTypedToken(tok string) string {
	if strings.TrimSpace(tok) == "" {
		return ""
	}
	return normalizeQuickToken(tok[1:])
}

func splitFilterTokens(input string) []string {
	in := strings.TrimSpace(input)
	if in == "" {
		return nil
	}
	var out []string
	var b strings.Builder
	inQuote := false
	runes := []rune(in)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		// Canonical bracket form for typed filters, supports spaces without escaping:
		// ~[Maple Trees], #[Deep Work], ^[Weekly Goal], @[Human Demo]
		if !inQuote && b.Len() == 0 && (r == '#' || r == '~' || r == '^' || r == '@') && i+1 < len(runes) && runes[i+1] == '[' {
			j := i + 2
			for j < len(runes) && runes[j] != ']' {
				j++
			}
			if j < len(runes) && runes[j] == ']' {
				out = append(out, string(runes[i:j+1]))
				i = j
				continue
			}
		}
		switch r {
		case '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case ' ', '\t':
			if inQuote {
				b.WriteRune(r)
				continue
			}
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func matchesAllInCSV(csv string, wants []string) bool {
	values := splitCSV(csv)
	if len(values) == 0 {
		return false
	}
	for _, want := range wants {
		want = strings.ToLower(strings.TrimSpace(want))
		if want == "" {
			continue
		}
		match := false
		for _, v := range values {
			lv := strings.ToLower(strings.TrimSpace(v))
			if fuzzyMatch(lv, want) || strings.Contains(lv, want) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}

func sortRows(rows []row, mode string, dir string, loc *time.Location) {
	mode = strings.TrimSpace(mode)
	dir = normalizeSortDirection(dir)
	sort.SliceStable(rows, func(i, j int) bool {
		if mode == "scheduled" {
			_, iOK := scheduledUnix(rows[i].Scheduled, loc)
			_, jOK := scheduledUnix(rows[j].Scheduled, loc)
			if iOK != jOK {
				return iOK
			}
		}
		cmp := compareRows(rows[i], rows[j], mode, loc)
		if cmp == 0 {
			return false
		}
		if dir == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
}

func compareRows(a row, b row, mode string, loc *time.Location) int {
	switch mode {
	case "priority":
		pa := priorityRank(a.Priority)
		pb := priorityRank(b.Priority)
		if pa != pb {
			return compareInt64(int64(pa), int64(pb))
		}
	case "scheduled":
		sa, saOK := scheduledUnix(a.Scheduled, loc)
		sb, sbOK := scheduledUnix(b.Scheduled, loc)
		if saOK && sbOK && sa != sb {
			return compareInt64(sa, sb)
		}
	default:
		ua := updatedUnix(a.UpdatedAt, loc)
		ub := updatedUnix(b.UpdatedAt, loc)
		if ua != ub {
			return compareInt64(ua, ub)
		}
	}
	ua := updatedUnix(a.UpdatedAt, loc)
	ub := updatedUnix(b.UpdatedAt, loc)
	if ua != ub {
		return compareInt64(ua, ub)
	}
	return compareText(strings.ToLower(strings.TrimSpace(a.ID)), strings.ToLower(strings.TrimSpace(b.ID)))
}

func compareInt64(a int64, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareText(a string, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func normalizeSortDirection(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "asc") {
		return "asc"
	}
	return "desc"
}

func defaultSortDirection(mode string) string {
	switch strings.TrimSpace(mode) {
	case "priority", "scheduled":
		return "asc"
	default:
		return "desc"
	}
}

func toggleSortDirection(v string) string {
	if normalizeSortDirection(v) == "asc" {
		return "desc"
	}
	return "asc"
}

func priorityRank(v string) int {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "now":
		return 0
	case "soon":
		return 1
	case "later":
		return 2
	default:
		return 99
	}
}

func updatedUnix(ts string, loc *time.Location) int64 {
	t, ok := parseTime(ts)
	if !ok {
		return 0
	}
	if loc == nil {
		loc = time.Local
	}
	return t.In(loc).Unix()
}

func scheduledUnix(ts string, loc *time.Location) (int64, bool) {
	t, ok := parseTime(ts)
	if !ok {
		return 0, false
	}
	if loc == nil {
		loc = time.Local
	}
	return t.In(loc).Unix(), true
}

func assigneeInitials(names string) string {
	parts := strings.Split(strings.TrimSpace(names), ",")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "-"
	}
	words := strings.Fields(strings.TrimSpace(parts[0]))
	if len(words) == 0 {
		return "-"
	}
	r := []rune(words[0])
	if len(r) == 0 {
		return "-"
	}
	return strings.ToUpper(string(r[0]))
}

func (m *model) assignedToCurrent(r row) bool {
	h := strings.TrimSpace(strings.ToLower(m.currentUserHint))
	if h == "" {
		return true
	}
	return strings.Contains(strings.ToLower(r.Assignees), h)
}

func (m *model) detailLineCount(r row, cols int, now time.Time) int {
	lines := []string{
		"title: " + r.Title,
		"due: " + NaturalDate(r.DueAt, m.loc, now),
		"scheduled: " + NaturalDate(r.Scheduled, m.loc, now),
		"updated: " + NaturalDate(r.UpdatedAt, m.loc, now),
		"projects: " + fallbackDash(strings.TrimSpace(r.Projects)),
		"goals: " + fallbackDash(strings.TrimSpace(r.Goals)),
		"areas: " + fallbackDash(strings.TrimSpace(r.Areas)),
		"groups: " + fallbackDash(strings.TrimSpace(r.Groups)),
		"tags: " + fallbackDash(strings.TrimSpace(r.Tags)),
		"assignees: " + fallbackDash(strings.TrimSpace(r.Assignees)),
	}
	total := 0
	for _, d := range lines {
		total += len(wrapText(d, cols-4))
	}
	return total
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

func dueSuffix(r row, now time.Time, loc *time.Location) string {
	if strings.TrimSpace(r.DueAt) == "" {
		return ""
	}
	if isOverdue(r, now, loc) {
		return " !"
	}
	return " ⚑"
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
		return palette{Accent: 209, BgAccent: 95, Open: 214, Done: 120, Archive: 208, Muted: 246, Warn: 220}
	case "mint-circuit":
		return palette{Accent: 79, BgAccent: 23, Open: 50, Done: 84, Archive: 117, Muted: 245, Warn: 159}
	case "paper-fruit":
		return palette{Accent: 167, BgAccent: 224, Open: 174, Done: 107, Archive: 131, Muted: 246, Warn: 180}
	case "midnight-arcade":
		return palette{Accent: 45, BgAccent: 17, Open: 81, Done: 119, Archive: 39, Muted: 110, Warn: 228}
	default:
		return palette{Accent: 81, BgAccent: 0, Open: 39, Done: 120, Archive: 208, Muted: 245, Warn: 220}
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

func (m *model) paintBanner(s string, cols int, p palette) string {
	line := fitLine(s, cols)
	if m.plain {
		return line
	}
	if strings.TrimSpace(m.scopeColor) != "" {
		theme := m.themes[m.themeIndex]
		bgHex := strings.TrimSpace(theme.Colors["panel"])
		if bgHex == "" {
			bgHex = strings.TrimSpace(theme.Colors["background"])
		}
		if rbg, gbg, bbg, ok := parseHexColor(bgHex); ok {
			if rfg, gfg, bfg, ok := parseHexColor(strings.TrimSpace(m.scopeColor)); ok {
				return fmt.Sprintf("\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm%s\x1b[K\x1b[0m", rfg, gfg, bfg, rbg, gbg, bbg, line)
			}
		}
	}
	if p.BgAccent > 0 {
		return fmt.Sprintf("\x1b[38;5;%d;48;5;%dm%s\x1b[K\x1b[0m", p.Accent, p.BgAccent, line)
	}
	return m.paintAccent(line, p.Accent)
}

func (m *model) paintTitleBar(s string, cols int, p palette) string {
	line := fitLine(s, cols)
	if m.plain {
		return line
	}
	theme := m.themes[m.themeIndex]
	bgHex := strings.TrimSpace(theme.Colors["panel"])
	if bgHex == "" {
		bgHex = strings.TrimSpace(theme.Colors["background"])
	}
	fgHex := strings.TrimSpace(theme.Colors["text"])
	if bgHex != "" {
		if strings.TrimSpace(fgHex) == "" {
			fgHex = readableTextHex(bgHex)
		} else if !hasReadableContrast(bgHex, fgHex) {
			fgHex = readableTextHex(bgHex)
		}
		if rbg, gbg, bbg, ok := parseHexColor(bgHex); ok {
			if rfg, gfg, bfg, ok := parseHexColor(fgHex); ok {
				return fmt.Sprintf("\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm%s\x1b[K\x1b[0m", rfg, gfg, bfg, rbg, gbg, bbg, line)
			}
		}
	}
	if p.BgAccent > 0 {
		return m.colorizeBg(line, p.Accent, p.BgAccent)
	}
	return m.paintAccent(line, p.Accent)
}

func (m *model) paintFilterBarLine(s string, cols int, _ palette) string {
	line := fitLine(s, cols)
	if m.plain {
		return line
	}
	// Keep this row unwrapped so token chips can safely apply their own colors.
	return line
}

func (m *model) paintSubheaderLine(s string, cols int, p palette) string {
	line := fitLine(s, cols)
	if m.plain {
		return line
	}
	theme := m.themes[m.themeIndex]
	bgHex := strings.TrimSpace(theme.Colors["panel"])
	fgHex := strings.TrimSpace(theme.Colors["text"])
	if bgHex != "" {
		if fgHex == "" || !hasReadableContrast(bgHex, fgHex) {
			fgHex = readableTextHex(bgHex)
		}
		if rbg, gbg, bbg, ok := parseHexColor(bgHex); ok {
			if rfg, gfg, bfg, ok := parseHexColor(fgHex); ok {
				return fmt.Sprintf("\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm%s\x1b[K\x1b[0m", rfg, gfg, bfg, rbg, gbg, bbg, line)
			}
		}
	}
	return m.paintMuted(line, p)
}

func (m *model) paintFooterLine(s string, cols int, p palette) string {
	line := fitLine(s, cols)
	if m.plain {
		return line
	}
	theme := m.themes[m.themeIndex]
	bgHex := strings.TrimSpace(theme.Colors["panel"])
	fgHex := strings.TrimSpace(theme.Colors["text"])
	if bgHex != "" {
		if fgHex == "" || !hasReadableContrast(bgHex, fgHex) {
			fgHex = readableTextHex(bgHex)
		}
		if rbg, gbg, bbg, ok := parseHexColor(bgHex); ok {
			if rfg, gfg, bfg, ok := parseHexColor(fgHex); ok {
				return fmt.Sprintf("\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm%s\x1b[K\x1b[0m", rfg, gfg, bfg, rbg, gbg, bbg, line)
			}
		}
	}
	return m.paintMuted(line, p)
}

func (m *model) paintMuted(s string, p palette) string {
	return m.colorize(s, p.Muted)
}

func (m *model) paintStatus(s string, status string, p palette) string {
	if m.plain {
		return s
	}
	theme := m.themes[m.themeIndex]
	colorHex := strings.TrimSpace(theme.Colors["accent"])
	switch status {
	case "completed":
		colorHex = strings.TrimSpace(theme.Colors["success"])
	case "archived":
		colorHex = strings.TrimSpace(theme.Colors["muted"])
	case "open":
		colorHex = strings.TrimSpace(theme.Colors["accent"])
	}
	if colorHex != "" {
		return m.paintHex(s, colorHex)
	}
	return m.colorize(s, p.Open)
}

func (m *model) paintStatusMarker(line string, marker string, status string, p palette) string {
	if m.plain {
		return line
	}
	return strings.Replace(line, marker, m.paintStatus(marker, status, p), 1)
}

func (m *model) paintDueSuffix(line string, title string, r row, now time.Time, p palette) string {
	if m.plain {
		return line
	}
	theme := m.themes[m.themeIndex]
	warnHex := strings.TrimSpace(theme.Colors["warning"])
	dangerHex := strings.TrimSpace(theme.Colors["danger"])
	if isOverdue(r, now, m.loc) {
		if dangerHex != "" {
			return strings.Replace(line, " !", m.paintHex(" !", dangerHex), 1)
		}
		return strings.Replace(line, " !", m.colorize(" !", 203), 1)
	}
	if strings.HasSuffix(title, " ⚑") {
		if warnHex != "" {
			return strings.Replace(line, " ⚑", m.paintHex(" ⚑", warnHex), 1)
		}
		return strings.Replace(line, " ⚑", m.colorize(" ⚑", p.Warn), 1)
	}
	return line
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

func (m *model) colorizeBg(s string, fg int, bg int) string {
	if m.plain {
		return s
	}
	return fmt.Sprintf("\x1b[38;5;%d;48;5;%dm%s\x1b[0m", fg, bg, s)
}

func (m *model) paintCollectionLine(line string, p palette) string {
	if m.plain {
		return line
	}
	theme := m.themes[m.themeIndex]
	chart1 := strings.TrimSpace(theme.Colors["chart1"])
	chart2 := strings.TrimSpace(theme.Colors["chart2"])
	chart3 := strings.TrimSpace(theme.Colors["chart3"])
	chart4 := strings.TrimSpace(theme.Colors["chart4"])
	accent2 := strings.TrimSpace(theme.Colors["accent2"])
	muted := strings.TrimSpace(theme.Colors["muted"])
	switch {
	case strings.Contains(line, "projects:"):
		if chart1 != "" {
			return m.paintHex(line, chart1)
		}
	case strings.Contains(line, "goals:"):
		if chart2 != "" {
			return m.paintHex(line, chart2)
		}
	case strings.Contains(line, "areas:"):
		if chart3 != "" {
			return m.paintHex(line, chart3)
		}
	case strings.Contains(line, "groups:"):
		if chart4 != "" {
			return m.paintHex(line, chart4)
		}
	case strings.Contains(line, "tags:"):
		if accent2 != "" {
			return m.paintHex(line, accent2)
		}
	}
	if muted != "" {
		return m.paintHex(line, muted)
	}
	return m.colorize(line, p.Muted)
}

func (m *model) paintSelected(s string, p palette) string {
	if m.plain {
		return s
	}
	theme := m.themes[m.themeIndex]
	bgHex := strings.TrimSpace(theme.Colors["panel"])
	fgHex := strings.TrimSpace(theme.Colors["text"])
	if bgHex != "" {
		if fgHex == "" || !hasReadableContrast(bgHex, fgHex) {
			fgHex = readableTextHex(bgHex)
		}
		if rbg, gbg, bbg, ok := parseHexColor(bgHex); ok {
			if rfg, gfg, bfg, ok := parseHexColor(fgHex); ok {
				line := fmt.Sprintf("\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm%s\x1b[0m", rfg, gfg, bfg, rbg, gbg, bbg, s)
				return strings.Replace(line, ">", fmt.Sprintf("\x1b[38;5;%dm>\x1b[0m", p.Warn), 1)
			}
		}
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
		case "[Z":
			return "shift_tab", nil
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
	case '\t':
		return "tab", nil
	case 127, 8:
		return "backspace", nil
	case '/':
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

func relativeLuminance(r, g, b int) float64 {
	lin := func(c int) float64 {
		x := float64(c) / 255.0
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func contrastRatio(bgHex, fgHex string) float64 {
	r1, g1, b1, ok1 := parseHexColor(bgHex)
	r2, g2, b2, ok2 := parseHexColor(fgHex)
	if !ok1 || !ok2 {
		return 0
	}
	l1 := relativeLuminance(r1, g1, b1)
	l2 := relativeLuminance(r2, g2, b2)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func hasReadableContrast(bgHex, fgHex string) bool {
	return contrastRatio(bgHex, fgHex) >= 4.5
}

func readableTextHex(bgHex string) string {
	black := "#111111"
	white := "#FFFFFF"
	if contrastRatio(bgHex, black) >= contrastRatio(bgHex, white) {
		return black
	}
	return white
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
