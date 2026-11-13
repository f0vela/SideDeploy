package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"sidedeploy/internal/deploy"
	"sidedeploy/internal/gitrepo"
	"sidedeploy/internal/projects"
)

// sidebarWidth is fixed: the source list keeps its width as the window does.
const sidebarWidth float32 = 210

// project returns the open project, or nil.
func (a *app) project() *projects.Project {
	if a.projIdx < 0 || a.projIdx >= len(a.store.Projects) {
		return nil
	}
	return &a.store.Projects[a.projIdx]
}

// targets returns where the open project deploys: its own targets, or those
// of pro-deployer.json while it has none.
func (a *app) targets() []deploy.Target {
	if p := a.project(); p != nil && len(p.Targets) > 0 {
		return p.Targets
	}
	if a.cfg != nil {
		return a.cfg.Active()
	}
	return nil
}

// activeTarget returns the target the project deploys to, or nil.
func (a *app) activeTarget() *deploy.Target {
	list := a.targets()
	if len(list) == 0 {
		return nil
	}
	name := ""
	if p := a.project(); p != nil {
		name = p.ActiveTarget
	}
	for i := range list {
		if list[i].Name == name {
			return &list[i]
		}
	}
	return &list[0]
}

func (a *app) saveStore() {
	if err := projects.Save(a.dataDir, a.store); err != nil {
		a.err = "No se pudo guardar projects.json: " + err.Error()
	}
}

func (a *app) projectsPane(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	ui.Column(c).Width(sidebarWidth).FillHeight().Background(t.Surface).Children(func() {
		// Room for the traffic lights, which sit over the sidebar.
		ui.Row(c).FillWidth().Height(max(bar.Height, 52)).DragWindow()
		if len(a.store.Projects) == 0 {
			ui.Text(c, "Abre una carpeta con repositorio Git y quedará en esta lista.").
				FontSize(11).TextColor(t.TextMuted).Padding(4, 14).Grow(1)
		} else {
			remove := ""
			sb := ui.Sidebar(c, &a.selPath, func() {
				ui.SidebarSection(c, "Proyectos", &a.projectsShown, func() {
					for i := range a.store.Projects {
						p := &a.store.Projects[i]
						item := ui.SidebarItem(c, p.Path, iconFolder, p.Name)
						if n := len(p.Targets); n > 0 {
							item.Children(func() { ui.Badge(c, strconv.Itoa(n)) })
						}
						item.ContextMenu(func(m *ui.Menu) {
							if m.Item("Quitar de la lista").Chosen() {
								remove = p.Path
							}
						})
					}
				})
			}).Width(sidebarWidth).Grow(1)

			if sb.Changed() {
				go a.openRepo(a.selPath)
			}
			if remove != "" {
				a.removeProject(remove)
			}
		}
		ui.Row(c).FillWidth().Height(1).Background(t.Border)
		ui.Toolbar(c, func() {
			if ui.Button(c, "").Label("Añadir proyecto").Tooltip("Añadir proyecto (⌘O)").
				Children(iconLabel(c, iconPlus, "")).Clicked() {
				go a.openRepoDialog()
			}
			ui.Spacer(c)
			if ui.Button(c, "").Label("Ajustes").Tooltip("Ajustes (⌘,)").
				Children(iconLabel(c, iconGear, "")).Clicked() {
				a.openSettings()
			}
		}).Label("Proyectos").Padding(6, 8)
	})
	ui.Row(c).FillHeight().Width(1).Background(t.Border)
}

// removeProject takes a project out of the list, not off the disk.
func (a *app) removeProject(path string) {
	a.store.Remove(path)
	a.saveStore()
	if a.repo != nil && a.repo.Root == path {
		a.closeProject()
	}
}

func (a *app) closeProject() {
	a.cancelAI()
	a.closeDeploy()
	a.repo = nil
	a.cfg = nil
	a.cfgPath = ""
	a.projIdx = -1
	a.selPath = ""
	a.branch = ""
	a.summary = ""
	a.status = gitrepo.Status{}
	a.rows = nil
	a.selected = -1
	a.selKey = ""
	a.selFile = ""
	a.diff = nil
	a.diffTitle = ""
	a.diffNote = ""
	a.diffSeq++
	a.loadingStatus = false
	a.loadingDiff = false
	a.refreshQueued = false
}

func (a *app) targetsModal(c *ui.Context) {
	if a.targetsOpen && a.project() == nil {
		a.targetsOpen = false
	}
	ui.Modal(c, &a.targetsOpen, func() {
		if a.targetEdit {
			a.targetForm(c)
			return
		}
		a.targetList(c)
	})
	a.confirmDeleteAlert(c)
}

