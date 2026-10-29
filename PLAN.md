# SideFtpGit — Plan de desarrollo

App nativa de escritorio (macOS) en **Go + MyGo** para revisar cambios de Git,
hacer commits (con mensaje asistido por IA desde el CLI) y subir los cambios
por **FTP/SFTP** con la misma lógica de `heminei.pro-deployer`.

Estado: **M0, M1, proyectos/destinos, M2, M3 y M4 implementados**; M5 pendiente.

## Implementado (M0 + M1 + proyectos/destinos + M2 + M3 + M4)

- Scaffold nativo MyGo 0.7.3 (`mygo.json`, `resources/icon.png`, `go.mod`).
- Stage por archivo con casilla en la tabla, y "Stage todo" / "Unstage todo".
- Commit con mensaje multilínea; corre en segundo plano con spinner y respeta
  hooks y firma del repo. Si un hook falla, su salida se muestra en un diálogo.
- Mensaje generado con Claude o Copilot desde el CLI, con el diff del stage (o
  de todo el working tree si no hay nada staged). Se copia a la caja para
  editarlo; nunca se commitea solo. Se puede cancelar mientras corre.
- La selección sigue al archivo cuando cambia de sección al hacer stage.
- Barra lateral de proyectos: cada carpeta abierta queda en la lista y se
  cambia con un clic; menú contextual para quitarla (no toca el disco).
- Editor de destinos por proyecto en la app: ver, añadir, editar, elegir el
  activo y eliminar destinos FTP/SFTP, con contraseña oculta y clave privada.
  Los destinos de `pro-deployer.json` se adoptan la primera vez que se abren.
- `projects.json` en `~/Library/Application Support/SideDeploy` (0600) guarda
  proyectos y destinos, y se puede editar a mano con el mismo formato de
  `pro-deployer.json`.
- Leer `.vscode/pro-deployer.json` o `pro-deployer.json`, resolver variables y
  mostrar el target activo y su carpeta remota.
- Listar cambios en dos secciones (Staged / Cambios) con modificados, nuevos,
  renombrados, borrados, untracked y conflictos.
- Diff unificado por archivo con `editor.DiffView`, avisos para binarios y
  archivos de más de 2 MB.
- Refrescar con el botón o `Cmd+R`; abrir repo con `Cmd+O`.
- Subida real por FTP/FTPS y SFTP desde un modal: selector de destino, lista
  local → remota, progreso, log de errores y cancelación. Respeta `ignore`,
  `include`, `checkGitignore`, `autoDelete` y `concurrency` de
  `pro-deployer.json`; crea carpetas remotas y reintenta una vez cada archivo.
  Con `autoDelete`, borrar más de un archivo remoto pide confirmación.
- SFTP verifica la host key contra `~/.ssh/known_hosts` y el `known_hosts`
  propio de la app; un host nuevo muestra su huella y solo sube si se confía
  en ella. Nunca se usa `InsecureIgnoreHostKey`. Los destinos FTP abren una
  conexión por worker; SFTP comparte una sola conexión SSH.
- Tests: parser de porcelain v2, integración con Git real, config/variables,
  rutas remotas, filtros, candidatos, cola (concurrencia, reintento, cancelar),
  known_hosts y un servidor SSH+SFTP en proceso; vista headless.

---

## 1. Alcance de la v1

- Abrir una carpeta que contenga un repo Git (diálogo nativo o drag & drop).
- Detectar la raíz del repo y la rama actual.
- Listar cambios separados en **Staged** y **Changes** (modificados, nuevos,
  renombrados, borrados).
- Ver el diff del archivo seleccionado.
- Agregar/quitar de staging (por archivo y “todo”).
- Commit con mensaje escrito o generado por IA (`claude` o `copilot` CLI).
- Leer `.vscode/pro-deployer.json` (o `pro-deployer.json` en la raíz) y sus
  targets FTP/SFTP.
- Subir los archivos cambiados al target elegido, con progreso y log.
- Persistir: último repo, tamaño de ventana/paneles, target y herramienta de IA.

### Fuera de la v1 (explícito)

- Editor de código o “upload on save” (no hay editor).
- Descargar archivos, borrar remotos manualmente, diff local vs remoto.
- Múltiples workspaces/repos en una misma ventana.
- Historial de commits, ramas, merge/rebase, push a Git.
- Llavero propio de contraseñas (se leen del config existente).
- Windows/Linux empaquetados (MyGo es multiplataforma, pero se prueba en macOS).

---

## 2. Qué replica de `pro-deployer` (revisado del .vsix)

