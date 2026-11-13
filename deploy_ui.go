package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"sidedeploy/internal/deploy"
	"sidedeploy/internal/gitrepo"
)

// statusChanges turns the work tree status into the paths a deploy looks at,
// without repeating a file Git lists twice.
func statusChanges(st gitrepo.Status) []deploy.Change {
	var out []deploy.Change
	for _, f := range st.Files {
		if !f.Staged() && !f.Unstaged() {
			continue
		}
		out = append(out, deploy.Change{Path: f.Path, OrigPath: f.OrigPath})
	}
	return out
}

// deployJobsFor maps candidates to jobs, leaving out what the filter hides.
// It returns the paths it could not map, to show instead of failing the run.
func deployJobsFor(t deploy.Target, cands []deploy.Candidate, f deploy.Filter) (jobs []deploy.Job, skipped int, notes []string) {
	for _, cand := range cands {
		if f.Skip(cand.Rel) {
			skipped++
			continue
		}
		remote, err := deploy.RemotePath(t, cand.Rel)
		if err != nil {
			notes = append(notes, err.Error())
			continue
		}
		jobs = append(jobs, deploy.Job{Rel: cand.Rel, Local: cand.Local, Remote: remote, Delete: cand.Delete})
	}
	return jobs, skipped, notes
}

// deployFilter follows pro-deployer's options: the app's own config never
// goes up, then ignore, include and, optionally, what Git ignores.
func (a *app) deployFilter(rels []string) (deploy.Filter, error) {
	f := deploy.Filter{Config: []string{"pro-deployer.json", ".vscode/pro-deployer.json"}}
	if a.repo != nil && a.cfgPath != "" {
		if rel, err := filepath.Rel(a.repo.Root, a.cfgPath); err == nil {
			f.Config = append(f.Config, filepath.ToSlash(rel))
		}
	}
	if a.cfg == nil {
		return f, nil
	}
	f.Ignore = a.cfg.Ignore
	f.Include = a.cfg.Include
	if !a.cfg.CheckGitignore {
		return f, nil
	}
	ignored, err := a.repo.Ignored(context.Background(), rels)
	if err != nil {
		return f, fmt.Errorf("git check-ignore: %w", err)
	}
	f.Ignored = ignored
	return f, nil
}

// deployTargetObj returns the target chosen in the deploy dialog.
func (a *app) deployTargetObj() *deploy.Target {
	list := a.targets()
	for i := range list {
		if list[i].Name == a.deployTarget {
			return &list[i]
		}
	}
	if len(list) > 0 {
		return &list[0]
	}
	return nil
}

func (a *app) deployWorkers() int {
	if a.cfg != nil && a.cfg.Concurrency > 0 {
		return a.cfg.Concurrency
	}
	return 5
}

// buildDeployJobs recomputes what the dialog lists and sends. It is cheap
// enough for the main thread; only git check-ignore runs a command.
func (a *app) buildDeployJobs() {
	a.deployJobs = nil
	a.deployUploads, a.deployDeletes = 0, 0
	a.deploySkipped = 0
	a.deployNotes = nil
	a.deployConfirm = false
	repo := a.repo
	tg := a.deployTargetObj()
	if repo == nil || tg == nil {
		return
	}
	cands := deploy.Candidates(repo.Root, statusChanges(a.status), a.deployAutoDelete)
	rels := make([]string, len(cands))
	for i, cand := range cands {
		rels[i] = cand.Rel
	}
	filter, err := a.deployFilter(rels)
	if err != nil {
		a.deployNotes = append(a.deployNotes, err.Error())
	}
	jobs, skipped, notes := deployJobsFor(*tg, cands, filter)
	a.deployJobs = jobs
	a.deploySkipped = skipped
	a.deployNotes = append(a.deployNotes, notes...)
	for _, job := range jobs {
		if job.Delete {
			a.deployDeletes++
		} else {
			a.deployUploads++
		}
	}
}

// openDeploy opens the upload dialog on the project's active target.
func (a *app) openDeploy() {
	tg := a.activeTarget()
	if a.repo == nil || tg == nil {
		return
	}
	a.deployOpen = true
	a.deployTarget = tg.Name
	a.deployAutoDelete = a.cfgAutoDelete()
	a.deployConfirm = false
	a.deployUnknown = nil
	a.deployErr = ""
	a.deployLog = nil
	a.deployFinished = ""
	a.deployDone, a.deployTotal, a.deployFailed = 0, 0, 0
	a.deployCancelled = false
	a.buildDeployJobs()
}

