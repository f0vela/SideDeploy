package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"sidedeploy/internal/plan"
)

// ov are the styles of a dialog: the same colors on the overlay background.
type ov struct {
	text, muted, accent, ok, bad, warn, title, key, sel, input lipgloss.Style
}

func (m *model) ovStyles() ov {
	t := m.st.t
	on := func(fg string) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(c(fg)).Background(c(t.Overlay))
	}
	return ov{
		text: on(t.Text), muted: on(t.Muted), accent: on(t.Accent).Bold(true),
		ok: on(t.Success), bad: on(t.Danger), warn: on(t.Warning),
		title: on(t.Text).Bold(true), key: on(t.Accent).Bold(true),
		sel:   lipgloss.NewStyle().Foreground(c(t.OnAcc)).Background(c(t.Accent)).Bold(true),
		input: lipgloss.NewStyle().Foreground(c(t.Text)).Background(c(t.Surface)),
	}
}

// frame wraps lines in the dialog box, each padded to the inner width.
func (m *model) frame(width int, lines []string) string {
	inner := width - 4
	out := make([]string, len(lines))
	o := m.ovStyles()
	for i, l := range lines {
		out[i] = paint(o.text, inner, 1, []string{l})
	}
	return m.st.dialog.Width(width).Render(strings.Join(out, "\n"))
}

func (m *model) dialogWidth(want int) int { return max(30, min(want, m.w-4)) }

// wrap breaks text into lines of at most w columns, styled.
func wrap(st lipgloss.Style, text string, w int) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		for _, l := range strings.Split(ansi.Wrap(para, w, ""), "\n") {
			out = append(out, st.Render(l))
		}
	}
	return out
}

// footerFit is footer for a limited width: it drops the last pairs, which
// are the least important, until the rest fits.
func (m *model) footerFit(o ov, width int, pairs ...string) string {
	for n := len(pairs); n >= 2; n -= 2 {
		if f := m.footer(o, pairs[:n]...); ansi.StringWidth(f) <= width {
			return f
		}
	}
	return m.footer(o, pairs[:2]...)
}

func (m *model) footer(o ov, pairs ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(pairs); i += 2 {
		if b.Len() > 0 {
			b.WriteString(o.muted.Render("  ·  "))
		}
		b.WriteString(o.key.Render(pairs[i]) + o.muted.Render(" "+pairs[i+1]))
	}
	return b.String()
}

// dialog draws the open dialog, or "" when there is none.
func (m *model) dialog() string {
	switch m.mode {
	case mAddProject:
		return m.addDialog()
	case mTargets:
		return m.targetsDialog()
	case mTargetForm:
		return m.formDialog()
	case mDeploy:
		return m.deployDialog()
	case mSettings:
		return m.settingsDialog()
	case mThemes:
		return m.themesDialog()
	case mConfirm:
		return m.confirmDialog()
	case mError:
		return m.errorDialog()
	case mHelp:
		return m.helpDialog()
	case mProjects:
		return m.projectsDialog()
	case mDiff:
		return m.diffDialog()
	}
	return ""
}

func (m *model) projectsDialog() string {
	o := m.ovStyles()
	w := m.dialogWidth(60)
	inner := w - 4
	lines := []string{o.title.Render("Proyectos"), ""}
	if len(m.store.Projects) == 0 {
		lines = append(lines, o.muted.Render("Todavía no hay proyectos. Pulsa o para añadir uno."))
	}
	visible := max(3, min(len(m.store.Projects), m.h-12))
	off := max(0, min(m.pCur-visible/2, len(m.store.Projects)-visible))
	for i := off; i < off+visible && i < len(m.store.Projects); i++ {
		p := m.store.Projects[i]
		mark := "  "
		if i == m.projIdx {
			mark = "● "
		}
		badge := ""
		if n := len(p.Targets); n > 0 {
			badge = fmt.Sprintf("  %d destino", n)
			if n > 1 {
				badge += "s"
			}
		}
		name := ansi.Truncate(p.Name, max(8, inner/2-4), "…")
		path := ansi.Truncate(shortPath(p.Path, inner), max(0, inner-ansi.StringWidth(name)-ansi.StringWidth(badge)-6), "…")
		text := " " + mark + name + "  " + path + badge
		if i == m.pCur {
			lines = append(lines, m.st.sel.Render(fmt.Sprintf("%-*s", inner, ansi.Truncate(text, inner, "…"))))
		} else {
			lines = append(lines, o.text.Render(" "+mark+name)+o.muted.Render("  "+path+badge))
		}
	}
	lines = append(lines, "", m.footerFit(o, inner, "enter", "abrir", "esc", "cerrar", "o", "añadir", "x", "quitar"))
	return m.frame(w, lines)
}

