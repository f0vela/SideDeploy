package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"sidedeploy/internal/aicommits"
	"sidedeploy/internal/projects"
)

func (m *model) onModalKey(k tea.KeyMsg) tea.Cmd {
	key := k.String()
	if key == "ctrl+c" {
		return tea.Quit
	}
	switch m.mode {
	case mAddProject:
		return m.addKey(k)
	case mTargets:
		return m.targetsKey(key)
	case mTargetForm:
		return m.formKey(k)
	case mDeploy:
		return m.deployKey(key)
	case mSettings:
		return m.settingsKey(k)
	case mThemes:
		return m.themesKey(key)
	case mConfirm:
		switch key {
		case "y", "enter":
			m.mode = mNone
			if m.confirmAction != nil {
				return m.confirmAction(m)
			}
		case "n", "esc", "q":
			m.mode = mNone
			m.confirmAction = nil
		}
	case mError:
		switch key {
		case "enter", "esc", "q", " ":
			m.mode = mNone
		}
	case mHelp:
		m.mode = mNone
	}
	return nil
}

// --- add project ---

func (m *model) addKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.mode = mNone
		m.addInput.Blur()
		return nil
	case "enter":
		path := trim(m.addInput.Value())
		if path == "" {
			return nil
		}
		m.mode = mNone
		m.addInput.Blur()
		return m.openRepoPath(expandPath(path))
	}
	var cmd tea.Cmd
	m.addInput, cmd = m.addInput.Update(k)
	return cmd
}

// --- targets ---

func (m *model) openTargets() {
	p := m.project()
	if p == nil {
		return
	}
	if p.Adopt(m.cfg) {
		m.saveStore()
	}
	m.mode = mTargets
	m.tCur, m.tConfirm, m.formErr = 0, -1, ""
	for i := range p.Targets {
		if p.Targets[i].Name == p.ActiveTarget {
			m.tCur = i
		}
	}
}

func (m *model) targetsKey(key string) tea.Cmd {
	p := m.project()
	if p == nil {
		m.mode = mNone
		return nil
	}
	if m.tConfirm >= 0 {
		switch key {
		case "y", "enter":
			p.DeleteTarget(m.tConfirm)
			m.saveStore()
			m.tCur = min(m.tCur, max(0, len(p.Targets)-1))
			m.tConfirm = -1
		case "n", "esc":
			m.tConfirm = -1
		}
		return nil
	}
	switch key {
	case "esc", "q", "d":
		m.mode = mNone
	case "up", "k":
		if m.tCur > 0 {
			m.tCur--
		}
	case "down", "j":
		if m.tCur < len(p.Targets)-1 {
			m.tCur++
		}
	case " ", "enter":
		if m.tCur < len(p.Targets) {
			p.ActiveTarget = p.Targets[m.tCur].Name
			m.saveStore()
		}
	case "a", "n":
		m.beginEdit(-1)
	case "e":
		if m.tCur < len(p.Targets) {
			m.beginEdit(m.tCur)
		}
	case "x", "delete", "backspace":
		if m.tCur < len(p.Targets) {
			m.tConfirm = m.tCur
		}
	}
	return nil
}

var formLabels = []string{"Nombre", "Tipo", "Host", "Puerto", "Usuario", "Contraseña", "Clave privada", "Carpeta remota", "baseDir"}

// formInput maps a form row to its text input; the type row has none.
func formInput(row int) int {
	switch {
	case row == 1:
		return -1
	case row == 0:
		return 0
	}
	return row - 1
}

