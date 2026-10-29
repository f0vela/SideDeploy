package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"sidedeploy/internal/deploy"
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
	ui.Column(c).Width(sidebarWidth).FillHeight().Background(t.Surface).Children(func() {
		ui.Row(c).FillWidth().Padding(10, 10).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "PROYECTOS").FontSize(10).Bold().TextColor(t.TextMuted)
			ui.Row(c).Grow(1)
			if ui.Button(c, "+").Clicked() {
				go a.openRepoDialog()
			}
		})
		if len(a.store.Projects) == 0 {
			ui.Text(c, "Abre una carpeta con repositorio Git y quedará en esta lista.").
				FontSize(11).TextColor(t.TextMuted).Padding(4, 10)
			return
		}

		remove := ""
		sb := ui.Sidebar(c, &a.selPath, func() {
			for i := range a.store.Projects {
				p := &a.store.Projects[i]
				item := ui.SidebarItem(c, p.Path, nil, p.Name)
				if n := len(p.Targets); n > 0 {
					item.Children(func() { ui.Badge(c, strconv.Itoa(n)) })
				}
				item.ContextMenu(func(m *ui.Menu) {
					if m.Item("Quitar de la lista").Chosen() {
						remove = p.Path
					}
				})
			}
		}).Width(sidebarWidth)

		if sb.Changed() {
			go a.openRepo(a.selPath)
		}
		if remove != "" {
			a.removeProject(remove)
		}
	})
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
	a.repo = nil
	a.cfg = nil
	a.projIdx = -1
	a.selPath = ""
	a.branch = ""
	a.summary = ""
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
}

func (a *app) targetList(c *ui.Context) {
	t := c.Theme()
	p := a.project()
	ui.Column(c).Width(560).Gap(10).Children(func() {
		ui.Text(c, "Destinos de "+p.Name).FontSize(18).Bold()
		ui.Text(c, "El marcado es al que se suben los cambios.").FontSize(11).TextColor(t.TextMuted)

		if a.confirmDelete >= 0 {
			a.confirmDeleteRow(c)
			return
		}

		if len(p.Targets) == 0 {
			ui.Text(c, "Todavía no hay destinos.").TextColor(t.TextMuted)
		}
		edit, del := -1, -1
		before := p.ActiveTarget
		for i := range p.Targets {
			tg := &p.Targets[i]
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Radio(c, &p.ActiveTarget, tg.Name, tg.Name).Grow(1)
				ui.Text(c, remoteSummary(*tg)).FontSize(11).TextColor(t.TextMuted).SingleLine()
				if ui.Button(c, "Editar").Clicked() {
					edit = i
				}
				if ui.Button(c, "Eliminar").Clicked() {
					del = i
				}
			})
		}
		if p.ActiveTarget != before {
			a.saveStore()
		}

		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "+ Añadir destino").Clicked() {
				a.beginEdit(-1)
			}
			ui.Row(c).Grow(1)
			if ui.Button(c, "Cerrar").Clicked() {
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

func (a *app) confirmDeleteRow(c *ui.Context) {
	t := c.Theme()
	p := a.project()
	name := ""
	if a.confirmDelete < len(p.Targets) {
		name = p.Targets[a.confirmDelete].Name
	}
	ui.Text(c, "¿Eliminar el destino “"+name+"”?").Bold()
	ui.Text(c, "Se borra de la lista de SideDeploy; el servidor no se toca.").FontSize(11).TextColor(t.TextMuted)
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		if ui.Button(c, "Cancelar").Clicked() {
			a.confirmDelete = -1
		}
		if ui.PrimaryButton(c, "Eliminar").Clicked() {
			a.deleteTarget(a.confirmDelete)
			a.confirmDelete = -1
		}
	})
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
	ui.Column(c).Width(560).Gap(12).Children(func() {
		ui.Text(c, title).FontSize(18).Bold()

		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.End).Children(func() {
			field(c, "Nombre", func() { ui.TextInput(c, &a.draft.Name).Grow(1) })
			field(c, "Tipo", func() { ui.Select(c, &a.draft.Type, []string{"sftp", "ftp"}).Width(110) })
			field(c, "Puerto", func() { ui.TextInput(c, &a.draftPort).Placeholder("por defecto").Width(110) })
		})
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.End).Children(func() {
			field(c, "Host", func() { ui.TextInput(c, &a.draft.Host).Placeholder("ftp.ejemplo.com").Grow(1) })
			field(c, "Usuario", func() { ui.TextInput(c, &a.draft.User).Width(180) })
		})
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.End).Children(func() {
			field(c, "Contraseña", func() { ui.TextInput(c, &a.draft.Password).Password().Grow(1) })
			field(c, "Clave privada", func() {
				ui.TextInput(c, &a.draftKey).Placeholder("~/.ssh/id_ed25519").Width(240)
			})
		})
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.End).Children(func() {
			field(c, "Carpeta remota", func() { ui.TextInput(c, &a.draft.Dir).Placeholder("/var/www/mi-sitio").Grow(1) })
			field(c, "baseDir", func() { ui.TextInput(c, &a.draft.BaseDir).Placeholder("/").Width(120) })
		})

		if a.formErr != "" {
			ui.Text(c, a.formErr).FontSize(12).TextColor(t.Danger)
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

// field stacks a label over an input.
func field(c *ui.Context, label string, build func()) {
	ui.Column(c).Gap(3).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(c.Theme().TextMuted)
		build()
	})
}
