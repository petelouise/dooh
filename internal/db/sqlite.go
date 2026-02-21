package db

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type SQLite struct {
	Path string
}

func New(path string) SQLite {
	return SQLite{Path: path}
}

func (s SQLite) Exec(sql string) error {
	cmd := exec.Command("sqlite3", s.Path, sql)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sqlite3 exec failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (s SQLite) QueryTSV(sql string) ([][]string, error) {
	cmd := exec.Command("sqlite3", "-noheader", "-separator", "\t", s.Path, sql)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sqlite3 query failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	if out.Len() == 0 {
		return nil, nil
	}
	text := strings.TrimRight(out.String(), "\n")
	lines := strings.Split(text, "\n")
	rows := make([][]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, strings.Split(line, "\t"))
	}
	return rows, nil
}

func Quote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}
