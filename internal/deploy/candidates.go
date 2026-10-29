package deploy

import (
	"os"
	"path/filepath"
)

// Change is one path Git reports and, when renamed, the path it came from.
type Change struct {
	Path     string
	OrigPath string
}

// Candidate is one file a deploy sends, or one remote file it removes.
type Candidate struct {
	Rel    string // path inside the repo, as Git writes it
	Local  string // absolute path on disk, empty for a deletion
	Delete bool
}

// Candidates lists what the work tree holds for the changes: one upload per
// file still on disk, one deletion per gone file when autoDelete is on.
// Symlinks and folders are left out.
func Candidates(root string, changes []Change, autoDelete bool) []Candidate {
	var out []Candidate
	seen := make(map[string]bool, len(changes))
	gone := func(rel string) bool {
		if rel == "" || seen[rel] {
			return false
		}
		seen[rel] = true
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			if autoDelete {
				out = append(out, Candidate{Rel: rel, Delete: true})
			}
			return true
		}
		if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
			return true
		}
		out = append(out, Candidate{Rel: rel, Local: filepath.Join(root, filepath.FromSlash(rel))})
		return true
	}
	for _, c := range changes {
		gone(c.Path)
		if c.OrigPath != "" {
			gone(c.OrigPath)
		}
	}
	return out
}
