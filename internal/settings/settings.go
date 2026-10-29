// Package settings reads what the first version of the app remembered, to
// adopt its last repo: projects.json keeps that now.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings is the older app state. It never holds credentials.
type Settings struct {
	LastRepo string `json:"lastRepo"`
}

// Load reads settings from dir, returning zero values when there are none.
func Load(dir string) Settings {
	var s Settings
	if dir == "" {
		return s
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}
