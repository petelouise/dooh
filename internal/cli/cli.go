package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"dooh/internal/tui"
)

// Run executes dooh CLI commands using only stdlib so it works fully offline.
func Run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}

	switch args[0] {
	case "version", "--version", "-v":
		_, _ = fmt.Fprintln(stdout, "0.1.0")
		return nil
	case "user":
		return runUser(args[1:], stdout)
	case "key":
		return runKey(args[1:], stdout)
	case "task":
		return runTask(args[1:], stdout)
	case "collection":
		return runCollection(args[1:], stdout)
	case "export":
		return runExport(args[1:], stdout)
	case "tui":
		return runTUI(args[1:], stdout)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "dooh (pronounced duo)")
	_, _ = fmt.Fprintln(w, "commands: user, key, task, collection, export, tui, version")
}

func runUser(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("user subcommand required")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("user create", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		name := fs.String("name", "", "user name")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *name == "" {
			return errors.New("--name is required")
		}
		_, _ = fmt.Fprintf(out, "TODO: create user %q\n", *name)
		return nil
	case "list":
		_, _ = fmt.Fprintln(out, "TODO: list users")
		return nil
	default:
		return fmt.Errorf("unknown user command %q", args[0])
	}
}

func runKey(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("key subcommand required")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("key create", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		user := fs.String("user", "", "user ID")
		scopes := fs.String("scopes", "", "comma-separated scopes")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *user == "" {
			return errors.New("--user is required")
		}
		_, _ = fmt.Fprintf(out, "TODO: create key for user %s with scopes %s\n", *user, *scopes)
		return nil
	case "revoke":
		fs := flag.NewFlagSet("key revoke", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		prefix := fs.String("prefix", "", "key prefix")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *prefix == "" {
			return errors.New("--prefix is required")
		}
		_, _ = fmt.Fprintf(out, "TODO: revoke key %s\n", *prefix)
		return nil
	default:
		return fmt.Errorf("unknown key command %q", args[0])
	}
}

func runTask(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("task subcommand required")
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("task add", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		title := fs.String("title", "", "title")
		priority := fs.String("priority", "later", "priority")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *title == "" {
			return errors.New("--title is required")
		}
		_, _ = fmt.Fprintf(out, "TODO: create task %q with priority %s\n", *title, *priority)
		return nil
	case "list":
		_, _ = fmt.Fprintln(out, "TODO: list tasks")
		return nil
	default:
		return fmt.Errorf("unknown task command %q", args[0])
	}
}

func runCollection(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("collection subcommand required")
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("collection add", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		name := fs.String("name", "", "name")
		kind := fs.String("kind", "project", "kind")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *name == "" {
			return errors.New("--name is required")
		}
		_, _ = fmt.Fprintf(out, "TODO: create collection %q kind=%s\n", *name, *kind)
		return nil
	default:
		return fmt.Errorf("unknown collection command %q", args[0])
	}
}

func runExport(args []string, out io.Writer) error {
	if len(args) < 1 || args[0] != "site" {
		return errors.New("usage: export site --out <dir>")
	}
	fs := flag.NewFlagSet("export site", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outDir := fs.String("out", "./site-data", "output directory")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "TODO: export site data to %s\n", *outDir)
	return nil
}

func runTUI(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	theme := fs.String("theme", "sunset-pop", "theme")
	listThemes := fs.Bool("list-themes", false, "list theme presets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	catalog, err := tui.LoadThemes("internal/tui/themes/presets.json")
	if err != nil {
		return err
	}
	if *listThemes {
		for _, item := range catalog.Themes {
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", item.ID, item.Name, item.Description)
		}
		return nil
	}

	selected := *theme
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
	_, _ = fmt.Fprintf(out, "TODO: launch TUI with theme %s (%s)\n", chosen.ID, chosen.Name)
	return nil
}