func (a *app) targetList(c *ui.Context) {
	t := c.Theme()
	p := a.project()
	ui.Column(c).Width(560).Gap(12).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconServer).FontSize(26).TextColor(t.Accent)
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "Destinos de "+p.Name).FontSize(16).Bold()
				ui.Text(c, "El marcado es al que se suben los cambios.").FontSize(11).TextColor(t.TextMuted)
			})
		})

		if len(p.Targets) == 0 {
			ui.Text(c, "Todavía no hay destinos.").TextColor(t.TextMuted).Padding(8, 0)
		}
		edit, del := -1, -1
		before := p.ActiveTarget
		ui.Column(c).FillWidth().Background(t.Surface).Radius(8).Children(func() {
			for i := range p.Targets {
				tg := &p.Targets[i]
				if i > 0 {
					ui.Row(c).FillWidth().Height(1).Background(t.Border)
				}
				ui.Row(c).FillWidth().Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Radio(c, &p.ActiveTarget, tg.Name, tg.Name).Grow(1)
					ui.Text(c, remoteSummary(*tg)).FontSize(11).TextColor(t.TextMuted).SingleLine().MaxWidth(220)
					ui.Toolbar(c, func() {
						if ui.Button(c, "").Label("Editar").Tooltip("Editar").Children(iconLabel(c, iconPencil, "")).Clicked() {
							edit = i
						}
						if ui.Button(c, "").Label("Eliminar").Tooltip("Eliminar").Children(iconLabel(c, iconTrash, "")).Clicked() {
							del = i
						}
					}).Label("Destino " + tg.Name)
				})
			}
		})
		if p.ActiveTarget != before {
			a.saveStore()
		}

		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "").Label("Añadir destino").Children(iconLabel(c, iconPlus, "Añadir destino")).Clicked() {
				a.beginEdit(-1)
			}
			ui.Row(c).Grow(1)
			if ui.PrimaryButton(c, "Listo").Clicked() || c.Shortcut(0, ui.KeyEscape) {
				a.targetsOpen = false
			}
		})

		if edit >= 0 {
			a.beginEdit(edit)
		}
		if del >= 0 {
			a.confirmDelete = del
		}
	})
}

// confirmDeleteAlert asks, as an AppKit alert, before a target goes.
func (a *app) confirmDeleteAlert(c *ui.Context) {
	p := a.project()
	if a.confirmDelete < 0 || p == nil || a.confirmDelete >= len(p.Targets) {
		a.confirmDelete = -1
		return
	}
	open := true
	name := p.Targets[a.confirmDelete].Name
	switch ui.AlertDialog(c, &open, "¿Eliminar el destino “"+name+"”?",
		"Se borra de la lista de SideDeploy; el servidor no se toca.", "Cancelar", "Eliminar") {
	case 1:
		a.deleteTarget(a.confirmDelete)
		a.confirmDelete = -1
	case 0, ui.AlertDismissed:
		a.confirmDelete = -1
	}
}

func (a *app) beginEdit(i int) {
	p := a.project()
	if p == nil {
		return
	}
	a.targetIndex = i
	a.targetNew = i < 0 || i >= len(p.Targets)
	a.draftPort = ""
	a.draftKey = ""
	if a.targetNew {
		a.draft = deploy.Target{Type: "sftp", Dir: "/", BaseDir: "/"}
	} else {
		a.draft = p.Targets[i]
		if a.draft.Port > 0 {
			a.draftPort = strconv.Itoa(a.draft.Port)
		}
		if a.draft.PrivateKey != nil {
			a.draftKey = *a.draft.PrivateKey
		}
	}
	a.formErr = ""
	a.targetEdit = true
}

