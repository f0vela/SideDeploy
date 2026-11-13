package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"sidedeploy/internal/aicommits"
	"sidedeploy/internal/deploy"
	"sidedeploy/internal/gitrepo"
	"sidedeploy/internal/plan"
	"sidedeploy/internal/projects"
	"sidedeploy/internal/settings"
	"sidedeploy/internal/textdiff"
)

type pane int

const (
	paneSide pane = iota
	paneFiles
	paneDiff
	paneCommit
)

type mode int

const (
	mNone mode = iota
	mAddProject
	mTargets
	mTargetForm
	mDeploy
	mSettings
	mThemes
	mConfirm
	mError
	mHelp
)

type model struct {
	w, h    int
	dataDir string
	prefs   prefs
	st      styles

	store    projects.Store
	projIdx  int
	sideCur  int
	showSide bool

	repo     *gitrepo.Repo
	cfg      *deploy.Config
	cfgPath  string
	status   gitrepo.Status
	rows     []gitrepo.Row
	sel      int
	selKey   string
	selPath  string
	filesOff int

	focus pane
	mode  mode
	gitMu *sync.Mutex

	loadingStatus bool
	refreshQueued bool
	opening       bool
	pendingOpen   string

	diff        viewport.Model
	diffLines   []textdiff.Line
	diffTitle   string
	diffNote    string
	diffStaged  bool
	diffLoading bool
	diffSeq     int

	msg      textarea.Model
	busy     bool
	pushBusy bool
	aiBusy   bool
	aiCancel context.CancelFunc
	spin     spinner.Model

	flash    string
	flashSeq int
	err      string

	errTitle, errHint, errText string

	confirmTitle, confirmText, confirmYes string
	confirmAction                         func(*model) tea.Cmd

	addInput textinput.Model

	// targets
	tCur      int
	tConfirm  int
	form      []textinput.Model
	formType  string
	formFocus int
	formNew   bool
	formIndex int
	formErr   string
	draft     deploy.Target

	// deploy
	dTarget    string
	dAuto      bool
	dPlan      plan.Plan
	dBusy      bool
	dCancel    context.CancelFunc
	dCancelled bool
	dDone      int
	dTotal     int
	dFailed    int
	dLog       []string
	dFinished  string
	dUnknown   *deploy.UnknownHostError
	dErr       string
	dConfirm   bool
	dScroll    int
	dCh        chan tea.Msg
	bar        progress.Model

	// settings
	sFocus  int
	sInputs [2]textinput.Model
	sMsg    string
	sErr    bool

	// theme picker
	thCur    int
	thOrigin string
}

func newModel(dataDir string, p prefs, startPath string) *model {
	m := &model{
		dataDir: dataDir, prefs: p, gitMu: &sync.Mutex{},
		projIdx: -1, sel: -1, tConfirm: -1, showSide: true, focus: paneFiles,
		w: 120, h: 36,
	}
	m.store = projects.Load(dataDir)
	if m.store.Active == "" && len(m.store.Projects) == 0 {
		m.store.Active = settings.Load(dataDir).LastRepo
	}
	m.applyTheme(p.Theme)

	m.msg = textarea.New()
	m.msg.Placeholder = "Mensaje del commit"
	m.msg.ShowLineNumbers = false
	m.msg.CharLimit = 0
	m.msg.SetHeight(3)
	m.spin = spinner.New()
	m.spin.Spinner = spinner.MiniDot
	m.addInput = textinput.New()
	m.addInput.Placeholder = "/ruta/al/repositorio"
	m.diff = viewport.New(10, 10)
	m.bar = progress.New(progress.WithoutPercentage(), progress.WithSolidFill("#7aa2f7"))
	for i := range m.sInputs {
		m.sInputs[i] = textinput.New()
		m.sInputs[i].Placeholder = "vacío = PATH"
	}
	m.restyle()

	open := startPath
	if open == "" {
		open = m.store.Active
	}
	if open != "" {
		m.opening = true
		m.pendingOpen = open
	}
	return m
}

func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spin.Tick}
	if m.pendingOpen != "" {
		cmds = append(cmds, openCmd(m.pendingOpen))
	}
	return tea.Batch(cmds...)
}

func (m *model) applyTheme(key string) {
	if _, ok := themes[key]; !ok {
		key = defaultTheme
	}
	m.prefs.Theme = key
	m.st = newStyles(themes[key])
}

