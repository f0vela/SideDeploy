package gitrepo

import "fmt"

// Row is one line of a list of changes: a section header or a file. A file
// that is staged and changed again shows in both sections.
type Row struct {
	Header bool
	Title  string
	File   File
	Staged bool
}

// BuildRows groups the files of a status under STAGED and CAMBIOS headers.
func BuildRows(st Status) []Row {
	var staged, changed []Row
	for _, f := range st.Files {
		if f.Staged() {
			staged = append(staged, Row{File: f, Staged: true})
		}
		if f.Unstaged() {
			changed = append(changed, Row{File: f})
		}
	}
	rows := make([]Row, 0, len(staged)+len(changed)+2)
	if len(staged) > 0 {
		rows = append(rows, Row{Header: true, Title: fmt.Sprintf("STAGED · %d", len(staged))})
		rows = append(rows, staged...)
	}
	if len(changed) > 0 {
		rows = append(rows, Row{Header: true, Title: fmt.Sprintf("CAMBIOS · %d", len(changed))})
		rows = append(rows, changed...)
	}
	return rows
}

// Key identifies a row across refreshes: its section and its path.
func (r Row) Key() string {
	if r.Header {
		return ""
	}
	return fmt.Sprintf("%t:%s", r.Staged, r.File.Path)
}

// Code is the letter that tells what happened to the file in its section.
func (r Row) Code() byte {
	if r.Staged {
		return r.File.Index
	}
	if r.File.Unmerged {
		return 'U'
	}
	if r.File.Untracked() {
		return '?'
	}
	return r.File.Worktree
}