func (m *model) diffDialog() string {
	o := m.ovStyles()
	w := m.w - 4
	vw, vh := m.diffModalSize()
	head := o.title.Render(" ")
	if m.sel >= 0 && m.sel < len(m.rows) && !m.rows[m.sel].Header {
		label := "Cambios"
		if m.diffStaged {
			label = "Staged"
		}
		idx, total := 0, 0
		for i, r := range m.rows {
			if !r.Header {
				total++
				if i == m.sel {
					idx = total
				}
			}
		}
		head = o.title.Render(ansi.Truncate(m.diffTitle, max(10, vw-24), "…")) + o.muted.Render("  "+label+fmt.Sprintf("  %d/%d", idx, total))
	}
	lines := []string{head, ""}
	switch {
	case m.diffLoading:
		lines = append(lines, o.muted.Render("Cargando diff…"))
	case len(m.diffLines) == 0:
		lines = append(lines, o.muted.Render(m.diffNote))
	default:
		lines = append(lines, strings.Split(m.diffModal.View(), "\n")...)
	}
	for len(lines) < vh+2 {
		lines = append(lines, "")
	}
	pct := fmt.Sprintf("%3.0f%%", m.diffModal.ScrollPercent()*100)
	lines = append(lines[:vh+2], "", m.footerFit(o, w-4-6, "esc", "cerrar", "↑↓", "desplazar", "←→", "archivo", "s", "stage")+o.muted.Render("  "+pct))
	return m.frame(w, lines)
}

func (m *model) addDialog() string {
	o := m.ovStyles()
	w := m.dialogWidth(64)
	m.addInput.Width = w - 10
	return m.frame(w, []string{
		o.title.Render("Añadir proyecto"),
		o.muted.Render("Ruta de la carpeta del repositorio Git."),
		"",
		o.input.Render(" ") + m.addInput.View() + o.input.Render(" "),
		"",
		m.footer(o, "enter", "abrir", "esc", "cancelar"),
	})
}

func (m *model) confirmDialog() string {
	o := m.ovStyles()
	w := m.dialogWidth(58)
	lines := []string{o.title.Render(m.confirmTitle), ""}
	lines = append(lines, wrap(o.muted, m.confirmText, w-4)...)
	lines = append(lines, "", m.footer(o, "y", m.confirmYes, "n", "cancelar"))
	return m.frame(w, lines)
}

func (m *model) errorDialog() string {
	o := m.ovStyles()
	w := m.dialogWidth(76)
	lines := []string{o.warn.Bold(true).Render("⚠ " + m.errTitle), ""}
	lines = append(lines, wrap(o.muted, m.errHint, w-4)...)
	lines = append(lines, "")
	body := wrap(o.text, m.errText, w-4)
	if len(body) > 10 {
		body = append(body[:10], o.muted.Render("…"))
	}
	lines = append(lines, body...)
	lines = append(lines, "", m.footer(o, "enter", "cerrar"))
	return m.frame(w, lines)
}

func (m *model) helpDialog() string {
	o := m.ovStyles()
	w := m.dialogWidth(78)
	row := func(k, d string) string {
		return o.key.Render(fmt.Sprintf("%-12s", k)) + o.text.Render(d)
	}
	sec := func(t string) string { return o.accent.Render(t) }
	lines := []string{o.title.Render("Atajos de SideDeploy"), "",
		sec("Navegación"),
		row("tab/⇧tab", "cambiar de panel (proyectos · cambios · diff · commit)"),
		row("↑ ↓  j k", "mover · en el diff, desplazar"),
		row("enter", "de cambios al diff · abrir proyecto"),
		row("b", "mostrar u ocultar la barra de proyectos"),
		row("P", "lista de proyectos en un diálogo (ventana angosta)"),
		row("v  enter", "ver el diff en un diálogo (←→ cambia de archivo)"),
		"",
		sec("Git"),
		row("espacio", "añadir o sacar el archivo del stage"),
		row("a / A", "stage de todo / sacar todo del stage"),
		row("c", "escribir el commit (ctrl+s lo confirma, esc sale)"),
		row("g  ctrl+g", "generar el mensaje con la IA · esc cancela"),
		row("i", "alternar IA: claude / copilot"),
		row("p  ctrl+p", "push (o publicar la rama)"),
		row("r", "actualizar"),
		"",
		sec("Despliegue y proyectos"),
		row("u", "subir los cambios al destino"),
		row("d", "destinos de despliegue"),
		row("o", "añadir proyecto · x en la barra lo quita"),
		"",
		sec("Apariencia"),
		row("t", "siguiente tema"),
		row("T  ctrl+t", "elegir tema con vista previa"),
		row(",", "ajustes (tema y rutas de IA)"),
		"",
		m.footer(o, "q", "salir", "?", "ayuda"),
	}
	return m.frame(w, lines)
}

