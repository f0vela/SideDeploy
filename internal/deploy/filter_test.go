package deploy

import "testing"

func TestFilter(t *testing.T) {
	f := Filter{
		Ignore:  []string{".git/**/*", "vendor/vimeo/psalm", "*.log", "storage/cache/**"},
		Include: []string{"**/*.php", "public/**"},
		Config:  []string{".vscode/pro-deployer.json"},
		Ignored: map[string]bool{"debug.tmp": true},
	}
	cases := []struct {
		rel  string
		want bool
	}{
		{".git/config", true}, // always
		{".vscode/pro-deployer.json", true},
		{"vendor/vimeo/psalm/a.php", true},
		{"vendor/vimeo/psalm", true},
		{"app/x.log", true},
		{"storage/cache/a/b.php", true},
		{"debug.tmp", true},
		{"app/Model.php", false},
		{"public/css/app.css", false},
		{"README.md", true}, // include is an allow list
		{"public", false},   // the folder itself matches public/**
	}
	for _, c := range cases {
		if got := f.Skip(c.rel); got != c.want {
			t.Errorf("Skip(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
}

func TestFilterWithoutInclude(t *testing.T) {
	f := Filter{Ignore: []string{"node_modules/**"}}
	for _, rel := range []string{"app/x.php", "README.md", ".env"} {
		if f.Skip(rel) {
			t.Errorf("Skip(%q) = true without an include list", rel)
		}
	}
	if !f.Skip("node_modules/x/index.js") {
		t.Error("node_modules was not skipped")
	}
}