func (a *app) targetForm(c *ui.Context) {
	t := c.Theme()
	title := "Nuevo destino"
	if !a.targetNew {
		title = "Editar destino"
	}
	ui.Column(c).Width(520).Gap(14).Children(func() {
		ui.Text(c, title).FontSize(16).Bold()

		ui.Form(c, func() {
			ui.Field(c, "Nombre", func() { ui.TextInput(c, &a.draft.Name).FillWidth() })
			ui.Field(c, "Tipo", func() { ui.Select(c, &a.draft.Type, []string{"sftp", "ftp"}).Width(110) })
			ui.Field(c, "Host", func() { ui.TextInput(c, &a.draft.Host).Placeholder("ftp.ejemplo.com").FillWidth() })
			ui.Field(c, "Puerto", func() { ui.TextInput(c, &a.draftPort).Placeholder("por defecto").Width(110) })
			ui.Field(c, "Usuario", func() { ui.TextInput(c, &a.draft.User).FillWidth() })
			ui.Field(c, "Contraseña", func() { ui.TextInput(c, &a.draft.Password).Password().FillWidth() })
			ui.Field(c, "Clave privada", func() {
				ui.TextInput(c, &a.draftKey).Placeholder("~/.ssh/id_ed25519").FillWidth()
			}).Description("Opcional. Para SFTP con clave en lugar de contraseña.")
			ui.Field(c, "Carpeta remota", func() {
				ui.TextInput(c, &a.draft.Dir).Placeholder("/var/www/mi-sitio").FillWidth()
			})
			ui.Field(c, "baseDir", func() { ui.TextInput(c, &a.draft.BaseDir).Placeholder("/").Width(140) }).
				Description("Parte de la ruta del repo que no se sube.")
		})

		if a.formErr != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconWarn).FontSize(13).TextColor(t.Danger)
				ui.Text(c, a.formErr).FontSize(12).TextColor(t.Danger)
			})
		}
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "Cancelar").Clicked() {
				a.targetEdit = false
				a.formErr = ""
			}
			if ui.PrimaryButton(c, "Guardar").Clicked() {
				a.saveTarget()
			}
		})
	})
}

func (a *app) saveTarget() {
	p := a.project()
	if p == nil {
		return
	}
	a.draft.Name = strings.TrimSpace(a.draft.Name)
	a.draft.Host = strings.TrimSpace(a.draft.Host)
	if a.draft.Name == "" {
		a.formErr = "Ponle un nombre al destino."
		return
	}
	if a.draft.Host == "" {
		a.formErr = "Falta el host."
		return
	}
	port := 0
	if s := strings.TrimSpace(a.draftPort); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 65535 {
			a.formErr = "El puerto va de 1 a 65535, o vacío para el de su tipo."
			return
		}
		port = n
	}
	a.draft.Port = port
	a.draftKey = strings.TrimSpace(a.draftKey)
	a.draft.PrivateKey = nil
	if a.draftKey != "" {
		key := a.draftKey
		a.draft.PrivateKey = &key
	}

	for i := range p.Targets {
		if (a.targetNew || i != a.targetIndex) && p.Targets[i].Name == a.draft.Name {
			a.formErr = "Ya hay un destino con ese nombre."
			return
		}
	}

	one := []deploy.Target{a.draft}
	deploy.Normalize(one)
	a.draft = one[0]

	if a.targetNew {
		p.Targets = append(p.Targets, a.draft)
	} else if a.targetIndex >= 0 && a.targetIndex < len(p.Targets) {
		if p.ActiveTarget == p.Targets[a.targetIndex].Name {
			p.ActiveTarget = a.draft.Name
		}
		p.Targets[a.targetIndex] = a.draft
	}
	if p.ActiveTarget == "" {
		p.ActiveTarget = a.draft.Name
	}
	a.saveStore()
	a.targetEdit = false
	a.formErr = ""
}

func (a *app) deleteTarget(i int) {
	p := a.project()
	if p == nil || i < 0 || i >= len(p.Targets) {
		return
	}
	name := p.Targets[i].Name
	p.Targets = append(p.Targets[:i], p.Targets[i+1:]...)
	if p.ActiveTarget == name {
		p.ActiveTarget = ""
		if len(p.Targets) > 0 {
			p.ActiveTarget = p.Targets[0].Name
		}
	}
	a.saveStore()
}

// openTargets opens the editor of the project's targets, adopting those of
// pro-deployer.json the first time so they can be edited here.
func (a *app) openTargets() {
	p := a.project()
	if p == nil {
		return
	}
	if len(p.Targets) == 0 && a.cfg != nil && len(a.cfg.Targets) > 0 {
		p.Targets = append([]deploy.Target(nil), a.cfg.Targets...)
		if len(a.cfg.ActiveTargets) > 0 {
			p.ActiveTarget = a.cfg.ActiveTargets[0]
		}
		if p.ActiveTarget == "" {
			p.ActiveTarget = p.Targets[0].Name
		}
		a.saveStore()
	}
	a.targetsOpen = true
	a.targetEdit = false
	a.confirmDelete = -1
	a.formErr = ""
}

func remoteSummary(t deploy.Target) string {
	return fmt.Sprintf("%s · %s:%d%s", t.Type, t.Host, t.Port, t.Dir)
}
