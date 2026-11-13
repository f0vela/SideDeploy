package main

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"sidedeploy/internal/aicommits"
	"sidedeploy/internal/deploy"
	"sidedeploy/internal/gitrepo"
	"sidedeploy/internal/textdiff"
)

// Messages the background work sends back to Update.
type (
	openedMsg struct {
		path    string
		repo    *gitrepo.Repo
		cfg     *deploy.Config
		cfgPath string
		cfgErr  error
		err     error
	}
	statusMsg struct {
		repo *gitrepo.Repo
		st   gitrepo.Status
		err  error
	}
	diffMsg struct {
		repo   *gitrepo.Repo
		seq    int
		title  string
		staged bool
		lines  []textdiff.Line
		note   string
		err    error
	}
	gitDoneMsg struct {
		repo *gitrepo.Repo
		err  error
	}
	commitDoneMsg struct {
		repo *gitrepo.Repo
		err  error
	}
	pushDoneMsg struct {
		repo *gitrepo.Repo
		err  error
	}
	aiDoneMsg struct {
		repo *gitrepo.Repo
		text string
		err  error
	}
	deployResultMsg struct{ r deploy.Result }
	deployDoneMsg   struct{}
	flashClearMsg   struct{ seq int }
)

func openCmd(path string) tea.Cmd {
	return func() tea.Msg {
		repo, err := gitrepo.Open(path)
		if err != nil {
			return openedMsg{path: path, err: err}
		}
		cfg, cfgPath, cfgErr := deploy.Load(repo.Root)
		return openedMsg{path: path, repo: repo, cfg: cfg, cfgPath: cfgPath, cfgErr: cfgErr}
	}
}

func statusCmd(repo *gitrepo.Repo) tea.Cmd {
	return func() tea.Msg {
		st, err := repo.Status(context.Background())
		return statusMsg{repo: repo, st: st, err: err}
	}
}

func diffCmd(repo *gitrepo.Repo, r gitrepo.Row, seq int) tea.Cmd {
	return func() tea.Msg {
		msg := diffMsg{repo: repo, seq: seq, title: r.File.Path, staged: r.Staged}
		if r.Staged && r.File.OrigPath != "" {
			msg.title = r.File.OrigPath + " → " + r.File.Path
		}
		d, err := repo.Diff(context.Background(), r.File, r.Staged)
		if err != nil {
			msg.err = err
			return msg
		}
		switch {
		case d.TooLarge:
			msg.note = "Archivo demasiado grande para el diff (más de 2 MB)."
		case d.Binary:
			msg.note = "Archivo binario."
		case d.Note != "":
			msg.note = d.Note
		default:
			msg.lines = textdiff.Compare(d.Old, d.New, 3)
			if len(msg.lines) == 0 {
				msg.note = "Sin cambios de contenido"
			}
		}
		return msg
	}
}

// gitCmd runs one write on the index at a time, then reports.
func (m *model) gitCmd(action func(*gitrepo.Repo, context.Context) error) tea.Cmd {
	repo := m.repo
	mu := m.gitMu
	if repo == nil {
		return nil
	}
	return func() tea.Msg {
		mu.Lock()
		defer mu.Unlock()
		return gitDoneMsg{repo: repo, err: action(repo, context.Background())}
	}
}

func (m *model) commitCmd(message string) tea.Cmd {
	repo, mu := m.repo, m.gitMu
	return func() tea.Msg {
		mu.Lock()
		defer mu.Unlock()
		return commitDoneMsg{repo: repo, err: repo.Commit(context.Background(), message)}
	}
}

func (m *model) pushCmd() tea.Cmd {
	repo, mu, upstream := m.repo, m.gitMu, m.status.Upstream
	return func() tea.Msg {
		mu.Lock()
		defer mu.Unlock()
		return pushDoneMsg{repo: repo, err: repo.Push(context.Background(), upstream)}
	}
}

// aiCmd asks the AI for a message; cancel stops it from the outside.
func (m *model) aiCmd(ctx context.Context) tea.Cmd {
	repo, tool, bin := m.repo, m.aiTool(), m.aiPath(m.aiTool())
	return func() tea.Msg {
		diff, err := repo.ChangeText(ctx)
		history := ""
		if err == nil {
			history = strings.Join(repo.RecentMessages(ctx, 10), "\n")
		}
		var text string
		if err == nil {
			text, err = aicommits.Generate(ctx, tool, bin, diff, history)
		}
		if err != nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		return aiDoneMsg{repo: repo, text: text, err: err}
	}
}

func isCanceled(err error) bool { return errors.Is(err, context.Canceled) }
func isDeadline(err error) bool { return errors.Is(err, context.DeadlineExceeded) }
func clearFlash(seq int) tea.Cmd {
	return tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg { return flashClearMsg{seq} })
}
func waitFor(ch chan tea.Msg) tea.Cmd { return func() tea.Msg { return <-ch } }
