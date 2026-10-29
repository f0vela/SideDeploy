package gitrepo

import (
	"bytes"
	"strconv"
	"strings"
)

// Status is the branch and the changes of a work tree.
type Status struct {
	Branch   string
	Detached bool
	Upstream string
	Ahead    int
	Behind   int
	Files    []File
}

// File is one path Git reports as changed. Index is its status in the index
// (the X of porcelain), Worktree its status on disk (the Y).
type File struct {
	Path     string
	OrigPath string
	Index    byte
	Worktree byte
	Unmerged bool
}

// Staged reports whether the change is in the index.
func (f File) Staged() bool {
	return !f.Unmerged && f.Index != 0 && f.Index != '.' && f.Index != '?'
}

// Unstaged reports whether the change is only in the work tree, or conflicts.
func (f File) Unstaged() bool {
	if f.Unmerged {
		return true
	}
	return f.Worktree != 0 && f.Worktree != '.'
}

// Untracked reports whether Git does not track the file at all.
func (f File) Untracked() bool {
	return f.Index == '?'
}

// Diff is the two sides of a change. Note explains why they are not shown.
type Diff struct {
	Old, New string
	Binary   bool
	TooLarge bool
	Note     string
}

// parseStatus parses git status --porcelain=v2 -z --branch.
func parseStatus(data []byte) Status {
	var st Status
	tokens := bytes.Split(data, []byte{0})
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if len(t) == 0 {
			continue
		}
		switch t[0] {
		case '#':
			parseBranch(&st, string(t[2:]))
		case '1':
			fields := strings.SplitN(string(t), " ", 9)
			if len(fields) != 9 {
				continue
			}
			st.Files = append(st.Files, fileFromXY(fields[1], fields[8], ""))
		case '2':
			fields := strings.SplitN(string(t), " ", 10)
			if len(fields) != 10 || i+1 >= len(tokens) {
				continue
			}
			orig := string(tokens[i+1])
			i++
			st.Files = append(st.Files, fileFromXY(fields[1], fields[9], orig))
		case 'u':
			fields := strings.SplitN(string(t), " ", 11)
			if len(fields) != 11 {
				continue
			}
			f := fileFromXY(fields[1], fields[10], "")
			f.Unmerged = true
			st.Files = append(st.Files, f)
		case '?':
			st.Files = append(st.Files, File{Path: string(t[2:]), Index: '?', Worktree: '?'})
		}
	}
	return st
}

func fileFromXY(xy, path, orig string) File {
	f := File{Path: path, OrigPath: orig}
	if len(xy) > 0 {
		f.Index = xy[0]
	}
	if len(xy) > 1 {
		f.Worktree = xy[1]
	}
	return f
}

func parseBranch(st *Status, line string) {
	fields := strings.SplitN(line, " ", 2)
	if len(fields) != 2 {
		return
	}
	value := fields[1]
	switch fields[0] {
	case "branch.head":
		if value == "(detached)" {
			st.Detached = true
			return
		}
		st.Branch = value
	case "branch.upstream":
		st.Upstream = value
	case "branch.ab":
		for _, part := range strings.Fields(value) {
			if len(part) < 2 {
				continue
			}
			n, err := strconv.Atoi(part[1:])
			if err != nil {
				continue
			}
			switch part[0] {
			case '+':
				st.Ahead = n
			case '-':
				st.Behind = n
			}
		}
	}
}
