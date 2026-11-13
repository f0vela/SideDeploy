package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"sidedeploy/internal/textdiff"
)

const (
	sideWidth    = 26
	sideMinWidth = 100
	commitRows   = 3
	// header, rule, rule, commit title, textarea, actions, status bar
	chromeRows = 1 + 1 + 1 + 1 + commitRows + 1 + 1
)

// sideVisible: the project list needs room, except while no repo is open,
// when it is the way in.
func (m *model) sideVisible() bool {
	return m.showSide && (m.w >= sideMinWidth || (m.repo == nil && m.w >= 60))
}

func (m *model) sideW() int {
	if m.sideVisible() {
		return sideWidth
	}
	return 0
}

func (m *model) bodyH() int { return max(3, m.h-chromeRows) }

func (m *model) filesW() int {
	mainW := m.w - m.sideW()
	return max(24, min(mainW*2/5, 52))
}

func (m *model) diffW() int { return max(10, m.w-m.sideW()-m.filesW()-1) }

// layout sizes the widgets for the window.
func (m *model) layout() {
	m.diff.Width = m.diffW()
	m.diff.Height = max(1, m.bodyH()-1)
	m.msg.SetWidth(max(10, m.w-m.sideW()-2))
	m.msg.SetHeight(commitRows)
	m.bar.Width = 40
	m.renderDiff()
}

