package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// prefs are what only the terminal app remembers. projects.json belongs to
// both apps and stays as it is; this file sits next to it.
type prefs struct {
	Theme string `json:"theme,omitempty"`
}

const prefsFile = "sdeploy.json"

func loadPrefs(dir string) prefs {
	var p prefs
	if data, err := os.ReadFile(filepath.Join(dir, prefsFile)); err == nil {
		_ = json.Unmarshal(data, &p)
	}
	if _, ok := themes[p.Theme]; !ok {
		p.Theme = defaultTheme
	}
	return p
}

func savePrefs(dir string, p prefs) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, prefsFile), append(data, '\n'), 0o644)
}
