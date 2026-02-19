package demo

import (
	"fmt"
	"strings"
	"time"

	"dooh/internal/db"
	"dooh/internal/idgen"
)

type SeedResult struct {
	Users       int
	Collections int
	Tasks       int
}

func Seed(sqlite db.SQLite) (SeedResult, error) {
	res := SeedResult{}

	humanID, created, err := ensureUser(sqlite, "Human Demo")
	if err != nil {
		return res, err
	}
	if created {
		res.Users++
	}

	agentID, created, err := ensureUser(sqlite, "Agent Demo")
	if err != nil {
		return res, err
	}
	if created {
		res.Users++
	}

	partnerID, created, err := ensureUser(sqlite, "Partner Demo")
	if err != nil {
		return res, err
	}
	if created {
		res.Users++
	}

	collections := []struct {
		Name  string
		Kind  string
		Color string
	}{
		{"Atlas Rewrite", "project", "#4D96FF"},
		{"Ops Automation", "project", "#5AB0FF"},
		{"Launch Readiness", "project", "#89C2FF"},
		{"Q1 Reliability Goal", "goal", "#FFD166"},
		{"Personal Systems Goal", "goal", "#E9C46A"},
		{"Work Area", "area", "#FF7A59"},
		{"Home Area", "area", "#F28482"},
		{"Platform Group", "class", "#70E000"},
		{"Growth Group", "class", "#80ED99"},
		{"Deep Work", "tag", "#2EC4B6"},
		{"Bugs", "tag", "#FF6B6B"},
		{"Docs", "tag", "#A9DEF9"},
	}

	collectionIDs := make([]string, 0, len(collections))
	for _, c := range collections {
		id, _, wasCreated, err := ensureCollection(sqlite, humanID, c.Name, c.Kind, c.Color)
		if err != nil {
			return res, err
		}
		if wasCreated {
			res.Collections++
		}
		collectionIDs = append(collectionIDs, id)
	}

	now := time.Now().UTC()
	tasks := []struct {
		Title      string
		Priority   string
		Status     string
		DaysDue    int
		DaysSch    int
		Owner      string
		Assignees  []string
		Collection []int
	}{
		{"Ship profile-aware auth flow", "now", "open", 1, 0, humanID, []string{humanID}, []int{0, 3, 5, 7, 9}},
		{"Polish TUI dashboard visuals", "soon", "open", 3, 1, agentID, []string{agentID}, []int{0, 3, 7, 9, 11}},
		{"Archive completed sprint tasks", "later", "completed", -1, -2, humanID, []string{humanID}, []int{1, 4, 5, 8}},
		{"Refine export metrics schema", "soon", "open", 4, 2, agentID, []string{agentID, humanID}, []int{1, 3, 7, 10}},
		{"Investigate flaky migration test", "now", "open", 0, 0, humanID, []string{humanID, agentID}, []int{0, 3, 7, 10}},
		{"Write weekly goals review", "later", "archived", -5, -6, humanID, []string{humanID}, []int{4, 5, 11}},
		{"Implement dependency cycle guard", "now", "open", 2, 1, agentID, []string{agentID}, []int{0, 3, 7, 10}},
		{"Tune collection color assignment", "soon", "completed", -2, -3, agentID, []string{agentID}, []int{1, 4, 9}},
		{"Prepare launch checklist", "later", "open", 7, 5, humanID, []string{humanID, partnerID}, []int{2, 3, 5, 11}},
		{"Backfill docs for rollback flow", "soon", "open", 5, 3, agentID, []string{agentID}, []int{2, 3, 11}},
		{"Draft onboarding playbook", "soon", "open", 2, 0, partnerID, []string{partnerID}, []int{2, 4, 8, 11}},
		{"Fix weekend rollover bug", "now", "open", -2, -1, agentID, []string{agentID, humanID}, []int{1, 3, 7, 10}},
		{"Home network backup audit", "later", "open", 6, 0, humanID, []string{humanID}, []int{4, 6, 8}},
		{"Pay quarterly cloud invoices", "later", "completed", -1, -1, humanID, []string{humanID}, []int{5, 8}},
		{"Review agent prompt safety", "now", "open", 1, 0, humanID, []string{humanID, partnerID}, []int{0, 3, 5, 11}},
		{"Tag cleanup for old tasks", "soon", "archived", -8, -9, agentID, []string{agentID}, []int{1, 4, 9}},
		{"Benchmark sqlite lock waits", "soon", "open", 2, 1, agentID, []string{agentID}, []int{1, 3, 7}},
		{"Create monthly goals template", "later", "open", 8, 4, partnerID, []string{partnerID, humanID}, []int{4, 8, 11}},
	}

	for _, t := range tasks {
		due := now.AddDate(0, 0, t.DaysDue).Format(time.RFC3339)
		sch := now.AddDate(0, 0, t.DaysSch).Format(time.RFC3339)
		if hasGoalCollection(t.Collection) {
			// Keep due dates on a minority of goal-linked tasks.
			titleLower := strings.ToLower(t.Title)
			if !strings.Contains(titleLower, "checklist") && !strings.Contains(titleLower, "migration") {
				due = ""
			}
		}
		taskID, created, err := ensureTask(sqlite, t.Title, t.Priority, t.Status, due, sch, t.Owner)
		if err != nil {
			return res, err
		}
		if created {
			res.Tasks++
		}
		for _, idx := range t.Collection {
			if idx < 0 || idx >= len(collectionIDs) {
				continue
			}
			if err := sqlite.Exec(fmt.Sprintf("INSERT OR IGNORE INTO task_collections(task_id,collection_id) VALUES(%s,%s);", db.Quote(taskID), db.Quote(collectionIDs[idx]))); err != nil {
				return res, err
			}
		}
		for _, assigneeID := range t.Assignees {
			if err := sqlite.Exec(fmt.Sprintf("INSERT OR IGNORE INTO task_assignees(task_id,user_id) VALUES(%s,%s);", db.Quote(taskID), db.Quote(assigneeID))); err != nil {
				return res, err
			}
		}
	}

	return res, nil
}

