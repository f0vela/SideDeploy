package gitrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseStatus(t *testing.T) {
	data := strings.Join([]string{
		"# branch.oid 1d54b9dacffe34fbfbc3fc6721bd81652f938ee0",
		"# branch.head main",
		"# branch.upstream origin/main",
		"# branch.ab +2 -1",
		"1 .D N... 100644 100644 000000 f2ad6c7f0115a6ba5b00456a849810e7ec0af2 f2ad6c7f0115a6ba5b00456a849810e7ec0af2 del.txt",
		"2 R. N... 100644 100644 100644 4bcfe98e640c828451131266 4bcfe98e640c828451131266 R100 dir/new.txt",
		"dir/old.txt",
		"1 M. N... 100644 100644 100644 78981922613b2afb6025042ff6bd878ac1994e85 c1827f07e114c20547dc6a7296588870a4b5b62c one.txt",
		"1 .M N... 100644 100644 100644 61780798228d17af2d34fce4cfbdf35556832472 61780798228d17af2d34fce4cfbdf35556832472 two words.txt",
		"1 MM N... 100644 100644 100644 78981922613b2afb6025042ff6bd878ac1994e85 78981922613b2afb6025042ff6bd878ac1994e85 both.txt",
		"? new.txt",
	}, "\x00") + "\x00"

	st := parseStatus([]byte(data))
	if st.Branch != "main" || st.Detached {
		t.Fatalf("branch = %q detached=%v", st.Branch, st.Detached)
	}
	if st.Upstream != "origin/main" || st.Ahead != 2 || st.Behind != 1 {
		t.Fatalf("upstream = %q ahead=%d behind=%d", st.Upstream, st.Ahead, st.Behind)
	}
	if len(st.Files) != 6 {
		t.Fatalf("files = %d, want 6: %+v", len(st.Files), st.Files)
	}

	byPath := make(map[string]File, len(st.Files))
	for _, f := range st.Files {
		byPath[f.Path] = f
	}

	if f := byPath["del.txt"]; f.Staged() || !f.Unstaged() || f.Worktree != 'D' {
		t.Errorf("del.txt = %+v", f)
	}
	if f := byPath["dir/new.txt"]; !f.Staged() || f.OrigPath != "dir/old.txt" || f.Index != 'R' {
		t.Errorf("rename = %+v", f)
	}
	if f := byPath["one.txt"]; !f.Staged() || f.Unstaged() {
		t.Errorf("one.txt = %+v", f)
	}
	if f := byPath["two words.txt"]; f.Staged() || !f.Unstaged() || f.Worktree != 'M' {
		t.Errorf("two words.txt = %+v", f)
	}
	if f := byPath["both.txt"]; !f.Staged() || !f.Unstaged() {
		t.Errorf("both.txt = %+v", f)
	}
	if f := byPath["new.txt"]; !f.Untracked() || f.Staged() || !f.Unstaged() {
		t.Errorf("new.txt = %+v", f)
	}
}

func TestRepoStatusAndDiff(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no está instalado")
	}
	root := t.TempDir()
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "Test")
	write(t, root, "one.txt", "a\n")
	write(t, root, "two words.txt", "b\n")
	write(t, root, "del.txt", "c\n")
	write(t, root, "dir/old.txt", "d\n")
	git(t, root, "add", "-A")
	git(t, root, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "init")

	write(t, root, "one.txt", "a2\n")
	git(t, root, "add", "one.txt")
	write(t, root, "two words.txt", "b2\n")
	write(t, root, "new.txt", "e\n")
	git(t, root, "mv", "dir/old.txt", "dir/new.txt")
	if err := os.Remove(filepath.Join(root, "del.txt")); err != nil {
		t.Fatal(err)
	}

	repo, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Root != resolved {
		t.Fatalf("root = %q, want %q", repo.Root, resolved)
	}
	st, err := repo.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch == "" {
		t.Fatal("empty branch")
	}

	byPath := make(map[string]File)
	for _, f := range st.Files {
		byPath[f.Path] = f
	}
	if f, ok := byPath["one.txt"]; !ok || !f.Staged() {
		t.Errorf("one.txt not staged: %+v", byPath)
	}
	if f, ok := byPath["new.txt"]; !ok || !f.Untracked() {
		t.Errorf("new.txt not untracked: %+v", byPath)
	}
	if f, ok := byPath["del.txt"]; !ok || f.Worktree != 'D' {
		t.Errorf("del.txt not deleted: %+v", byPath)
	}
	if f, ok := byPath["dir/new.txt"]; !ok || f.OrigPath != "dir/old.txt" {
		t.Errorf("rename missing: %+v", byPath)
	}

	d, err := repo.Diff(t.Context(), byPath["one.txt"], true)
	if err != nil {
		t.Fatal(err)
	}
	if d.Old != "a\n" || d.New != "a2\n" {
		t.Errorf("staged diff = %q -> %q", d.Old, d.New)
	}
	d, err = repo.Diff(t.Context(), byPath["two words.txt"], false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Old != "b\n" || d.New != "b2\n" {
		t.Errorf("unstaged diff = %q -> %q", d.Old, d.New)
	}
	d, err = repo.Diff(t.Context(), byPath["new.txt"], false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Old != "" || d.New != "e\n" {
		t.Errorf("untracked diff = %q -> %q", d.Old, d.New)
	}
	d, err = repo.Diff(t.Context(), byPath["dir/new.txt"], true)
	if err != nil {
		t.Fatal(err)
	}
	if d.Old != "d\n" || d.New != "d\n" || d.Note == "" {
		t.Errorf("rename diff = %+v", d)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, root, path, text string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "Test")
	return root
}

func TestStageUnstageAndCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no está instalado")
	}
	root := newRepo(t)
	write(t, root, "one.txt", "a\n")
	write(t, root, "two.txt", "b\n")
	repo, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	if err := repo.StageAll(ctx); err != nil {
		t.Fatal(err)
	}
	st := status(t, repo)
	if len(st.Files) != 2 || !st.Files[0].Staged() {
		t.Fatalf("staged = %+v", st.Files)
	}

	// A repo without commits unstages back to untracked.
	if err := repo.Unstage(ctx, "one.txt"); err != nil {
		t.Fatal(err)
	}
	st = status(t, repo)
	if f := byPath(st, "one.txt"); f.Untracked() != true || f.Staged() {
		t.Fatalf("one.txt = %+v", f)
	}

	if err := repo.Commit(ctx, "primer commit"); err != nil {
		t.Fatal(err)
	}
	st = status(t, repo)
	if len(st.Files) != 1 || !st.Files[0].Untracked() {
		t.Fatalf("after commit = %+v", st.Files)
	}

	// Stage a change and a new file, then take them out again.
	write(t, root, "two.txt", "b2\n")
	if err := repo.Stage(ctx, "one.txt", "two.txt"); err != nil {
		t.Fatal(err)
	}
	st = status(t, repo)
	if f := byPath(st, "one.txt"); !f.Staged() || f.Worktree != '.' {
		t.Fatalf("one.txt = %+v", f)
	}
	if err := repo.UnstageAll(ctx); err != nil {
		t.Fatal(err)
	}
	st = status(t, repo)
	if f := byPath(st, "one.txt"); f.Staged() {
		t.Fatalf("one.txt still staged: %+v", f)
	}

	// The message may not be empty.
	if err := repo.Commit(ctx, "   "); err == nil {
		t.Fatal("committed an empty message")
	}
}

