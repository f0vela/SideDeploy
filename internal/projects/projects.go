// Package projects remembers the projects the app opened and the deploy
// targets of each, in projects.json in the user data directory.
package projects

import (
	"encoding/json"
	"os"
	"path/filepath"

	"sidedeploy/internal/deploy"
)

// Project is one repo the app knows, with its deploy targets.
type Project struct {
	Name         string          `json:"name"`
	Path         string          `json:"path"`
	ActiveTarget string          `json:"activeTarget,omitempty"`
	Targets      []deploy.Target `json:"targets,omitempty"`
}

// Store is the content of projects.json, which the user may edit by hand.
type Store struct {
	Active   string    `json:"active,omitempty"`
	Projects []Project `json:"projects"`
}

const fileName = "projects.json"

// Load reads the store of dir. A missing or broken file gives an empty store.
func Load(dir string) Store {
	var s Store
	if dir == "" {
		return s
	}
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		return s
	}
	if json.Unmarshal(data, &s) != nil {
		return Store{}
	}
	for i := range s.Projects {
		deploy.Normalize(s.Projects[i].Targets)
	}
	return s
}

// Save writes the store to dir. It holds credentials, so only the user
// may read it.
func Save(dir string, s Store) error {
	if dir == "" {
		return nil
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, fileName+".tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, fileName))
}

// Index returns the position of the project at path, or -1.
func (s *Store) Index(path string) int {
	for i := range s.Projects {
		if s.Projects[i].Path == path {
			return i
		}
	}
	return -1
}

// Add gives the position of the project at path, adding it named after its
// folder when it is new.
func (s *Store) Add(path string) int {
	if i := s.Index(path); i >= 0 {
		return i
	}
	s.Projects = append(s.Projects, Project{Name: filepath.Base(path), Path: path})
	return len(s.Projects) - 1
}

// Remove drops the project at path from the list, not from the disk.
func (s *Store) Remove(path string) {
	i := s.Index(path)
	if i < 0 {
		return
	}
	s.Projects = append(s.Projects[:i], s.Projects[i+1:]...)
	if s.Active == path {
		s.Active = ""
	}
}

// Active returns the target the project deploys to, or nil.
func (p *Project) Active() *deploy.Target {
	for i := range p.Targets {
		if p.Targets[i].Name == p.ActiveTarget {
			return &p.Targets[i]
		}
	}
	if len(p.Targets) > 0 {
		return &p.Targets[0]
	}
	return nil
}