// paint clips lines to w, pads them with the style's background and fills
// the rest of the h rows, so a pane has no holes.
func paint(style lipgloss.Style, w, h int, lines []string) string {
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		l := ""
		if i < len(lines) {
			l = ansi.Truncate(lines[i], w, "…")
		}
		if pad := w - ansi.StringWidth(l); pad > 0 {
			l += style.Render(strings.Repeat(" ", pad))
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func (m *model) rule(w int) string {
	return m.st.app.Foreground(c(m.st.t.Border)).Render(strings.Repeat("─", w))
}

func (m *model) View() string {
	if m.w < 40 || m.h < 12 {
		return m.st.app.Width(m.w).Height(m.h).Render("Ventana muy pequeña")
	}
	base := m.mainView()
	if d := m.dialog(); d != "" {
		return overlay(base, d, m.w, m.h)
	}
	return base
}

func (m *model) mainView() string {
	sideW, mainW := m.sideW(), m.w-m.sideW()
	var right string
	if m.repo == nil {
		right = m.welcomeView(mainW, m.h-1)
	} else {
		body := lipgloss.JoinHorizontal(lipgloss.Top,
			m.filesView(m.filesW(), m.bodyH()),
			m.rule1(m.bodyH()),
			m.diffPaneView(m.diffW(), m.bodyH()))
		right = strings.Join([]string{
			m.headerView(mainW),
			m.rule(mainW),
			body,
			m.rule(mainW),
			m.commitView(mainW),
		}, "\n")
	}
	right += "\n" + m.statusView(mainW)
	if sideW == 0 {
		return right
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.sideView(sideW, m.h), right)
}

func (m *model) rule1(h int) string {
	l := m.st.app.Foreground(c(m.st.t.Border)).Render("│")
	return strings.TrimSuffix(strings.Repeat(l+"\n", h), "\n")
}

// --- header ---

func (m *model) headerView(w int) string {
	s := m.st
	left := s.title.Render(" " + filepath.Base(m.repo.Root))
	left += s.muted.Render("  " + shortPath(m.repo.Root, max(10, w/3)))
	if b := m.branch(); b != "" {
		left += s.text.Render("  ") + s.badge.Render("⎇ "+b)
	}
	hints := s.key.Render("u") + s.muted.Render(" Subir  ") +
		s.key.Render("d") + s.muted.Render(" Destinos  ") +
		s.key.Render("t") + s.muted.Render(" Tema  ") +
		s.key.Render("?") + s.muted.Render(" Ayuda ")
	gap := w - ansi.StringWidth(left) - ansi.StringWidth(hints)
	if gap < 1 {
		return paint(s.app, w, 1, []string{left})
	}
	return left + s.text.Render(strings.Repeat(" ", gap)) + hints
}

func shortPath(p string, w int) string {
	if ansi.StringWidth(p) <= w {
		return p
	}
	return "…" + ansi.TruncateLeft(p, ansi.StringWidth(p)-w+1, "")
}

// --- sidebar ---

func (m *model) sideView(w, h int) string {
	s := m.st
	focus := m.focus == paneSide
	title := s.side.Foreground(c(s.t.Muted)).Bold(true)
	if focus {
		title = title.Foreground(c(s.t.Accent))
	}
	lines := []string{title.Render(" PROYECTOS"), ""}
	if len(m.store.Projects) == 0 {
		lines = append(lines, s.side.Foreground(c(s.t.Muted)).Render(" Pulsa o para añadir"),
			s.side.Foreground(c(s.t.Muted)).Render(" un repositorio Git."))
	}
	for i, p := range m.store.Projects {
		mark := "  "
		if i == m.projIdx {
			mark = "● "
		}
		badge := ""
		if n := len(p.Targets); n > 0 {
			badge = fmt.Sprintf(" %d", n)
		}
		name := ansi.Truncate(p.Name, max(1, w-2-ansi.StringWidth(mark)-ansi.StringWidth(badge)), "…")
		gap := max(0, w-2-ansi.StringWidth(mark)-ansi.StringWidth(name)-ansi.StringWidth(badge))
		text := " " + mark + name + strings.Repeat(" ", gap) + badge + " "
		switch {
		case i == m.sideCur && focus:
			lines = append(lines, s.sel.Width(w).Render(text))
		case i == m.sideCur:
			lines = append(lines, s.selDim.Width(w).Render(text))
		default:
			lines = append(lines, s.side.Render(text))
		}
	}
	foot := s.side.Foreground(c(s.t.Muted))
	hints := []string{
		s.side.Foreground(c(s.t.Accent)).Render(" o") + foot.Render(" añadir  ") + s.side.Foreground(c(s.t.Accent)).Render("x") + foot.Render(" quitar"),
		s.side.Foreground(c(s.t.Accent)).Render(" ,") + foot.Render(" ajustes ") + s.side.Foreground(c(s.t.Accent)).Render("b") + foot.Render(" ocultar"),
	}
	for len(lines) < h-len(hints) {
		lines = append(lines, "")
	}
	lines = append(lines[:min(len(lines), h-len(hints))], hints...)
	return paint(s.side, w, h, lines)
}

// --- files ---

func (m *model) filesView(w, h int) string {
	s := m.st
	title := s.header
	if m.focus == paneFiles {
		title = s.accent
	}
	count := ""
	if n := len(m.status.Files); n > 0 {
		count = fmt.Sprintf("  %d", n)
	}
	lines := []string{title.Render(" Archivos") + s.muted.Render(count)}

	listH := h - 1
	if len(m.rows) == 0 {
		lines = append(lines, "", s.ok.Render(" ✓ Sin cambios"), s.muted.Render(" El working tree está limpio."))
		return paint(s.app, w, h, lines)
	}
	// Keep the cursor in view.
	if m.sel >= 0 {
		if m.sel < m.filesOff+1 {
			m.filesOff = max(0, m.sel-1)
		}
		if m.sel >= m.filesOff+listH {
			m.filesOff = m.sel - listH + 1
		}
	}
	m.filesOff = max(0, min(m.filesOff, max(0, len(m.rows)-listH)))
	for i := m.filesOff; i < len(m.rows) && i < m.filesOff+listH; i++ {
		lines = append(lines, m.fileRow(i, w))
	}
	return paint(s.app, w, h, lines)
}

func (m *model) fileRow(i, w int) string {
	s := m.st
	r := m.rows[i]
	if r.Header {
		return s.header.Render(" " + strings.ToUpper(r.Title[:1]) + strings.ToLower(r.Title[1:]))
	}
	mark := "○"
	if r.Staged {
		mark = "●"
	}
	code := string(r.Code())
	name := r.File.Path
	if r.Staged && r.File.OrigPath != "" {
		name = r.File.OrigPath + " → " + r.File.Path
	}
	plain := fmt.Sprintf("  %s %s  %s", mark, code, name)
	if i == m.sel {
		st := s.selDim
		if m.focus == paneFiles {
			st = s.sel
		}
		return st.Width(w).Render(ansi.Truncate(plain, w, "…"))
	}
	markStyle := s.muted
	if r.Staged {
		markStyle = s.ok
	}
	return s.text.Render("  ") + markStyle.Render(mark) + s.text.Render(" ") +
		s.statusStyle(r.Code()).Render(code) + s.text.Render("  "+name)
}

func (s styles) statusStyle(code byte) lipgloss.Style {
	switch code {
	case 'A':
		return s.ok.Bold(true)
	case 'D', 'U':
		return s.bad.Bold(true)
	case 'R', 'C', '?':
		return s.warn.Bold(true)
	}
	return s.info.Bold(true)
}

// --- diff ---

func (m *model) diffPaneView(w, h int) string {
	s := m.st
	title := s.header
	if m.focus == paneDiff {
		title = s.accent
	}
	head := title.Render(" Diff")
	if m.diffTitle != "" {
		label := "Cambios"
		if m.diffStaged {
			label = "Staged"
		}
		head = title.Render(" "+m.diffTitle) + s.text.Render("  ") + s.badge.Render(label)
	}
	switch {
	case m.diffLoading:
		return paint(s.app, w, h, []string{head, "", " " + m.spin.View() + s.muted.Render(" Cargando diff…")})
	case len(m.diffLines) == 0:
		return paint(s.app, w, h, []string{head, "", s.muted.Render(" " + m.diffNote)})
	}
	rest := strings.Split(m.diff.View(), "\n")
	return paint(s.app, w, h, append([]string{head}, rest...))
}

// renderDiff paints the diff lines for the current width and theme.
func (m *model) renderDiff() {
	if len(m.diffLines) == 0 {
		m.diff.SetContent("")
		return
	}
	s := m.st
	w := m.diff.Width
	numW := 4
	var b strings.Builder
	for i, l := range m.diffLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		num := func(n int) string {
			if n == 0 {
				return strings.Repeat(" ", numW)
			}
			return fmt.Sprintf("%*d", numW, n)
		}
		switch l.Kind {
		case textdiff.Hunk:
			b.WriteString(paint(s.app, w, 1, []string{s.hunk.Render(" " + l.Text)}))
		case textdiff.Add:
			b.WriteString(diffLine(s.add, s.gutter, num(l.New), "+", l.Text, w, numW))
		case textdiff.Del:
			b.WriteString(diffLine(s.del, s.gutter, num(l.Old), "-", l.Text, w, numW))
		default:
			b.WriteString(diffLine(s.text, s.gutter, num(l.New), " ", l.Text, w, numW))
		}
	}
	m.diff.SetContent(b.String())
}

func diffLine(body, gutter lipgloss.Style, num, sign, text string, w, numW int) string {
	text = strings.ReplaceAll(text, "\t", "    ")
	line := gutter.Render(" "+num+" ") + body.Render(sign+" "+text)
	line = ansi.Truncate(line, w, "")
	if pad := w - ansi.StringWidth(line); pad > 0 {
		line += body.Render(strings.Repeat(" ", pad))
	}
	return line
}

// --- commit ---

func (m *model) commitView(w int) string {
	s := m.st
	title := s.header
	if m.focus == paneCommit {
		title = s.accent
	}
	hint := m.commitHint()
	head := title.Render(" Commit") + s.muted.Render("  "+hint)
	tool := "IA: " + m.aiTool()
	head = padBetween(s.app, head, s.muted.Render(tool+" "), w)

	ta := strings.Split(m.msg.View(), "\n")
	box := make([]string, 0, commitRows)
	for i := 0; i < commitRows; i++ {
		l := ""
		if i < len(ta) {
			l = ta[i]
		}
		box = append(box, s.input.Render(" ")+l+s.input.Render(" "))
	}

	canCommit := !m.busy && m.stagedCount() > 0 && trim(m.msg.Value()) != ""
	var acts []string
	acts = append(acts, m.chip("ctrl+s", "Commit", canCommit, true))
	if m.aiBusy {
		acts = append(acts, m.spin.View()+s.muted.Render(" escribiendo… ")+s.key.Render("esc")+s.muted.Render(" cancelar"))
	} else {
		acts = append(acts, m.chip("g", "Generar", m.stagedCount()+m.changedCount() > 0, false))
	}
	acts = append(acts, m.chip("p", m.pushLabel(), m.pushable() && !m.pushBusy && !m.busy, false))
	if m.busy || m.pushBusy {
		acts = append(acts, m.spin.View())
	}
	line := " " + strings.Join(acts, s.app.Render("  "))
	lines := []string{head}
	lines = append(lines, box...)
	lines = append(lines, line)
	for i, l := range lines {
		if i == 0 || i == len(lines)-1 {
			lines[i] = paint(s.app, w, 1, []string{l})
		} else {
			lines[i] = paint(s.input, w, 1, []string{l})
		}
	}
	return strings.Join(lines, "\n")
}

func padBetween(st lipgloss.Style, left, right string, w int) string {
	gap := w - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return left
	}
	return left + st.Render(strings.Repeat(" ", gap)) + right
}

