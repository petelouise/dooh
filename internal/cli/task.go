package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"dooh/internal/db"
	"dooh/internal/idgen"
)

func runTask(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return printTaskHelp(out)
	}
	switch args[0] {
	case "add":
		return runTaskAdd(rt, args[1:], out)
	case "list":
		return runTaskList(rt, args[1:], out)
	case "show":
		return runTaskShow(rt, args[1:], out)
	case "update":
		return runTaskUpdate(rt, args[1:], out)
	case "complete":
		return runTaskStatus(rt, args[1:], out, "completed", "task.completed")
	case "reopen":
		return runTaskStatus(rt, args[1:], out, "open", "task.reopened")
	case "archive":
		return runTaskStatus(rt, args[1:], out, "archived", "task.archived")
	case "block":
		return runTaskBlock(rt, args[1:], out, true)
	case "unblock":
		return runTaskBlock(rt, args[1:], out, false)
	case "subtask":
		return runTaskSubtask(rt, args[1:], out)
	case "assign":
		return runTaskAssign(rt, args[1:], out)
	case "collection":
		return runTaskCollection(rt, args[1:], out)
	case "delete":
		return runTaskDelete(rt, args[1:], out)
	case "help", "--help", "-h":
		return printTaskHelp(out)
	default:
		return fmt.Errorf("unknown task command %q\n\nrun 'dooh task help' for available subcommands", args[0])
	}
}

func printTaskHelp(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "task subcommands:")
	_, _ = fmt.Fprintln(out, "  add          create a new task")
	_, _ = fmt.Fprintln(out, "  list         list tasks (with optional filters)")
	_, _ = fmt.Fprintln(out, "  show         show detail for a single task")
	_, _ = fmt.Fprintln(out, "  update       update task fields (title, priority, due, ...)")
	_, _ = fmt.Fprintln(out, "  complete     mark task as completed")
	_, _ = fmt.Fprintln(out, "  reopen       reopen a completed or archived task")
	_, _ = fmt.Fprintln(out, "  archive      archive a task")
	_, _ = fmt.Fprintln(out, "  delete       soft-delete a task")
	_, _ = fmt.Fprintln(out, "  block        add a dependency blocker")
	_, _ = fmt.Fprintln(out, "  unblock      remove a dependency blocker")
	_, _ = fmt.Fprintln(out, "  subtask      manage subtask relationships (add|remove)")
	_, _ = fmt.Fprintln(out, "  assign       manage assignees (add|remove)")
	_, _ = fmt.Fprintln(out, "  collection   manage collection membership (add|remove)")
	return nil
}