func (m *model) themesDialog() string {
	o := m.ovStyles()
	w := m.dialogWidth(50)
	keys := themeKeys()
	visible := max(5, min(len(keys), m.h-10))
	off := max(0, min(m.thCur-visible/2, len(keys)-visible))
	lines := []string{o.title.Render("Tema"), o.muted.Render("Se aplica al moverte; enter lo guarda."), ""}
	for i := off; i < off+visible && i < len(keys); i++ {
		t := themes[keys[i]]
		sw := ""
		for _, col := range []string{t.Bg, t.Surface, t.Accent, t.Success, t.Warning, t.Danger} {
			sw += lipgloss.NewStyle().Foreground(c(col)).Background(c(t.Overlay)).Render("██")
		}
		kind := "oscuro"
		if !t.Dark {
			kind = "claro"
		}
		cur := "  "
		if keys[i] == m.prefs.Theme {
			cur = "✓ "
		}
		name := fmt.Sprintf(" %s%-22s %-6s", cur, t.Name, kind)
		name = paint(o.text, w-4-13, 1, []string{name})
		if i == m.thCur {
			lines = append(lines, m.st.sel.Render(ansi.Strip(name))+" "+sw)
		} else {
			lines = append(lines, name+" "+sw)
		}
	}
	lines = append(lines, "", m.footer(o, "↑↓", "elegir", "enter", "guardar", "esc", "cancelar"))
	return m.frame(w, lines)
}

func (m *model) settingsDialog() string {
	o := m.ovStyles()
	w := m.dialogWidth(72)
	label := func(i int, text string) string {
		st := o.muted
		if m.sFocus == i {
			st = o.accent
		}
		return st.Render(fmt.Sprintf("%-9s", text))
	}
	themeName := themes[m.prefs.Theme].Name
	themeRow := label(0, "Tema") + o.text.Render("‹ "+themeName+" ›")
	if m.sFocus == 0 {
		themeRow = label(0, "Tema") + o.sel.Render(" ‹ "+themeName+" › ") + o.muted.Render("  enter: lista")
	}
	lines := []string{o.title.Render("Ajustes"), "", o.accent.Render("Apariencia"), themeRow, "",
		o.accent.Render("Herramientas de IA"), o.muted.Render("Generan el mensaje del commit. Vacío: se busca en el PATH.")}
	for i, tool := range []string{"claude", "copilot"} {
		m.sInputs[i].Width = w - 22
		lines = append(lines, label(i+1, strings.ToUpper(tool[:1])+tool[1:])+o.input.Render(" ")+m.sInputs[i].View()+o.input.Render(" "))
		if bin, err := m.resolved(tool, m.sInputs[i].Value()); err != nil {
			lines = append(lines, o.muted.Render("          ✗ "+firstLine(err.Error())))
		} else {
			lines = append(lines, o.ok.Render("          ✓ "+bin))
		}
	}
	if m.sMsg != "" {
		st := o.ok
		if m.sErr {
			st = o.bad
		}
		lines = append(lines, "", st.Render(m.sMsg))
	}
	lines = append(lines, "", m.footer(o, "tab", "siguiente", "ctrl+s", "guardar", "esc", "cerrar"))
	return m.frame(w, lines)
}

