package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

// onMouse focuses the pane under a click, picks a file or project, and
// scrolls the diff with the wheel.
func (m *model) onMouse(e tea.MouseMsg) tea.Cmd {
	if m.mode != mNone || m.w < 40 {
		return nil
	}
	sideW := m.sideW()
	inSide := e.X < sideW
	x := e.X - sideW // inside the right-hand area

	switch e.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		d := 1
		if e.Button == tea.MouseButtonWheelUp {
			d = -1
		}
		switch {
		case inSide:
			m.sideCur = max(0, min(len(m.store.Projects)-1, m.sideCur+d))
		case m.repo != nil && x > m.filesW():
			m.diff.ScrollDown(3 * d)
		case m.repo != nil && e.Y >= 2 && e.Y < 2+m.bodyH():
			return m.moveSel(d)
		}
		return nil
	case tea.MouseButtonLeft:
		if e.Action != tea.MouseActionPress {
			return nil
		}
	default:
		return nil
	}

	if inSide {
		if i := e.Y - 2; i >= 0 && i < len(m.store.Projects) {
			m.sideCur = i
			cmd := m.setFocus(paneSide)
			return tea.Batch(cmd, m.openRepoPath(m.store.Projects[i].Path))
		}
		return m.setFocus(paneSide)
	}
	if m.repo == nil {
		return nil
	}
	bodyTop, bodyEnd := 2, 2+m.bodyH()
	switch {
	case e.Y >= bodyTop && e.Y < bodyEnd && x < m.filesW():
		cmd := m.setFocus(paneFiles)
		if i := m.filesOff + e.Y - bodyTop - 1; i >= 0 && i < len(m.rows) && !m.rows[i].Header {
			m.sel = i
			return tea.Batch(cmd, m.selectRow())
		}
		return cmd
	case e.Y >= bodyTop && e.Y < bodyEnd:
		return m.setFocus(paneDiff)
	case e.Y > bodyEnd && e.Y < m.h-1:
		// The commit box and its buttons.
		return m.setFocus(paneCommit)
	}
	return nil
}