// closeDeploy closes the dialog and stops a running upload.
func (a *app) closeDeploy() {
	if a.deployCancel != nil {
		a.deployCancel()
		a.deployCancel = nil
	}
	a.deployOpen = false
	a.deployBusy = false
	a.deployConfirm = false
	a.deployJobs = nil
	a.deployLog = nil
	a.deployUnknown = nil
	a.deployFinished = ""
}

func (a *app) cancelDeploy() {
	if a.deployCancel == nil {
		return
	}
	a.deployCancelled = true
	a.deployCancel()
}

// startDeploy sends the listed jobs. A new SSH host stops the run on the
// first connection and asks to be trusted before anything goes up.
func (a *app) startDeploy() {
	repo := a.repo
	tg := a.deployTargetObj()
	if repo == nil || tg == nil || a.deployBusy || a.win == nil || len(a.deployJobs) == 0 {
		return
	}
	target := *tg
	jobs := append([]deploy.Job(nil), a.deployJobs...)
	workers := a.deployWorkers()
	ctx, cancel := context.WithCancel(context.Background())

	a.deployBusy = true
	a.deployCancelled = false
	a.deployConfirm = false
	a.deployCancel = cancel
	a.deployDone, a.deployTotal, a.deployFailed = 0, len(jobs), 0
	a.deployLog = nil
	a.deployUnknown = nil
	a.deployErr = ""
	a.deployFinished = ""

	go func() {
		open := deploy.NewOpener(target, a.dataDir)
		deploy.Run(ctx, open, jobs, workers, func(r deploy.Result) {
			a.win.Update(func() {
				if repo != a.repo {
					return
				}
				a.deployDone++
				if r.Err == nil {
					return
				}
				if u, ok := deploy.AsUnknownHost(r.Err); ok {
					a.deployUnknown = u
					return
				}
				a.deployFailed++
				if len(a.deployLog) < 200 {
					a.deployLog = append(a.deployLog, r.Job.Rel+": "+r.Err.Error())
				}
			})
		})
		a.win.Update(func() {
			cancel()
			if repo != a.repo {
				return
			}
			a.deployBusy = false
			a.deployCancel = nil
			if a.deployUnknown != nil {
				// Every job failed on the same new host: ask once, not 200
				// times.
				a.deployDone, a.deployFailed = 0, 0
				a.deployLog = nil
				return
			}
			switch {
			case a.deployCancelled:
				a.deployFinished = fmt.Sprintf("Cancelado: %d de %d archivos.", a.deployDone, a.deployTotal)
			case a.deployFailed > 0:
				a.deployFinished = fmt.Sprintf("%d de %d archivos con error.", a.deployFailed, a.deployTotal)
			default:
				a.deployFinished = fmt.Sprintf("Listo: %d archivos en %s.", a.deployDone, target.Name)
			}
		})
	}()
}

// trustHost saves the offered key and retries the upload.
func (a *app) trustHost() {
	u := a.deployUnknown
	if u == nil || a.deployBusy {
		return
	}
	if err := deploy.TrustHost(a.dataDir, u); err != nil {
		a.deployErr = "No se pudo guardar la huella: " + err.Error()
		return
	}
	a.deployUnknown = nil
	a.deployErr = ""
	a.startDeploy()
}

func (a *app) deployModal(c *ui.Context) {
	if !a.deployOpen {
		return
	}
	m := ui.Modal(c, &a.deployOpen, func() { a.deployBody(c) })
	// Uploading is not a thing to close by a stray click.
	if m.Dismissed() && a.deployBusy {
		a.deployOpen = true
	}
}