func (m *model) targetsDialog() string {
	o := m.ovStyles()
	w := m.dialogWidth(84)
	p := m.project()
	if p == nil {
		return ""
	}
	lines := []string{o.title.Render("Destinos de " + p.Name),
		o.muted.Render("El marcado (•) es al que se suben los cambios."), ""}
	if len(p.Targets) == 0 {
		lines = append(lines, o.muted.Render("Todavía no hay destinos. Pulsa a para añadir uno."))
	}
	for i, tg := range p.Targets {
		mark := "( )"
		if tg.Name == p.ActiveTarget {
			mark = "(•)"
		}
		summary := fmt.Sprintf("%s · %s:%d%s", tg.Type, tg.Host, tg.Port, tg.Dir)
		nameW := 18
		text := fmt.Sprintf(" %s %-*s %s", mark, nameW, ansi.Truncate(tg.Name, nameW, "…"), summary)
		text = ansi.Truncate(text, w-4, "…")
		if i == m.tCur {
			lines = append(lines, m.st.sel.Render(fmt.Sprintf("%-*s", w-4, text)))
		} else {
			lines = append(lines, o.text.Render(text))
		}
	}
	lines = append(lines, "")
	if m.tConfirm >= 0 && m.tConfirm < len(p.Targets) {
		lines = append(lines, o.warn.Bold(true).Render("¿Eliminar el destino “"+p.Targets[m.tConfirm].Name+"”?"),
			o.muted.Render("Se borra de la lista de SideDeploy; el servidor no se toca."),
			m.footer(o, "y", "eliminar", "n", "cancelar"))
	} else {
		lines = append(lines, m.footer(o, "↑↓", "elegir", "espacio", "activar", "a", "añadir", "e", "editar", "x", "eliminar", "esc", "cerrar"))
	}
	return m.frame(w, lines)
}

func (m *model) formDialog() string {
	o := m.ovStyles()
	w := m.dialogWidth(74)
	title := "Nuevo destino"
	if !m.formNew {
		title = "Editar destino"
	}
	lines := []string{o.title.Render(title), ""}
	labelW := 15
	for row, label := range formLabels {
		st := o.muted
		if m.formFocus == row {
			st = o.accent
		}
		l := st.Render(fmt.Sprintf("%*s  ", labelW, label))
		if row == 1 {
			v := " " + m.formType + " "
			if m.formFocus == row {
				l += o.sel.Render("‹" + v + "›")
			} else {
				l += o.text.Render("‹" + v + "›")
			}
		} else {
			i := formInput(row)
			m.form[i].Width = w - labelW - 10
			l += o.input.Render(" ") + m.form[i].View() + o.input.Render(" ")
		}
		lines = append(lines, l)
	}
	if m.formErr != "" {
		lines = append(lines, "", o.bad.Render("⚠ "+m.formErr))
	}
	lines = append(lines, "", m.footer(o, "tab", "siguiente", "←→", "tipo", "ctrl+s", "guardar", "esc", "cancelar"))
	return m.frame(w, lines)
}

