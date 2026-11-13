package textdiff

import "testing"

func kinds(ls []Line) string {
	s := ""
	for _, l := range ls {
		s += map[Kind]string{Same: " ", Add: "+", Del: "-", Hunk: "@"}[l.Kind]
	}
	return s
}

func TestCompare(t *testing.T) {
	got := Compare("a\nb\nc\nd\n", "a\nB\nc\nd\ne\n", 1)
	var adds, dels int
	for _, l := range got {
		switch l.Kind {
		case Add:
			adds++
		case Del:
			dels++
		}
	}
	if adds != 2 || dels != 1 {
		t.Fatalf("adds=%d dels=%d in %v", adds, dels, got)
	}
}

func TestCompareNewAndEmpty(t *testing.T) {
	if got := Compare("", "x\ny\n", 3); kinds(got) != "@++" {
		t.Fatalf("new file = %q", kinds(got))
	}
	if got := Compare("x\n", "", 3); kinds(got) != "@-" {
		t.Fatalf("deleted file = %q", kinds(got))
	}
	if got := Compare("same\n", "same\n", 3); got != nil {
		t.Fatalf("same = %v", got)
	}
}

func TestFoldsLongRuns(t *testing.T) {
	old, new := "", ""
	for i := 0; i < 100; i++ {
		old += "line\n"
		new += "line\n"
	}
	got := Compare("first\n"+old+"last\n", "FIRST\n"+new+"LAST\n", 2)
	hunks := 0
	for _, l := range got {
		if l.Kind == Hunk {
			hunks++
		}
	}
	if hunks != 2 {
		t.Fatalf("hunks = %d, want 2", hunks)
	}
}
