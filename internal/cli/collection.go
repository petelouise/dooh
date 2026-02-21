package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"dooh/internal/db"
	"dooh/internal/idgen"
)

var palette = []string{"#FF7A59", "#FFD166", "#2EC4B6", "#4D96FF", "#FF6B6B", "#70E000", "#00E5FF"}

func runCollection(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return printCollectionHelp(out)
	}
	switch args[0] {
	case "add":
		return runCollectionAdd(rt, args[1:], out)
	case "list":
		return runCollectionList(rt, args[1:], out)
	case "show":
		return runCollectionShow(rt, args[1:], out)
	case "link":
		return runCollectionLink(rt, args[1:], out, true)
	case "unlink":
		return runCollectionLink(rt, args[1:], out, false)
	case "help", "--help", "-h":
		return printCollectionHelp(out)
	default:
		return fmt.Errorf("unknown collection command %q\n\nrun 'dooh collection help' for available subcommands", args[0])
	}
}

func printCollectionHelp(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "collection subcommands:")
	_, _ = fmt.Fprintln(out, "  add      create a new collection")
	_, _ = fmt.Fprintln(out, "  list     list all collections")
	_, _ = fmt.Fprintln(out, "  show     show detail for a single collection")
	_, _ = fmt.Fprintln(out, "  link     link parent -> child collection")
	_, _ = fmt.Fprintln(out, "  unlink   remove parent -> child link")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "run 'dooh collection <subcommand> --help' for flags and examples")
	return nil
}

func printCollectionAddHelp(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "usage: dooh collection add --name <string> [flags]")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "create a new collection (project, goal, area, tag, etc.)")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "required:")
	_, _ = fmt.Fprintln(out, "  --name <string>   collection name")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "optional:")
	_, _ = fmt.Fprintln(out, "  --kind <string>   project|goal|area|tag|class|custom (default: project)")
	_, _ = fmt.Fprintln(out, "  --color <hex>     hex color code (default: random from palette)")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "examples:")
	_, _ = fmt.Fprintln(out, "  dooh --json collection add --name \"Pollinator Patrol\" --kind project")
	_, _ = fmt.Fprintln(out, "  dooh --json collection add --name \"Q1 Goal\" --kind goal --color \"#4D96FF\"")
	return nil
}

func printCollectionListHelp(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "usage: dooh collection list")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "list all collections (projects, goals, areas, tags, etc.)")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "example:")
	_, _ = fmt.Fprintln(out, "  dooh --json collection list")
	return nil
}

func printCollectionShowHelp(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "usage: dooh collection show --id <id>")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "show collection details including member tasks, parent collections, and child collections")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "required:")
	_, _ = fmt.Fprintln(out, "  --id <id>   collection short_id or full ID (e.g. c_abc123)")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "example:")
	_, _ = fmt.Fprintln(out, "  dooh --json collection show --id c_abc123")
	return nil
}

func printCollectionLinkHelp(verb string, out io.Writer) error {
	_, _ = fmt.Fprintf(out, "usage: dooh collection %s --parent <id> --child <id>\n", verb)
	_, _ = fmt.Fprintln(out, "")
	if verb == "link" {
		_, _ = fmt.Fprintln(out, "create a parent -> child hierarchy between two collections")
		_, _ = fmt.Fprintln(out, "note: cycle detection prevents circular collection hierarchies")
	} else {
		_, _ = fmt.Fprintln(out, "remove a parent -> child hierarchy link between two collections")
	}
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "required:")
	_, _ = fmt.Fprintln(out, "  --parent <id>   parent collection short_id or full ID")
	_, _ = fmt.Fprintln(out, "  --child <id>    child collection short_id or full ID")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintf(out, "example:\n  dooh collection %s --parent c_abc123 --child c_xyz789\n", verb)
	return nil
}

