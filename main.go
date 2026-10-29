package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"sidedeploy/internal/deploy"
	"sidedeploy/internal/gitrepo"
	"sidedeploy/internal/projects"
	"sidedeploy/internal/settings"
)

// app is the state the window shows. The view reads it on the main thread;
// background work publishes changes through Window.Update.
type app struct {
	win     *mygo.Window
	dataDir string

	settings settings.Settings
	store    projects.Store
	projIdx  int
	selPath  string

	repo *gitrepo.Repo
	cfg  *deploy.Config

	branch  string
	summary string

	rows     []row
	files    ui.ListState
	selected int
	selKey   string
	selFile  string

	diff       *editor.DiffState
	diffTitle  string
	diffStaged bool
	diffNote   string
	diffSeq    int

	targetsOpen   bool
	targetEdit    bool
	targetNew     bool
	targetIndex   int
	confirmDelete int
	draft         deploy.Target
	draftPort     string
	draftKey      string
	formErr       string

	commitMsg string
	busy      bool
	gitMu     sync.Mutex
	errorOpen bool
	errorText string
	flash     string

	split         float32
	loadingStatus bool
	loadingDiff   bool
	refreshQueued bool
	opening       bool
	err           string
}

// row is one line of the changes table: a section header or a file.
type row struct {
	Header bool
	Title  string
	File   gitrepo.File
	Staged bool
}

func newApp() *app {
	return &app{split: 350, selected: -1, projIdx: -1, confirmDelete: -1}
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()

	if c.Shortcut(ui.Cmd, ui.KeyO) {
		go a.openRepoDialog()
	}
	if c.Shortcut(ui.Cmd, ui.KeyR) {
		a.refresh()
	}

	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.toolbar(c)
		if a.err != "" {
			a.errorBanner(c)
		}
		ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
			a.projectsPane(c)
			ui.Column(c).Grow(1).FillHeight().Children(func() {
				if a.repo == nil {
					a.welcome(c)
					return
				}
				ui.Split(c, &a.split, func() { a.filesPane(c) }, func() { a.diffPane(c) }).Grow(1).FillWidth()
				a.commitBar(c)
				a.statusBar(c)
			})
		})
	})
	a.targetsModal(c)
	a.errorModal(c)
	a.flashToast(c)
}

func (a *app) toolbar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Padding(10, 14).Gap(10).AlignItems(ui.Center).Children(func() {
		if ui.Button(c, "Abrir repo").Clicked() {
			go a.openRepoDialog()
		}
		if a.repo != nil {
			ui.Text(c, filepath.Base(a.repo.Root)).Bold().SingleLine()
			ui.Text(c, a.repo.Root).FontSize(11).TextColor(t.TextMuted).SingleLine().MaxWidth(360)
			if a.branch != "" {
				ui.Badge(c, a.branch)
			}
		}
		ui.Row(c).Grow(1)
		if a.repo != nil {
			if ui.Button(c, a.targetsLabel()).Clicked() {
				a.openTargets()
			}
			if a.loadingStatus {
				ui.Spinner(c)
			}
			if ui.Button(c, "Actualizar").Clicked() {
				a.refresh()
			}
		}
	})
	ui.Row(c).FillWidth().Height(1).Background(t.Border)
}

func (a *app) errorBanner(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Padding(8, 14).Gap(10).AlignItems(ui.Center).Background(t.Danger.Alpha(0.12)).Children(func() {
		ui.Text(c, a.err).FontSize(12).TextColor(t.Danger).Grow(1)
		if ui.Button(c, "Cerrar").Clicked() {
			a.err = ""
		}
	})
}

func (a *app) welcome(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(12).Padding(24).Children(func() {
		ui.Text(c, "SideDeploy").FontSize(30).Bold()
		if a.opening {
			ui.Spinner(c)
			ui.Text(c, "Abriendo repositorio…").TextColor(t.TextMuted)
			return
		}
		ui.Text(c, "Revisa los cambios de tu repo y súbelos por FTP/SFTP.").TextColor(t.TextMuted)
		if len(a.store.Projects) > 0 {
			ui.Text(c, "Elige un proyecto a la izquierda, o abre otra carpeta.").FontSize(12).TextColor(t.TextMuted)
		}
		if ui.PrimaryButton(c, "Abrir repositorio").Clicked() {
			go a.openRepoDialog()
		}
		ui.Text(c, "También con ⌘O").FontSize(11).TextColor(t.TextMuted)
	})
}

