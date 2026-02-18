package tui

import (
	"encoding/json"
	"fmt"
	"os"
)

// ThemeCatalog is read from presets.json to keep design choices declarative.
type ThemeCatalog struct {
	Default string  `json:"default"`
	Themes  []Theme `json:"themes"`
}

type Theme struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Colors      map[string]string `json:"colors"`
}

func LoadThemes(path string) (ThemeCatalog, error) {
	var catalog ThemeCatalog
	b, err := os.ReadFile(path)
	if err != nil {
		return catalog, fmt.Errorf("read themes file: %w", err)
	}
	if err := json.Unmarshal(b, &catalog); err != nil {
		return catalog, fmt.Errorf("parse themes file: %w", err)
	}
	if len(catalog.Themes) == 0 {
		return catalog, fmt.Errorf("theme catalog has no themes")
	}
	return catalog, nil
}

func (c ThemeCatalog) ThemeByID(id string) (Theme, bool) {
	for _, t := range c.Themes {
		if t.ID == id {
			return t, true
		}
	}
	return Theme{}, false
}