func runTaskAdd(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("task add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	title := fs.String("title", "", "title")
	priority := fs.String("priority", "later", "priority (now|soon|later)")
	description := fs.String("description", "", "task description")
	due := fs.String("due", "", "due date (ISO8601)")
	scheduled := fs.String("scheduled", "", "scheduled date (ISO8601)")
	estimate := fs.Int("estimate", 0, "estimated minutes")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *title == "" {
		return errors.New("--title is required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	id, err := idgen.ULIDLike()
	if err != nil {
		return err
	}
	shortID, err := idgen.Short("t")
	if err != nil {
		return err
	}

	setClauses := []string{
		fmt.Sprintf("id=%s", db.Quote(id)),
		fmt.Sprintf("short_id=%s", db.Quote(shortID)),
		fmt.Sprintf("title=%s", db.Quote(*title)),
		fmt.Sprintf("priority=%s", db.Quote(*priority)),
		fmt.Sprintf("created_by=%s", db.Quote(p.UserID)),
		fmt.Sprintf("updated_by=%s", db.Quote(p.UserID)),
	}
	cols := "id,short_id,title,priority,created_by,updated_by"
	vals := fmt.Sprintf("%s,%s,%s,%s,%s,%s",
		db.Quote(id), db.Quote(shortID), db.Quote(*title), db.Quote(*priority), db.Quote(p.UserID), db.Quote(p.UserID))

	extras := ""
	if strings.TrimSpace(*description) != "" {
		cols += ",description"
		vals += "," + db.Quote(*description)
	}
	if strings.TrimSpace(*due) != "" {
		cols += ",due_at"
		vals += "," + db.Quote(*due)
	}
	if strings.TrimSpace(*scheduled) != "" {
		cols += ",scheduled_at"
		vals += "," + db.Quote(*scheduled)
	}
	if *estimate > 0 {
		cols += ",estimated_minutes"
		vals += "," + fmt.Sprintf("%d", *estimate)
	}
	_ = setClauses
	_ = extras

	sql := fmt.Sprintf("INSERT INTO tasks(%s) VALUES(%s);", cols, vals)
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "task.created", "task", id, map[string]string{"short_id": shortID, "title": *title}); err != nil {
		return err
	}

	if rt.opts.JSON {
		result := map[string]any{
			"id":       id,
			"short_id": shortID,
			"title":    *title,
			"priority": *priority,
			"status":   "open",
		}
		if strings.TrimSpace(*description) != "" {
			result["description"] = *description
		}
		if strings.TrimSpace(*due) != "" {
			result["due_at"] = *due
		}
		if strings.TrimSpace(*scheduled) != "" {
			result["scheduled_at"] = *scheduled
		}
		if *estimate > 0 {
			result["estimated_minutes"] = *estimate
		}
		return writeJSON(out, result)
	}
	_, _ = fmt.Fprintf(out, "created task %s (%s)\n", shortID, *title)
	return nil
}

func runTaskList(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("task list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	status := fs.String("status", "open", "filter by status (open|completed|archived|all)")
	priority := fs.String("priority", "all", "filter by priority (now|soon|later|all)")
	assignee := fs.String("assignee", "", "filter by assignee user ID")
	collection := fs.String("collection", "", "filter by collection short_id or ID")
	limit := fs.Int("limit", 100, "max tasks to return")
	offset := fs.Int("offset", 0, "skip this many tasks")
	sort := fs.String("sort", "updated", "sort by (updated|priority|scheduled|created)")
	order := fs.String("order", "desc", "sort order (asc|desc)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	if _, err := mustReadAuth(rt, sqlite, *apiKey, "tasks:read"); err != nil {
		return err
	}

	where := []string{"t.deleted_at IS NULL"}
	if *status != "all" && strings.TrimSpace(*status) != "" {
		where = append(where, fmt.Sprintf("t.status=%s", db.Quote(*status)))
	}
	if *priority != "all" && strings.TrimSpace(*priority) != "" {
		where = append(where, fmt.Sprintf("t.priority=%s", db.Quote(*priority)))
	}

	joins := ""
	if strings.TrimSpace(*assignee) != "" {
		joins += fmt.Sprintf(" JOIN task_assignees ta ON ta.task_id=t.id AND ta.user_id=%s", db.Quote(*assignee))
	}
	if strings.TrimSpace(*collection) != "" {
		joins += fmt.Sprintf(" JOIN task_collections tc ON tc.task_id=t.id JOIN collections col ON col.id=tc.collection_id AND (col.short_id=%s OR col.id=%s) AND col.deleted_at IS NULL", db.Quote(*collection), db.Quote(*collection))
	}

	sortCol := "t.updated_at"
	switch strings.TrimSpace(*sort) {
	case "priority":
		sortCol = "CASE t.priority WHEN 'now' THEN 1 WHEN 'soon' THEN 2 WHEN 'later' THEN 3 ELSE 4 END"
	case "scheduled":
		sortCol = "COALESCE(t.scheduled_at,'9999')"
	case "created":
		sortCol = "t.created_at"
	}
	sortOrder := "DESC"
	if strings.ToLower(strings.TrimSpace(*order)) == "asc" {
		sortOrder = "ASC"
	}

	query := fmt.Sprintf("SELECT DISTINCT t.short_id,t.title,t.status,t.priority,t.updated_at,t.due_at,t.scheduled_at,t.id FROM tasks t%s WHERE %s ORDER BY %s %s LIMIT %d OFFSET %d;",
		joins, strings.Join(where, " AND "), sortCol, sortOrder, *limit, *offset)
	rows, err := sqlite.QueryTSV(query)
	if err != nil {
		return err
	}

	if rt.opts.JSON {
		tasks := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			if len(r) >= 8 {
				t := map[string]any{
					"short_id":   r[0],
					"title":      r[1],
					"status":     r[2],
					"priority":   r[3],
					"updated_at": r[4],
					"id":         r[7],
				}
				if r[5] != "" {
					t["due_at"] = r[5]
				}
				if r[6] != "" {
					t["scheduled_at"] = r[6]
				}
				tasks = append(tasks, t)
			}
		}
		return writeJSON(out, tasks)
	}

	_, _ = fmt.Fprintln(out, style("TITLE                                     STATUS     PRIORITY UPDATED                  TASK_ID", "1"))
	_, _ = fmt.Fprintln(out, strings.Repeat("-", 100))
	for _, r := range rows {
		if len(r) >= 5 {
			_, _ = fmt.Fprintf(out, "%-40s %s %s %-24s %s\n",
				truncate(r[1], 40),
				statusCell(r[2], 10),
				priorityCell(r[3], 8),
				truncate(r[4], 24),
				r[0],
			)
		}
	}
	return nil
}

func runTaskShow(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("task show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("id", "", "task id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return errors.New("--id is required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	if _, err := mustReadAuth(rt, sqlite, *apiKey, "tasks:read"); err != nil {
		return err
	}

	rows, err := sqlite.QueryTSV(fmt.Sprintf(
		"SELECT id,short_id,title,description,status,priority,due_at,scheduled_at,estimated_minutes,rollover_enabled,skip_weekends,completed_at,archived_at,created_by,updated_by,created_at,updated_at,version FROM tasks WHERE (id=%s OR short_id=%s) AND deleted_at IS NULL LIMIT 1;",
		db.Quote(*target), db.Quote(*target)))
	if err != nil {
		return err
	}
	if len(rows) == 0 || len(rows[0]) < 18 {
		return fmt.Errorf("unknown task %s", *target)
	}
	r := rows[0]

	// Fetch assignees
	assigneeRows, err := sqlite.QueryTSV(fmt.Sprintf(
		"SELECT u.id,u.name FROM task_assignees ta JOIN users u ON u.id=ta.user_id WHERE ta.task_id=%s ORDER BY u.name;", db.Quote(r[0])))
	if err != nil {
		return err
	}

	// Fetch blockers
	blockerRows, err := sqlite.QueryTSV(fmt.Sprintf(
		"SELECT t.id,t.short_id,t.title,t.status FROM task_dependencies td JOIN tasks t ON t.id=td.blocked_by_task_id WHERE td.task_id=%s AND t.deleted_at IS NULL ORDER BY t.short_id;", db.Quote(r[0])))
	if err != nil {
		return err
	}

	// Fetch subtasks
	subtaskRows, err := sqlite.QueryTSV(fmt.Sprintf(
		"SELECT t.id,t.short_id,t.title,t.status FROM task_subtasks ts JOIN tasks t ON t.id=ts.child_task_id WHERE ts.parent_task_id=%s AND t.deleted_at IS NULL ORDER BY t.short_id;", db.Quote(r[0])))
	if err != nil {
		return err
	}

	// Fetch collections
	collectionRows, err := sqlite.QueryTSV(fmt.Sprintf(
		"SELECT c.id,c.short_id,c.name,c.kind FROM task_collections tc JOIN collections c ON c.id=tc.collection_id WHERE tc.task_id=%s AND c.deleted_at IS NULL ORDER BY c.kind,c.name;", db.Quote(r[0])))
	if err != nil {
		return err
	}

	if rt.opts.JSON {
		task := map[string]any{
			"id":                r[0],
			"short_id":          r[1],
			"title":             r[2],
			"description":       r[3],
			"status":            r[4],
			"priority":          r[5],
			"created_by":        r[13],
			"updated_by":        r[14],
			"created_at":        r[15],
			"updated_at":        r[16],
			"version":           parseIntDefault(r[17], 1),
		}
		if r[6] != "" {
			task["due_at"] = r[6]
		}
		if r[7] != "" {
			task["scheduled_at"] = r[7]
		}
		if r[8] != "" {
			task["estimated_minutes"] = parseIntDefault(r[8], 0)
		}
		if r[9] == "1" {
			task["rollover_enabled"] = true
		}
		if r[10] == "1" {
			task["skip_weekends"] = true
		}
		if r[11] != "" {
			task["completed_at"] = r[11]
		}
		if r[12] != "" {
			task["archived_at"] = r[12]
		}

		assignees := make([]map[string]string, 0, len(assigneeRows))
		for _, a := range assigneeRows {
			if len(a) >= 2 {
				assignees = append(assignees, map[string]string{"user_id": a[0], "name": a[1]})
			}
		}
		task["assignees"] = assignees

		blockers := make([]map[string]string, 0, len(blockerRows))
		for _, b := range blockerRows {
			if len(b) >= 4 {
				blockers = append(blockers, map[string]string{"id": b[0], "short_id": b[1], "title": b[2], "status": b[3]})
			}
		}
		task["blockers"] = blockers

		subtasks := make([]map[string]string, 0, len(subtaskRows))
		for _, s := range subtaskRows {
			if len(s) >= 4 {
				subtasks = append(subtasks, map[string]string{"id": s[0], "short_id": s[1], "title": s[2], "status": s[3]})
			}
		}
		task["subtasks"] = subtasks

		collections := make([]map[string]string, 0, len(collectionRows))
		for _, c := range collectionRows {
			if len(c) >= 4 {
				collections = append(collections, map[string]string{"id": c[0], "short_id": c[1], "name": c[2], "kind": c[3]})
			}
		}
		task["collections"] = collections

		return writeJSON(out, task)
	}

	// Human-readable output
	_, _ = fmt.Fprintf(out, "task %s\n", r[1])
	_, _ = fmt.Fprintf(out, "title:       %s\n", r[2])
	_, _ = fmt.Fprintf(out, "status:      %s\n", r[4])
	_, _ = fmt.Fprintf(out, "priority:    %s\n", r[5])
	if r[3] != "" {
		_, _ = fmt.Fprintf(out, "description: %s\n", r[3])
	}
	if r[6] != "" {
		_, _ = fmt.Fprintf(out, "due:         %s\n", r[6])
	}
	if r[7] != "" {
		_, _ = fmt.Fprintf(out, "scheduled:   %s\n", r[7])
	}
	if r[8] != "" && r[8] != "0" {
		_, _ = fmt.Fprintf(out, "estimate:    %s min\n", r[8])
	}
	_, _ = fmt.Fprintf(out, "created:     %s by %s\n", r[15], r[13])
	_, _ = fmt.Fprintf(out, "updated:     %s by %s\n", r[16], r[14])
	if len(assigneeRows) > 0 {
		names := make([]string, 0, len(assigneeRows))
		for _, a := range assigneeRows {
			if len(a) >= 2 {
				names = append(names, a[1]+" ("+a[0]+")")
			}
		}
		_, _ = fmt.Fprintf(out, "assignees:   %s\n", strings.Join(names, ", "))
	}
	if len(blockerRows) > 0 {
		_, _ = fmt.Fprintln(out, "blockers:")
		for _, b := range blockerRows {
			if len(b) >= 4 {
				_, _ = fmt.Fprintf(out, "  %s %s [%s]\n", b[1], b[2], b[3])
			}
		}
	}
	if len(subtaskRows) > 0 {
		_, _ = fmt.Fprintln(out, "subtasks:")
		for _, s := range subtaskRows {
			if len(s) >= 4 {
				_, _ = fmt.Fprintf(out, "  %s %s [%s]\n", s[1], s[2], s[3])
			}
		}
	}
	if len(collectionRows) > 0 {
		_, _ = fmt.Fprintln(out, "collections:")
		for _, c := range collectionRows {
			if len(c) >= 4 {
				_, _ = fmt.Fprintf(out, "  %s %s (%s)\n", c[1], c[2], c[3])
			}
		}
	}
	return nil
}

func runTaskUpdate(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("task update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("id", "", "task id or short id")
	title := fs.String("title", "", "new title")
	priority := fs.String("priority", "", "new priority (now|soon|later)")
	description := fs.String("description", "", "new description")
	due := fs.String("due", "", "new due date (ISO8601, or 'clear' to remove)")
	scheduled := fs.String("scheduled", "", "new scheduled date (ISO8601, or 'clear' to remove)")
	estimate := fs.Int("estimate", -1, "new estimated minutes (0 to clear)")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return errors.New("--id is required")
	}

	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)

	taskID, shortID, _, err := resolveTask(sqlite, *target)
	if err != nil {
		return err
	}

	sets := []string{
		fmt.Sprintf("updated_by=%s", db.Quote(p.UserID)),
		"version=version+1",
	}
	changes := map[string]string{}

	if strings.TrimSpace(*title) != "" {
		sets = append(sets, fmt.Sprintf("title=%s", db.Quote(*title)))
		changes["title"] = *title
	}
	if strings.TrimSpace(*priority) != "" {
		sets = append(sets, fmt.Sprintf("priority=%s", db.Quote(*priority)))
		changes["priority"] = *priority
	}
	if *description != "" {
		sets = append(sets, fmt.Sprintf("description=%s", db.Quote(*description)))
		changes["description"] = *description
	}
	if strings.TrimSpace(*due) != "" {
		if strings.ToLower(*due) == "clear" {
			sets = append(sets, "due_at=NULL")
			changes["due_at"] = ""
		} else {
			sets = append(sets, fmt.Sprintf("due_at=%s", db.Quote(*due)))
			changes["due_at"] = *due
		}
	}
	if strings.TrimSpace(*scheduled) != "" {
		if strings.ToLower(*scheduled) == "clear" {
			sets = append(sets, "scheduled_at=NULL")
			changes["scheduled_at"] = ""
		} else {
			sets = append(sets, fmt.Sprintf("scheduled_at=%s", db.Quote(*scheduled)))
			changes["scheduled_at"] = *scheduled
		}
	}
	if *estimate >= 0 {
		if *estimate == 0 {
			sets = append(sets, "estimated_minutes=NULL")
			changes["estimated_minutes"] = ""
		} else {
			sets = append(sets, fmt.Sprintf("estimated_minutes=%d", *estimate))
			changes["estimated_minutes"] = fmt.Sprintf("%d", *estimate)
		}
	}

	if len(changes) == 0 {
		return errors.New("no fields to update (provide --title, --priority, --description, --due, --scheduled, or --estimate)")
	}

	sql := fmt.Sprintf("UPDATE tasks SET %s WHERE id=%s AND deleted_at IS NULL;",
		strings.Join(sets, ","), db.Quote(taskID))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	changes["task_id"] = taskID
	changes["short_id"] = shortID
	if err := writeEvent(sqlite, p, "task.updated", "task", taskID, changes); err != nil {
		return err
	}

	if rt.opts.JSON {
		changes["id"] = taskID
		return writeJSON(out, changes)
	}
	_, _ = fmt.Fprintf(out, "updated task %s\n", shortID)
	return nil
}

func runTaskDelete(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("task delete", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("id", "", "task id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return errors.New("--id is required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:delete")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	sql := fmt.Sprintf("UPDATE tasks SET deleted_at = strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ','now'), updated_by=%s, version=version+1 WHERE (id=%s OR short_id=%s) AND deleted_at IS NULL;",
		db.Quote(p.UserID), db.Quote(*target), db.Quote(*target))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "task.deleted", "task", *target, map[string]string{"target": *target}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "deleted task %s\n", *target)
	return nil
}

func runTaskStatus(rt runtime, args []string, out io.Writer, status string, eventName string) error {
	fs := flag.NewFlagSet("task status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("id", "", "task id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return errors.New("--id is required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	taskID, shortID, _, err := resolveTask(sqlite, *target)
	if err != nil {
		return err
	}
	if status == "completed" {
		blocked, err := hasOpenBlockers(sqlite, taskID)
		if err != nil {
			return err
		}
		if blocked {
			return errors.New("cannot complete task while blockers are open")
		}
	}
	extra := ""
	if status == "completed" {
		extra = ", completed_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')"
	}
	if status == "archived" {
		extra = ", archived_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')"
	}
	if status == "open" {
		extra = ", completed_at = NULL, archived_at = NULL"
	}
	sql := fmt.Sprintf("UPDATE tasks SET status=%s, updated_by=%s, version=version+1 %s WHERE id=%s AND deleted_at IS NULL;",
		db.Quote(status), db.Quote(p.UserID), extra, db.Quote(taskID))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, eventName, "task", taskID, map[string]string{"task_id": taskID, "short_id": shortID, "status": status}); err != nil {
		return err
	}
	if err := syncParentsForChild(sqlite, taskID, p.UserID); err != nil {
		return err
	}
	if rt.opts.JSON {
		return writeJSON(out, map[string]string{"short_id": shortID, "id": taskID, "status": status})
	}
	_, _ = fmt.Fprintf(out, "%s task %s\n", status, shortID)
	return nil
}

func runTaskBlock(rt runtime, args []string, out io.Writer, add bool) error {
	fs := flag.NewFlagSet("task block", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("id", "", "task id or short id")
	by := fs.String("by", "", "blocking task id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" || *by == "" {
		return errors.New("--id and --by are required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	taskID, _, _, err := resolveTask(sqlite, *target)
	if err != nil {
		return err
	}
	blockerID, _, _, err := resolveTask(sqlite, *by)
	if err != nil {
		return err
	}
	if taskID == blockerID {
		return errors.New("task cannot block itself")
	}
	if add {
		hasPath, err := hasDependencyPath(sqlite, blockerID, taskID)
		if err != nil {
			return err
		}
		if hasPath {
			return errors.New("dependency cycle detected")
		}
		sql := fmt.Sprintf("INSERT OR IGNORE INTO task_dependencies(task_id,blocked_by_task_id) VALUES(%s,%s);", db.Quote(taskID), db.Quote(blockerID))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		if err := writeEvent(sqlite, p, "task.blocked", "task", taskID, map[string]string{"task_id": taskID, "blocked_by_task_id": blockerID}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "task %s now blocked by %s\n", *target, *by)
		return nil
	}
	sql := fmt.Sprintf("DELETE FROM task_dependencies WHERE task_id=%s AND blocked_by_task_id=%s;", db.Quote(taskID), db.Quote(blockerID))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "task.unblocked", "task", taskID, map[string]string{"task_id": taskID, "blocked_by_task_id": blockerID}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "task %s unblocked by %s\n", *target, *by)
	return nil
}

func runTaskSubtask(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("task subtask subcommand required: add|remove")
	}
	add := false
	switch args[0] {
	case "add":
		add = true
	case "remove":
		add = false
	default:
		return fmt.Errorf("unknown task subtask command %q (available: add, remove)", args[0])
	}
	fs := flag.NewFlagSet("task subtask", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	parent := fs.String("parent", "", "parent task id or short id")
	child := fs.String("child", "", "child task id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *parent == "" || *child == "" {
		return errors.New("--parent and --child are required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	parentID, _, _, err := resolveTask(sqlite, *parent)
	if err != nil {
		return err
	}
	childID, _, _, err := resolveTask(sqlite, *child)
	if err != nil {
		return err
	}
	if parentID == childID {
		return errors.New("task cannot be a subtask of itself")
	}
	if add {
		hasPath, err := hasSubtaskPath(sqlite, childID, parentID)
		if err != nil {
			return err
		}
		if hasPath {
			return errors.New("subtask cycle detected")
		}
		sql := fmt.Sprintf("INSERT OR IGNORE INTO task_subtasks(parent_task_id,child_task_id) VALUES(%s,%s);", db.Quote(parentID), db.Quote(childID))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		if err := writeEvent(sqlite, p, "task.subtask_added", "task", parentID, map[string]string{"parent_task_id": parentID, "child_task_id": childID}); err != nil {
			return err
		}
		if err := syncParentStatus(sqlite, parentID, p.UserID); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "added subtask %s -> %s\n", *parent, *child)
		return nil
	}
	sql := fmt.Sprintf("DELETE FROM task_subtasks WHERE parent_task_id=%s AND child_task_id=%s;", db.Quote(parentID), db.Quote(childID))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "task.subtask_removed", "task", parentID, map[string]string{"parent_task_id": parentID, "child_task_id": childID}); err != nil {
		return err
	}
	if err := syncParentStatus(sqlite, parentID, p.UserID); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "removed subtask %s -> %s\n", *parent, *child)
	return nil
}

func runTaskAssign(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("task assign subcommand required: add|remove")
	}
	add := false
	switch args[0] {
	case "add":
		add = true
	case "remove":
		add = false
	default:
		return fmt.Errorf("unknown task assign command %q (available: add, remove)", args[0])
	}
	fs := flag.NewFlagSet("task assign", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("id", "", "task id or short id")
	user := fs.String("user", "", "user id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *target == "" || *user == "" {
		return errors.New("--id and --user are required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	taskID, _, _, err := resolveTask(sqlite, *target)
	if err != nil {
		return err
	}
	ok, err := userExists(sqlite, *user)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("unknown user %s", *user)
	}
	if add {
		sql := fmt.Sprintf("INSERT OR IGNORE INTO task_assignees(task_id,user_id) VALUES(%s,%s);", db.Quote(taskID), db.Quote(*user))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		if err := writeEvent(sqlite, p, "task.assignee_added", "task", taskID, map[string]string{"task_id": taskID, "user_id": *user}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "assigned task %s to %s\n", *target, *user)
		return nil
	}
	sql := fmt.Sprintf("DELETE FROM task_assignees WHERE task_id=%s AND user_id=%s;", db.Quote(taskID), db.Quote(*user))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "task.assignee_removed", "task", taskID, map[string]string{"task_id": taskID, "user_id": *user}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "unassigned task %s from %s\n", *target, *user)
	return nil
}

func runTaskCollection(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("task collection subcommand required: add|remove")
	}
	add := false
	switch args[0] {
	case "add":
		add = true
	case "remove":
		add = false
	default:
		return fmt.Errorf("unknown task collection command %q (available: add, remove)", args[0])
	}
	fs := flag.NewFlagSet("task collection", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("id", "", "task id or short id")
	coll := fs.String("collection", "", "collection id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *target == "" || *coll == "" {
		return errors.New("--id and --collection are required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:write", "collections:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	taskID, _, _, err := resolveTask(sqlite, *target)
	if err != nil {
		return err
	}
	collID, collShortID, err := resolveCollection(sqlite, *coll)
	if err != nil {
		return err
	}
	if add {
		sql := fmt.Sprintf("INSERT OR IGNORE INTO task_collections(task_id,collection_id) VALUES(%s,%s);", db.Quote(taskID), db.Quote(collID))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		if err := writeEvent(sqlite, p, "task.collection_added", "task", taskID, map[string]string{"task_id": taskID, "collection_id": collID}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "added task %s to collection %s\n", *target, collShortID)
		return nil
	}
	sql := fmt.Sprintf("DELETE FROM task_collections WHERE task_id=%s AND collection_id=%s;", db.Quote(taskID), db.Quote(collID))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "task.collection_removed", "task", taskID, map[string]string{"task_id": taskID, "collection_id": collID}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "removed task %s from collection %s\n", *target, collShortID)
	return nil
}

// --- helpers ---

func resolveTask(sqlite db.SQLite, target string) (id string, shortID string, title string, err error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT id,short_id,title FROM tasks WHERE (id=%s OR short_id=%s) AND deleted_at IS NULL LIMIT 1;", db.Quote(target), db.Quote(target)))
	if err != nil {
		return "", "", "", err
	}
	if len(rows) == 0 || len(rows[0]) < 3 {
		return "", "", "", fmt.Errorf("unknown task %s", target)
	}
	return rows[0][0], rows[0][1], rows[0][2], nil
}

func hasOpenBlockers(sqlite db.SQLite, taskID string) (bool, error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT 1 FROM task_dependencies td JOIN tasks b ON b.id=td.blocked_by_task_id WHERE td.task_id=%s AND b.deleted_at IS NULL AND b.status!='completed' LIMIT 1;", db.Quote(taskID)))
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func hasDependencyPath(sqlite db.SQLite, startTaskID string, targetTaskID string) (bool, error) {
	query := fmt.Sprintf(`
WITH RECURSIVE walk(id) AS (
  SELECT blocked_by_task_id FROM task_dependencies WHERE task_id=%s
  UNION
  SELECT td.blocked_by_task_id FROM task_dependencies td JOIN walk w ON td.task_id=w.id
)
SELECT 1 FROM walk WHERE id=%s LIMIT 1;`, db.Quote(startTaskID), db.Quote(targetTaskID))
	rows, err := sqlite.QueryTSV(query)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func hasSubtaskPath(sqlite db.SQLite, startTaskID string, targetTaskID string) (bool, error) {
	query := fmt.Sprintf(`
WITH RECURSIVE walk(id) AS (
  SELECT child_task_id FROM task_subtasks WHERE parent_task_id=%s
  UNION
  SELECT ts.child_task_id FROM task_subtasks ts JOIN walk w ON ts.parent_task_id=w.id
)
SELECT 1 FROM walk WHERE id=%s LIMIT 1;`, db.Quote(startTaskID), db.Quote(targetTaskID))
	rows, err := sqlite.QueryTSV(query)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func syncParentsForChild(sqlite db.SQLite, childTaskID string, actorUserID string) error {
	parents, err := sqlite.QueryTSV(fmt.Sprintf("SELECT parent_task_id FROM task_subtasks WHERE child_task_id=%s;", db.Quote(childTaskID)))
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, r := range parents {
		if len(r) == 0 {
			continue
		}
		if err := syncParentStatusRecursive(sqlite, r[0], actorUserID, seen); err != nil {
			return err
		}
	}
	return nil
}

func syncParentStatus(sqlite db.SQLite, parentTaskID string, actorUserID string) error {
	return syncParentStatusRecursive(sqlite, parentTaskID, actorUserID, map[string]bool{})
}

func syncParentStatusRecursive(sqlite db.SQLite, parentTaskID string, actorUserID string, seen map[string]bool) error {
	if seen[parentTaskID] {
		return nil
	}
	seen[parentTaskID] = true

	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT COUNT(*), SUM(CASE WHEN c.status='completed' THEN 1 ELSE 0 END) FROM task_subtasks ts JOIN tasks c ON c.id=ts.child_task_id WHERE ts.parent_task_id=%s AND c.deleted_at IS NULL;", db.Quote(parentTaskID)))
	if err != nil {
		return err
	}
	if len(rows) > 0 && len(rows[0]) >= 2 {
		total := parseIntDefault(rows[0][0], 0)
		done := parseIntDefault(rows[0][1], 0)
		if total > 0 {
			if done == total {
				sql := fmt.Sprintf("UPDATE tasks SET status='completed', completed_at=COALESCE(completed_at,strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ','now')), updated_by=%s, version=version+1 WHERE id=%s AND deleted_at IS NULL;", db.Quote(actorUserID), db.Quote(parentTaskID))
				if err := sqlite.Exec(sql); err != nil {
					return err
				}
			} else {
				sql := fmt.Sprintf("UPDATE tasks SET status='open', completed_at=NULL, updated_by=%s, version=version+1 WHERE id=%s AND deleted_at IS NULL AND status='completed';", db.Quote(actorUserID), db.Quote(parentTaskID))
				if err := sqlite.Exec(sql); err != nil {
					return err
				}
			}
		}
	}
	parents, err := sqlite.QueryTSV(fmt.Sprintf("SELECT parent_task_id FROM task_subtasks WHERE child_task_id=%s;", db.Quote(parentTaskID)))
	if err != nil {
		return err
	}
	for _, r := range parents {
		if len(r) == 0 {
			continue
		}
		if err := syncParentStatusRecursive(sqlite, r[0], actorUserID, seen); err != nil {
			return err
		}
	}
	return nil
}
