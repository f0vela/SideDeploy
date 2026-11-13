// Package aicommits asks the Claude or Copilot command line for a commit
// message, in an empty temporary folder so it reads no file of the repo.
package aicommits

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Claude and Copilot are the command lines the app can ask.
const (
	Claude  = "claude"
	Copilot = "copilot"
)

// Tools are the command lines the app can ask, Claude first.
var Tools = []string{Claude, Copilot}

// Resolve returns the executable to run for tool: the path configured in
// Ajustes when there is one, or the one on PATH.
func Resolve(tool, custom string) (string, error) {
	custom = strings.TrimSpace(custom)
	if custom != "" {
		custom = expandHome(custom)
		info, err := os.Stat(custom)
		if err != nil {
			return "", fmt.Errorf("no encuentro «%s»", custom)
		}
		if info.IsDir() {
			return "", fmt.Errorf("«%s» es una carpeta, no un ejecutable", custom)
		}
		if info.Mode().Perm()&0o111 == 0 {
			return "", fmt.Errorf("«%s» no tiene permiso de ejecución", custom)
		}
		return custom, nil
	}
	bin, err := exec.LookPath(tool)
	if err != nil {
		return "", fmt.Errorf("no encuentro «%s» en el PATH", tool)
	}
	return bin, nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// Generate asks tool for a commit message describing diff, in the style of
// history, the recent commit subjects of the repo. bin is the executable
// configured in Ajustes, empty for the one on PATH. It gives up after 2
// minutes, or when ctx is canceled.
func Generate(ctx context.Context, tool, bin, diff, history string) (string, error) {
	if strings.TrimSpace(diff) == "" {
		return "", errors.New("no hay cambios que describir")
	}
	bin, err := Resolve(tool, bin)
	if err != nil {
		return "", err
	}
	args := []string{"-p", prompt(diff, history)}
	if tool == Copilot {
		// Only the answer, without its statistics.
		args = append(args, "-s")
	}

	dir, err := os.MkdirTemp("", "sidedeploy-ai-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "TERM=dumb")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s: %s", tool, msg)
	}
	msg := Clean(string(out))
	if msg == "" {
		return "", fmt.Errorf("%s no devolvió ningún mensaje", tool)
	}
	return msg, nil
}

func prompt(diff, history string) string {
	var b strings.Builder
	b.WriteString("Escribe el mensaje de commit para los cambios de abajo.\n")
	b.WriteString("- Sigue el idioma y el estilo de los mensajes recientes; si no hay, usa español.\n")
	b.WriteString("- Primera línea de 72 caracteres como máximo, en imperativo, sin punto final.\n")
	b.WriteString("- Si hace falta, añade una línea en blanco y un cuerpo breve en viñetas.\n")
	b.WriteString("- Responde solo con el mensaje: sin bloques de código, comillas ni explicación.\n")
	if history != "" {
		b.WriteString("\nMensajes recientes:\n")
		b.WriteString(history)
		b.WriteString("\n")
	}
	b.WriteString("\nCambios:\n")
	b.WriteString(diff)
	return b.String()
}

// Clean turns what a command line answered into a commit message: no code
// fence, no quotes around it, no "Mensaje:" prefix.
func Clean(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}
	s = unquote(s)
	for {
		lower := strings.ToLower(s)
		cut := 0
		for _, p := range []string{"commit message:", "mensaje del commit:", "mensaje de commit:", "mensaje:"} {
			if strings.HasPrefix(lower, p) {
				cut = len(p)
				break
			}
		}
		if cut == 0 {
			break
		}
		s = strings.TrimSpace(s[cut:])
	}
	return strings.TrimSpace(unquote(s))
}

func unquote(s string) string {
	for _, q := range []string{"\"", "'", "`"} {
		if len(s) >= 2 && strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			s = strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	return s
}