func (a *app) filesPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).FillWidth().Padding(10, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Cambios").Bold()
			ui.Row(c).Grow(1)
			if a.changedCount() > 0 {
				if ui.Button(c, "Stage todo").Tooltip("Añade todos los cambios al stage").Clicked() {
					a.stageAll()
				}
			}
			if a.stagedCount() > 0 {
				if ui.Button(c, "Unstage todo").Tooltip("Saca todos los cambios del stage").Clicked() {
					a.unstageAll()
				}
			}
		})
		ui.Row(c).FillWidth().Height(1).Background(t.Border)

		if len(a.rows) == 0 {
			ui.Column(c).Grow(1).Center().Gap(6).Padding(24).Children(func() {
				ui.Text(c, "Sin cambios").TextColor(t.TextMuted)
				ui.Text(c, "El working tree está limpio.").FontSize(11).TextColor(t.TextMuted)
			})
			return
		}

		a.files.Selected = &a.selected
		a.files.Header = func(i int) bool { return a.rows[i].Header }
		a.files.Label = func(i int) string {
			if a.rows[i].Header {
				return a.rows[i].Title
			}
			return a.rows[i].File.Path
		}

		cols := []ui.TableColumn{
			{Title: "", ID: "check", Width: 34, Fixed: true},
			{Title: "", ID: "status", Width: 30, Fixed: true},
			{Title: "Archivo", ID: "path"},
		}
		table := ui.Table(c, &a.files, cols, len(a.rows), func(i, col int) {
			r := a.rows[i]
			if r.Header {
				if col == 0 {
					ui.Text(c, r.Title).FontSize(11).Bold().TextColor(t.TextMuted).
						Padding(6, 12).Background(t.Surface).FillWidth()
				}
				return
			}
			switch col {
			case 0:
				checked := r.Staged
				if ui.Checkbox(c, &checked, "").Label("Stage " + r.File.Path).Changed() {
					if checked {
						a.stageFile(r.File)
					} else {
						a.unstageFile(r.File)
					}
				}
			case 1:
				code := a.rowCode(i)
				color := statusColor(code, t)
				if i == a.selected {
					color = t.AccentText
				}
				ui.Text(c, string(code)).FontSize(11).Bold().TextAlign(ui.Center).TextColor(color)
			case 2:
				name := r.File.Path
				if r.Staged && r.File.OrigPath != "" {
					name = r.File.OrigPath + "  →  " + r.File.Path
				}
				ui.Text(c, name).SingleLine()
			}
		}).Grow(1)

		if table.Changed() {
			if a.selected >= 0 && a.selected < len(a.rows) && !a.rows[a.selected].Header {
				a.selKey = rowKey(a.rows[a.selected])
				a.loadDiff()
			}
		}
	})
}

func (a *app) diffPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).FillWidth().Padding(10, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			if a.diffTitle == "" {
				ui.Text(c, "Diff").Bold().TextColor(t.TextMuted)
			} else {
				ui.Text(c, a.diffTitle).Bold().SingleLine().Grow(1)
				label := "Cambios"
				if a.diffStaged {
					label = "Staged"
				}
				ui.Badge(c, label)
			}
		})
		ui.Row(c).FillWidth().Height(1).Background(t.Border)

		switch {
		case a.loadingDiff:
			a.diffMessage(c, "Cargando diff…", true)
		case a.diff != nil:
			editor.DiffView(c, a.diff).Grow(1).FillWidth()
		case a.diffNote != "":
			a.diffMessage(c, a.diffNote, false)
		case len(a.rows) == 0:
			a.diffMessage(c, "No hay cambios para mostrar.", false)
		default:
			a.diffMessage(c, "Selecciona un archivo para ver sus cambios.", false)
		}
	})
}

func (a *app) diffMessage(c *ui.Context, text string, spinner bool) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(8).Padding(24).Children(func() {
		if spinner {
			ui.Spinner(c)
		}
		ui.Text(c, text).TextColor(t.TextMuted).TextAlign(ui.Center)
	})
}

