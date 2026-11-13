package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"sidedeploy/internal/aicommits"
	"sidedeploy/internal/gitrepo"
)

// gitWrite runs one command on the index at a time, then reloads the status.
// A second click while one runs queues behind it instead of failing on the
// index lock.
func (a *app) gitWrite(action func(*gitrepo.Repo, context.Context) error) {
	repo := a.repo
	if repo == nil || a.win == nil {
		return
	}
	go func() {
		a.gitMu.Lock()
		err := action(repo, context.Background())
		a.gitMu.Unlock()
		a.win.Update(func() {
			if repo != a.repo {
				return
			}
			if err != nil {
				a.err = err.Error()
			}
			a.refresh()
		})
	}()
}

func (a *app) stageFile(f gitrepo.File) {
	a.gitWrite(func(r *gitrepo.Repo, ctx context.Context) error { return r.Stage(ctx, f.Path) })
}

func (a *app) unstageFile(f gitrepo.File) {
	a.gitWrite(func(r *gitrepo.Repo, ctx context.Context) error { return r.Unstage(ctx, f.Path) })
}

func (a *app) stageAll() {
	a.gitWrite(func(r *gitrepo.Repo, ctx context.Context) error { return r.StageAll(ctx) })
}

func (a *app) unstageAll() {
	a.gitWrite(func(r *gitrepo.Repo, ctx context.Context) error { return r.UnstageAll(ctx) })
}

// commit records the index; the hooks may take a while, so it runs off the
// main thread and shows their output when they refuse.
func (a *app) commit() {
	repo := a.repo
	if repo == nil || a.busy {
		return
	}
	msg := strings.TrimSpace(a.commitMsg)
	if msg == "" {
		a.err = "Escribe un mensaje para el commit."
		return
	}
	if a.stagedCount() == 0 {
		a.err = "No hay cambios en el stage."
		return
	}
	a.busy = true
	a.err = ""
	go func() {
		a.gitMu.Lock()
		err := repo.Commit(context.Background(), msg)
		a.gitMu.Unlock()
		a.win.Update(func() {
			a.busy = false
			if repo != a.repo {
				return
			}
			if err != nil {
				a.showError("Git no completó la operación",
					"Suele ser un hook del repo. El índice quedó como estaba.",
					err.Error())
			} else {
				a.commitMsg = ""
				a.flash = "Commit hecho"
			}
			a.refresh()
		})
	}()
}

func (a *app) commitBar(c *ui.Context) {
	t := c.Theme()
	canCommit := !a.busy && a.stagedCount() > 0 && strings.TrimSpace(a.commitMsg) != ""
	if canCommit && c.Shortcut(ui.Cmd, ui.KeyEnter) {
		a.commit()
	}

	ui.Row(c).FillWidth().Height(1).Background(t.Border)
	ui.Column(c).FillWidth().Padding(10, 14, 12, 14).Gap(8).Background(t.Surface).Children(func() {
		ui.TextArea(c, &a.commitMsg).Placeholder("Mensaje del commit").Lines(2, 6).FillWidth().
			Disabled(a.busy)

		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			a.aiControls(c)
			ui.Text(c, a.commitHint()).FontSize(11).TextColor(t.TextMuted).SingleLine()
			ui.Row(c).Grow(1)

			if a.busy || a.pushBusy {
				ui.Spinner(c)
			}
			a.pushButton(c)

			btn := ui.PrimaryButton(c, "").Label("Commit").Children(iconLabel(c, iconCheck, "Commit")).
				Tooltip("Hace commit de lo que hay en el stage (⌘↵)")
			if !canCommit {
				btn = btn.Disabled(true).Tooltip("Añade cambios al stage y escribe un mensaje")
			}
			if btn.Clicked() {
				a.commit()
			}
		})
	})
}

