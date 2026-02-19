package tui

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"dooh/internal/db"
)

type taskItem struct {
	ID         string
	Title      string
	Status     string
	Priority   string
	DueAt      string
	Scheduled  string
	UpdatedAt  string
	Collection string
}

type collectionItem struct {
	ID    string
	Name  string
	Kind  string
	Color string
	Count int
}

type dashboardData struct {
	Tasks       []taskItem
	Collections []collectionItem
	Counts      map[string]int
}

type app struct {
	sqlite         db.SQLite
	themes         []Theme
	themeIndex     int
	filter         string
	statusFilter   string
	priorityFilter string
	selected       int
	limit          int
	loc            *time.Location
	help           bool
}

func RunInteractive(in io.Reader, out io.Writer, sqlite db.SQLite, catalog ThemeCatalog, themeID string, filter string, limit int, loc *time.Location) error {
	if limit <= 0 {
		limit = 12
	}
	a := app{
		sqlite:         sqlite,
		themes:         catalog.Themes,
		themeIndex:     0,
		filter:         strings.TrimSpace(filter),
		statusFilter:   "all",
		priorityFilter: "all",
		selected:       0,
		limit:          limit,
		loc:            loc,
	}
	for i, t := range catalog.Themes {
		if t.ID == themeID {
			a.themeIndex = i
			break
		}
	}

	r := bufio.NewReader(in)
	for {
		rendered, err := a.render()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(out, "\x1b[2J\x1b[H")
		_, _ = fmt.Fprint(out, rendered)
		_, _ = fmt.Fprint(out, "\n\x1b[2mcmd (j/k move, /text filter, s status, p priority, t theme, h help, q quit): \x1b[0m")

		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		cmd := strings.TrimSpace(line)
		if cmd == "q" {
			_, _ = fmt.Fprint(out, "\x1b[2J\x1b[H")
			return nil
		}
		a.handle(cmd)
	}
}

func RenderDashboard(sqlite db.SQLite, theme Theme, filter string, limit int, loc *time.Location) (string, error) {
	a := app{
		sqlite:         sqlite,
		themes:         []Theme{theme},
		themeIndex:     0,
		filter:         strings.TrimSpace(filter),
		statusFilter:   "all",
		priorityFilter: "all",
		selected:       0,
		limit:          limit,
		loc:            loc,
	}
	return a.render()
}

func (a *app) handle(cmd string) {
	switch {
	case cmd == "":
		return
	case cmd == "j":
		a.selected++
	case cmd == "k":
		a.selected--
	case cmd == "s":
		a.statusFilter = cycle([]string{"all", "open", "completed", "archived"}, a.statusFilter)
		a.selected = 0
	case cmd == "p":
		a.priorityFilter = cycle([]string{"all", "now", "soon", "later"}, a.priorityFilter)
		a.selected = 0
	case cmd == "t":
		if len(a.themes) > 0 {
			a.themeIndex = (a.themeIndex + 1) % len(a.themes)
		}
	case cmd == "h":
		a.help = !a.help
	case strings.HasPrefix(cmd, "/"):
		a.filter = strings.TrimSpace(strings.TrimPrefix(cmd, "/"))
		a.selected = 0
	}
}

func (a *app) render() (string, error) {
	theme := a.themes[a.themeIndex]
	data, err := a.loadData()
	if err != nil {
		return "", err
	}

	tasks := a.filteredTasks(data.Tasks)
	if a.selected < 0 {
		a.selected = 0
	}
	if a.selected >= len(tasks) && len(tasks) > 0 {
		a.selected = len(tasks) - 1
	}
	if len(tasks) == 0 {
		a.selected = 0
	}

	now := time.Now()
	var b strings.Builder
	fg := func(hex, text string) string {
		r, g, bl := hexToRGB(hex)
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", r, g, bl, text)
	}

	open := data.Counts["open"]
	completed := data.Counts["completed"]
	archived := data.Counts["archived"]
	total := open + completed + archived
	if total == 0 {
		total = 1
	}

	b.WriteString(fg(theme.Colors["accent"], "dooh interactive") + "  ")
	b.WriteString(fg(theme.Colors["muted"], "theme="+theme.Name+"  "))
	b.WriteString(fg(theme.Colors["muted"], "filter=/"+a.filter+"  status="+a.statusFilter+"  priority="+a.priorityFilter+"\n"))
	b.WriteString(fg(theme.Colors["muted"], strings.Repeat("-", 118)+"\n"))
	b.WriteString(fmt.Sprintf("open %s  completed %s  archived %s\n",
		bar(open, total, theme.Colors["accent"]),
		bar(completed, total, theme.Colors["success"]),
		bar(archived, total, theme.Colors["warning"]),
	))

	left := make([]string, 0, a.limit+4)
	left = append(left, fg(theme.Colors["text"], fmt.Sprintf("Tasks (%d)", len(tasks))))
	left = append(left, fg(theme.Colors["muted"], fmt.Sprintf("%-38s %-10s %-8s %-19s", "Title", "Status", "Priority", "Updated")))
	left = append(left, fg(theme.Colors["muted"], strings.Repeat("-", 80)))

	start := 0
	if a.selected >= a.limit {
		start = a.selected - a.limit + 1
	}
	end := start + a.limit
	if end > len(tasks) {
		end = len(tasks)
	}
	for i := start; i < end; i++ {
		t := tasks[i]
		prefix := "  "
		if i == a.selected {
			prefix = fg(theme.Colors["accent2"], "> ")
		}
		stColor := theme.Colors["accent"]
		if t.Status == "completed" {
			stColor = theme.Colors["success"]
		}
		if t.Status == "archived" {
			stColor = theme.Colors["warning"]
		}
		line := fmt.Sprintf("%-38s %-10s %-8s %-19s", truncateText(t.Title, 38), t.Status, t.Priority, NaturalDate(t.UpdatedAt, a.loc, now))
		left = append(left, prefix+fg(stColor, line))
	}
	for len(left) < a.limit+6 {
		left = append(left, "")
	}

	right := make([]string, 0, a.limit+4)
	right = append(right, fg(theme.Colors["text"], "Detail"))
	right = append(right, fg(theme.Colors["muted"], strings.Repeat("-", 36)))
	if len(tasks) > 0 {
		t := tasks[a.selected]
		right = append(right, fg(theme.Colors["accent"], truncateText(t.Title, 36)))
		right = append(right, "id: "+t.ID)
		right = append(right, "status: "+t.Status)
		right = append(right, "priority: "+t.Priority)
		right = append(right, "due: "+NaturalDate(t.DueAt, a.loc, now))
		right = append(right, "scheduled: "+NaturalDate(t.Scheduled, a.loc, now))
		right = append(right, "updated: "+NaturalDate(t.UpdatedAt, a.loc, now))
		right = append(right, "collections: "+truncateText(t.Collection, 36))
	} else {
		right = append(right, fg(theme.Colors["muted"], "No tasks match current filters."))
	}
	right = append(right, "")
	right = append(right, fg(theme.Colors["text"], "Collections"))
	for i, c := range data.Collections {
		if i >= 8 {
			break
		}
		line := fmt.Sprintf("%-12s %-7s %2d", truncateText(c.Name, 12), c.Kind, c.Count)
		right = append(right, fg(c.Color, line))
	}
	if a.help {
		right = append(right, "")
		right = append(right, fg(theme.Colors["muted"], "help"))
		right = append(right, "j/k: select")
		right = append(right, "/text: filter")
		right = append(right, "s: status  p: priority")
		right = append(right, "t: theme  q: quit")
	}

	b.WriteString(mergeColumns(left, right, 82))
	return b.String(), nil
}