func (m *model) beginEdit(i int) tea.Cmd {
	p := m.project()
	if p == nil {
		return nil
	}
	m.formIndex = i
	m.formNew = i < 0 || i >= len(p.Targets)
	if m.formNew {
		m.draft = projects.NewTarget()
	} else {
		m.draft = p.Targets[i]
	}
	port, key := projects.DraftFields(m.draft)
	vals := []string{m.draft.Name, m.draft.Host, port, m.draft.User, m.draft.Password, key, m.draft.Dir, m.draft.BaseDir}
	holders := []string{"mi-servidor", "ftp.ejemplo.com", "por defecto", "", "", "~/.ssh/id_ed25519", "/var/www/mi-sitio", "/"}
	m.form = make([]textinput.Model, len(vals))
	for j := range m.form {
		in := textinput.New()
		in.Prompt = ""
		in.SetValue(vals[j])
		in.Placeholder = holders[j]
		in.CharLimit = 0
		if j == 4 {
			in.EchoMode = textinput.EchoPassword
		}
		m.form[j] = in
	}
	m.formType = m.draft.Type
	m.formErr = ""
	m.mode = mTargetForm
	m.restyle()
	return m.focusForm(0)
}

func (m *model) focusForm(row int) tea.Cmd {
	m.formFocus = (row + len(formLabels)) % len(formLabels)
	var cmd tea.Cmd
	for i := range m.form {
		m.form[i].Blur()
	}
	if i := formInput(m.formFocus); i >= 0 {
		cmd = m.form[i].Focus()
	}
	return cmd
}

func (m *model) saveForm() {
	p := m.project()
	if p == nil {
		return
	}
	d := m.draft
	d.Name, d.Type, d.Host = m.form[0].Value(), m.formType, m.form[1].Value()
	d.User, d.Password = m.form[3].Value(), m.form[4].Value()
	d.Dir, d.BaseDir = m.form[6].Value(), m.form[7].Value()
	index := m.formIndex
	if m.formNew {
		index = -1
	}
	if err := p.SaveTarget(index, d, m.form[2].Value(), m.form[5].Value()); err != nil {
		m.formErr = err.Error()
		return
	}
	m.saveStore()
	m.formErr = ""
	m.mode = mTargets
}

func (m *model) formKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.mode, m.formErr = mTargets, ""
		return nil
	case "ctrl+s":
		m.saveForm()
		return nil
	case "tab", "down":
		return m.focusForm(m.formFocus + 1)
	case "shift+tab", "up":
		return m.focusForm(m.formFocus - 1)
	case "enter":
		if m.formFocus == len(formLabels)-1 {
			m.saveForm()
			return nil
		}
		return m.focusForm(m.formFocus + 1)
	}
	if m.formFocus == 1 {
		switch k.String() {
		case "left", "right", " ", "h", "l":
			if m.formType == "sftp" {
				m.formType = "ftp"
			} else {
				m.formType = "sftp"
			}
		}
		return nil
	}
	var cmd tea.Cmd
	if i := formInput(m.formFocus); i >= 0 {
		m.form[i], cmd = m.form[i].Update(k)
	}
	return cmd
}

// --- deploy ---

func (m *model) deployKey(key string) tea.Cmd {
	if m.dUnknown != nil {
		switch key {
		case "y", "enter":
			return m.trustHost()
		case "n", "esc":
			m.dUnknown, m.dErr = nil, ""
		}
		return nil
	}
	if m.dBusy {
		switch key {
		case "c", "esc", "x":
			m.cancelDeploy()
		}
		return nil
	}
	if m.dConfirm {
		switch key {
		case "y", "enter":
			m.dConfirm = false
			return m.startDeploy()
		case "n", "esc":
			m.dConfirm = false
		}
		return nil
	}
	list := m.targets()
	switch key {
	case "esc", "q":
		m.closeDeploy()
	case "left", "h", "right", "l", "[", "]":
		if len(list) > 1 {
			cur := 0
			for i := range list {
				if list[i].Name == m.dTarget {
					cur = i
				}
			}
			d := 1
			if key == "left" || key == "h" || key == "[" {
				d = -1
			}
			m.dTarget = list[(cur+d+len(list))%len(list)].Name
			m.dFinished = ""
			m.buildPlan()
		}
	case "a":
		m.dAuto = !m.dAuto
		m.dFinished = ""
		m.buildPlan()
	case "r":
		m.dFinished = ""
		return m.refresh()
	case "up", "k":
		m.dScroll = max(0, m.dScroll-1)
	case "down", "j":
		m.dScroll = min(max(0, len(m.dPlan.Jobs)-1), m.dScroll+1)
	case "enter", "u":
		if len(m.dPlan.Jobs) == 0 || m.loadingStatus {
			return nil
		}
		if m.dPlan.Deletes > 1 {
			m.dConfirm = true
			return nil
		}
		return m.startDeploy()
	}
	return nil
}

