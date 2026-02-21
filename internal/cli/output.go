package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func writeJSON(w io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

func printWriteContext(out io.Writer, rt runtime, dbPath string, p principal) {
	if rt.opts.Quiet || rt.opts.JSON {
		return
	}
	_, _ = fmt.Fprintf(out, "%s profile=%s mode=%s user=%s key=%s db=%s\n",
		style("context", "2"),
		rt.opts.Profile,
		displayActor(p.Actor),
		p.UserID,
		p.KeyPrefix,
		dbPath,
	)
	if rt.aiProfileEnforced && p.Actor == "agent" {
		_, _ = fmt.Fprintln(out, "ai context active (profile auto-set to ai)")
	}
}

func style(text string, code string) string {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func statusCell(v string, width int) string {
	raw := fmt.Sprintf("%-*s", width, strings.ToLower(strings.TrimSpace(v)))
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "completed":
		return style(raw, "38;5;78")
	case "archived":
		return style(raw, "38;5;179")
	case "in_progress":
		return style(raw, "38;5;214")
	case "open":
		return style(raw, "38;5;81")
	default:
		return raw
	}
}

func priorityCell(v string, width int) string {
	raw := fmt.Sprintf("%-*s", width, strings.ToLower(strings.TrimSpace(v)))
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "now":
		return style(raw, "38;5;203")
	case "soon":
		return style(raw, "38;5;221")
	case "later":
		return style(raw, "38;5;111")
	default:
		return raw
	}
}

func kindCell(v string, width int) string {
	raw := fmt.Sprintf("%-*s", width, strings.ToLower(strings.TrimSpace(v)))
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "project":
		return style(raw, "38;5;75")
	case "goal":
		return style(raw, "38;5;220")
	case "tag":
		return style(raw, "38;5;43")
	case "class":
		return style(raw, "38;5;119")
	case "area":
		return style(raw, "38;5;209")
	default:
		return raw
	}
}

func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func parseIntDefault(v string, def int) int {
	if strings.TrimSpace(v) == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}
