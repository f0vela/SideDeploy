package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"sidedeploy/internal/gitrepo"
)

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.layout()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case flashClearMsg:
		if msg.seq == m.flashSeq {
			m.flash = ""
		}
		return m, nil

	case openedMsg:
		return m, m.onOpened(msg)

	case statusMsg:
		if msg.repo != m.repo {
			return m, nil
		}
		m.loadingStatus = false
		var cmd tea.Cmd
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			cmd = m.applyStatus(msg.st)
		}
		if m.refreshQueued {
			m.refreshQueued = false
			return m, tea.Batch(cmd, m.refresh())
		}
		return m, cmd

	case diffMsg:
		if msg.repo != m.repo || msg.seq != m.diffSeq {
			return m, nil
		}
		m.diffLoading = false
		m.diffTitle, m.diffStaged = msg.title, msg.staged
		m.diffLines, m.diffNote = msg.lines, msg.note
		if msg.err != nil {
			m.diffLines, m.diffNote = nil, msg.err.Error()
		}
		m.diff.GotoTop()
		m.renderDiff()
		return m, nil

	case gitDoneMsg:
		if msg.repo != m.repo {
			return m, nil
		}
		if msg.err != nil {
			m.err = msg.err.Error()
		}
		return m, m.refresh()

	case commitDoneMsg:
		m.busy = false
		if msg.repo != m.repo {
			return m, nil
		}
		var cmd tea.Cmd
		if msg.err != nil {
			m.showError("Git no completó la operación",
				"Suele ser un hook del repo. El índice quedó como estaba.", msg.err.Error())
		} else {
			m.msg.Reset()
			cmd = m.flashMsg("Commit hecho")
		}
		return m, tea.Batch(cmd, m.refresh())

	case pushDoneMsg:
		m.pushBusy = false
		if msg.repo != m.repo {
			return m, nil
		}
		var cmd tea.Cmd
		if msg.err != nil {
			m.showError("No se pudo hacer push",
				"Git no pide contraseña aquí: hace falta un credential helper o un agente SSH, y que el remoto acepte la rama.",
				msg.err.Error())
		} else {
			cmd = m.flashMsg("Push hecho")
		}
		return m, tea.Batch(cmd, m.refresh())

	case aiDoneMsg:
		m.aiBusy = false
		m.aiCancel = nil
		if msg.repo != m.repo {
			return m, nil
		}
		switch {
		case isCanceled(msg.err):
			// The user canceled: keep what they had written.
		case isDeadline(msg.err):
			m.showError("La IA tardó demasiado",
				"El límite son 2 minutos. Prueba de nuevo o con menos cambios en el stage.",
				"El comando no terminó a tiempo.")
		case msg.err != nil:
			m.showError("La IA no generó el mensaje",
				"Revisa la ruta en Ajustes (,) y que la herramienta tenga la sesión iniciada.", msg.err.Error())
		default:
			m.msg.SetValue(msg.text)
			// Land in the box, ready to edit what the AI wrote.
			return m, m.setFocus(paneCommit)
		}
		return m, nil

	case deployResultMsg:
		return m, m.onDeployResult(msg)
	case deployDoneMsg:
		m.onDeployDone()
		return m, nil

	case tea.KeyMsg:
		return m, m.onKey(msg)

	case tea.MouseMsg:
		return m, m.onMouse(msg)
	}

	// Anything else (cursor blinks, ...) goes to the widget that has focus.
	return m, m.passToWidgets(msg)
}

func (m *model) passToWidgets(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch m.mode {
	case mNone:
		if m.focus == paneCommit {
			m.msg, cmd = m.msg.Update(msg)
		}
	case mAddProject:
		m.addInput, cmd = m.addInput.Update(msg)
	case mTargetForm:
		if i := formInput(m.formFocus); i >= 0 && i < len(m.form) {
			m.form[i], cmd = m.form[i].Update(msg)
		}
	case mSettings:
		if m.sFocus > 0 {
			m.sInputs[m.sFocus-1], cmd = m.sInputs[m.sFocus-1].Update(msg)
		}
	}
	return cmd
}

// onOpened adopts the repo an open command found.
func (m *model) onOpened(msg openedMsg) tea.Cmd {
	m.opening = false
	m.pendingOpen = ""
	if msg.err != nil {
		m.err = msg.err.Error()
		m.syncSide()
		return nil
	}
	m.closeDeploy()
	if m.aiCancel != nil {
		m.aiCancel()
	}
	m.repo, m.cfg, m.cfgPath = msg.repo, msg.cfg, msg.cfgPath
	m.projIdx = m.store.Add(msg.repo.Root)
	m.store.Active = msg.repo.Root
	m.saveStore()
	m.syncSide()
	m.status = gitrepo.Status{}
	m.rows, m.sel, m.selKey, m.selPath = nil, -1, "", ""
	m.filesOff = 0
	m.diffLines, m.diffTitle, m.diffNote = nil, "", ""
	m.diffSeq++
	m.diffLoading = false
	m.loadingStatus, m.refreshQueued = false, false
	m.renderDiff()
	if msg.cfgErr != nil {
		m.err = "Config: " + msg.cfgErr.Error()
	}
	cmd := m.refresh()
	if m.focus == paneSide {
		return tea.Batch(cmd, m.setFocus(paneFiles))
	}
	return cmd
}