// restyle pushes the theme into the widgets that keep styles of their own.
func (m *model) restyle() {
	t := m.st.t
	m.bar.FullColor = t.Accent
	m.bar.EmptyColor = t.Border
	m.spin.Style = m.st.accent
	ta := &m.msg
	for _, s := range []*textarea.Style{&ta.FocusedStyle, &ta.BlurredStyle} {
		s.Base = m.st.input
		s.Text = m.st.input
		s.CursorLine = m.st.input
		s.Placeholder = m.st.input.Foreground(c(t.Muted))
		s.Prompt = m.st.input
	}
	ta.Cursor.Style = m.st.input.Foreground(c(t.Accent)).Reverse(true)
	ta.Prompt = " "
	styleInput := func(in *textinput.Model) {
		in.TextStyle = m.st.input
		in.PromptStyle = m.st.input.Foreground(c(t.Accent))
		in.PlaceholderStyle = m.st.input.Foreground(c(t.Muted))
		in.Cursor.Style = m.st.input.Foreground(c(t.Accent)).Reverse(true)
	}
	styleInput(&m.addInput)
	for i := range m.sInputs {
		styleInput(&m.sInputs[i])
	}
	for i := range m.form {
		styleInput(&m.form[i])
	}
	m.renderDiff()
}

// setTheme switches the theme now, and keeps it for the next run.
func (m *model) setTheme(key string, save bool) {
	m.applyTheme(key)
	m.restyle()
	if save {
		if err := savePrefs(m.dataDir, m.prefs); err != nil {
			m.err = "No se pudo guardar el tema: " + err.Error()
		}
	}
}

func (m *model) project() *projects.Project {
	if m.projIdx < 0 || m.projIdx >= len(m.store.Projects) {
		return nil
	}
	return &m.store.Projects[m.projIdx]
}

// targets are where the open project deploys: its own, or those of
// pro-deployer.json while it has none.
func (m *model) targets() []deploy.Target {
	if p := m.project(); p != nil && len(p.Targets) > 0 {
		return p.Targets
	}
	if m.cfg != nil {
		return m.cfg.Active()
	}
	return nil
}

func (m *model) activeTarget() *deploy.Target {
	list := m.targets()
	if len(list) == 0 {
		return nil
	}
	if p := m.project(); p != nil {
		for i := range list {
			if list[i].Name == p.ActiveTarget {
				return &list[i]
			}
		}
	}
	return &list[0]
}

func (m *model) saveStore() {
	if err := projects.Save(m.dataDir, m.store); err != nil {
		m.err = "No se pudo guardar projects.json: " + err.Error()
	}
}

func (m *model) aiTool() string {
	for _, t := range aicommits.Tools {
		if m.store.AITool == t {
			return t
		}
	}
	return aicommits.Claude
}

func (m *model) aiPath(tool string) string {
	if tool == aicommits.Copilot {
		return m.store.CopilotPath
	}
	return m.store.ClaudePath
}

func (m *model) stagedCount() int {
	seen := map[string]bool{}
	for _, r := range m.rows {
		if r.Staged {
			seen[r.File.Path] = true
		}
	}
	return len(seen)
}

func (m *model) changedCount() int {
	seen := map[string]bool{}
	for _, r := range m.rows {
		if !r.Header && !r.Staged {
			seen[r.File.Path] = true
		}
	}
	return len(seen)
}

func (m *model) branch() string {
	if m.status.Detached {
		return "HEAD separado"
	}
	return m.status.Branch
}

func (m *model) pushable() bool {
	return m.repo != nil && !m.status.Detached && m.status.Branch != "" &&
		(m.status.Ahead > 0 || m.status.Upstream == "")
}

func (m *model) pushLabel() string {
	switch {
	case m.status.Upstream == "":
		return "Publicar rama"
	case m.status.Ahead > 0:
		return fmt.Sprintf("Push · %d", m.status.Ahead)
	}
	return "Push"
}

func (m *model) showError(title, hint, text string) {
	m.errTitle, m.errHint, m.errText = title, hint, text
	m.mode = mError
}

func (m *model) flashMsg(s string) tea.Cmd {
	m.flash = s
	m.flashSeq++
	return clearFlash(m.flashSeq)
}

// expandPath turns what the user typed into an absolute folder.
func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
