// Package gitrepo reads Git state through the git command line.
package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Repo is a Git work tree.
type Repo struct {
	Root string
}

// Open finds the top-level work tree containing path.
func Open(path string) (*Repo, error) {
	if path == "" {
		return nil, errors.New("no se eligió ninguna carpeta")
	}
	out, err := runGit(context.Background(), path, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("no es un repositorio Git: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return nil, errors.New("no es un repositorio Git")
	}
	return &Repo{Root: root}, nil
}

// Status reads the branch and every change in the work tree.
func (r *Repo) Status(ctx context.Context) (Status, error) {
	out, err := r.run(ctx, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all")
	if err != nil {
		return Status{}, err
	}
	return parseStatus(out), nil
}

// Diff reads the old and new text of one change, for the diff view.
func (r *Repo) Diff(ctx context.Context, f File, staged bool) (Diff, error) {
	if f.Unmerged {
		return Diff{Note: "Conflicto sin resolver"}, nil
	}

	var d Diff
	if staged {
		oldPath := f.Path
		if f.OrigPath != "" {
			oldPath = f.OrigPath
		}
		old, found, tooLarge, err := r.blob(ctx, "HEAD:"+oldPath)
		if err != nil {
			return d, err
		}
		if tooLarge {
			return Diff{TooLarge: true}, nil
		}
		if found {
			d.Old = old
		}
		if f.Index != 'D' {
			newText, found, tooLarge, err := r.blob(ctx, ":"+f.Path)
			if err != nil {
				return d, err
			}
			if tooLarge {
				return Diff{TooLarge: true}, nil
			}
			if found {
				d.New = newText
			}
		}
	} else {
		if !f.Untracked() {
			old, found, tooLarge, err := r.blob(ctx, ":"+f.Path)
			if err != nil {
				return d, err
			}
			if tooLarge {
				return Diff{TooLarge: true}, nil
			}
			if found {
				d.Old = old
			}
		}
		if f.Worktree != 'D' {
			newText, found, tooLarge, err := workingFile(r.Root, f.Path)
			if err != nil {
				return d, err
			}
			if tooLarge {
				return Diff{TooLarge: true}, nil
			}
			if found {
				d.New = newText
			}
		}
	}

	if isBinary(d.Old) || isBinary(d.New) {
		return Diff{Binary: true}, nil
	}
	if d.Old == d.New {
		d.Note = "Sin cambios de contenido"
	}
	return d, nil
}

// maxDiffBytes caps the text a diff holds. Larger files show a notice.
const maxDiffBytes = 2 << 20

// blob reads a blob by its revision expression, such as HEAD:path or :path.
func (r *Repo) blob(ctx context.Context, rev string) (text string, found bool, tooLarge bool, err error) {
	sizeOut, err := r.run(ctx, "cat-file", "-s", rev)
	if err != nil {
		// The object does not exist: a new or deleted file has no side.
		return "", false, false, nil
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(sizeOut)), 10, 64)
	if err != nil {
		return "", false, false, nil
	}
	if size > maxDiffBytes {
		return "", true, true, nil
	}
	out, err := r.run(ctx, "show", rev)
	if err != nil {
		return "", false, false, nil
	}
	return string(out), true, false, nil
}

func workingFile(root, path string) (text string, found bool, tooLarge bool, err error) {
	full := filepath.Join(root, filepath.FromSlash(path))
	info, err := os.Stat(full)
	if err != nil {
		return "", false, false, nil
	}
	if info.IsDir() {
		return "", false, false, nil
	}
	if info.Size() > maxDiffBytes {
		return "", true, true, nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", false, false, err
	}
	return string(data), true, false, nil
}

func isBinary(s string) bool {
	if n := 8000; len(s) > n {
		s = s[:n]
	}
	return strings.IndexByte(s, 0) >= 0
}

// maxChangeText caps what ChangeText returns, in bytes.
const maxChangeText = 30 << 10

// ChangeText returns what a commit message should describe: the staged diff
// when there is one, else every change plus the untracked files, capped.
func (r *Repo) ChangeText(ctx context.Context) (string, error) {
	staged, err := r.run(ctx, "diff", "--cached", "--no-color", "--no-ext-diff")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(staged)) != "" {
		return clampText(string(staged)), nil
	}

	out, err := r.run(ctx, "diff", "--no-color", "--no-ext-diff")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.Write(out)

	st, err := r.Status(ctx)
	if err != nil {
		return "", err
	}
	for _, f := range st.Files {
		if !f.Untracked() {
			continue
		}
		text, found, tooLarge, err := workingFile(r.Root, f.Path)
		if err != nil || !found || tooLarge || isBinary(text) {
			continue
		}
		b.WriteString(untrackedDiff(f.Path, text))
	}
	return clampText(b.String()), nil
}

