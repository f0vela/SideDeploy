// Package plan decides what a deploy sends: the changes Git reports, the
// files a target filters out and the remote path of each. Both apps, the
// window and the terminal one, share it.
package plan

import (
	"context"
	"fmt"
	"path/filepath"

	"sidedeploy/internal/deploy"
	"sidedeploy/internal/gitrepo"
)

// Changes turns the work tree status into the paths a deploy looks at,
// without repeating a file Git lists twice.
func Changes(st gitrepo.Status) []deploy.Change {
	var out []deploy.Change
	for _, f := range st.Files {
		if !f.Staged() && !f.Unstaged() {
			continue
		}
		out = append(out, deploy.Change{Path: f.Path, OrigPath: f.OrigPath})
	}
	return out
}

// JobsFor maps candidates to jobs, leaving out what the filter hides. It
// returns the paths it could not map, to show instead of failing the run.
func JobsFor(t deploy.Target, cands []deploy.Candidate, f deploy.Filter) (jobs []deploy.Job, skipped int, notes []string) {
	for _, cand := range cands {
		if f.Skip(cand.Rel) {
			skipped++
			continue
		}
		remote, err := deploy.RemotePath(t, cand.Rel)
		if err != nil {
			notes = append(notes, err.Error())
			continue
		}
		jobs = append(jobs, deploy.Job{Rel: cand.Rel, Local: cand.Local, Remote: remote, Delete: cand.Delete})
	}
	return jobs, skipped, notes
}

// Filter follows pro-deployer's options: the app's own config never goes up,
// then ignore, include and, optionally, what Git ignores.
func Filter(repo *gitrepo.Repo, cfg *deploy.Config, cfgPath string, rels []string) (deploy.Filter, error) {
	f := deploy.Filter{Config: []string{"pro-deployer.json", ".vscode/pro-deployer.json"}}
	if repo != nil && cfgPath != "" {
		if rel, err := filepath.Rel(repo.Root, cfgPath); err == nil {
			f.Config = append(f.Config, filepath.ToSlash(rel))
		}
	}
	if cfg == nil {
		return f, nil
	}
	f.Ignore = cfg.Ignore
	f.Include = cfg.Include
	if !cfg.CheckGitignore {
		return f, nil
	}
	ignored, err := repo.Ignored(context.Background(), rels)
	if err != nil {
		return f, fmt.Errorf("git check-ignore: %w", err)
	}
	f.Ignored = ignored
	return f, nil
}

// Workers is the number of connections a deploy opens.
func Workers(cfg *deploy.Config) int {
	if cfg != nil && cfg.Concurrency > 0 {
		return cfg.Concurrency
	}
	return 5
}

// Plan is what one deploy would do.
type Plan struct {
	Jobs    []deploy.Job
	Uploads int
	Deletes int
	Skipped int
	Notes   []string
}

// Build lists the jobs for target given the repo status. autoDelete removes
// from the server what is gone from the repo.
func Build(repo *gitrepo.Repo, cfg *deploy.Config, cfgPath string, st gitrepo.Status, target *deploy.Target, autoDelete bool) Plan {
	var p Plan
	if repo == nil || target == nil {
		return p
	}
	cands := deploy.Candidates(repo.Root, Changes(st), autoDelete)
	rels := make([]string, len(cands))
	for i, cand := range cands {
		rels[i] = cand.Rel
	}
	filter, err := Filter(repo, cfg, cfgPath, rels)
	if err != nil {
		p.Notes = append(p.Notes, err.Error())
	}
	jobs, skipped, notes := JobsFor(*target, cands, filter)
	p.Jobs, p.Skipped = jobs, skipped
	p.Notes = append(p.Notes, notes...)
	for _, job := range jobs {
		if job.Delete {
			p.Deletes++
		} else {
			p.Uploads++
		}
	}
	return p
}

// Summary describes a plan in one line, as "3 para subir · 1 para borrar".
func (p Plan) Summary() string {
	var parts []string
	if p.Uploads > 0 {
		parts = append(parts, fmt.Sprintf("%d para subir", p.Uploads))
	}
	if p.Deletes > 0 {
		parts = append(parts, fmt.Sprintf("%d para borrar", p.Deletes))
	}
	if p.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d filtrados", p.Skipped))
	}
	if len(parts) == 0 {
		return "Sin archivos que subir."
	}
	out := parts[0]
	for _, s := range parts[1:] {
		out += " · " + s
	}
	return out
}

// Outcome describes how a finished run went, in the words both apps show.
func Outcome(target string, done, total, failed int, cancelled bool) string {
	switch {
	case cancelled:
		return fmt.Sprintf("Cancelado: %d de %d archivos.", done, total)
	case failed > 0:
		return fmt.Sprintf("%d de %d archivos con error.", failed, total)
	default:
		return fmt.Sprintf("Listo: %d archivos en %s.", done, target)
	}
}