func runCollectionAdd(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("collection add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	name := fs.String("name", "", "name")
	kind := fs.String("kind", "project", "kind (project|goal|area|tag|class|custom)")
	color := fs.String("color", "", "hex color")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return printCollectionAddHelp(out)
		}
		return err
	}
	if *name == "" {
		return errors.New("--name is required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "collections:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	id, err := idgen.ULIDLike()
	if err != nil {
		return err
	}
	shortID, err := idgen.Short("c")
	if err != nil {
		return err
	}
	col := strings.TrimSpace(*color)
	if col == "" {
		col = palette[int(time.Now().UnixNano())%len(palette)]
	}
	sql := fmt.Sprintf("INSERT INTO collections(id,short_id,name,kind,color_hex,created_by,updated_by) VALUES(%s,%s,%s,%s,%s,%s,%s);",
		db.Quote(id), db.Quote(shortID), db.Quote(*name), db.Quote(*kind), db.Quote(col), db.Quote(p.UserID), db.Quote(p.UserID))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "collection.created", "collection", id, map[string]string{"short_id": shortID, "name": *name}); err != nil {
		return err
	}
	if rt.opts.JSON {
		return writeJSON(out, map[string]string{
			"id":        id,
			"short_id":  shortID,
			"name":      *name,
			"kind":      *kind,
			"color_hex": col,
		})
	}
	_, _ = fmt.Fprintf(out, "created collection %s (%s) color=%s\n", shortID, *name, col)
	return nil
}

func runCollectionList(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("collection list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return printCollectionListHelp(out)
		}
		return err
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	if _, err := mustReadAuth(rt, sqlite, *apiKey, "collections:read"); err != nil {
		return err
	}
	rows, err := sqlite.QueryTSV("SELECT short_id,name,kind,color_hex,updated_at,id FROM collections WHERE deleted_at IS NULL ORDER BY updated_at DESC;")
	if err != nil {
		return err
	}
	if rt.opts.JSON {
		collections := make([]map[string]string, 0, len(rows))
		for _, r := range rows {
			if len(r) >= 6 {
				collections = append(collections, map[string]string{
					"short_id":   r[0],
					"name":       r[1],
					"kind":       r[2],
					"color_hex":  r[3],
					"updated_at": r[4],
					"id":         r[5],
				})
			}
		}
		return writeJSON(out, collections)
	}
	_, _ = fmt.Fprintln(out, style("NAME                 KIND       COLOR      UPDATED                  COLLECTION_ID", "1"))
	_, _ = fmt.Fprintln(out, strings.Repeat("-", 92))
	for _, r := range rows {
		if len(r) >= 5 {
			_, _ = fmt.Fprintf(out, "%-20s %-10s %-10s %-24s %s\n",
				truncate(r[1], 20),
				kindCell(r[2], 10),
				style(truncate(r[3], 10), "38;5;45"),
				truncate(r[4], 24),
				r[0],
			)
		}
	}
	return nil
}