func (a *app) filteredTasks(in []taskItem) []taskItem {
	f := strings.ToLower(strings.TrimSpace(a.filter))
	out := make([]taskItem, 0, len(in))
	for _, t := range in {
		if a.statusFilter != "all" && t.Status != a.statusFilter {
			continue
		}
		if a.priorityFilter != "all" && t.Priority != a.priorityFilter {
			continue
		}
		if f != "" {
			h := strings.ToLower(strings.Join([]string{t.Title, t.Status, t.Priority, t.ID, t.Collection}, " "))
			if !strings.Contains(h, f) {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

func (a *app) loadData() (dashboardData, error) {
	var d dashboardData
	d.Counts = map[string]int{"open": 0, "completed": 0, "archived": 0}

	taskRows, err := a.sqlite.QueryTSV(`
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
		return d, err
	}
	d.Tasks = make([]taskItem, 0, len(taskRows))
	for _, r := range taskRows {
		if len(r) < 8 {
			continue
		}
		d.Tasks = append(d.Tasks, taskItem{
			ID:         r[0],
			Title:      r[1],
			Status:     r[2],
			Priority:   r[3],
			DueAt:      r[4],
			Scheduled:  r[5],
			UpdatedAt:  r[6],
			Collection: r[7],
		})
	}

	metricsRows, err := a.sqlite.QueryTSV("SELECT status,COUNT(*) FROM tasks WHERE deleted_at IS NULL GROUP BY status;")
	if err != nil {
		return d, err
	}
	for _, r := range metricsRows {
		if len(r) < 2 {
			continue
		}
		n, _ := strconv.Atoi(r[1])
		d.Counts[r[0]] = n
	}

	collectionRows, err := a.sqlite.QueryTSV("SELECT c.short_id,c.name,c.kind,c.color_hex,COUNT(tc.task_id) FROM collections c LEFT JOIN task_collections tc ON c.id=tc.collection_id WHERE c.deleted_at IS NULL GROUP BY c.id ORDER BY c.updated_at DESC;")
	if err != nil {
		return d, err
	}
	d.Collections = make([]collectionItem, 0, len(collectionRows))
	for _, r := range collectionRows {
		if len(r) < 5 {
			continue
		}
		n, _ := strconv.Atoi(r[4])
		d.Collections = append(d.Collections, collectionItem{
			ID:    r[0],
			Name:  r[1],
			Kind:  r[2],
			Color: r[3],
			Count: n,
		})
	}

	return d, nil
}

func cycle(values []string, current string) string {
	if len(values) == 0 {
		return current
	}
	for i, v := range values {
		if v == current {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}

func truncateText(s string, n int) string {
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

func mergeColumns(left []string, right []string, leftWidth int) string {
	max := len(left)
	if len(right) > max {
		max = len(right)
	}
	if leftWidth < 20 {
		leftWidth = 20
	}
	var b strings.Builder
	for i := 0; i < max; i++ {
		l := ""
		r := ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		b.WriteString(padANSI(l, leftWidth))
		b.WriteString("  ")
		b.WriteString(r)
		b.WriteString("\n")
	}
	return b.String()
}

func padANSI(s string, width int) string {
	plain := stripANSI(s)
	if len(plain) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(plain))
}

func stripANSI(s string) string {
	var b strings.Builder
	esc := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if esc {
			if ch >= 'A' && ch <= 'z' {
				esc = false
			}
			continue
		}
		if ch == 0x1b {
			esc = true
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}
