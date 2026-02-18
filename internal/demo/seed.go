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

	collections := []struct {
		Name  string
		Kind  string
		Color string
	}{
		{"Project Atlas", "project", "#4D96FF"},
		{"Q1 Goals", "goal", "#FFD166"},
		{"Deep Work", "tag", "#2EC4B6"},
		{"Personal Ops", "area", "#FF7A59"},
		{"Engineering", "class", "#70E000"},
		{"Bugs", "tag", "#FF6B6B"},
	}

	collectionIDs := make([]string, 0, len(collections))
	for _, c := range collections {
		id, shortID, wasCreated, err := ensureCollection(sqlite, humanID, c.Name, c.Kind, c.Color)
		if err != nil {
			return res, err
		}
		if wasCreated {
			res.Collections++
		}
		_ = shortID
		collectionIDs = append(collectionIDs, id)
	}

	now := time.Now().UTC()
	tasks := []struct {
		Title    string
		Priority string
		Status   string
		DaysDue  int
		DaysSch  int
		Owner    string
	}{
		{"Ship profile-aware auth flow", "now", "open", 1, 0, humanID},
		{"Polish TUI dashboard visuals", "soon", "open", 3, 1, agentID},
		{"Archive completed sprint tasks", "later", "completed", -1, -2, humanID},
		{"Refine export metrics schema", "soon", "open", 4, 2, agentID},
		{"Investigate flaky migration test", "now", "open", 0, 0, humanID},
		{"Write weekly goals review", "later", "archived", -5, -6, humanID},
		{"Implement dependency cycle guard", "now", "open", 2, 1, agentID},
		{"Tune collection color assignment", "soon", "completed", -2, -3, agentID},
		{"Prepare launch checklist", "later", "open", 7, 5, humanID},
		{"Backfill docs for rollback flow", "soon", "open", 5, 3, agentID},
	}

	for i, t := range tasks {
		due := now.AddDate(0, 0, t.DaysDue).Format(time.RFC3339)
		sch := now.AddDate(0, 0, t.DaysSch).Format(time.RFC3339)
		if err := ensureTask(sqlite, t.Title, t.Priority, t.Status, due, sch, t.Owner); err != nil {
			return res, err
		}
		res.Tasks++

		rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT id FROM tasks WHERE title=%s LIMIT 1;", db.Quote(t.Title)))
		if err != nil {
			return res, err
		}
		if len(rows) == 0 || len(rows[0]) == 0 {
			continue
		}
		taskID := rows[0][0]
		c1 := collectionIDs[i%len(collectionIDs)]
		c2 := collectionIDs[(i+2)%len(collectionIDs)]
		if err := sqlite.Exec(fmt.Sprintf("INSERT OR IGNORE INTO task_collections(task_id,collection_id) VALUES(%s,%s);", db.Quote(taskID), db.Quote(c1))); err != nil {
			return res, err
		}
		if err := sqlite.Exec(fmt.Sprintf("INSERT OR IGNORE INTO task_collections(task_id,collection_id) VALUES(%s,%s);", db.Quote(taskID), db.Quote(c2))); err != nil {
			return res, err
		}
	}

	return res, nil
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

func ensureTask(sqlite db.SQLite, title string, priority string, status string, due string, scheduled string, ownerID string) error {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT id FROM tasks WHERE title=%s LIMIT 1;", db.Quote(title)))
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		return nil
	}
	id, err := idgen.ULIDLike()
	if err != nil {
		return err
	}
	shortID, err := idgen.Short("t")
	if err != nil {
		return err
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
	return sqlite.Exec(sql)
}
