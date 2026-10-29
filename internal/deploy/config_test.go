package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SIDE_TEST_USER", "deployer")
	if err := os.MkdirAll(filepath.Join(root, ".vscode"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `{
		"activeTargets": ["CDD FTP"],
		"concurrency": 3,
		"ignore": [".git/**/*"],
		"targets": [
			{
				"name": "CDD FTP",
				"type": "SFTP",
				"host": "example.com",
				"user": "${env:SIDE_TEST_USER}",
				"dir": "/var/www/${workspaceFolderBasename}",
				"baseDir": "/",
				"privateKey": null,
				"password": "secret"
			},
			{
				"name": "My FTP",
				"type": "ftp",
				"host": "localhost",
				"dir": "/",
				"secure": false
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(root, ".vscode", "pro-deployer.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	// A root-level file must lose to the .vscode one.
	if err := os.WriteFile(filepath.Join(root, "pro-deployer.json"), []byte(`{"targets":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, path, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(root, ".vscode", "pro-deployer.json") {
		t.Fatalf("path = %q", path)
	}
	if loaded.Concurrency != 3 || len(loaded.Targets) != 2 {
		t.Fatalf("config = %+v", loaded)
	}
	sftp := loaded.Targets[0]
	if sftp.Type != "sftp" {
		t.Errorf("type = %q", sftp.Type)
	}
	if sftp.Port != 22 {
		t.Errorf("sftp port = %d, want 22", sftp.Port)
	}
	if sftp.User != "deployer" {
		t.Errorf("user = %q", sftp.User)
	}
	if sftp.Dir != "/var/www/"+filepath.Base(root) {
		t.Errorf("dir = %q", sftp.Dir)
	}
	if sftp.TransferDataType != "binary" || sftp.KeepaliveIntervalMs != 10000 {
		t.Errorf("defaults = %+v", sftp)
	}
	if loaded.Targets[1].Port != 21 {
		t.Errorf("ftp port = %d, want 21", loaded.Targets[1].Port)
	}

	active := loaded.Active()
	if len(active) != 1 || active[0].Name != "CDD FTP" {
		t.Fatalf("active = %+v", active)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	cfg, path, err := Load(t.TempDir())
	if err != nil || cfg != nil || path != "" {
		t.Fatalf("cfg=%+v path=%q err=%v", cfg, path, err)
	}
}

func TestResolveVars(t *testing.T) {
	t.Setenv("SIDE_TEST_HOST", "ftp.example.com")
	root := "/srv/www/misite"
	got := resolveVars("${env:SIDE_TEST_HOST}:${workspaceFolderBasename}${pathSeparator}${workspaceFolder}", root)
	want := "ftp.example.com:misite" + string(filepath.Separator) + root
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestActiveEmptyIsAll(t *testing.T) {
	cfg := &Config{Targets: []Target{{Name: "a"}, {Name: "b"}}}
	if got := cfg.Active(); len(got) != 2 {
		t.Fatalf("active = %+v", got)
	}
}