La extensión está compilada en `out/*.js`; su comportamiento es replicable en Go.
Lo relevante:

| Comportamiento | Implementación en la app |
|---|---|
| Config en `${workspaceFolder}/.vscode/pro-deployer.json` | Se busca primero ahí; fallback a `<repo>/pro-deployer.json` |
| `targets[]` con `type: ftp \| sftp` | Dos clientes detrás de una interfaz común |
| `activeTargets[]` | Filtra los targets seleccionables |
| `dir` = carpeta remota base | `remote = dir + "/" + rutaRelativa` |
| `baseDir` = subcarpeta local | Se quita ese prefijo de la ruta relativa (igual que `getRelativePath`) |
| `ignore` / `include` (globs, dotfiles) | `doublestar` + reglas por defecto (`.git/**`, config) |
| `checkGitignore` | `git check-ignore` en lote (Git ya es dependencia) |
| `concurrency: 5` | Worker pool con límite configurable |
| Crear carpetas remotas si no existen | `mkdir -p` remoto cacheado |
| `autoDelete` | Si un archivo local borrado/renombrado aparece en Git, borrar el remoto |
| FTP `secure` / `transferDataType` / keepalive | Flags de `jlaffaye/ftp` |
| SFTP password / privateKey / passphrase / agent | `pkg/sftp` + `x/crypto/ssh` |
| Variables `${workspaceFolder}`, `${env:VAR}`, etc. | Resolución al cargar el config |
| Upload on save, status bar, quick pick, command palette | No aplica: se reemplaza por “Subir cambios” en la UI |

**Decisión clave:** se sube el **working tree** (staged + unstaged + untracked),
igual que “Upload All Uncommitted Files” de pro-deployer. No se suben objetos
del commit; se sube el archivo tal como está en disco. La UI lo dirá claro.

---

## 3. Stack y dependencias

- Go 1.27.x.
- `github.com/egoist/mygo` **v0.7.3**, UI nativa (`ui`, `ui/editor`), sin webview.
- Git: **CLI** (`git`), no `go-git`. Razones: respeta credenciales, hooks,
  firma de commits y configuración del usuario; menos dependencias.
- FTP: `github.com/jlaffaye/ftp` (v0.2.4).
- SFTP: `github.com/pkg/sftp` + `golang.org/x/crypto/ssh` (+ `knownhosts`).
- Globs: `github.com/bmatcuk/doublestar/v4`.
- Sin base de datos: JSON plano.

---

## 4. Arquitectura

```
SideFtpGit/
├── main.go                  estado de la app, vista, atajos, menú
├── mygo.json                nombre, identifier, versión, icono
├── resources/icon.png
├── internal/
│   ├── gitrepo/             status, diff, stage, commit (CLI)
│   ├── aicommits/           claude/copilot, prompt, limpieza, timeout
│   ├── deploy/              config, filtros, rutas, clientes, cola
│   └── settings/            preferencias de la app
└── PLAN.md
```

### 4.1 `internal/gitrepo`

Todo con `exec.CommandContext` y `git -C <repo>`:

- `Root(path)`: `git rev-parse --show-toplevel`.
- `Status()`: `git status --porcelain=v2 -z --branch --untracked-files=all`.
  Modelo:

  ```go
  type File struct {
      Path     string // ruta relativa al repo
      OrigPath string // para renombrados
      Index    byte   // X de porcelain v2
      Worktree byte   // Y
      Staged   bool
      Untracked bool
  }
  ```

- `Diff(path, staged)`: `git diff --cached --` o `git diff --`; untracked vía
  `git diff --no-index /dev/null <path>` (exit 1 esperado) o lectura directa.
- `Stage(paths)`: `git add -A -- <paths>`.
- `Unstage(paths)`: `git restore --staged -- <paths>` (fallback `git reset HEAD --`).
- `Commit(msg, amend)`: `git commit -m <msg>`; captura stdout/stderr y timeout
  (los hooks pueden tardar o quedarse esperando).
- `LastCommit()` para mostrar el último commit en la barra superior (opcional).

Se parsea el formato `-z` (separado por NUL) para soportar espacios y saltos
de línea en nombres.

### 4.2 `internal/aicommits`

- Detecta `claude` y `copilot` en `PATH`; el usuario elige en la UI y la
  elección queda en `aiTool` de `projects.json` (Claude por defecto).
- Prompt: diff staged si existe; si no, diff del working tree más el texto de
  los archivos untracked. Se recorta a 30 KB y se marca “diff truncado”.
