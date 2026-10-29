package deploy

import (
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Filter decides which changed paths a deploy leaves out. It follows the
// pro-deployer options: ignore first, include as an allow list, and the
// paths Git ignores when the caller fills Ignored.
type Filter struct {
	Ignore  []string
	Include []string
	// Config lists paths that never go up, such as the config file itself.
	Config []string
	// Ignored holds the paths Git ignores, from git check-ignore.
	Ignored map[string]bool
}

// Skip reports whether rel stays out of the deploy.
func (f Filter) Skip(rel string) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	if rel == "" || rel == ".git" || strings.HasPrefix(rel, ".git/") {
		return true
	}
	for _, p := range f.Config {
		if rel == strings.TrimPrefix(filepath.ToSlash(p), "./") {
			return true
		}
	}
	for _, p := range f.Ignore {
		if matchPattern(p, rel) {
			return true
		}
	}
	if len(f.Include) > 0 {
		included := false
		for _, p := range f.Include {
			if matchPattern(p, rel) {
				included = true
				break
			}
		}
		if !included {
			return true
		}
	}
	return f.Ignored[rel]
}

// matchPattern follows micromatch closely enough for the pro-deployer
// options: a path matches a pattern, and everything below a plain folder
// pattern matches it too.
func matchPattern(pattern, rel string) bool {
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")
	if pattern == "" {
		return false
	}
	if ok, err := doublestar.Match(pattern, rel); err == nil && ok {
		return true
	}
	if !strings.ContainsAny(pattern, "*?[") {
		ok, err := doublestar.Match(strings.TrimSuffix(pattern, "/")+"/**", rel)
		return err == nil && ok
	}
	return false
}