func (a *app) statusBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Height(1).Background(t.Border)
	ui.Row(c).FillWidth().Padding(6, 14).Gap(12).AlignItems(ui.Center).Children(func() {
		branch := a.branch
		if branch == "" {
			branch = "sin rama"
		}
		ui.Text(c, "⎇ "+branch).FontSize(11).TextColor(t.TextMuted)
		if a.summary != "" {
			ui.Text(c, a.summary).FontSize(11).TextColor(t.TextMuted)
		}
		ui.Row(c).Grow(1)
		if tg := a.activeTarget(); tg != nil {
			dir := strings.TrimRight(tg.Dir, "/")
			if dir == "" {
				dir = "/"
			}
			ui.Text(c, "→ "+tg.Name+"  "+dir).FontSize(11).TextColor(t.TextMuted).SingleLine().MaxWidth(420)
		} else if a.repo != nil {
			ui.Text(c, "Sin destino de despliegue").FontSize(11).TextColor(t.TextMuted)
		}
	})
}

// targetsLabel names the targets button in the toolbar.
func (a *app) targetsLabel() string {
	if n := len(a.targets()); n > 0 {
		return fmt.Sprintf("Destinos · %d", n)
	}
	return "Destinos"
}

func (a *app) rowCode(i int) byte {
	r := a.rows[i]
	if r.Staged {
		return r.File.Index
	}
	if r.File.Unmerged {
		return 'U'
	}
	if r.File.Untracked() {
		return '?'
	}
	return r.File.Worktree
}

func statusColor(code byte, t *ui.Theme) ui.Color {
	switch code {
	case 'A':
		return t.Success
	case 'D', 'U':
		return t.Danger
	case 'R', 'C', '?':
		return t.Warning
	default:
		return t.Accent
	}
}

func buildRows(st gitrepo.Status) []row {
	var staged, changed []row
	for _, f := range st.Files {
		if f.Staged() {
			staged = append(staged, row{File: f, Staged: true})
		}
		if f.Unstaged() {
			changed = append(changed, row{File: f})
		}
	}
	rows := make([]row, 0, len(staged)+len(changed)+2)
	if len(staged) > 0 {
		rows = append(rows, row{Header: true, Title: fmt.Sprintf("STAGED · %d", len(staged))})
		rows = append(rows, staged...)
	}
	if len(changed) > 0 {
		rows = append(rows, row{Header: true, Title: fmt.Sprintf("CAMBIOS · %d", len(changed))})
		rows = append(rows, changed...)
	}
	return rows
}

func rowKey(r row) string {
	if r.Header {
		return ""
	}
	return fmt.Sprintf("%t:%s", r.Staged, r.File.Path)
}

func indexOfPath(rows []row, path string) int {
	if path == "" {
		return -1
	}
	for i, r := range rows {
		if !r.Header && r.File.Path == path {
			return i
		}
	}
	return -1
}

func indexOfKey(rows []row, key string) int {
	if key == "" {
		return -1
	}
	for i, r := range rows {
		if !r.Header && rowKey(r) == key {
			return i
		}
	}
	return -1
}

func firstFileIndex(rows []row) int {
	for i, r := range rows {
		if !r.Header {
			return i
		}
	}
	return -1
}

func (a *app) openRepoDialog() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      a.win,
		Title:       "Abrir repositorio",
		Message:     "Elige la carpeta que contiene el repositorio Git.",
		ButtonLabel: "Abrir",
		Directory:   true,
	})
	if err != nil {
		a.report("No se pudo abrir el diálogo: " + err.Error())
		return
	}
	if len(paths) == 0 {
		return
	}
	a.openRepo(paths[0])
}

func (a *app) openRepo(path string) {
	a.win.Update(func() {
		a.opening = true
		a.err = ""
	})

	repo, err := gitrepo.Open(path)
	if err != nil {
		a.win.Update(func() {
			a.opening = false
			a.err = err.Error()
			a.selPath = a.store.Active
		})
		return
	}
	cfg, _, cfgErr := deploy.Load(repo.Root)

	a.win.Update(func() {
		a.opening = false
		a.repo = repo
		a.cfg = cfg
		a.projIdx = a.store.Add(repo.Root)
		a.store.Active = repo.Root
		a.selPath = repo.Root
		a.saveStore()
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
		if cfgErr != nil {
			a.err = "Config: " + cfgErr.Error()
		}
		a.refresh()
	})
}

