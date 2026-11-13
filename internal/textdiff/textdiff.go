// Package textdiff compares two texts line by line, for terminals that have
// no diff widget of their own.
package textdiff

import (
	"fmt"
	"strings"
)

// Kind is what a line did between the two texts.
type Kind int

const (
	Same Kind = iota
	Add
	Del
	Hunk // a "@@ -a,b +c,d @@" header
	Gap  // a note such as "… 120 líneas sin cambios"
)

// Line is one line of a diff, with its numbers in the old and new texts, zero
// when it has none there.
type Line struct {
	Kind     Kind
	Text     string
	Old, New int
}

// maxCells caps the table of the comparison: past it the changed middle is
// shown whole, as one deletion and one addition.
const maxCells = 4_000_000

// Compare returns the lines of the diff of old and new, each change with
// context lines around it and the long unchanged runs folded.
func Compare(old, new string, context int) []Line {
	a, b := split(old), split(new)

	// Trim what both texts share at either end: most diffs are small.
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	midA, midB := a[pre:len(a)-suf], b[pre:len(b)-suf]

	var all []Line
	for i := 0; i < pre; i++ {
		all = append(all, Line{Kind: Same, Text: a[i], Old: i + 1, New: i + 1})
	}
	oi, ni := pre+1, pre+1
	for _, op := range diffMiddle(midA, midB) {
		switch op.kind {
		case Same:
			all = append(all, Line{Kind: Same, Text: op.text, Old: oi, New: ni})
			oi++
			ni++
		case Del:
			all = append(all, Line{Kind: Del, Text: op.text, Old: oi})
			oi++
		case Add:
			all = append(all, Line{Kind: Add, Text: op.text, New: ni})
			ni++
		}
	}
	for i := 0; i < suf; i++ {
		all = append(all, Line{Kind: Same, Text: a[len(a)-suf+i], Old: oi + i, New: ni + i})
	}
	return fold(all, context)
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

type op struct {
	kind Kind
	text string
}

// diffMiddle is the longest common subsequence of a and b as operations.
func diffMiddle(a, b []string) []op {
	if len(a) == 0 || len(b) == 0 || len(a)*len(b) > maxCells {
		out := make([]op, 0, len(a)+len(b))
		for _, s := range a {
			out = append(out, op{Del, s})
		}
		for _, s := range b {
			out = append(out, op{Add, s})
		}
		return out
	}
	n, m := len(a), len(b)
	// lcs[i][j] is the length of the common subsequence of a[i:] and b[j:].
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	out := make([]op, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, op{Same, a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, op{Del, a[i]})
			i++
		default:
			out = append(out, op{Add, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, op{Del, a[i]})
	}
	for ; j < m; j++ {
		out = append(out, op{Add, b[j]})
	}
	return out
}

// fold keeps context lines around each change and replaces the rest with a
// hunk header, as git does.
func fold(all []Line, context int) []Line {
	keep := make([]bool, len(all))
	changed := false
	for i, l := range all {
		if l.Kind == Same {
			continue
		}
		changed = true
		for k := max(0, i-context); k <= min(len(all)-1, i+context); k++ {
			keep[k] = true
		}
	}
	if !changed {
		return nil
	}
	var out []Line
	for i := 0; i < len(all); {
		if !keep[i] {
			i++
			continue
		}
		j := i
		for j < len(all) && keep[j] {
			j++
		}
		oldStart, newStart := firstNumber(all[i:j])
		oldN, newN := 0, 0
		for _, l := range all[i:j] {
			if l.Kind != Add {
				oldN++
			}
			if l.Kind != Del {
				newN++
			}
		}
		out = append(out, Line{Kind: Hunk, Text: fmt.Sprintf("@@ -%d,%d +%d,%d @@", oldStart, oldN, newStart, newN)})
		out = append(out, all[i:j]...)
		i = j
	}
	return out
}

func firstNumber(ls []Line) (oldStart, newStart int) {
	for _, l := range ls {
		if oldStart == 0 && l.Old > 0 {
			oldStart = l.Old
		}
		if newStart == 0 && l.New > 0 {
			newStart = l.New
		}
	}
	if oldStart == 0 {
		oldStart = 1
	}
	if newStart == 0 {
		newStart = 1
	}
	return oldStart, newStart
}