func runCollectionShow(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("collection show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("id", "", "collection id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return printCollectionShowHelp(out)
		}
		return err
	}
	if *target == "" {
		return errors.New("--id is required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	if _, err := mustReadAuth(rt, sqlite, *apiKey, "collections:read"); err != nil {
		return err
	}

	rows, err := sqlite.QueryTSV(fmt.Sprintf(
		"SELECT id,short_id,name,kind,color_hex,description,created_by,updated_by,created_at,updated_at FROM collections WHERE (id=%s OR short_id=%s) AND deleted_at IS NULL LIMIT 1;",
		db.Quote(*target), db.Quote(*target)))
	if err != nil {
		return err
	}
	if len(rows) == 0 || len(rows[0]) < 10 {
		return fmt.Errorf("unknown collection %s", *target)
	}
	r := rows[0]

	// Fetch member tasks
	taskRows, err := sqlite.QueryTSV(fmt.Sprintf(
		"SELECT t.short_id,t.title,t.status,t.priority FROM task_collections tc JOIN tasks t ON t.id=tc.task_id WHERE tc.collection_id=%s AND t.deleted_at IS NULL ORDER BY t.updated_at DESC;",
		db.Quote(r[0])))
	if err != nil {
		return err
	}

	// Fetch parent/child collections
	parentRows, err := sqlite.QueryTSV(fmt.Sprintf(
		"SELECT c.short_id,c.name,c.kind FROM collection_links cl JOIN collections c ON c.id=cl.parent_collection_id WHERE cl.child_collection_id=%s AND c.deleted_at IS NULL ORDER BY c.name;",
		db.Quote(r[0])))
	if err != nil {
		return err
	}
	childRows, err := sqlite.QueryTSV(fmt.Sprintf(
		"SELECT c.short_id,c.name,c.kind FROM collection_links cl JOIN collections c ON c.id=cl.child_collection_id WHERE cl.parent_collection_id=%s AND c.deleted_at IS NULL ORDER BY c.name;",
		db.Quote(r[0])))
	if err != nil {
		return err
	}

	if rt.opts.JSON {
		coll := map[string]any{
			"id":          r[0],
			"short_id":    r[1],
			"name":        r[2],
			"kind":        r[3],
			"color_hex":   r[4],
			"description": r[5],
			"created_by":  r[6],
			"updated_by":  r[7],
			"created_at":  r[8],
			"updated_at":  r[9],
		}
		tasks := make([]map[string]string, 0, len(taskRows))
		for _, t := range taskRows {
			if len(t) >= 4 {
				tasks = append(tasks, map[string]string{"short_id": t[0], "title": t[1], "status": t[2], "priority": t[3]})
			}
		}
		coll["tasks"] = tasks

		parents := make([]map[string]string, 0, len(parentRows))
		for _, p := range parentRows {
			if len(p) >= 3 {
				parents = append(parents, map[string]string{"short_id": p[0], "name": p[1], "kind": p[2]})
			}
		}
		coll["parents"] = parents

		children := make([]map[string]string, 0, len(childRows))
		for _, c := range childRows {
			if len(c) >= 3 {
				children = append(children, map[string]string{"short_id": c[0], "name": c[1], "kind": c[2]})
			}
		}
		coll["children"] = children
		return writeJSON(out, coll)
	}

	_, _ = fmt.Fprintf(out, "collection %s\n", r[1])
	_, _ = fmt.Fprintf(out, "name:    %s\n", r[2])
	_, _ = fmt.Fprintf(out, "kind:    %s\n", r[3])
	_, _ = fmt.Fprintf(out, "color:   %s\n", r[4])
	if r[5] != "" {
		_, _ = fmt.Fprintf(out, "desc:    %s\n", r[5])
	}
	_, _ = fmt.Fprintf(out, "created: %s by %s\n", r[8], r[6])
	_, _ = fmt.Fprintf(out, "updated: %s by %s\n", r[9], r[7])
	if len(taskRows) > 0 {
		_, _ = fmt.Fprintf(out, "tasks (%d):\n", len(taskRows))
		for _, t := range taskRows {
			if len(t) >= 4 {
				_, _ = fmt.Fprintf(out, "  %s %-30s [%s] %s\n", t[0], truncate(t[1], 30), t[2], t[3])
			}
		}
	}
	if len(parentRows) > 0 {
		_, _ = fmt.Fprintln(out, "parents:")
		for _, p := range parentRows {
			if len(p) >= 3 {
				_, _ = fmt.Fprintf(out, "  %s %s (%s)\n", p[0], p[1], p[2])
			}
		}
	}
	if len(childRows) > 0 {
		_, _ = fmt.Fprintln(out, "children:")
		for _, c := range childRows {
			if len(c) >= 3 {
				_, _ = fmt.Fprintf(out, "  %s %s (%s)\n", c[0], c[1], c[2])
			}
		}
	}
	return nil
}

