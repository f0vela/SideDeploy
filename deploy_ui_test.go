package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"sidedeploy/internal/deploy"
	"sidedeploy/internal/gitrepo"
	"sidedeploy/internal/projects"
)

func TestStatusChangesSkipsCleanFiles(t *testing.T) {
	st := gitrepo.Status{Files: []gitrepo.File{
		{Path: "a.php", Index: 'M', Worktree: 'M'}, // staged and modified: once
		{Path: "b.php", Index: 'A', Worktree: '.'},
		{Path: "c.php", Index: '.', Worktree: '.'},
		{Path: "d.php", Index: '?', Worktree: '?'},
	}}
	got := statusChanges(st)
	if len(got) != 3 {
		t.Fatalf("changes = %+v, want a, b and d", got)
	}
	if got[0].Path != "a.php" || got[1].Path != "b.php" || got[2].Path != "d.php" {
		t.Fatalf("changes = %+v", got)
	}
}

func TestDeployJobs(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"app/a.php", "app/new.php", "README.md"} {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "app", "a.php"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	st := gitrepo.Status{Files: []gitrepo.File{
		{Path: "app/a.php", Index: 'M', Worktree: 'M'},
		{Path: "app/gone.php", Index: ' ', Worktree: 'D'},
		{Path: "app/new.php", OrigPath: "app/old.php", Index: 'R', Worktree: '.'},
		{Path: "README.md", Index: '.', Worktree: 'M'},
		{Path: "link", Index: '?', Worktree: '?'},
	}}
	filter := deploy.Filter{Include: []string{"app/**"}}
	cands := deploy.Candidates(root, statusChanges(st), true)
	jobs, skipped, notes := deployJobsFor(deploy.Target{Dir: "/var/www", BaseDir: "/"}, cands, filter)
	if len(notes) != 0 {
		t.Fatalf("notes = %v", notes)
	}
	if skipped != 1 {
		t.Fatalf("skipped = %d, want README", skipped)
	}

	want := map[string]deploy.Job{
		"app/a.php":    {Rel: "app/a.php", Local: filepath.Join(root, "app", "a.php"), Remote: "/var/www/app/a.php"},
		"app/gone.php": {Rel: "app/gone.php", Remote: "/var/www/app/gone.php", Delete: true},
		"app/new.php":  {Rel: "app/new.php", Local: filepath.Join(root, "app", "new.php"), Remote: "/var/www/app/new.php"},
		"app/old.php":  {Rel: "app/old.php", Remote: "/var/www/app/old.php", Delete: true},
	}
	if len(jobs) != len(want) {
		t.Fatalf("jobs = %+v", jobs)
	}
	for _, job := range jobs {
		w, ok := want[job.Rel]
		if !ok {
			t.Errorf("unexpected job %+v", job)
			continue
		}
		if job != w {
			t.Errorf("job %q = %+v, want %+v", job.Rel, job, w)
		}
		delete(want, job.Rel)
	}
	if len(want) != 0 {
		t.Errorf("missing jobs: %+v", want)
	}

	// Without autoDelete nothing is removed from the server.
	if jobs, _, _ := deployJobsFor(deploy.Target{Dir: "/srv"}, deploy.Candidates(root, statusChanges(st), false), deploy.Filter{}); len(jobs) != 3 {
		t.Fatalf("jobs without autoDelete = %+v", jobs)
	}
}

func TestDeployJobsRejectsEscapingPaths(t *testing.T) {
	cands := []deploy.Candidate{{Rel: "../secret", Local: "/tmp/secret"}}
	jobs, skipped, notes := deployJobsFor(deploy.Target{Dir: "/srv"}, cands, deploy.Filter{})
	if len(jobs) != 0 || skipped != 0 || len(notes) != 1 {
		t.Fatalf("jobs = %+v skipped = %d notes = %v", jobs, skipped, notes)
	}
}

func TestDeployConfirmBeforeDeleting(t *testing.T) {
	a := newApp()
	a.repo = &gitrepo.Repo{}
	a.store = projects.Store{Projects: []projects.Project{{
		Name:    "p",
		Targets: []deploy.Target{{Name: "t", Type: "sftp", Host: "h", User: "u", Port: 22, Dir: "/d"}},
	}}}
	a.projIdx = 0
	a.cfg = &deploy.Config{}
	a.deployOpen = true
	a.deployTarget = "t"
	a.deployAutoDelete = true
	a.deployJobs = []deploy.Job{
		{Rel: "a.php", Remote: "/d/a.php"},
		{Rel: "b.php", Remote: "/d/b.php"},
		{Rel: "c.php", Remote: "/d/c.php", Delete: true},
		{Rel: "d.php", Remote: "/d/d.php", Delete: true},
	}
	a.deployUploads, a.deployDeletes = 2, 2

	tt := ui.NewTester(func(c *ui.Context) { a.deployBody(c) }, 700, 500)
	tt.Frame()
	if err := tt.Click("Subir 4 archivos"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Se borrarán 2 archivos en el servidor.") {
		t.Fatalf("no confirmation shown, texts = %q", tt.Texts())
	}
	if a.deployBusy {
		t.Fatal("upload started without confirmation")
	}

	if err := tt.Click("Cancelar"); err != nil {
		t.Fatal(err)
	}
	if a.deployConfirm {
		t.Fatal("confirmation still pending after cancel")
	}
}
