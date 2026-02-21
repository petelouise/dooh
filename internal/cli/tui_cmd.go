package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"dooh/internal/db"
	"dooh/internal/tui"
)

func runTUI(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	theme := fs.String("theme", "", "theme")
	listThemes := fs.Bool("list-themes", false, "list theme presets")
	filter := fs.String("filter", "", "filter tasks by text")
	limit := fs.Int("limit", 12, "max tasks to display")
	static := fs.Bool("static", false, "render once and exit")
	plain := fs.Bool("plain", false, "disable ANSI and render plain table")
	renderer := fs.String("renderer", "tea", "renderer: tea|legacy|auto")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	catalog, err := tui.LoadThemes("internal/tui/themes/presets.json")
	if err != nil {
		return err
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustReadAuth(rt, sqlite, *apiKey, "tasks:read", "collections:read")
	if err != nil {
		return err
	}
	identity := tui.Identity{Actor: displayActor(p.Actor), UserID: p.UserID, UserName: p.UserName}
	if *listThemes {
		if rt.opts.JSON {
			themes := make([]map[string]string, 0, len(catalog.Themes))
			for _, item := range catalog.Themes {
				themes = append(themes, map[string]string{
					"id":          item.ID,
					"name":        item.Name,
					"description": item.Description,
				})
			}
			return writeJSON(out, themes)
		}
		for _, item := range catalog.Themes {
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", item.ID, item.Name, item.Description)
		}
		return nil
	}

	selected := strings.TrimSpace(*theme)
	selected = resolveTheme(rt, selected)
	if selected == "" {
		selected = catalog.Default
	}
	chosen, ok := catalog.ThemeByID(selected)
	if !ok {
		ids := make([]string, 0, len(catalog.Themes))
		for _, item := range catalog.Themes {
			ids = append(ids, item.ID)
		}
		return fmt.Errorf("unknown theme %q (available: %s)", selected, strings.Join(ids, ", "))
	}
	loc, err := time.LoadLocation(rt.profile.Timezone)
	if err != nil {
		loc = time.Local
	}
	if *static {
		panel, err := tui.RenderDashboard(sqlite, chosen, *filter, *limit, loc, identity, *plain)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(out, panel)
		return nil
	}
	if *plain {
		panel, err := tui.RenderDashboard(sqlite, chosen, *filter, *limit, loc, identity, true)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(out, panel)
		return nil
	}
	if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
		panel, err := tui.RenderDashboard(sqlite, chosen, *filter, *limit, loc, identity, true)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(out, panel)
		return nil
	}
	r := strings.ToLower(strings.TrimSpace(*renderer))
	switch r {
	case "", "tea", "auto":
		if err := tui.RunInteractiveTea(os.Stdin, out, sqlite, catalog, chosen.ID, *filter, *limit, loc, identity, false); err != nil {
			if r == "tea" {
				return err
			}
		} else {
			return nil
		}
		return tui.RunInteractive(os.Stdin, out, sqlite, catalog, chosen.ID, *filter, *limit, loc, identity, false)
	case "legacy":
		return tui.RunInteractive(os.Stdin, out, sqlite, catalog, chosen.ID, *filter, *limit, loc, identity, false)
	default:
		return errors.New("--renderer must be tea, legacy, or auto")
	}
}