// --- settings ---

func (m *model) openSettings() {
	m.mode = mSettings
	m.sFocus = 0
	m.sInputs[0].SetValue(m.store.ClaudePath)
	m.sInputs[1].SetValue(m.store.CopilotPath)
	m.sMsg, m.sErr = "", false
	m.focusSettings(0)
}

func (m *model) focusSettings(i int) {
	m.sFocus = (i + 3) % 3
	for j := range m.sInputs {
		m.sInputs[j].Blur()
	}
	if m.sFocus > 0 {
		m.sInputs[m.sFocus-1].Focus()
	}
}

func (m *model) saveSettings() {
	m.store.ClaudePath = trim(m.sInputs[0].Value())
	m.store.CopilotPath = trim(m.sInputs[1].Value())
	if err := projects.Save(m.dataDir, m.store); err != nil {
		m.sErr, m.sMsg = true, "No se pudo guardar: "+err.Error()
		return
	}
	m.sErr, m.sMsg = false, "Guardado."
}

func (m *model) settingsKey(k tea.KeyMsg) tea.Cmd {
	key := k.String()
	switch key {
	case "esc":
		m.mode = mNone
		return nil
	case "ctrl+s":
		m.saveSettings()
		return nil
	case "tab", "down":
		m.focusSettings(m.sFocus + 1)
		return nil
	case "shift+tab", "up":
		m.focusSettings(m.sFocus - 1)
		return nil
	}
	if m.sFocus == 0 {
		switch key {
		case "left", "h":
			m.cycleTheme(-1)
		case "right", "l":
			m.cycleTheme(1)
		case "enter":
			m.openThemes()
		}
		return nil
	}
	if key == "enter" {
		m.saveSettings()
		return nil
	}
	var cmd tea.Cmd
	m.sInputs[m.sFocus-1], cmd = m.sInputs[m.sFocus-1].Update(k)
	return cmd
}

// resolved tells where a tool resolves to with the path typed so far.
func (m *model) resolved(tool string, typed string) (string, error) {
	return aicommits.Resolve(tool, trim(typed))
}

// --- theme picker ---

func (m *model) openThemes() {
	m.thOrigin = m.prefs.Theme
	keys := themeKeys()
	for i, k := range keys {
		if k == m.prefs.Theme {
			m.thCur = i
		}
	}
	m.mode = mThemes
}

func (m *model) themesKey(key string) tea.Cmd {
	keys := themeKeys()
	move := func(d int) {
		m.thCur = (m.thCur + d + len(keys)) % len(keys)
		m.setTheme(keys[m.thCur], false)
	}
	switch key {
	case "up", "k":
		move(-1)
	case "down", "j", "t":
		move(1)
	case "pgup":
		move(-5)
	case "pgdown":
		move(5)
	case "home":
		move(-m.thCur)
	case "end":
		move(len(keys) - 1 - m.thCur)
	case "enter", " ":
		m.setTheme(keys[m.thCur], true)
		m.mode = mNone
		return m.flashMsg("Tema: " + themes[keys[m.thCur]].Name)
	case "esc", "q":
		m.setTheme(m.thOrigin, false)
		m.mode = mNone
	}
	return nil
}