func TestCommitRunsHooks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no está instalado")
	}
	root := newRepo(t)
	write(t, root, "one.txt", "a\n")
	repo, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if err := repo.StageAll(ctx); err != nil {
		t.Fatal(err)
	}

	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'lint: roto'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	err = repo.Commit(ctx, "no debería pasar")
	if err == nil {
		t.Fatal("the hook did not block the commit")
	}
	if !strings.Contains(err.Error(), "lint: roto") {
		t.Fatalf("error = %v, want the hook output", err)
	}
	if st := status(t, repo); len(st.Files) != 1 || !st.Files[0].Staged() {
		t.Fatalf("the index was lost: %+v", st.Files)
	}
}

func status(t *testing.T, repo *Repo) Status {
	t.Helper()
	st, err := repo.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func byPath(st Status, path string) File {
	for _, f := range st.Files {
		if f.Path == path {
			return f
		}
	}
	return File{}
}

func TestChangeText(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no está instalado")
	}
	root := newRepo(t)
	repo, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	write(t, root, "app.txt", "uno\n")
	write(t, root, "suelto.txt", "x\n")

	// With nothing staged, every change shows, untracked files included.
	text, err := repo.ChangeText(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"+uno", "+x", "+++ b/suelto.txt"} {
		if !strings.Contains(text, want) {
			t.Errorf("ChangeText lacks %q:\n%s", want, text)
		}
	}

	// The staged side wins when there is one, so the message matches the
	// commit even if the work tree kept moving.
	if err := repo.StageAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "primer commit"); err != nil {
		t.Fatal(err)
	}
	write(t, root, "app.txt", "dos\n")
	if err := repo.Stage(ctx, "app.txt"); err != nil {
		t.Fatal(err)
	}
	write(t, root, "app.txt", "tres\n")
	write(t, root, "otro.txt", "y\n")
	text, err = repo.ChangeText(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "-uno") || !strings.Contains(text, "+dos") {
		t.Errorf("staged diff missing:\n%s", text)
	}
	if strings.Contains(text, "+tres") || strings.Contains(text, "+y") {
		t.Errorf("ChangeText read past the index:\n%s", text)
	}

	// Without anything staged, the work tree and the untracked files show.
	if err := repo.UnstageAll(ctx); err != nil {
		t.Fatal(err)
	}
	text, err = repo.ChangeText(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-uno", "+tres", "+y"} {
		if !strings.Contains(text, want) {
			t.Errorf("ChangeText lacks %q:\n%s", want, text)
		}
	}

	got := repo.RecentMessages(ctx, 5)
	if len(got) != 1 || got[0] != "primer commit" {
		t.Fatalf("recent messages = %q", got)
	}
}

func TestChangeTextCaps(t *testing.T) {
	root := newRepo(t)
	repo, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "grande.txt", strings.Repeat("línea de relleno\n", 5000))
	text, err := repo.ChangeText(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(text, "… (diff truncado)\n") {
		t.Fatalf("a big change was not capped, ends with %q", text[max(0, len(text)-60):])
	}
	if len(text) > maxChangeText+40 {
		t.Fatalf("capped text = %d bytes", len(text))
	}
	if !utf8.ValidString(text) {
		t.Fatal("the cap broke a UTF-8 rune")
	}
}