func runCollectionLink(rt runtime, args []string, out io.Writer, add bool) error {
	verb := "unlink"
	if add {
		verb = "link"
	}
	fs := flag.NewFlagSet("collection link", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	parent := fs.String("parent", "", "parent collection id or short id")
	child := fs.String("child", "", "child collection id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return printCollectionLinkHelp(verb, out)
		}
		return err
	}
	if *parent == "" || *child == "" {
		return errors.New("--parent and --child are required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "collections:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	parentID, _, err := resolveCollection(sqlite, *parent)
	if err != nil {
		return err
	}
	childID, _, err := resolveCollection(sqlite, *child)
	if err != nil {
		return err
	}
	if parentID == childID {
		return errors.New("collection cannot link to itself")
	}
	if add {
		hasPath, err := hasCollectionPath(sqlite, childID, parentID)
		if err != nil {
			return err
		}
		if hasPath {
			return errors.New("collection hierarchy cycle detected")
		}
		sql := fmt.Sprintf("INSERT OR IGNORE INTO collection_links(parent_collection_id,child_collection_id) VALUES(%s,%s);", db.Quote(parentID), db.Quote(childID))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		if err := rebuildCollectionClosure(sqlite); err != nil {
			return err
		}
		if err := writeEvent(sqlite, p, "collection.linked", "collection", parentID, map[string]string{"parent_collection_id": parentID, "child_collection_id": childID}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "linked collection %s -> %s\n", *parent, *child)
		return nil
	}
	sql := fmt.Sprintf("DELETE FROM collection_links WHERE parent_collection_id=%s AND child_collection_id=%s;", db.Quote(parentID), db.Quote(childID))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := rebuildCollectionClosure(sqlite); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "collection.unlinked", "collection", parentID, map[string]string{"parent_collection_id": parentID, "child_collection_id": childID}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "unlinked collection %s -> %s\n", *parent, *child)
	return nil
}

// --- helpers ---

func resolveCollection(sqlite db.SQLite, target string) (id string, shortID string, err error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT id,short_id FROM collections WHERE (id=%s OR short_id=%s) AND deleted_at IS NULL LIMIT 1;", db.Quote(target), db.Quote(target)))
	if err != nil {
		return "", "", err
	}
	if len(rows) == 0 || len(rows[0]) < 2 {
		return "", "", fmt.Errorf("unknown collection %s", target)
	}
	return rows[0][0], rows[0][1], nil
}

func hasCollectionPath(sqlite db.SQLite, startCollectionID string, targetCollectionID string) (bool, error) {
	query := fmt.Sprintf(`
WITH RECURSIVE walk(id) AS (
  SELECT child_collection_id FROM collection_links WHERE parent_collection_id=%s
  UNION
  SELECT cl.child_collection_id FROM collection_links cl JOIN walk w ON cl.parent_collection_id=w.id
)
SELECT 1 FROM walk WHERE id=%s LIMIT 1;`, db.Quote(startCollectionID), db.Quote(targetCollectionID))
	rows, err := sqlite.QueryTSV(query)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func rebuildCollectionClosure(sqlite db.SQLite) error {
	sql := `
BEGIN;
DELETE FROM collection_closure;
INSERT INTO collection_closure(ancestor_collection_id, descendant_collection_id, depth)
SELECT id, id, 0 FROM collections WHERE deleted_at IS NULL;
WITH RECURSIVE paths(ancestor_id, descendant_id, depth) AS (
  SELECT parent_collection_id, child_collection_id, 1 FROM collection_links
  UNION ALL
  SELECT p.ancestor_id, cl.child_collection_id, p.depth + 1
  FROM paths p
  JOIN collection_links cl ON cl.parent_collection_id = p.descendant_id
)
INSERT OR REPLACE INTO collection_closure(ancestor_collection_id, descendant_collection_id, depth)
SELECT ancestor_id, descendant_id, MIN(depth)
FROM paths
GROUP BY ancestor_id, descendant_id;
COMMIT;`
	return sqlite.Exec(sql)
}