// RecentMessages returns the subjects of the last n commits, newest first, so
// the AI keeps their language and style.
func (r *Repo) RecentMessages(ctx context.Context, n int) []string {
	out, err := r.run(ctx, "log", "--no-merges", "-n", strconv.Itoa(n), "--pretty=format:%s")
	if err != nil {
		return nil
	}
	var list []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			list = append(list, line)
		}
	}
	return list
}

func untrackedDiff(path, text string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n--- /dev/null\n+++ b/%s\n", path)
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		b.WriteString("+" + line + "\n")
	}
	return b.String()
}

func clampText(s string) string {
	if len(s) <= maxChangeText {
		return s
	}
	cut := s[:maxChangeText]
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		cut = cut[:i]
	}
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "\n… (diff truncado)\n"
}

// Ignored reports which of the paths Git ignores, for checkGitignore.
func (r *Repo) Ignored(ctx context.Context, paths []string) (map[string]bool, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", r.Root, "check-ignore", "--stdin", "-z")
	cmd.Env = append(os.Environ(), "GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0")
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := cmd.Output()
	if err != nil {
		// Exit 1 means none of them is ignored.
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	ignored := make(map[string]bool)
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			ignored[p] = true
		}
	}
	return ignored, nil
}

// Stage adds the paths to the index, deletions included.
func (r *Repo) Stage(ctx context.Context, paths ...string) error {
	_, err := r.run(ctx, append([]string{"add", "-A", "--"}, paths...)...)
	return err
}

// StageAll adds every change of the work tree to the index.
func (r *Repo) StageAll(ctx context.Context) error {
	_, err := r.run(ctx, "add", "-A")
	return err
}

// Unstage takes the paths out of the index, keeping the work tree as it is.
func (r *Repo) Unstage(ctx context.Context, paths ...string) error {
	if r.hasCommits(ctx) {
		_, err := r.run(ctx, append([]string{"reset", "-q", "HEAD", "--"}, paths...)...)
		return err
	}
	_, err := r.run(ctx, append([]string{"rm", "--cached", "-r", "-q", "--"}, paths...)...)
	return err
}

// UnstageAll takes every change out of the index.
func (r *Repo) UnstageAll(ctx context.Context) error {
	if r.hasCommits(ctx) {
		_, err := r.run(ctx, "reset", "-q", "HEAD")
		return err
	}
	_, err := r.run(ctx, "rm", "--cached", "-r", "-q", ".")
	return err
}

// Commit records the index under message, running the hooks the repo sets.
func (r *Repo) Commit(ctx context.Context, message string) error {
	if strings.TrimSpace(message) == "" {
		return errors.New("el mensaje del commit está vacío")
	}
	_, err := r.run(ctx, "commit", "-m", message)
	return err
}

// Remotes lists the names of the configured remotes.
func (r *Repo) Remotes(ctx context.Context) []string {
	out, err := r.run(ctx, "remote")
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

// Push sends the current branch to its upstream. A branch without one goes
// to the first remote (origin when there is one) and starts tracking it.
// Git never asks for a password here: it needs a credential helper or an
// SSH agent, and says so when it lacks them.
func (r *Repo) Push(ctx context.Context, upstream string) error {
	args := []string{"push"}
	if upstream == "" {
		remotes := r.Remotes(ctx)
		if len(remotes) == 0 {
			return errors.New("el repo no tiene ningún remoto configurado")
		}
		remote := remotes[0]
		for _, name := range remotes {
			if name == "origin" {
				remote = name
			}
		}
		args = append(args, "-u", remote, "HEAD")
	}
	_, err := runGitTimeout(ctx, 2*time.Minute, r.Root, args...)
	return err
}

func (r *Repo) hasCommits(ctx context.Context) bool {
	_, err := r.run(ctx, "rev-parse", "--verify", "HEAD")
	return err == nil
}

func (r *Repo) run(ctx context.Context, args ...string) ([]byte, error) {
	return runGit(ctx, r.Root, args...)
}

func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return runGitTimeout(ctx, 30*time.Second, dir, args...)
}

func runGitTimeout(ctx context.Context, d time.Duration, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, errors.New(msg)
	}
	return out, nil
}
