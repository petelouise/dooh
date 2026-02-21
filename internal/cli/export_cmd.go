package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"dooh/internal/db"
	"dooh/internal/exporter"
)

func runExport(rt runtime, args []string, out io.Writer) error {
	if len(args) < 1 || args[0] != "site" {
		return errors.New("usage: export site --out <dir>")
	}
	fs := flag.NewFlagSet("export site", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outDir := fs.String("out", "", "output directory")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	resolvedOut := strings.TrimSpace(*outDir)
	if resolvedOut == "" {
		resolvedOut = rt.profile.ExportDir
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	if _, err := mustReadAuth(rt, sqlite, *apiKey, "export:run"); err != nil {
		return err
	}
	if err := exporter.ExportSite(sqlite, resolvedOut); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "exported site data to %s\n", resolvedOut)
	return nil
}