func (m *model) deployDialog() string {
	o := m.ovStyles()
	w := m.dialogWidth(96)
	inner := w - 4

	if u := m.dUnknown; u != nil {
		lines := []string{o.warn.Bold(true).Render("⚠ Host SSH nuevo"), "", o.text.Render(u.Host),
			o.muted.Render("Huella " + u.Fingerprint)}
		lines = append(lines, wrap(o.muted, "Este host no está en known_hosts. Compruébala con el servidor antes de confiar.", inner)...)
		if m.dErr != "" {
			lines = append(lines, o.bad.Render(m.dErr))
		}
		lines = append(lines, "", m.footer(o, "y", "confiar y subir", "n", "cancelar"))
		return m.frame(w, lines)
	}

	tg := m.deployTargetObj()
	lines := []string{o.title.Render("⬆ Subir cambios")}
	if tg != nil {
		dir := strings.TrimRight(tg.Dir, "/")
		if dir == "" {
			dir = "/"
		}
		name := o.accent.Render(tg.Name)
		if len(m.targets()) > 1 {
			name = o.muted.Render("‹ ") + name + o.muted.Render(" ›")
		}
		lines = append(lines, name+o.muted.Render(fmt.Sprintf("   %s · %s@%s:%d · %s", tg.Type, tg.User, tg.Host, tg.Port, dir)))
	}
	box := "[ ]"
	if m.dAuto {
		box = "[x]"
	}
	lines = append(lines, "",
		o.key.Render("a ")+o.text.Render(box+" Borrar en el servidor lo que ya no está en el repo")+
			o.muted.Render(fmt.Sprintf("    %d conexiones", plan.Workers(m.cfg))),
		o.muted.Render(m.dPlan.Summary()))

	// the list
	if n := len(m.dPlan.Jobs); n > 0 {
		visible := min(n, max(3, min(10, m.h-24)))
		off := max(0, min(m.dScroll-visible/2, n-visible))
		lines = append(lines, "")
		for i := off; i < off+visible; i++ {
			job := m.dPlan.Jobs[i]
			tag := o.ok.Render("subir ")
			if job.Delete {
				tag = o.bad.Render("borrar")
			}
			cur := "  "
			if i == m.dScroll && !m.dBusy {
				cur = "› "
			}
			dir := ""
			if tg != nil {
				dir = strings.TrimRight(tg.Dir, "/") + "/"
			}
			remote := strings.TrimPrefix(job.Remote, dir)
			rel := ansi.Truncate(job.Rel, inner/2, "…")
			line := o.text.Render(cur) + tag + o.text.Render("  "+rel)
			if gap := inner - ansi.StringWidth(line) - ansi.StringWidth(remote) - 1; gap > 2 {
				line += o.text.Render(strings.Repeat(" ", gap)) + o.muted.Render(ansi.Truncate(remote, inner-ansi.StringWidth(line)-2, "…"))
			}
			lines = append(lines, line)
		}
		if n > visible {
			lines = append(lines, o.muted.Render(fmt.Sprintf("  … %d archivos en total (↑↓ para ver más)", n)))
		}
	}

	// progress and result
	switch {
	case m.dBusy:
		label := "Conectando…"
		value := 0.0
		if m.dDone > 0 {
			label = fmt.Sprintf("%d de %d archivos", m.dDone, m.dTotal)
		}
		if m.dTotal > 0 {
			value = float64(m.dDone) / float64(m.dTotal)
		}
		m.bar.Width = max(10, inner-2)
		lines = append(lines, "", o.muted.Render(label)+o.text.Render("  ")+m.spin.View())
		if m.dFailed > 0 {
			lines[len(lines)-1] += o.bad.Render(fmt.Sprintf("   %d con error", m.dFailed))
		}
		lines = append(lines, m.bar.ViewAs(value))
	case m.dFinished != "":
		ic, st := "✓ ", o.ok
		if m.dFailed > 0 || m.dCancelled {
			ic, st = "⚠ ", o.warn
		}
		lines = append(lines, "", st.Render(ic+m.dFinished))
	}
	for _, n := range m.dPlan.Notes {
		lines = append(lines, o.bad.Render(ansi.Truncate(n, inner, "…")))
	}
	if len(m.dLog) > 0 {
		lines = append(lines, "")
		for i, l := range m.dLog {
			if i == 6 {
				lines = append(lines, o.muted.Render(fmt.Sprintf("… y %d más", len(m.dLog)-6)))
				break
			}
			lines = append(lines, o.bad.Render(ansi.Truncate(l, inner, "…")))
		}
	}

	// confirmation and keys
	lines = append(lines, "")
	switch {
	case m.dConfirm && !m.dBusy:
		lines = append(lines, o.bad.Bold(true).Render(fmt.Sprintf("🗑 Se borrarán %d archivos en el servidor.", m.dPlan.Deletes)),
			m.footer(o, "y", "sí, borrar y subir", "n", "cancelar"))
	case m.dBusy:
		if m.dCancelled {
			lines = append(lines, o.muted.Render("Cancelando…"))
		} else {
			lines = append(lines, m.footer(o, "c", "cancelar envío"))
		}
	default:
		pairs := []string{}
		if n := len(m.dPlan.Jobs); n > 0 {
			pairs = append(pairs, "enter", fmt.Sprintf("subir %d archivos", n))
		}
		pairs = append(pairs, "r", "recalcular", "esc", "cerrar")
		lines = append(lines, m.footer(o, pairs...))
	}
	return m.frame(w, lines)
}

// overlay draws dlg over the middle of base.
func overlay(base, dlg string, w, h int) string {
	bl := strings.Split(base, "\n")
	dl := strings.Split(dlg, "\n")
	dw := lipgloss.Width(dlg)
	x := max(0, (w-dw)/2)
	y := max(0, (h-len(dl))/2)
	const reset = "\x1b[0m"
	for i, d := range dl {
		yy := y + i
		if yy >= len(bl) {
			break
		}
		left := ansi.Truncate(bl[yy], x, "")
		right := ansi.TruncateLeft(bl[yy], x+dw, "")
		bl[yy] = left + reset + d + reset + right
	}
	return strings.Join(bl, "\n")
}