func (m *model) commitHint() string {
	switch n := m.stagedCount(); n {
	case 0:
		return "Nada en el stage"
	case 1:
		return "1 archivo en el stage"
	default:
		return fmt.Sprintf("%d archivos en el stage", n)
	}
}

// chip draws an action with its key, dimmed while it cannot run.
func (m *model) chip(key, label string, on, primary bool) string {
	s := m.st
	if !on {
		return s.button.Foreground(c(s.t.Muted)).Render(key + " " + label)
	}
	if primary {
		return s.primary.Render(key + " " + label)
	}
	return s.button.Render(s.key.Background(c(s.t.Surface)).Render(key) + s.button.Render(" "+label))
}

// --- status bar ---

func (m *model) statusView(w int) string {
	s := m.st
	bar := s.side.Foreground(c(s.t.Muted))
	left := bar.Render(" ")
	if m.repo != nil {
		b := m.branch()
		if b == "" {
			b = "sin rama"
		}
		left += s.side.Foreground(c(s.t.Text)).Render("⎇ " + b)
		if m.status.Upstream != "" && (m.status.Ahead > 0 || m.status.Behind > 0) {
			left += bar.Render(fmt.Sprintf("  ↑%d ↓%d", m.status.Ahead, m.status.Behind))
		}
		left += bar.Render("  " + m.summary())
		if m.loadingStatus || m.opening {
			left += bar.Render("  ") + m.spin.View()
		}
	} else if m.opening {
		left += bar.Render("Abriendo repositorio… ") + m.spin.View()
	}

	right := ""
	switch {
	case m.err != "":
		right = s.side.Foreground(c(s.t.Danger)).Render("✗ "+firstLine(m.err)+"  ") + s.side.Foreground(c(s.t.Muted)).Render("esc ")
	case m.flash != "":
		right = s.side.Foreground(c(s.t.Success)).Render("✓ " + m.flash + " ")
	case m.repo != nil:
		if tg := m.activeTarget(); tg != nil {
			dir := strings.TrimRight(tg.Dir, "/")
			if dir == "" {
				dir = "/"
			}
			right = bar.Render("→ " + tg.Name + "  " + dir + " ")
		} else {
			right = bar.Render("Sin destino de despliegue ")
		}
	}
	right = ansi.Truncate(right, max(0, w-ansi.StringWidth(left)-2), "…")
	return paint(s.side, w, 1, []string{padBetween(s.side, left, right, w)})
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// --- welcome ---

func (m *model) welcomeView(w, h int) string {
	s := m.st
	var body []string
	add := func(l string) { body = append(body, l) }
	center := func(l string) string {
		pad := max(0, (w-ansi.StringWidth(l))/2)
		return s.app.Render(strings.Repeat(" ", pad)) + l
	}
	add(center(s.accent.Render("⬆ SideDeploy")))
	add("")
	if m.opening {
		add(center(m.spin.View() + s.muted.Render(" Abriendo repositorio…")))
	} else {
		add(center(s.muted.Render("Revisa los cambios de tu repo y súbelos por FTP/SFTP.")))
		if len(m.store.Projects) > 0 {
			add(center(s.muted.Render("Elige un proyecto a la izquierda, o abre otra carpeta.")))
		}
		add("")
		add(center(s.primary.Render("o  Abrir repositorio")))
		add("")
		add(center(s.muted.Render("t cambia el tema · ? ayuda · q salir")))
	}
	top := max(0, (h-len(body))/2)
	lines := append(make([]string, top), body...)
	return paint(s.app, w, h, lines)
}
