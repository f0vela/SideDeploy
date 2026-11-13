package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"sidedeploy/internal/deploy"
	"sidedeploy/internal/gitrepo"
	"sidedeploy/internal/projects"
)

func TestWelcomeView(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1200, 760)
	if !tt.HasText("SideDeploy") || !tt.HasText("Abrir repositorio") {
		t.Fatalf("texts = %q", tt.Texts())
	}
}

func TestChangesView(t *testing.T) {
	a := newApp()
	a.repo = &gitrepo.Repo{Root: "/tmp/example-repo"}
	a.branch = "main"
	a.rows = buildRows(gitrepo.Status{Files: []gitrepo.File{
		{Path: "staged.go", Index: 'M', Worktree: '.'},
		{Path: "old.txt", OrigPath: "older.txt", Index: 'R', Worktree: '.'},
		{Path: "work.txt", Index: '.', Worktree: 'M'},
		{Path: "new.txt", Index: '?', Worktree: '?'},
		{Path: "both.txt", Index: 'M', Worktree: 'M'},
	}})
	a.selected = firstFileIndex(a.rows)
	a.selKey = rowKey(a.rows[a.selected])
	a.diffTitle = a.rows[a.selected].File.Path
	a.diffNote = "Sin cambios de contenido"

	tt := ui.NewTester(a.view, 1200, 760)
	for _, want := range []string{
		"STAGED · 3",
		"CAMBIOS · 3",
		"staged.go",
		"older.txt  →  old.txt",
		"work.txt",
		"new.txt",
		"main",
	} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	if staged, changed := a.stagedCount(), a.changedCount(); staged != 3 || changed != 3 {
		t.Errorf("counts = %d staged, %d changed", staged, changed)
	}
}

func TestRowsSplitStagedAndWorktree(t *testing.T) {
	rows := buildRows(gitrepo.Status{Files: []gitrepo.File{
		{Path: "both.txt", Index: 'M', Worktree: 'M'},
	}})
	if len(rows) != 4 {
		t.Fatalf("rows = %+v", rows)
	}
	if !rows[0].Header || !rows[1].Staged || !rows[2].Header || rows[3].Staged {
		t.Fatalf("unexpected groups: %+v", rows)
	}
}

func testApp(t *testing.T) *app {
	t.Helper()
	a := newApp()
	a.dataDir = t.TempDir()
	return a
}

