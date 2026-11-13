package main

import (
	"testing"

	"sidedeploy/internal/projects"
)

func TestSaveSettings(t *testing.T) {
	a := newApp()
	a.dataDir = t.TempDir()
	a.store = projects.Store{Projects: []projects.Project{{Name: "p", Path: "/tmp/p"}}}
	a.draftClaude = "  /opt/homebrew/bin/claude  "
	a.draftCopilot = ""

	a.saveSettings()

	got := projects.Load(a.dataDir)
	if got.ClaudePath != "/opt/homebrew/bin/claude" || got.CopilotPath != "" {
		t.Fatalf("saved paths = %q, %q", got.ClaudePath, got.CopilotPath)
	}
	if len(got.Projects) != 1 {
		t.Fatalf("saving settings lost the projects: %+v", got.Projects)
	}
	if a.settingsErr || a.settingsMsg != "Guardado." {
		t.Fatalf("message = %q, err = %v", a.settingsMsg, a.settingsErr)
	}
}
