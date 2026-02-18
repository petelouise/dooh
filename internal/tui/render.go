package tui

import (
	"fmt"
	"strconv"
	"strings"

	"dooh/internal/db"
)

func RenderDashboard(sqlite db.SQLite, theme Theme, filter string, limit int) (string, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT short_id,title,status,priority,COALESCE(due_at,''),COALESCE(scheduled_at,''),updated_at FROM tasks WHERE deleted_at IS NULL ORDER BY updated_at DESC LIMIT %d;", limit*4))
	if err != nil {
		return "", err
	}
	metricsRows, err := sqlite.QueryTSV("SELECT status,COUNT(*) FROM tasks WHERE deleted_at IS NULL GROUP BY status;")
	if err != nil {
		return "", err
	}
	collectionRows, err := sqlite.QueryTSV("SELECT c.short_id,c.name,c.kind,c.color_hex,COUNT(tc.task_id) FROM collections c LEFT JOIN task_collections tc ON c.id=tc.collection_id WHERE c.deleted_at IS NULL GROUP BY c.id ORDER BY c.updated_at DESC LIMIT 10;")
	if err != nil {
		return "", err
	}

	f := strings.ToLower(strings.TrimSpace(filter))
	filtered := make([][]string, 0, len(rows))
	for _, r := range rows {
		if len(r) < 7 {
			continue
		}
		if f == "" || strings.Contains(strings.ToLower(strings.Join(r, " ")), f) {
			filtered = append(filtered, r)
		}
		if len(filtered) >= limit {
			break
		}
	}

	counts := map[string]int{"open": 0, "completed": 0, "archived": 0}
	for _, r := range metricsRows {
		if len(r) < 2 {
			continue
		}
		n, _ := strconv.Atoi(r[1])
		counts[r[0]] = n
	}
	total := counts["open"] + counts["completed"] + counts["archived"]
	if total == 0 {
		total = 1
	}

	var b strings.Builder
	fg := func(hex, text string) string {
		r, g, bl := hexToRGB(hex)
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", r, g, bl, text)
	}

	b.WriteString(fg(theme.Colors["accent"], "dooh dashboard") + "  ")
	b.WriteString(fg(theme.Colors["muted"], "("+theme.Name+")\n"))
	b.WriteString(fg(theme.Colors["muted"], strings.Repeat("-", 64)+"\n"))

	b.WriteString(fg(theme.Colors["text"], "Tasks\n"))
	b.WriteString(fmt.Sprintf("open %s  completed %s  archived %s\n",
		bar(counts["open"], total, theme.Colors["accent"]),
		bar(counts["completed"], total, theme.Colors["success"]),
		bar(counts["archived"], total, theme.Colors["warning"]),
	))
	if f != "" {
		b.WriteString(fg(theme.Colors["muted"], "filter: "+f+"\n"))
	}

	for _, r := range filtered {
		statusColor := theme.Colors["accent"]
		if r[2] == "completed" {
			statusColor = theme.Colors["success"]
		}
		if r[2] == "archived" {
			statusColor = theme.Colors["warning"]
		}
		line := fmt.Sprintf("%s  %-9s %-6s  %s", r[0], r[2], r[3], r[1])
		b.WriteString(fg(statusColor, line) + "\n")
	}

	b.WriteString("\n" + fg(theme.Colors["text"], "Collections\n"))
	for _, r := range collectionRows {
		if len(r) < 5 {
			continue
		}
		line := fmt.Sprintf("%s  %-16s %-8s tasks=%s", r[0], r[1], r[2], r[4])
		b.WriteString(fg(r[3], line) + "\n")
	}

	b.WriteString("\n" + fg(theme.Colors["muted"], "Tip: use `dooh tui --filter <text>` for fuzzy-ish contains filtering.\n"))
	return b.String(), nil
}

func bar(v int, total int, hex string) string {
	if total <= 0 {
		total = 1
	}
	width := 12
	filled := (v * width) / total
	if filled < 1 && v > 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	r, g, b := hexToRGB(hex)
	seg := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m %d", r, g, b, seg, v)
}

func hexToRGB(hex string) (int, int, int) {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) != 6 {
		return 255, 255, 255
	}
	r, _ := strconv.ParseInt(h[0:2], 16, 64)
	g, _ := strconv.ParseInt(h[2:4], 16, 64)
	b, _ := strconv.ParseInt(h[4:6], 16, 64)
	return int(r), int(g), int(b)
}
