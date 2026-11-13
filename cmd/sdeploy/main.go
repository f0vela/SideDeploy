// Command sdeploy is the terminal version of SideDeploy: the same projects,
// changes, commits, pushes and FTP/SFTP deploys, in a Bubble Tea interface.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// version follows mygo.json; release builds may set it with -ldflags.
var version = "0.6.0"

const usage = `sdeploy — SideDeploy en la terminal

Uso:
  sdeploy [ruta]              abre la interfaz (el último proyecto, o la ruta)
  sdeploy --theme <nombre>    abre con otro tema, solo esta vez
  sdeploy theme               muestra el tema actual y los disponibles
  sdeploy theme <nombre>      cambia el tema y lo guarda
  sdeploy deploy [ruta]       sube los cambios sin interfaz
        --target <nombre>     destino (por defecto el activo del proyecto)
        --delete              borra en el servidor lo que ya no está en el repo
        -n, --dry-run         solo muestra qué se enviaría
        -y, --yes             no pregunta antes de enviar, de borrar ni de confiar en un host
  sdeploy version

Dentro de la interfaz: ? ayuda · t siguiente tema · T elegir tema · , ajustes.
`

// dataDir is where projects.json lives, the same folder the window app uses.
func dataDir() string {
	if d := os.Getenv("SDEPLOY_DATA"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "SideDeploy")
}

func main() {
	args := os.Args[1:]
	dir := dataDir()
	p := loadPrefs(dir)

	themeOverride := ""
	var rest []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--theme" || a == "-t":
			if i+1 >= len(args) {
				fail("falta el nombre del tema")
			}
			i++
			themeOverride = args[i]
		case strings.HasPrefix(a, "--theme="):
			themeOverride = strings.TrimPrefix(a, "--theme=")
		default:
			rest = append(rest, a)
		}
	}

	if len(rest) > 0 {
		switch rest[0] {
		case "help", "-h", "--help":
			fmt.Print(usage)
			return
		case "version", "-v", "--version":
			fmt.Println("sdeploy", version)
			return
		case "theme", "themes":
			themeCommand(dir, p, rest[1:])
			return
		case "deploy":
			if themeOverride != "" {
				// Colors do not matter without the interface.
				themeOverride = ""
			}
			os.Exit(deployCommand(dir, rest[1:]))
		}
	}

	if themeOverride != "" {
		key, ok := lookupTheme(themeOverride)
		if !ok {
			fail("tema desconocido: " + themeOverride + " (mira sdeploy theme)")
		}
		p.Theme = key
	}

	start := ""
	if len(rest) > 0 {
		start = expandPath(rest[0])
	}
	m := newModel(dir, p, start)
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fail(err.Error())
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "sdeploy:", msg)
	os.Exit(1)
}

func themeCommand(dir string, p prefs, args []string) {
	if len(args) > 0 {
		key, ok := lookupTheme(strings.Join(args, " "))
		if !ok {
			fail("tema desconocido: " + strings.Join(args, " "))
		}
		p.Theme = key
		if err := savePrefs(dir, p); err != nil {
			fail(err.Error())
		}
		fmt.Println("Tema:", themes[key].Name)
		return
	}
	fmt.Println("Tema actual:", themes[p.Theme].Name)
	fmt.Println()
	for _, k := range themeKeys() {
		mark := "  "
		if k == p.Theme {
			mark = "✓ "
		}
		kind := "oscuro"
		if !themes[k].Dark {
			kind = "claro"
		}
		fmt.Printf("%s%-22s %-7s %s\n", mark, k, kind, themes[k].Name)
	}
}