func (a *app) refresh() {
	repo := a.repo
	if repo == nil || a.win == nil {
		return
	}
	if a.loadingStatus {
		a.refreshQueued = true
		return
	}
	a.loadingStatus = true
	go func() {
		st, err := repo.Status(context.Background())
		a.win.Update(func() {
			if repo != a.repo {
				return
			}
			a.loadingStatus = false
			if err != nil {
				a.err = err.Error()
			} else {
				a.applyStatus(st)
			}
			if a.refreshQueued {
				a.refreshQueued = false
				a.refresh()
			}
		})
	}()
}

func (a *app) applyStatus(st gitrepo.Status) {
	a.branch = st.Branch
	if st.Detached {
		a.branch = "HEAD separado"
	}
	staged, changed := 0, 0
	for _, f := range st.Files {
		if f.Staged() {
			staged++
		}
		if f.Unstaged() {
			changed++
		}
	}
	a.summary = fmt.Sprintf("%d staged · %d cambios", staged, changed)

	oldKey := a.selKey
	a.rows = buildRows(st)
	a.selected = indexOfKey(a.rows, oldKey)
	if a.selected < 0 {
		// The same file moved between the staged and the changed section.
		a.selected = indexOfPath(a.rows, a.selFile)
	}
	if a.selected < 0 {
		a.selected = firstFileIndex(a.rows)
	}
	if a.selected >= 0 {
		a.selKey = rowKey(a.rows[a.selected])
		a.loadDiff()
		return
	}
	a.selKey = ""
	a.selFile = ""
	a.diff = nil
	a.diffTitle = ""
	a.diffNote = ""
	a.loadingDiff = false
	a.diffSeq++
}

func (a *app) loadDiff() {
	if a.selected < 0 || a.selected >= len(a.rows) || a.rows[a.selected].Header {
		return
	}
	r := a.rows[a.selected]
	repo := a.repo
	if repo == nil {
		return
	}
	if a.win == nil {
		a.loadingDiff = false
		return
	}

	a.selFile = r.File.Path
	a.diffTitle = r.File.Path
	a.diffStaged = r.Staged
	a.diffNote = ""
	a.diff = nil
	a.loadingDiff = true
	a.diffSeq++
	seq := a.diffSeq

	go func() {
		d, err := repo.Diff(context.Background(), r.File, r.Staged)
		a.win.Update(func() {
			if seq != a.diffSeq || repo != a.repo {
				return
			}
			a.loadingDiff = false
			if err != nil {
				a.diffNote = err.Error()
				return
			}
			switch {
			case d.TooLarge:
				a.diffNote = "Archivo demasiado grande para el diff (más de 2 MB)."
			case d.Binary:
				a.diffNote = "Archivo binario."
			case d.Note != "":
				a.diffNote = d.Note
			default:
				old := editor.NewBuffer(d.Old)
				nw := editor.NewBuffer(d.New)
				lang := editor.LanguageFor(r.File.Path)
				old.SetLanguage(lang)
				nw.SetLanguage(lang)
				a.diff = &editor.DiffState{
					Old:          old,
					New:          nw,
					Layout:       editor.DiffUnified,
					ContextLines: 3,
					Wrap:         true,
				}
			}
		})
	}()
}

// report shows a message on the window, from any goroutine.
func (a *app) report(msg string) {
	if a.win == nil {
		return
	}
	a.win.Update(func() { a.err = msg })
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
			a.dataDir = dir
			a.settings = settings.Load(dir)
			a.store = projects.Load(dir)
			// The first version kept only the last repo: adopt it.
			if a.store.Active == "" && len(a.store.Projects) == 0 {
				a.store.Active = a.settings.LastRepo
			}
			a.selPath = a.store.Active
		}
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "SideDeploy",
			Width:     1280,
			Height:    820,
			MinWidth:  900,
			MinHeight: 560,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
		if a.store.Active != "" {
			go a.openRepo(a.store.Active)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
