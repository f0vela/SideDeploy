package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

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
				a.errorText = err.Error()
				a.errorOpen = true
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
	ui.Row(c).FillWidth().Height(1).Background(t.Border)
	ui.Row(c).FillWidth().Padding(8, 12).Gap(8).AlignItems(ui.End).Children(func() {
		ui.TextArea(c, &a.commitMsg).Placeholder("Mensaje del commit").Lines(1, 5).MinHeight(34).Grow(1)
		if a.busy {
			ui.Spinner(c)
		}
		btn := ui.PrimaryButton(c, "Commit")
		if a.busy || a.stagedCount() == 0 || strings.TrimSpace(a.commitMsg) == "" {
			btn = btn.Disabled(true).Tooltip("Añade cambios al stage y escribe un mensaje")
		}
		if btn.Clicked() {
			a.commit()
		}
	})
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
		ui.Text(c, "Git no completó la operación").FontSize(18).Bold()
		ui.Text(c, "Suele ser un hook del repo. El índice quedó como estaba.").
			FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Scroll(c).Height(220).FillWidth().Children(func() {
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