- Instrucción: mismo idioma y estilo que los últimos 10 asuntos de commit (si
  no hay, español), primera línea ≤ 72 caracteres en imperativo, cuerpo breve
  opcional, sin explicación ni bloques de código.
- Ejecución segura:
  - Claude: `claude -p`.
  - Copilot: `copilot -p … -s`.
  - Ambos con cwd = directorio temporal vacío, `NO_COLOR=1` y sin rutas del
    repo, así no leen ni escriben archivos.
- Timeout 2 min, cancelable desde la barra; el stderr del CLI se muestra en el
  diálogo de error.
- Limpieza: quitar fences ```, comillas y prefijos “Commit message:” /
  “Mensaje:”, conservando el cuerpo opcional.
- Si falla, se mantiene el mensaje escrito y se muestra el error; nunca se
  commitea automáticamente.

### 4.3 `internal/deploy`

```go
type Config struct {
    ActiveTargets []string `json:"activeTargets"`
    Concurrency   int      `json:"concurrency"`
    Ignore        []string `json:"ignore"`
    Include       []string `json:"include"`
    CheckGitignore bool    `json:"checkGitignore"`
    AutoDelete    bool     `json:"autoDelete"`
    Targets       []Target `json:"targets"`
}

type Target struct {
    Name, Type, Host, User, Password string
    Port int
    Dir, BaseDir string
    PrivateKey, Passphrase *string
    UseAuthAgent bool
    AuthAgentPath *string
    TransferDataType string
    Secure any // false | true | "implicit"
    KeepaliveIntervalMs int
    SecureOptions *struct{ RejectUnauthorized bool }
}
```

- `ResolveVariables(cfg, repo)` para `${workspaceFolder}`,
  `${workspaceFolderBasename}`, `${userHome}`, `${pathSeparator}`, `${env:VAR}`.
- `RemotePath(target, rel)`: quita `baseDir`, valida que no escape con `..`,
  normaliza `/`, devuelve `dir + "/" + rel`.
- `ShouldSkip(rel, cfg, repo)`: `.git/**`, archivo de config, `ignore`,
  `include` (si hay), y `.gitignore` opcional vía `git check-ignore --stdin -z`.
- `Client` interface:

  ```go
  type Client interface {
      Upload(ctx context.Context, local, remote string) error
      MkdirAll(ctx context.Context, remoteDir string) error
      Delete(ctx context.Context, remote string) error
      Close() error
  }
  ```

  - FTP: una conexión por worker (N = `concurrency`), modo pasivo, TLS según
    `secure`, tipo ASCII/binario, keepalive.
  - SFTP: una conexión SSH compartida, host key verificada contra
    `~/.ssh/known_hosts`; si el host es nuevo, diálogo con fingerprint para
    confiar (TOFU) y guardarlo en el known_hosts de la app.
- Cola: worker pool, estados `pending/running/ok/error/skipped`, cancelación por
  contexto, reintento único en errores transitorios, log con rutas y mensajes
  **sin contraseñas**.
- Borrados: los archivos `D` se omiten salvo que `autoDelete` esté activo; los
  renombrados suben la ruta nueva y, con `autoDelete`, borran la vieja.

### 4.4 `internal/projects` y `internal/settings`

`~/Library/Application Support/SideDeploy/projects.json` (0600):

```json
{
  "active": "/ruta/al/repo",
  "projects": [
    {
      "name": "vid.casadedios.org",
      "path": "/ruta/al/repo",
      "activeTarget": "CDD FTP",
      "targets": [ { "name": "CDD FTP", "type": "sftp", ... } ]
    }
  ]
}
```

- Los `targets` usan los mismos campos que `pro-deployer.json`, así que se
  pueden copiar de un archivo al otro.
- Precedencia: si el proyecto tiene `targets` propios manda este archivo; si
  no, se leen (solo lectura) los de `pro-deployer.json`. Al abrir el editor se
  adoptan los del JSON del repo para poder editarlos.
- `settings.json` es legado: solo se lee `lastRepo` una vez, para migrarlo.
- Tamaño de ventana lo persiste MyGo (`StateKey`). Nunca se guardan contraseñas
  fuera de `projects.json`.

---

## 5. UI propuesta (MyGo nativo)

Una sola ventana, ~1280×820, tema claro/oscuro del sistema, con una barra
lateral (`ui.Sidebar`) que lista los proyectos y el resto dividido en cambios
y diff. Los destinos del proyecto se editan en un `ui.Modal`.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ SideFtpGit  [Abrir repo]  vid.casadedios.org  ⎇ main   [↻]   Target: CDD FTP  │
├────────────────────────┬─────────────────────────────────────────────────────┤
│ CAMBIOS                │ Diff: app/Models/Video.php          [Unified|Split] │
│ ┌ Staged (2) ────────┐ │ ┌─────────────────────────────────────────────────┐ │
│ │ M app/.../Video.php│ │ │                                                 │ │
│ │ A app/views/new.php│ │ │   (editor.DiffView)                             │ │
│ └────────────────────┘ │ │                                                 │ │
│ ┌ Changes (3) ───────┐ │ └─────────────────────────────────────────────────┘ │
│ │ M config/app.php   │ │ ┌─ [Commit] [Deploy] ─────────────────────────────┐ │
│ │ ? public/css/x.css │ │ │ Mensaje: _________________________________ [IA ▾]│ │
│ │ D app/old.php      │ │ │ [Generar con IA]              [Commit]          │ │
│ └────────────────────┘ │ └─────────────────────────────────────────────────┘ │
│ [Stage all] [Unstage]  │                                                     │
└────────────────────────┴─────────────────────────────────────────────────────┘
```

- **Tabla de cambios:** `ui.Table` con secciones Staged/Changes, icono de estado,
  selección múltiple, menú contextual (stage/unstage, copiar ruta, ver diff,
  subir solo ese archivo).
- **Diff:** `editor.DiffView` con buffers del contenido antes/después
  (`git show HEAD:path` vs disco; para untracked, buffer vacío vs archivo).
  Si el diff es binario o muy grande, mensaje y botón “abrir en editor externo”.
- **Panel inferior:** `ui.Segmented` Commit/Deploy.
  - Commit: `ui.TextInput` multilínea, selector Claude/Copilot, botón IA,
    botón Commit (deshabilitado si no hay staged o mensaje).
  - Deploy (implementado como modal, como el editor de destinos): selector de
    target, lista local → remota, checkboxes, “Subir N archivos”, `ui.Progress`,
    log desplazable, “Cancelar” y confirmación de borrados.
- **Estados:** vacío (“Abre una carpeta con un repo Git”), sin config
  (“No se encontró pro-deployer.json”), sin target activo, error de conexión.
- **Atajos:** `Cmd+O` abrir repo, `Cmd+R` refrescar, `Cmd+Enter` commit,
  `Cmd+G` generar mensaje, `Cmd+Shift+U` subir cambios, `Cmd+K` limpiar log.
- **Toasts** para éxito/error; el detalle va al log.

Threading: la vista lee el estado en el hilo principal; todo trabajo de Git,
IA y red corre en goroutines y actualiza con `win.Update(...)`.

---

## 6. Flujos

### Abrir repo
1. `Dialog.Open({Directory: true})`.
2. `git rev-parse --show-toplevel`; si falla, error claro.
3. Cargar config (`.vscode/pro-deployer.json` → fallback raíz).
4. Cargar `settings.json`; restaurar target/IA.
5. `Status()` + primer diff.

### Commit
1. Seleccionar archivos → Stage.
2. (Opcional) “Generar con IA”: diff staged → CLI → mensaje editable.
3. Commit → refrescar status → toast con hash corto.
4. Si hay hooks y fallan, mostrar salida y no limpiar el mensaje.

### Deploy
1. Refrescar status.
2. Construir lista de candidatos: archivos cambiados del working tree,
   aplicando filtros y mapeo remoto.
3. Elegir target (activo por defecto).
4. Subir seleccionados/todo con progreso; crear carpetas; borrar remotos si
   `autoDelete` y el archivo fue borrado/renombrado.
5. Resumen: `N subidos, M omitidos, K errores`; el log queda visible.

---

## 7. Seguridad y límites

- **Contraseñas en texto plano** en `pro-deployer.json` (comportamiento actual
  de la extensión). La app no las copia a su config ni las escribe en logs.
  Se recomienda `${env:VAR}` y `.gitignore` para ese archivo.
- **SFTP:** host key verificada (known_hosts + TOFU). Nunca `InsecureIgnoreHostKey`.
- **FTPS:** si `secure` está activo, validar certificado por defecto; si el
  config pide `rejectUnauthorized: false`, aceptarlo pero avisar en el log.
- **Rutas remotas:** rechazar `..`, rutas absolutas inesperadas y cualquier
  destino fuera de `dir`.
- **IA:** solo recibe el texto del diff; sin acceso a archivos ni shell; timeout;
  el usuario revisa el mensaje antes de commitear.
- **Borrado remoto** solo con `autoDelete`; confirmación si son > 1 archivos.
- **Symlinks:** se omiten en v1 (evita subir fuera del repo).

---

## 8. Hitos y criterios de aceptación

| # | Hito | Criterio de aceptación | Estimado |
|---|---|---|---|
| M0 ✅ | Base | App nativa abre, elige carpeta, detecta repo, lee config y muestra target; settings persisten | 0.5 d |
| M1 ✅ | Git lectura | Lista staged/unstaged/untracked/renombrados/borrados; branch; diff por archivo; refrescar | 1 d |
| M1.5 ✅ | Proyectos y destinos | Lista de proyectos en barra lateral; alta/baja; editor de destinos FTP/SFTP por proyecto en la app y en `projects.json` | 1 d |
| M2 ✅ | Git escritura | Stage/unstage por archivo y todo; commit con mensaje; errores de hooks visibles | 0.5 d |
| M3 ✅ | IA | Claude y Copilot generan mensaje editable; timeout/cancelación; limpieza de salida | 0.5 d |
| M4 ✅ | Deploy | FTP y SFTP reales; mapeo de rutas; ignore/include/gitignore; concurrencia; mkdir; autoDelete; progreso y cancelar | 2 d |
| M5 | Pulido | Atajos, toasts, estados vacíos/error, icono, `mygo build` (.app + .dmg), README | 1 d |

Total aproximado: **5.5–6 días** de desarrollo.

### Pruebas

- `internal/git/status_test.go`: fixtures de porcelain v2 con rename, espacios,
  untracked, deleted.
- `internal/deploy/paths_test.go`: tabla de `baseDir`/`dir`/relativa → remota,
  incluyendo `..` y barras.
- `internal/deploy/filter_test.go`: ignore/include/gitignore/config.
- `internal/aicommits/clean_test.go`: fences, comillas, truncado.
- `internal/deploy/queue_test.go`: cliente falso; orden, concurrencia, errores,
  cancelación, mkdir cacheado.
- `internal/deploy/sftp_test.go` + `sftp_server_test.go`: known_hosts/TOFU y un
  servidor SSH+SFTP en proceso (subir, mkdir, borrar).
- `internal/deploy/candidates_test.go`: dedupe, borrados, renombres, symlinks.
- `deploy_ui_test.go`: candidatos → jobs, filtros y confirmación de borrados.
- Test de vista MyGo (sin ventana) para la tabla, el panel de commit y el modal
  de subida.
- Manual: target “My FTP” local y el SFTP real de `pro-deployer.json`
  (archivo inocuo) al final.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go tool mygo build`.

---

## 9. Riesgos

| Riesgo | Mitigación |
|---|---|
| MyGo es joven (v0.7.3); API puede cambiar | Fijar versión; validar `DiffView`, tablas y diálogos en M0 |
| `editor.DiffView` necesita buffers completos | Cargar contenido bajo demanda y limitar tamaño; fallback a texto |
| SFTP con password/agente/known_hosts | Implementar y probar contra el servidor real temprano (M4) |
| FTP con TLS/keepalive/concurrencia | Empezar secuencial, luego pool; probar contra “My FTP” |
| Copilot CLI exige permisos en modo `-p` | Ejecutarlo en temp dir y deshabilitar herramientas; si no es posible, dejar Claude como default |
| Globs de micromatch ≠ doublestar | Cubrir los casos del config; documentar diferencias |
| Diff enorme o binario | Truncar y avisar; no bloquear la UI |
| Hooks de Git cuelgan | Timeout + cancelar + mostrar salida |
| Credenciales en logs/IA | Redacción y prompts solo con diff |

---

## 10. Decisiones pendientes (con recomendación)

1. **Nombre**: `SideFtpGit` (display “Side FTP Git”) — recomendado, o “SideDeploy”.
2. **Qué se sube**: working tree completo (staged + unstaged + untracked), como
   pro-deployer — recomendado; alternativa: solo el último commit.
3. **IA por defecto**: Claude, con Copilot seleccionable — recomendado.
4. **Editar targets en la app**: no en v1; solo leer y abrir el JSON. Un editor
   de targets es una v2.
5. **Host key SFTP**: known_hosts + TOFU — recomendado; alternativa estricta
   (falla si el host no está).
6. **Idioma UI**: español — recomendado.

---

## 11. Orden de arranque sugerido

Empezar por **M0 + M1** en una sola pasada: scaffold MyGo, abrir repo, leer
config y mostrar la tabla de cambios con diff. Eso valida las dos piezas de
mayor riesgo (UI nativa y parsing de Git) antes de invertir en FTP/SFTP.
