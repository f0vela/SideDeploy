# sdeploy

La versión de terminal de SideDeploy: Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss).

Comparte con la app de ventana los mismos datos (`projects.json`, `pro-deployer.json`, `known_hosts`) y el mismo código de Git, destinos y despliegue (`internal/`), así que un proyecto o un destino creado en una aparece en la otra.

```sh
go build -o build/sdeploy ./cmd/sdeploy
build/sdeploy            # abre el último proyecto
build/sdeploy ~/sitio    # abre una carpeta
```

## Paridad con la app de ventana

| SideDeploy (ventana) | sdeploy |
| --- | --- |
| Barra lateral de proyectos, badge con nº de destinos, quitar de la lista | Panel «Proyectos» (`o` añadir, `x` quitar, `b` ocultar) |
| Lista de cambios Staged / Cambios, stage por archivo, stage todo | Panel «Archivos» (`espacio`, `a`, `A`) |
| Diff con números de línea y colores | Panel «Diff» (unificado, con contexto y desplazamiento) |
| Mensaje de commit, ⌘↵ | Cuadro de commit (`c`, `ctrl+s`) |
| Generar mensaje con Claude o Copilot | `g` / `ctrl+g`, `i` alterna la herramienta, `esc` cancela |
| Push / Publicar rama, ↑↓ en la barra de estado | `p` / `ctrl+p` |
| Destinos: lista, activo, añadir, editar, eliminar, importar de pro-deployer.json | `d` |
| Subir: destino, borrar remoto, progreso, errores, cancelar, confirmar >1 borrado, confiar en host SSH | `u` (y `sdeploy deploy` sin interfaz) |
| Ajustes: rutas de Claude y Copilot | `,` |

## Temas

Dracula, Tokyo Night (y Storm), Catppuccin (Mocha, Macchiato, Frappé, Latte), Nord, Gruvbox (oscuro y claro), Solarized (oscuro y claro), One Dark, Rosé Pine, Monokai, Kanagawa, Everforest y GitHub (oscuro y claro).

- `t` pasa al siguiente tema y lo guarda.
- `T` o `ctrl+t` abre el selector con vista previa en vivo (`enter` guarda, `esc` vuelve al anterior).
- `,` → Ajustes → Tema, con `←` `→`.
- `sdeploy theme` lista los temas; `sdeploy theme dracula` lo cambia; `sdeploy --theme nord` lo usa solo esa vez.

Agregar uno es una entrada más en `themes` (`theme.go`). La preferencia vive en `sdeploy.json`, junto a `projects.json`.

## Sin interfaz

```sh
sdeploy deploy --dry-run            # muestra qué se enviaría y a dónde, sin enviar
sdeploy deploy                      # el proyecto activo, su destino activo (pregunta antes)
sdeploy deploy ~/sitio --target prod --delete -y
```

Sale con 0 si todo subió, 1 si hubo errores y 130 si se canceló con ctrl+c.

## Datos

Usa la carpeta de la app de ventana (`~/Library/Application Support/SideDeploy` en macOS). `SDEPLOY_DATA` la cambia.