// syncSide puts the sidebar cursor on the open project.
func (m *model) syncSide() {
	if m.projIdx >= 0 {
		m.sideCur = m.projIdx
	}
	if m.sideCur >= len(m.store.Projects) {
		m.sideCur = max(0, len(m.store.Projects)-1)
	}
}

func (m *model) refresh() tea.Cmd {
	if m.repo == nil {
		return nil
	}
	if m.loadingStatus {
		m.refreshQueued = true
		return nil
	}
	m.loadingStatus = true
	return statusCmd(m.repo)
}

func (m *model) applyStatus(st gitrepo.Status) tea.Cmd {
	m.status = st
	m.rows = gitrepo.BuildRows(st)
	prev := m.selKey
	m.sel = -1
	for i, r := range m.rows {
		if prev != "" && r.Key() == prev {
			m.sel = i
		}
	}
	if m.sel < 0 && m.selPath != "" {
		for i, r := range m.rows {
			if !r.Header && r.File.Path == m.selPath {
				m.sel = i
				break
			}
		}
	}
	if m.sel < 0 {
		m.sel = m.firstFile()
	}
	return m.selectRow()
}

func (m *model) firstFile() int {
	for i, r := range m.rows {
		if !r.Header {
			return i
		}
	}
	return -1
}

// selectRow records the row under the cursor and loads its diff.
func (m *model) selectRow() tea.Cmd {
	m.diffSeq++
	if m.sel < 0 || m.sel >= len(m.rows) || m.rows[m.sel].Header {
		m.selKey = ""
		m.diffLines, m.diffTitle, m.diffNote = nil, "", ""
		if len(m.rows) == 0 {
			m.diffNote = "No hay cambios para mostrar."
		} else {
			m.diffNote = "Selecciona un archivo para ver sus cambios."
		}
		m.diffLoading = false
		m.renderDiff()
		return nil
	}
	r := m.rows[m.sel]
	m.selKey, m.selPath = r.Key(), r.File.Path
	m.diffLoading = true
	return diffCmd(m.repo, r, m.diffSeq)
}

// moveSel moves the cursor over the file rows, skipping headers.
func (m *model) moveSel(delta int) tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	i := m.sel
	for {
		i += delta
		if i < 0 || i >= len(m.rows) {
			return nil
		}
		if !m.rows[i].Header {
			break
		}
	}
	m.sel = i
	return m.selectRow()
}

func (m *model) startCommit() tea.Cmd {
	text := strings.TrimSpace(m.msg.Value())
	switch {
	case m.repo == nil || m.busy:
		return nil
	case text == "":
		m.err = "Escribe un mensaje para el commit."
		return nil
	case m.stagedCount() == 0:
		m.err = "No hay cambios en el stage."
		return nil
	}
	m.busy = true
	m.err = ""
	return m.commitCmd(text)
}

func (m *model) startPush() tea.Cmd {
	if m.repo == nil || m.pushBusy || m.busy {
		return nil
	}
	if !m.pushable() {
		return m.flashMsg("No hay commits pendientes de enviar")
	}
	m.pushBusy = true
	m.err = ""
	return m.pushCmd()
}

func (m *model) startAI() tea.Cmd {
	if m.repo == nil || m.aiBusy {
		return nil
	}
	if m.stagedCount()+m.changedCount() == 0 {
		return m.flashMsg("No hay cambios que describir")
	}
	m.aiBusy = true
	m.err = ""
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	m.aiCancel = cancel
	return m.aiCmd(ctx)
}

func (m *model) cycleTheme(delta int) tea.Cmd {
	keys := themeKeys()
	cur := 0
	for i, k := range keys {
		if k == m.prefs.Theme {
			cur = i
		}
	}
	next := keys[(cur+delta+len(keys))%len(keys)]
	m.setTheme(next, true)
	return m.flashMsg("Tema: " + themes[next].Name)
}

func (m *model) openRepoPath(path string) tea.Cmd {
	m.opening = true
	m.err = ""
	return openCmd(path)
}

func (m *model) removeProject(path string) {
	m.store.Remove(path)
	m.saveStore()
	if m.repo != nil && m.repo.Root == path {
		m.closeProject()
	}
	m.projIdx = m.store.Index(m.currentRoot())
	m.syncSide()
}

func (m *model) currentRoot() string {
	if m.repo != nil {
		return m.repo.Root
	}
	return ""
}

func (m *model) closeProject() {
	if m.aiCancel != nil {
		m.aiCancel()
	}
	m.closeDeploy()
	m.repo, m.cfg, m.cfgPath = nil, nil, ""
	m.projIdx = -1
	m.status = gitrepo.Status{}
	m.rows, m.sel, m.selKey, m.selPath = nil, -1, "", ""
	m.diffLines, m.diffTitle, m.diffNote = nil, "", ""
	m.diffSeq++
	m.loadingStatus, m.refreshQueued, m.diffLoading = false, false, false
	m.renderDiff()
}

func (m *model) summary() string {
	staged, changed := 0, 0
	for _, f := range m.status.Files {
		if f.Staged() {
			staged++
		}
		if f.Unstaged() {
			changed++
		}
	}
	return fmt.Sprintf("%d staged · %d cambios", staged, changed)
}
