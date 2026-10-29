package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCandidates(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "app/a.php", "a")
	writeFile(t, root, "app/b.php", "b")
	if err := os.Symlink(filepath.Join(root, "app/a.php"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	changes := []Change{
		{Path: "app/a.php"},
		{Path: "app/a.php"}, // staged and modified: once
		{Path: "app/gone.php"},
		{Path: "app/b.php"},
		{Path: "link"},
	}
	got := Candidates(root, changes, true)
	want := map[string]Candidate{
		"app/a.php":    {Rel: "app/a.php", Local: filepath.Join(root, "app/a.php")},
		"app/b.php":    {Rel: "app/b.php", Local: filepath.Join(root, "app/b.php")},
		"app/gone.php": {Rel: "app/gone.php", Delete: true},
	}
	for _, c := range got {
		w, ok := want[c.Rel]
		if !ok {
			t.Errorf("unexpected candidate %+v", c)
			continue
		}
		if c != w {
			t.Errorf("candidate %q = %+v, want %+v", c.Rel, c, w)
		}
		delete(want, c.Rel)
	}
	if len(want) != 0 {
		t.Errorf("missing candidates: %+v", want)
	}

	// Without autoDelete the deleted file is left alone.
	for _, c := range Candidates(root, changes, false) {
		if c.Delete {
			t.Errorf("delete offered without autoDelete: %+v", c)
		}
	}

	// A rename sends the new path and deletes the old one.
	writeFile(t, root, "app/new.php", "n")
	got = Candidates(root, []Change{{Path: "app/new.php", OrigPath: "app/old.php"}}, true)
	if len(got) != 2 || got[0].Rel != "app/new.php" || !got[1].Delete || got[1].Rel != "app/old.php" {
		t.Fatalf("rename candidates = %+v", got)
	}
}

func writeFile(t *testing.T, root, rel, text string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