// aiControls picks the tool that writes the message and runs it.
func (a *app) aiControls(c *ui.Context) {
	tool := a.aiTool()
	sel := ui.Select(c, &tool, aicommits.Tools).Width(100).
		Tooltip("Herramienta que escribe el mensaje").Disabled(a.aiBusy)
	if sel.Changed() {
		a.store.AITool = tool
		a.saveStore()
	}
	if a.aiBusy {
		ui.Spinner(c)
		if ui.Button(c, "Cancelar").Clicked() {
			a.cancelAI()
		}
		return
	}
	gen := ui.Button(c, "").Label("Generar").Children(iconLabel(c, iconSparkle, "Generar")).
		Tooltip("Escribe el mensaje con la IA a partir del diff")
	if a.stagedCount()+a.changedCount() == 0 {
		gen = gen.Disabled(true).Tooltip("No hay cambios que describir")
	}
	if gen.Clicked() {
		a.generateMessage()
	}
}

// commitHint says what the next commit takes.
func (a *app) commitHint() string {
	switch n := a.stagedCount(); n {
	case 0:
		return "Nada en el stage"
	case 1:
		return "1 archivo en el stage"
	default:
		return fmt.Sprintf("%d archivos en el stage", n)
	}
}

// pushable reports whether there is something for the remote to receive: new
// commits, or a branch that has no upstream yet.
func (a *app) pushable() bool {
	if a.repo == nil || a.status.Detached || a.branch == "" {
		return false
	}
	return a.status.Ahead > 0 || a.status.Upstream == ""
}

func (a *app) pushButton(c *ui.Context) {
	label, tip := "Push", "Envía los commits al remoto"
	switch {
	case a.status.Upstream == "":
		label, tip = "Publicar rama", "Sube la rama al remoto y la deja enlazada"
	case a.status.Ahead > 0:
		label = fmt.Sprintf("Push · %d", a.status.Ahead)
	}
	btn := ui.Button(c, "").Label(label).Children(iconLabel(c, iconPush, label)).Tooltip(tip)
	if !a.pushable() || a.pushBusy || a.busy {
		btn = btn.Disabled(true)
		if a.status.Ahead == 0 && a.status.Upstream != "" {
			btn = btn.Tooltip("No hay commits pendientes de enviar")
		}
	}
	if btn.Clicked() {
		a.push()
	}
}

// push sends the branch to its remote off the main thread.
func (a *app) push() {
	repo := a.repo
	if repo == nil || a.pushBusy || a.win == nil {
		return
	}
	upstream := a.status.Upstream
	a.pushBusy = true
	a.err = ""
	go func() {
		a.gitMu.Lock()
		err := repo.Push(context.Background(), upstream)
		a.gitMu.Unlock()
		a.win.Update(func() {
			a.pushBusy = false
			if repo != a.repo {
				return
			}
			if err != nil {
				a.showError("No se pudo hacer push",
					"Git no pide contraseña aquí: hace falta un credential helper o un agente SSH, y que el remoto acepte la rama.",
					err.Error())
			} else {
				a.flash = "Push hecho"
			}
			a.refresh()
		})
	}()
}

// stagedCount counts the files in the index, a staged and changed file once.
func (a *app) stagedCount() int {
	seen := make(map[string]bool, len(a.rows))
	for _, r := range a.rows {
		if r.Staged {
			seen[r.File.Path] = true
		}
	}
	return len(seen)
}

// changedCount counts the files Git still has to receive in the index.
func (a *app) changedCount() int {
	seen := make(map[string]bool, len(a.rows))
	for _, r := range a.rows {
		if !r.Header && !r.Staged {
			seen[r.File.Path] = true
		}
	}
	return len(seen)
}

func (a *app) errorModal(c *ui.Context) {
	ui.Modal(c, &a.errorOpen, func() {
		title := a.errorTitle
		if title == "" {
			title = "Git no completó la operación"
		}
		hint := a.errorHint
		if hint == "" {
			hint = "Suele ser un hook del repo. El índice quedó como estaba."
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconWarn).FontSize(26).TextColor(c.Theme().Warning)
			ui.Text(c, title).FontSize(16).Bold()
		})
		ui.Text(c, hint).FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Scroll(c).MaxHeight(220).FillWidth().Children(func() {
			ui.Text(c, a.errorText).FontSize(12)
		})
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			if ui.PrimaryButton(c, "Cerrar").Clicked() {
				a.errorOpen = false
			}
		})
	})
}

// flashToast shows a pending message once, from the view it needs.
func (a *app) flashToast(c *ui.Context) {
	if a.flash == "" {
		return
	}
	c.Toast(a.flash)
	a.flash = ""
}