func hasGoalCollection(indices []int) bool {
	for _, idx := range indices {
		if idx == 3 || idx == 4 {
			return true
		}
	}
	return false
}

func ensureUser(sqlite db.SQLite, name string) (id string, created bool, err error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT id FROM users WHERE name=%s LIMIT 1;", db.Quote(name)))
	if err != nil {
		return "", false, err
	}
	if len(rows) > 0 && len(rows[0]) > 0 {
		return rows[0][0], false, nil
	}
	id, err = idgen.ULIDLike()
	if err != nil {
		return "", false, err
	}
	if err := sqlite.Exec(fmt.Sprintf("INSERT INTO users(id,name,status) VALUES(%s,%s,'active');", db.Quote(id), db.Quote(name))); err != nil {
		return "", false, err
	}
	return id, true, nil
}

func ensureCollection(sqlite db.SQLite, ownerID string, name string, kind string, color string) (id string, shortID string, created bool, err error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT id,short_id FROM collections WHERE name=%s LIMIT 1;", db.Quote(name)))
	if err != nil {
		return "", "", false, err
	}
	if len(rows) > 0 && len(rows[0]) >= 2 {
		return rows[0][0], rows[0][1], false, nil
	}
	id, err = idgen.ULIDLike()
	if err != nil {
		return "", "", false, err
	}
	shortID, err = idgen.Short("c")
	if err != nil {
		return "", "", false, err
	}
	sql := fmt.Sprintf("INSERT INTO collections(id,short_id,name,kind,color_hex,created_by,updated_by) VALUES(%s,%s,%s,%s,%s,%s,%s);",
		db.Quote(id), db.Quote(shortID), db.Quote(name), db.Quote(kind), db.Quote(color), db.Quote(ownerID), db.Quote(ownerID))
	if err := sqlite.Exec(sql); err != nil {
		return "", "", false, err
	}
	return id, shortID, true, nil
}

func ensureTask(sqlite db.SQLite, title string, priority string, status string, due string, scheduled string, ownerID string) (string, bool, error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT id FROM tasks WHERE title=%s LIMIT 1;", db.Quote(title)))
	if err != nil {
		return "", false, err
	}
	if len(rows) > 0 && len(rows[0]) > 0 {
		return rows[0][0], false, nil
	}
	id, err := idgen.ULIDLike()
	if err != nil {
		return "", false, err
	}
	shortID, err := idgen.Short("t")
	if err != nil {
		return "", false, err
	}
	completedAt := "NULL"
	archivedAt := "NULL"
	if status == "completed" {
		completedAt = "strftime('%Y-%m-%dT%H:%M:%fZ','now')"
	}
	if status == "archived" {
		archivedAt = "strftime('%Y-%m-%dT%H:%M:%fZ','now')"
	}
	sql := strings.Join([]string{
		"INSERT INTO tasks(id,short_id,title,status,priority,due_at,scheduled_at,completed_at,archived_at,created_by,updated_by)",
		fmt.Sprintf("VALUES(%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s);",
			db.Quote(id), db.Quote(shortID), db.Quote(title), db.Quote(status), db.Quote(priority), db.Quote(due), db.Quote(scheduled), completedAt, archivedAt, db.Quote(ownerID), db.Quote(ownerID)),
	}, " ")
	if err := sqlite.Exec(sql); err != nil {
		return "", false, err
	}
	return id, true, nil
}
