package main

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"sidedeploy/internal/aicommits"
	"sidedeploy/internal/projects"
)

// installMenu puts Ajustes in the app menu, with the standard menus around it.
func (a *app) installMenu() {
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			mygo.Separator(),
			{Label: "Ajustes…", Accelerator: "Cmd+,", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.openSettings()
			}},
			mygo.Separator(),
			{Role: mygo.RoleServices},
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleFileMenu},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleViewMenu},
		{Role: mygo.RoleWindowMenu},
	}))
}

// openSettings shows the settings window, creating it the first time.
func (a *app) openSettings() {
	if a.win == nil {
		return
	}
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		return
	}
	a.draftClaude = a.store.ClaudePath
	a.draftCopilot = a.store.CopilotPath
	a.settingsMsg = ""
	a.settingsErr = false
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:         "Ajustes",
		Parent:        a.win,
		Width:         620,
		Height:        340,
		MinWidth:      520,
		MinHeight:     300,
		StateKey:      "settings",
		TitleBarStyle: mygo.TitleBarHiddenInset,
		Content:       ui.View(a.settingsView),
	})
	a.settingsWin = win
	win.OnClosed(func() { a.settingsWin = nil })
}

func (a *app) settingsView(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Column(c).Grow(1).FillWidth().Padding(max(bar.Height, 0)+20, 28, 16, 28).Gap(16).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconSparkle).FontSize(26).TextColor(t.Accent)
				ui.Column(c).Gap(1).Children(func() {
					ui.Text(c, "Herramientas de IA").FontSize(16).Bold()
					ui.Text(c, "Generan el mensaje del commit. Vacío: se busca en el PATH.").
						FontSize(11).TextColor(t.TextMuted)
				})
			})
			ui.Form(c, func() {
				a.cliPathField(c, "Claude", aicommits.Claude, &a.draftClaude)
				a.cliPathField(c, "Copilot", aicommits.Copilot, &a.draftCopilot)
			})
			if a.settingsMsg != "" {
				color := t.TextMuted
				ic := iconCheck
				if a.settingsErr {
					color, ic = t.Danger, iconWarn
				}
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, ic).FontSize(13).TextColor(color)
					ui.Text(c, a.settingsMsg).FontSize(12).TextColor(color)
				})
			}
		})
		ui.Row(c).FillWidth().Height(1).Background(t.Border)
		ui.Row(c).FillWidth().Padding(10, 20).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "Cerrar").Clicked() || c.Shortcut(ui.Cmd, ui.KeyW) {
				if a.settingsWin != nil {
					a.settingsWin.Hide()
				}
			}
			if ui.PrimaryButton(c, "Guardar").Clicked() {
				a.saveSettings()
			}
		})
	})
}

// cliPathField edits the executable of one AI tool and shows where it
// resolves to right now.
func (a *app) cliPathField(c *ui.Context, label, tool string, value *string) {
	t := c.Theme()
	bin, err := aicommits.Resolve(tool, *value)
	ui.Field(c, label, func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, value).Placeholder("vacío = PATH").Grow(1)
			if ui.Button(c, "Elegir…").Clicked() {
				go a.pickCLI(tool, value)
			}
		})
	}).Description(resolvedNote(bin, err)).Error(errText(err))
	_ = t
}

func resolvedNote(bin string, err error) string {
	if err != nil {
		return ""
	}
	return "Se usará " + bin
}

func errText(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

// pickCLI lets the user choose the executable with the system dialog.
func (a *app) pickCLI(tool string, value *string) {
	win := a.settingsWin
	if win == nil {
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      win,
		Title:       "Ruta de " + tool,
		Message:     "Elige el ejecutable de " + tool + ".",
		ButtonLabel: "Elegir",
	})
	if err != nil {
		win.Update(func() {
			a.settingsErr = true
			a.settingsMsg = "No se pudo abrir el diálogo: " + err.Error()
		})
		return
	}
	if len(paths) == 0 {
		return
	}
	win.Update(func() {
		*value = paths[0]
		a.settingsErr = false
		a.settingsMsg = ""
	})
}

func (a *app) saveSettings() {
	a.store.ClaudePath = strings.TrimSpace(a.draftClaude)
	a.store.CopilotPath = strings.TrimSpace(a.draftCopilot)
	if err := projects.Save(a.dataDir, a.store); err != nil {
		a.settingsErr = true
		a.settingsMsg = "No se pudo guardar: " + err.Error()
		return
	}
	a.settingsErr = false
	a.settingsMsg = "Guardado."
}
