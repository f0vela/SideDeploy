package projects

import (
	"os"
	"path/filepath"
	"testing"

	"sidedeploy/internal/deploy"
)

func TestAddAndRemove(t *testing.T) {
	var s Store
	if i := s.Add("/srv/site-a"); i != 0 {
		t.Fatalf("index = %d", i)
	}
	if i := s.Add("/srv/site-b"); i != 1 {
		t.Fatalf("index = %d", i)
	}
	if i := s.Add("/srv/site-a"); i != 0 || len(s.Projects) != 2 {
		t.Fatalf("add dedup = %d with %d projects", i, len(s.Projects))
	}
	if name := s.Projects[0].Name; name != "site-a" {
		t.Fatalf("name = %q", name)
	}
	s.Active = "/srv/site-a"
	s.Remove("/srv/site-a")
	if len(s.Projects) != 1 || s.Projects[0].Path != "/srv/site-b" {
		t.Fatalf("projects = %+v", s.Projects)
	}
	if s.Active != "" {
		t.Fatalf("active = %q, want empty", s.Active)
	}
	s.Remove("/nope")
	if len(s.Projects) != 1 {
		t.Fatalf("remove of a stranger changed the list: %+v", s.Projects)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := Store{Active: "/srv/site", Projects: []Project{{
		Name:         "site",
		Path:         "/srv/site",
		ActiveTarget: "CDD",
		Targets: []deploy.Target{
			{Name: "CDD", Type: "sftp", Host: "example.com", User: "root", Password: "s3cret", Dir: "/var/www"},
			{Name: "other", Type: "ftp", Host: "ftp.example.com", Port: 2121, Dir: "/"},
		},
	}}}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("permissions = %o, want 600", perm)
	}

	got := Load(dir)
	if got.Active != "/srv/site" || len(got.Projects) != 1 {
		t.Fatalf("store = %+v", got)
	}
	p := got.Projects[0]
	if p.Name != "site" || p.ActiveTarget != "CDD" || len(p.Targets) != 2 {
		t.Fatalf("project = %+v", p)
	}
	if tg := p.Targets[0]; tg.Host != "example.com" || tg.Password != "s3cret" || tg.Dir != "/var/www" {
		t.Fatalf("target = %+v", tg)
	}
	if tg := p.Targets[0]; tg.Port != 22 {
		t.Fatalf("port = %d, want the sftp default 22", tg.Port)
	}
	if tg := p.Targets[1]; tg.Port != 2121 {
		t.Fatalf("ftp port = %d, want 2121", tg.Port)
	}
}

func TestLoadBrokenFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := Load(dir); len(s.Projects) != 0 || s.Active != "" {
		t.Fatalf("store = %+v", s)
	}
}

func TestActive(t *testing.T) {
	p := Project{Targets: []deploy.Target{{Name: "a"}, {Name: "b"}}}
	if got := p.Active(); got == nil || got.Name != "a" {
		t.Fatalf("active without a choice = %+v", got)
	}
	p.ActiveTarget = "b"
	if got := p.Active(); got == nil || got.Name != "b" {
		t.Fatalf("active = %+v", got)
	}
	p.ActiveTarget = "gone"
	if got := p.Active(); got == nil || got.Name != "a" {
		t.Fatalf("active fallback = %+v", got)
	}
	empty := Project{}
	if got := empty.Active(); got != nil {
		t.Fatalf("active of an empty project = %+v", got)
	}
}