func TestProjectsPane(t *testing.T) {
	a := testApp(t)
	a.store.Projects = []projects.Project{
		{Name: "sitio-a", Path: "/tmp/a"},
		{Name: "sitio-b", Path: "/tmp/b", Targets: []deploy.Target{{Name: "CDD"}}},
	}
	a.selPath = "/tmp/a"

	tt := ui.NewTester(a.view, 1280, 820)
	for _, want := range []string{"Proyectos", "sitio-a", "sitio-b", "Añadir proyecto"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestTargetsModal(t *testing.T) {
	a := testApp(t)
	a.store.Projects = []projects.Project{{
		Name:         "sitio",
		Path:         "/tmp/a",
		ActiveTarget: "CDD",
		Targets: []deploy.Target{
			{Name: "CDD", Type: "sftp", Host: "example.com", Port: 5222, Dir: "/var/www/sitio"},
		},
	}}
	a.projIdx = 0
	a.targetsOpen = true

	tt := ui.NewTester(a.view, 1280, 820)
	for _, want := range []string{"Destinos de sitio", "CDD", "Editar", "Eliminar", "Añadir destino"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestTargetForm(t *testing.T) {
	a := testApp(t)
	a.store.Projects = []projects.Project{{Name: "sitio", Path: "/tmp/a"}}
	a.projIdx = 0
	a.targetsOpen = true
	a.beginEdit(-1)

	tt := ui.NewTester(a.view, 1280, 820)
	for _, want := range []string{"Nuevo destino", "Nombre", "Host", "Carpeta remota", "Guardar"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestSaveTarget(t *testing.T) {
	a := testApp(t)
	a.store.Projects = []projects.Project{{Name: "sitio", Path: "/tmp/a"}}
	a.projIdx = 0

	a.beginEdit(-1)
	if a.draft.Type != "sftp" || a.draft.Dir != "/" {
		t.Fatalf("draft defaults = %+v", a.draft)
	}
	a.saveTarget()
	if a.formErr == "" {
		t.Fatal("a target without a name was saved")
	}

	a.draft.Name = "CDD"
	a.draft.Host = "example.com"
	a.saveTarget()
	p := a.project()
	if len(p.Targets) != 1 || p.ActiveTarget != "CDD" {
		t.Fatalf("project = %+v", p)
	}
	if got := p.Targets[0]; got.Port != 22 || got.TransferDataType != "binary" || got.Dir != "/" {
		t.Fatalf("defaults not applied: %+v", got)
	}
	if a.targetEdit {
		t.Fatal("the form stayed open after saving")
	}

	a.beginEdit(-1)
	a.draft.Name = "CDD"
	a.draft.Host = "other.example.com"
	a.saveTarget()
	if a.formErr == "" || len(a.project().Targets) != 1 {
		t.Fatalf("duplicate name accepted: %+v", a.project().Targets)
	}

	a.beginEdit(0)
	a.draft.Name = "CDD producción"
	a.draftPort = "5222"
	a.draft.Password = "s3cret"
	a.saveTarget()
	p = a.project()
	if p.Targets[0].Name != "CDD producción" || p.Targets[0].Port != 5222 {
		t.Fatalf("target = %+v", p.Targets[0])
	}
	if p.ActiveTarget != "CDD producción" {
		t.Fatalf("active = %q after renaming it", p.ActiveTarget)
	}

	a.beginEdit(0)
	a.draftPort = "nope"
	a.saveTarget()
	if a.formErr == "" {
		t.Fatal("a bad port was accepted")
	}

	a.targetEdit = false
	a.deleteTarget(0)
	p = a.project()
	if len(p.Targets) != 0 || p.ActiveTarget != "" {
		t.Fatalf("after delete = %+v", p)
	}

	// The store on disk keeps what was saved.
	if got := projects.Load(a.dataDir); len(got.Projects) != 1 || len(got.Projects[0].Targets) != 0 {
		t.Fatalf("stored = %+v", got)
	}
}

func TestOpenTargetsAdoptsConfig(t *testing.T) {
	a := testApp(t)
	a.store.Projects = []projects.Project{{Name: "sitio", Path: "/tmp/a"}}
	a.projIdx = 0
	a.cfg = &deploy.Config{
		ActiveTargets: []string{"b"},
		Targets:       []deploy.Target{{Name: "a"}, {Name: "b"}},
	}

	a.openTargets()
	p := a.project()
	if len(p.Targets) != 2 || p.ActiveTarget != "b" {
		t.Fatalf("adopted = %+v", p)
	}
	if got := a.activeTarget(); got == nil || got.Name != "b" {
		t.Fatalf("active target = %+v", got)
	}
	if got := projects.Load(a.dataDir); len(got.Projects[0].Targets) != 2 {
		t.Fatalf("not stored: %+v", got)
	}
}

func TestAIToolDefault(t *testing.T) {
	a := testApp(t)
	if got := a.aiTool(); got != "claude" {
		t.Fatalf("default tool = %q", got)
	}
	a.store.AITool = "copilot"
	if got := a.aiTool(); got != "copilot" {
		t.Fatalf("chosen tool = %q", got)
	}
	a.store.AITool = "otra"
	if got := a.aiTool(); got != "claude" {
		t.Fatalf("unknown tool = %q, want the default", got)
	}
}

func TestCommitBarAI(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.commitBar, 900, 120)
	for _, want := range []string{"claude", "Generar", "Commit"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}

	a.aiBusy = true
	tt = ui.NewTester(a.commitBar, 900, 120)
	if !tt.HasText("Cancelar") {
		t.Fatalf("busy bar lacks Cancelar: %q", tt.Texts())
	}
	if tt.HasText("Generar") {
		t.Fatalf("busy bar still offers Generar: %q", tt.Texts())
	}
}

func TestShowError(t *testing.T) {
	a := testApp(t)
	a.showError("La IA no generó el mensaje", "Revisa el login.", "no encuentro «claude»")
	if !a.errorOpen || a.errorTitle == "" || a.errorHint == "" {
		t.Fatalf("error = %+v", a)
	}
	tt := ui.NewTester(a.view, 1280, 820)
	for _, want := range []string{"La IA no generó el mensaje", "Revisa el login.", "no encuentro «claude»"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}
