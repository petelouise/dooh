package exporter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"dooh/internal/db"
)

func ExportSite(sqlite db.SQLite, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	tasks, err := sqlite.QueryTSV("SELECT short_id,title,status,priority,COALESCE(due_at,''),COALESCE(scheduled_at,''),updated_at,COALESCE(started_at,'') FROM tasks WHERE deleted_at IS NULL ORDER BY updated_at DESC;")
	if err != nil {
		return err
	}
	collections, err := sqlite.QueryTSV("SELECT short_id,name,kind,color_hex,updated_at FROM collections WHERE deleted_at IS NULL ORDER BY updated_at DESC;")
	if err != nil {
		return err
	}
	metricsRows, err := sqlite.QueryTSV("SELECT status,COUNT(*) FROM tasks WHERE deleted_at IS NULL GROUP BY status;")
	if err != nil {
		return err
	}

	taskOut := make([]map[string]string, 0, len(tasks))
	for _, r := range tasks {
		if len(r) < 8 {
			continue
		}
		task := map[string]string{
			"id":           r[0],
			"title":        r[1],
			"status":       r[2],
			"priority":     r[3],
			"due_at":       r[4],
			"scheduled_at": r[5],
			"updated_at":   r[6],
		}
		if r[7] != "" {
			task["started_at"] = r[7]
		}
		taskOut = append(taskOut, task)
	}

	collectionOut := make([]map[string]string, 0, len(collections))
	for _, r := range collections {
		if len(r) < 5 {
			continue
		}
		collectionOut = append(collectionOut, map[string]string{
			"id":         r[0],
			"name":       r[1],
			"kind":       r[2],
			"color_hex":  r[3],
			"updated_at": r[4],
		})
	}

	metrics := map[string]string{}
	for _, r := range metricsRows {
		if len(r) < 2 {
			continue
		}
		metrics[r[0]] = r[1]
	}

	now := time.Now().UTC().Format(time.RFC3339)
	index := map[string]any{
		"generated_at":     now,
		"task_count":       len(taskOut),
		"collection_count": len(collectionOut),
	}
	manifest := map[string]any{
		"generated_at":   now,
		"schema_version": "v1",
		"files":          []string{"index.json", "tasks.json", "collections.json", "metrics.json"},
	}

	if err := writeJSON(filepath.Join(outDir, "tasks.json"), taskOut); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "collections.json"), collectionOut); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "metrics.json"), metrics); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "index.json"), index); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "manifest.json"), manifest); err != nil {
		return err
	}
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json %s: %w", path, err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write file %s: %w", path, err)
	}
	return nil
}
