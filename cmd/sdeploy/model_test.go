package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"sidedeploy/internal/deploy"
)

// run feeds a command's message back to the model, following batches and
// skipping timers and blocking waits.
func run(t *testing.T, m *model, cmd tea.Cmd) { runDepth(t, m, cmd, 0) }

func runDepth(t *testing.T, m *model, cmd tea.Cmd, depth int) {
	t.Helper()
	if cmd == nil || depth > 6 {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(400 * time.Millisecond):
		return
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			runDepth(t, m, c, depth+1)
		}
		return
	}
	if msg == nil {
		return
	}
	_, next := m.Update(msg)
	runDepth(t, m, next, depth+1)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(t *testing.T, m *model, keys ...string) {
	t.Helper()
	for _, k := range keys {
		_, cmd := m.Update(key(k))
		run(t, m, cmd)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newTestModel opens a repo with one staged and one untracked file.
func newTestModel(t *testing.T) (*model, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no está instalado")
	}
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	git(t, repo, "config", "user.email", "t@example.com")
	git(t, repo, "config", "user.name", "T")
	os.WriteFile(filepath.Join(repo, "a.php"), []byte("one\ntwo\n"), 0o644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "first")
	os.WriteFile(filepath.Join(repo, "a.php"), []byte("one\n2\nthree\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "b.php"), []byte("new\n"), 0o644)

	data := t.TempDir()
	m := newModel(data, loadPrefs(data), repo)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	run(t, m, openCmd(repo))
	return m, repo
}

func TestOpensAndShowsChanges(t *testing.T) {
	m, _ := newTestModel(t)
	view := ansi.Strip(m.View())
	for _, want := range []string{"Archivos", "a.php", "b.php", "@@ -1,2 +1,3 @@", "Commit", "PROYECTOS"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if lines := strings.Count(m.View(), "\n") + 1; lines != 36 {
		t.Errorf("view has %d lines, want 36", lines)
	}
}

func TestStageCommitFlow(t *testing.T) {
	m, repo := newTestModel(t)
	press(t, m, " ") // stage the selected file
	if m.stagedCount() != 1 {
		t.Fatalf("staged = %d", m.stagedCount())
	}
	press(t, m, "c")
	if m.focus != paneCommit {
		t.Fatal("c should focus the commit box")
	}
	press(t, m, "h", "o", "l", "a")
	press(t, m, "ctrl+s")
	if m.msg.Value() != "" || m.stagedCount() != 0 {
		t.Fatalf("commit did not go through: msg=%q staged=%d err=%q", m.msg.Value(), m.stagedCount(), m.err)
	}
	out, _ := exec.Command("git", "-C", repo, "log", "--format=%s", "-1").Output()
	if strings.TrimSpace(string(out)) != "hola" {
		t.Fatalf("last commit = %q", out)
	}
}

func TestThemeCycleAndPersist(t *testing.T) {
	m, _ := newTestModel(t)
	before := m.prefs.Theme
	press(t, m, "t")
	if m.prefs.Theme == before {
		t.Fatal("t should change the theme")
	}
	if got := loadPrefs(m.dataDir).Theme; got != m.prefs.Theme {
		t.Fatalf("saved theme = %q, want %q", got, m.prefs.Theme)
	}
	// The picker previews and esc restores.
	cur := m.prefs.Theme
	press(t, m, "T", "down", "down")
	if m.prefs.Theme == cur {
		t.Fatal("the picker should preview")
	}
	press(t, m, "esc")
	if m.prefs.Theme != cur {
		t.Fatalf("esc should restore %q, got %q", cur, m.prefs.Theme)
	}
}

func TestEveryThemeRendersFullHeight(t *testing.T) {
	m, _ := newTestModel(t)
	for _, k := range themeKeys() {
		m.setTheme(k, false)
		if n := strings.Count(m.View(), "\n") + 1; n != 36 {
			t.Errorf("theme %s: %d lines", k, n)
		}
	}
}

func TestTargetsAndDeployDialogs(t *testing.T) {
	m, _ := newTestModel(t)
	press(t, m, "d")
	if m.mode != mTargets {
		t.Fatalf("mode = %v", m.mode)
	}
	press(t, m, "a")
	if m.mode != mTargetForm {
		t.Fatal("a should open the form")
	}
	press(t, m, "s", "i", "t", "e") // name
	press(t, m, "tab", "tab")       // type, host
	press(t, m, "e", "x", ".", "c", "o", "m")
	press(t, m, "ctrl+s")
	p := m.project()
	if m.mode != mTargets || len(p.Targets) != 1 || p.Targets[0].Name != "site" || p.Targets[0].Host != "ex.com" {
		t.Fatalf("target not saved: mode=%v targets=%+v err=%q", m.mode, p.Targets, m.formErr)
	}
	if p.Targets[0].Port != 22 {
		t.Fatalf("port = %d, want 22 for sftp", p.Targets[0].Port)
	}
	press(t, m, "esc", "u")
	if m.mode != mDeploy || len(m.dPlan.Jobs) != 2 {
		t.Fatalf("deploy: mode=%v jobs=%d plan=%+v", m.mode, len(m.dPlan.Jobs), m.dPlan)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"Subir cambios", "a.php", "b.php", "2 para subir"} {
		if !strings.Contains(view, want) {
			t.Errorf("deploy dialog lacks %q:\n%s", want, view)
		}
	}
	press(t, m, "esc")
	if m.mode != mNone {
		t.Fatal("esc should close the dialog")
	}
}

func TestPushAndSettings(t *testing.T) {
	m, repo := newTestModel(t)
	remote := t.TempDir()
	git(t, remote, "init", "-q", "--bare")
	git(t, repo, "remote", "add", "origin", remote)
	press(t, m, "r")
	if !m.pushable() || m.pushLabel() != "Publicar rama" {
		t.Fatalf("pushable=%v label=%q", m.pushable(), m.pushLabel())
	}
	press(t, m, "p")
	if m.status.Upstream == "" || m.pushable() {
		t.Fatalf("push failed: upstream=%q err=%q mode=%v", m.status.Upstream, m.errText, m.mode)
	}

	press(t, m, ",")
	if m.mode != mSettings {
		t.Fatal(", should open settings")
	}
	press(t, m, "esc")
}

func TestLookupTheme(t *testing.T) {
	for _, name := range []string{"dracula", "Dracula", "Tokyo Night", "tokyo-night", "catppuccin-frappe", "Catppuccin Frappé"} {
		if _, ok := lookupTheme(name); !ok {
			t.Errorf("lookupTheme(%q) failed", name)
		}
	}
	if _, ok := lookupTheme("nope"); ok {
		t.Error("nope should not exist")
	}
}

var _ = deploy.Target{}

func TestManualCommitMessage(t *testing.T) {
	m, repo := newTestModel(t)
	press(t, m, " ")

	// A click on the box focuses it; letters that are shortcuts elsewhere
	// are just text there.
	_, cmd := m.Update(tea.MouseMsg{X: 60, Y: m.bodyH() + 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	run(t, m, cmd)
	if m.focus != paneCommit {
		t.Fatalf("click should focus the commit box, focus = %v", m.focus)
	}
	press(t, m, "q", "u", "i", "t", "a", "r")
	if m.mode != mNone || m.msg.Value() != "quitar" {
		t.Fatalf("typed %q, mode %v", m.msg.Value(), m.mode)
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	run(t, m, cmd)
	press(t, m, "d", "e", "t", "a", "l", "l", "e")
	if got := m.msg.Value(); got != "quitar\ndetalle" {
		t.Fatalf("multiline message = %q", got)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "6/72") {
		t.Errorf("view should count the subject:\n%s", view)
	}

	// esc leaves the box without losing the text.
	press(t, m, "esc")
	if m.focus == paneCommit || m.msg.Value() == "" {
		t.Fatalf("esc: focus=%v text=%q", m.focus, m.msg.Value())
	}
	press(t, m, "c", "ctrl+s")
	out, _ := exec.Command("git", "-C", repo, "log", "--format=%B", "-1").Output()
	if strings.TrimSpace(string(out)) != "quitar\n\ndetalle" && strings.TrimSpace(string(out)) != "quitar\ndetalle" {
		t.Fatalf("commit message = %q", out)
	}
}

func TestAIMessageIsEditable(t *testing.T) {
	m, _ := newTestModel(t)
	_, cmd := m.Update(aiDoneMsg{repo: m.repo, text: "feat: algo"})
	run(t, m, cmd)
	if m.focus != paneCommit || m.msg.Value() != "feat: algo" {
		t.Fatalf("focus=%v text=%q", m.focus, m.msg.Value())
	}
	press(t, m, "!")
	if m.msg.Value() != "feat: algo!" {
		t.Fatalf("the AI text should be editable, got %q", m.msg.Value())
	}
}