func (a *app) deployBody(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(640).Gap(12).Children(func() {
		if a.deployUnknown != nil {
			a.deployTrust(c)
			return
		}

		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Icon(c, iconUpload).FontSize(24).TextColor(t.Accent)
			ui.Text(c, "Subir cambios").FontSize(16).Bold().Grow(1)
			list := a.targets()
			before := a.deployTarget
			if len(list) > 1 {
				names := make([]string, len(list))
				for i := range list {
					names[i] = list[i].Name
				}
				ui.Select(c, &a.deployTarget, names).Width(220).Disabled(a.deployBusy)
			} else if len(list) == 1 {
				ui.Text(c, list[0].Name).FontSize(12).TextColor(t.TextMuted)
			}
			if a.deployTarget != before {
				a.deployFinished = ""
				a.buildDeployJobs()
			}
		})

		if tg := a.deployTargetObj(); tg != nil {
			dir := strings.TrimRight(tg.Dir, "/")
			if dir == "" {
				dir = "/"
			}
			ui.Text(c, fmt.Sprintf("%s · %s@%s:%d · %s", tg.Type, tg.User, tg.Host, tg.Port, dir)).
				FontSize(11).TextColor(t.TextMuted).SingleLine()
		}

		ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Center).Children(func() {
			if ui.Checkbox(c, &a.deployAutoDelete,
				"Borrar en el servidor lo que ya no está en el repo").FontSize(11).
				Disabled(a.deployBusy).Changed() {
				a.deployFinished = ""
				a.buildDeployJobs()
			}
			ui.Row(c).Grow(1)
			ui.Text(c, fmt.Sprintf("%d conexiones", a.deployWorkers())).FontSize(11).TextColor(t.TextMuted)
		})

		a.deploySummary(c)
		a.deployList(c)
		a.deployProgress(c)
		a.deployErrors(c)
		if a.deployConfirm && !a.deployBusy {
			a.deployConfirmRow(c)
		}

		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if a.deployBusy {
				if a.deployCancelled {
					ui.Text(c, "Cancelando…").FontSize(12).TextColor(t.TextMuted)
				} else if ui.Button(c, "Cancelar envío").Clicked() {
					a.cancelDeploy()
				}
			} else if ui.Button(c, "Cerrar").Clicked() {
				a.deployOpen = false
			}
			ui.Row(c).Grow(1)
			if !a.deployBusy {
				if ui.Button(c, "Recalcular").Tooltip("Vuelve a leer los cambios del repo").Clicked() {
					a.deployFinished = ""
					a.refresh()
				}
				label := "Subir"
				if n := len(a.deployJobs); n > 0 {
					label = fmt.Sprintf("Subir %d archivos", n)
				}
				subir := ui.PrimaryButton(c, "").Label(label).Children(iconLabel(c, iconUpload, label)).Disabled(len(a.deployJobs) == 0 || a.loadingStatus)
				if subir.Clicked() {
					if a.deployDeletes > 1 {
						a.deployConfirm = true
					} else {
						a.startDeploy()
					}
				}
			}
		})
	})
}

// deployConfirmRow asks once before a run removes more than one remote file.
func (a *app) deployConfirmRow(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Padding(8, 10).Gap(8).AlignItems(ui.Center).
		Background(t.Danger.Alpha(0.1)).Radius(6).Children(func() {
		ui.Icon(c, iconTrash).FontSize(14).TextColor(t.Danger)
		ui.Text(c, fmt.Sprintf("Se borrarán %d archivos en el servidor.", a.deployDeletes)).
			FontSize(12).TextColor(t.Danger).Grow(1)
		if ui.Button(c, "Cancelar").Clicked() {
			a.deployConfirm = false
		}
		if ui.PrimaryButton(c, "Sí, borrar y subir").Clicked() {
			a.deployConfirm = false
			a.startDeploy()
		}
	})
}

// cfgAutoDelete is the option pro-deployer.json asks for, to know when the
// check box left its default.
func (a *app) cfgAutoDelete() bool {
	return a.cfg != nil && a.cfg.AutoDelete
}

func (a *app) deploySummary(c *ui.Context) {
	t := c.Theme()
	var parts []string
	if a.deployUploads > 0 {
		parts = append(parts, fmt.Sprintf("%d para subir", a.deployUploads))
	}
	if a.deployDeletes > 0 {
		parts = append(parts, fmt.Sprintf("%d para borrar", a.deployDeletes))
	}
	if a.deploySkipped > 0 {
		parts = append(parts, fmt.Sprintf("%d filtrados", a.deploySkipped))
	}
	text := "Sin archivos que subir."
	if len(parts) > 0 {
		text = strings.Join(parts, " · ")
	}
	ui.Text(c, text).FontSize(12).TextColor(t.TextMuted)
}

