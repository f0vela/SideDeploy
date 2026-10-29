package deploy

import "testing"

func TestRemotePath(t *testing.T) {
	cases := []struct {
		name string
		t    Target
		rel  string
		want string
		bad  bool
	}{
		{"root dir", Target{Dir: "/var/www", BaseDir: "/"}, "app/x.php", "/var/www/app/x.php", false},
		{"dir without slash", Target{Dir: "var/www/", BaseDir: "/"}, "x.php", "/var/www/x.php", false},
		{"base dir strips", Target{Dir: "/srv", BaseDir: "/src"}, "src/app/x.php", "/srv/app/x.php", false},
		{"base dir relative", Target{Dir: "/srv", BaseDir: "src/"}, "src/app/x.php", "/srv/app/x.php", false},
		{"base dir not a prefix", Target{Dir: "/srv", BaseDir: "src"}, "srcx/a.php", "/srv/srcx/a.php", false},
		{"dots inside a name", Target{Dir: "/srv", BaseDir: "/"}, "app/..x/a.php", "/srv/app/..x/a.php", false},
		{"empty", Target{Dir: "/srv"}, "", "", true},
		{"dot", Target{Dir: "/srv"}, ".", "", true},
		{"absolute", Target{Dir: "/srv"}, "/etc/passwd", "", true},
		{"escapes", Target{Dir: "/srv"}, "../x", "", true},
		{"escapes inside", Target{Dir: "/srv"}, "app/../../x", "", true},
		{"base dir itself", Target{Dir: "/srv", BaseDir: "/src"}, "src", "", true},
	}
	for _, c := range cases {
		got, err := RemotePath(c.t, c.rel)
		if c.bad {
			if err == nil {
				t.Errorf("%s: RemotePath(%q) = %q, want an error", c.name, c.rel, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: RemotePath(%q) = %q, %v; want %q", c.name, c.rel, got, err, c.want)
		}
	}
}
