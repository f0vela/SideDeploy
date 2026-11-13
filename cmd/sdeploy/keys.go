package main

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"sidedeploy/internal/gitrepo"
)

// onKey routes a key to the open dialog, or to the pane that has focus.
func (m *model) onKey(k tea.KeyMsg) tea.Cmd {
	if m.mode != mNone {
		return m.onModalKey(k)
	}
	key := k.String()

	// These work wherever the focus is, text box included.
	switch key {
	case "ctrl+c":
		return tea.Quit
	case "ctrl+t":
		m.openThemes()
		return nil
	case "tab":
		return m.cycleFocus(1)
	case "shift+tab":
		return m.cycleFocus(-1)
	case "esc":
		if m.aiBusy && m.aiCancel != nil {
			m.aiCancel()
			return nil
		}
		if m.err != "" {
			m.err = ""
			return nil
		}
		if m.focus == paneCommit {
			return m.setFocus(paneFiles)
		}
	}

	if m.focus == paneCommit {
		switch key {
		case "ctrl+s":
			return m.startCommit()
		case "ctrl+g":
			return m.startAI()
		case "ctrl+p":
			return m.startPush()
		}
		var cmd tea.Cmd
		m.msg, cmd = m.msg.Update(k)
		return cmd
	}

	// Keys of the whole window.
	switch key {
	case "q":
		return tea.Quit
	case "?":
		m.mode = mHelp
		return nil
	case ",":
		m.openSettings()
		return nil
	case "t":
		return m.cycleTheme(1)
	case "T":
		m.openThemes()
		return nil
	case "o":
		m.mode = mAddProject
		m.addInput.SetValue("")
		m.addInput.Focus()
		return nil
	case "b":
		m.showSide = !m.showSide
		if !m.showSide && m.focus == paneSide {
			m.focus = paneFiles
		}
		m.layout()
		return nil
	case "r":
		return m.refresh()
	case "c":
		if m.repo != nil {
			return m.setFocus(paneCommit)
		}
		return nil
	case "g":
		return m.startAI()
	case "p":
		return m.startPush()
	case "i":
		next := "claude"
		if m.aiTool() == "claude" {
			next = "copilot"
		}
		m.store.AITool = next
		m.saveStore()
		return m.flashMsg("IA: " + next)
	case "u":
		if m.deployReady() {
			m.openDeploy()
		} else if m.repo != nil && m.activeTarget() == nil {
			return m.flashMsg("No hay destino de despliegue: pulsa d para crear uno")
		}
		return nil
	case "d":
		m.openTargets()
		return nil
	case "enter":
		if m.focus == paneFiles && m.sel >= 0 {
			return m.setFocus(paneDiff)
		}
	}

	switch m.focus {
	case paneSide:
		return m.sideKey(key)
	case paneFiles:
		return m.filesKey(key)
	case paneDiff:
		return m.diffKey(k)
	}
	return nil
}

func (m *model) deployReady() bool {
	return m.repo != nil && m.activeTarget() != nil && len(m.status.Files) > 0 &&
		!m.loadingStatus && !m.dBusy
}

func (m *model) panes() []pane {
	ps := []pane{paneFiles, paneDiff, paneCommit}
	if m.sideVisible() {
		ps = append([]pane{paneSide}, ps...)
	}
	if m.repo == nil {
		return []pane{paneSide}
	}
	return ps
}

func (m *model) cycleFocus(delta int) tea.Cmd {
	ps := m.panes()
	cur := 0
	for i, p := range ps {
		if p == m.focus {
			cur = i
		}
	}
	return m.setFocus(ps[(cur+delta+len(ps))%len(ps)])
}

func (m *model) setFocus(p pane) tea.Cmd {
	m.focus = p
	if p == paneCommit {
		m.msg.Placeholder = "Escribe el mensaje del commit…  (ctrl+s lo confirma, esc sale)"
		return m.msg.Focus()
	}
	m.msg.Placeholder = "Pulsa c, o haz clic aquí, para escribir el mensaje del commit"
	m.msg.Blur()
	return nil
}

func (m *model) sideKey(key string) tea.Cmd {
	n := len(m.store.Projects)
	switch key {
	case "up", "k":
		if m.sideCur > 0 {
			m.sideCur--
		}
	case "down", "j":
		if m.sideCur < n-1 {
			m.sideCur++
		}
	case "home":
		m.sideCur = 0
	case "end", "G":
		m.sideCur = max(0, n-1)
	case "enter", " ", "right", "l":
		if m.sideCur < n {
			return m.openRepoPath(m.store.Projects[m.sideCur].Path)
		}
	case "x", "delete", "backspace":
		if m.sideCur < n {
			p := m.store.Projects[m.sideCur]
			m.confirm("¿Quitar “"+p.Name+"” de la lista?",
				"Se quita de SideDeploy; la carpeta no se toca.", "Quitar",
				func(m *model) tea.Cmd { m.removeProject(p.Path); return nil })
		}
	}
	return nil
}

func (m *model) filesKey(key string) tea.Cmd {
	switch key {
	case "up", "k":
		return m.moveSel(-1)
	case "down", "j":
		return m.moveSel(1)
	case "pgup":
		var cmd tea.Cmd
		for i := 0; i < 10; i++ {
			cmd = m.moveSel(-1)
		}
		return cmd
	case "pgdown":
		var cmd tea.Cmd
		for i := 0; i < 10; i++ {
			cmd = m.moveSel(1)
		}
		return cmd
	case "home":
		m.sel = m.firstFile()
		return m.selectRow()
	case " ", "s", "x":
		return m.toggleStage()
	case "a":
		return m.gitCmd(func(r *gitrepo.Repo, ctx context.Context) error { return r.StageAll(ctx) })
	case "A", "U":
		return m.gitCmd(func(r *gitrepo.Repo, ctx context.Context) error { return r.UnstageAll(ctx) })
	}
	return nil
}

func (m *model) toggleStage() tea.Cmd {
	if m.sel < 0 || m.sel >= len(m.rows) || m.rows[m.sel].Header {
		return nil
	}
	r := m.rows[m.sel]
	if r.Staged {
		return m.gitCmd(func(repo *gitrepo.Repo, ctx context.Context) error { return repo.Unstage(ctx, r.File.Path) })
	}
	return m.gitCmd(func(repo *gitrepo.Repo, ctx context.Context) error { return repo.Stage(ctx, r.File.Path) })
}

func (m *model) diffKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "left", "h", "esc":
		return m.setFocus(paneFiles)
	case "j":
		m.diff.ScrollDown(1)
		return nil
	case "k":
		m.diff.ScrollUp(1)
		return nil
	case "home":
		m.diff.GotoTop()
		return nil
	case "G", "end":
		m.diff.GotoBottom()
		return nil
	case " ":
		m.diff.PageDown()
		return nil
	}
	var cmd tea.Cmd
	m.diff, cmd = m.diff.Update(k)
	return cmd
}

// confirm asks a yes/no question before running action.
func (m *model) confirm(title, text, yes string, action func(*model) tea.Cmd) {
	m.confirmTitle, m.confirmText, m.confirmYes, m.confirmAction = title, text, yes, action
	m.mode = mConfirm
}

func trim(s string) string { return strings.TrimSpace(s) }