func (a *app) deployList(c *ui.Context) {
	t := c.Theme()
	if len(a.deployJobs) == 0 {
		return
	}
	dir := ""
	if tg := a.deployTargetObj(); tg != nil {
		dir = strings.TrimRight(tg.Dir, "/")
	}
	ui.Scroll(c).FillWidth().Height(deployListHeight(len(a.deployJobs))).Padding(4, 8).Gap(3).
		Background(t.Surface).Radius(8).Children(func() {
		for _, job := range a.deployJobs {
			job := job
			remote := strings.TrimPrefix(job.Remote, dir+"/")
			if remote == "" {
				remote = job.Remote
			}
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Width(52).AlignItems(ui.Center).Children(func() {
					if job.Delete {
						ui.Badge(c, "borrar")
					} else {
						ui.Icon(c, iconUpload).FontSize(12).TextColor(t.Success)
					}
				})
				ui.Text(c, job.Rel).FontSize(12).SingleLine().Grow(1)
				ui.Text(c, remote).FontSize(11).TextColor(t.TextMuted).SingleLine().MaxWidth(280)
			})
		}
	})
}

// deployListHeight keeps short lists compact and long ones scrolling.
func deployListHeight(n int) float32 {
	h := float32(24*n + 12)
	if h > 230 {
		h = 230
	}
	return h
}

func (a *app) deployProgress(c *ui.Context) {
	if !a.deployBusy && a.deployFinished == "" {
		return
	}
	t := c.Theme()
	if a.deployBusy {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			label := "Conectando…"
			if a.deployDone > 0 {
				label = fmt.Sprintf("%d de %d archivos", a.deployDone, a.deployTotal)
			}
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).Grow(1)
			if a.deployFailed > 0 {
				ui.Text(c, fmt.Sprintf("%d con error", a.deployFailed)).FontSize(11).TextColor(t.Danger)
			}
		})
		if a.deployDone == 0 {
			ui.Progress(c, -1).Label("Conectando…")
		} else {
			value := 0.0
			if a.deployTotal > 0 {
				value = float64(a.deployDone) / float64(a.deployTotal)
			}
			ui.Progress(c, value).Label(fmt.Sprintf("%d de %d", a.deployDone, a.deployTotal))
		}
		return
	}
	color := t.Success
	if a.deployFailed > 0 || a.deployCancelled {
		color = t.Warning
	}
	ic := iconCheck
	if color == t.Warning {
		ic = iconWarn
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, ic).FontSize(14).TextColor(color)
		ui.Text(c, a.deployFinished).FontSize(12).TextColor(color)
	})
}

func (a *app) deployErrors(c *ui.Context) {
	t := c.Theme()
	if len(a.deployNotes) > 0 {
		ui.Text(c, strings.Join(a.deployNotes, "\n")).FontSize(11).TextColor(t.Danger)
	}
	if len(a.deployLog) == 0 {
		return
	}
	ui.Scroll(c).FillWidth().Height(deployLogHeight(len(a.deployLog))).Padding(4, 8).Gap(2).
		Background(t.Danger.Alpha(0.08)).Radius(8).Children(func() {
		for _, line := range a.deployLog {
			ui.Text(c, line).FontSize(11).TextColor(t.Danger).SingleLine()
		}
	})
}

// deployLogHeight keeps a few errors compact and a long log scrolling.
func deployLogHeight(n int) float32 {
	h := float32(16*n + 8)
	if h > 90 {
		h = 90
	}
	return h
}

func (a *app) deployTrust(c *ui.Context) {
	t := c.Theme()
	u := a.deployUnknown
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconWarn).FontSize(26).TextColor(t.Warning)
		ui.Text(c, "Host SSH nuevo").FontSize(16).Bold()
	})
	ui.Text(c, u.Host).FontSize(13)
	ui.Text(c, "Huella "+u.Fingerprint).FontSize(12).TextColor(t.TextMuted)
	ui.Text(c, "Este host no está en known_hosts. Compruébala con el servidor antes de confiar.").
		FontSize(11).TextColor(t.TextMuted)
	if a.deployErr != "" {
		ui.Text(c, a.deployErr).FontSize(11).TextColor(t.Danger)
	}
	ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
		if ui.Button(c, "Cancelar").Clicked() {
			a.deployUnknown = nil
			a.deployErr = ""
		}
		if ui.PrimaryButton(c, "Confiar y subir").Clicked() {
			a.trustHost()
		}
	})
}
