package tui

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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

//go:embed themes/presets.json
var embeddedThemes []byte

func LoadThemes(path string) (ThemeCatalog, error) {
	var catalog ThemeCatalog
	var (
		b   []byte
		err error
	)
	if strings.TrimSpace(path) == "" {
		b = embeddedThemes
	} else {
		b, err = os.ReadFile(path)
		if err != nil {
			// Fallback keeps runtime/test behavior stable even if cwd is package-local.
			b = embeddedThemes
		}
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
