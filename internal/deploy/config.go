// Package deploy reads the PRO Deployer configuration and maps local paths
// to the remote ones.
package deploy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the part of pro-deployer.json this app uses.
type Config struct {
	ActiveTargets  []string `json:"activeTargets"`
	Concurrency    int      `json:"concurrency"`
	Ignore         []string `json:"ignore"`
	Include        []string `json:"include"`
	CheckGitignore bool     `json:"checkGitignore"`
	AutoDelete     bool     `json:"autoDelete"`
	Targets        []Target `json:"targets"`
}

// Target is one FTP or SFTP server.
type Target struct {
	Name                string        `json:"name"`
	Type                string        `json:"type"`
	Host                string        `json:"host"`
	Port                int           `json:"port"`
	User                string        `json:"user"`
	Password            string        `json:"password"`
	Dir                 string        `json:"dir"`
	BaseDir             string        `json:"baseDir"`
	PrivateKey          *string       `json:"privateKey"`
	Passphrase          *string       `json:"passphrase"`
	UseAuthAgent        bool          `json:"useAuthAgent"`
	AuthAgentPath       *string       `json:"authAgentPath"`
	TransferDataType    string        `json:"transferDataType"`
	Secure              any           `json:"secure"`
	KeepaliveIntervalMs int           `json:"keepaliveIntervalMs"`
	SecureOptions       *SecureOption `json:"secureOptions"`
}

// SecureOption is the TLS verification setting of an FTP target.
type SecureOption struct {
	RejectUnauthorized *bool `json:"rejectUnauthorized"`
}

// Load reads the config of a repo: .vscode/pro-deployer.json first, then
// pro-deployer.json at its root. A missing file is not an error.
func Load(repoRoot string) (*Config, string, error) {
	candidates := []string{
		filepath.Join(repoRoot, ".vscode", "pro-deployer.json"),
		filepath.Join(repoRoot, "pro-deployer.json"),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, path, err
		}
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, path, fmt.Errorf("%s: %w", path, err)
		}
		cfg.applyDefaults()
		cfg.resolve(repoRoot)
		return &cfg, path, nil
	}
	return nil, "", nil
}

// Active returns the targets the config enables, or all of them when the
// list is empty.
func (c *Config) Active() []Target {
	if c == nil {
		return nil
	}
	if len(c.ActiveTargets) == 0 {
		return c.Targets
	}
	active := make(map[string]bool, len(c.ActiveTargets))
	for _, name := range c.ActiveTargets {
		active[name] = true
	}
	out := make([]Target, 0, len(c.Targets))
	for _, t := range c.Targets {
		if active[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

func (c *Config) applyDefaults() {
	if c.Concurrency <= 0 {
		c.Concurrency = 5
	}
	Normalize(c.Targets)
}

// Normalize fills what a target may leave out: its type in lowercase, the
// port of that type, the remote and base folders, binary mode, keepalive.
func Normalize(targets []Target) {
	for i := range targets {
		t := &targets[i]
		t.Type = strings.ToLower(strings.TrimSpace(t.Type))
		switch t.Type {
		case "ftp":
			if t.Port == 0 {
				t.Port = 21
			}
		case "sftp":
			if t.Port == 0 {
				t.Port = 22
			}
		}
		if t.Dir == "" {
			t.Dir = "/"
		}
		if t.BaseDir == "" {
			t.BaseDir = "/"
		}
		if t.TransferDataType == "" {
			t.TransferDataType = "binary"
		}
		if t.KeepaliveIntervalMs == 0 {
			t.KeepaliveIntervalMs = 10000
		}
	}
}

func (c *Config) resolve(repoRoot string) {
	for i := range c.ActiveTargets {
		c.ActiveTargets[i] = resolveVars(c.ActiveTargets[i], repoRoot)
	}
	for i := range c.Targets {
		t := &c.Targets[i]
		t.Name = resolveVars(t.Name, repoRoot)
		t.Host = resolveVars(t.Host, repoRoot)
		t.User = resolveVars(t.User, repoRoot)
		t.Password = resolveVars(t.Password, repoRoot)
		t.Dir = resolveVars(t.Dir, repoRoot)
		t.BaseDir = resolveVars(t.BaseDir, repoRoot)
		if t.PrivateKey != nil {
			*t.PrivateKey = resolveVars(*t.PrivateKey, repoRoot)
		}
		if t.Passphrase != nil {
			*t.Passphrase = resolveVars(*t.Passphrase, repoRoot)
		}
		if t.AuthAgentPath != nil {
			*t.AuthAgentPath = resolveVars(*t.AuthAgentPath, repoRoot)
		}
	}
}

// resolveVars substitutes the variables pro-deployer documents.
func resolveVars(s, repoRoot string) string {
	if !strings.Contains(s, "${") {
		return s
	}
	home, _ := os.UserHomeDir()
	s = strings.NewReplacer(
		"${workspaceFolder}", repoRoot,
		"${workspaceFolderBasename}", filepath.Base(repoRoot),
		"${userHome}", home,
		"${pathSeparator}", string(filepath.Separator),
	).Replace(s)
	for {
		start := strings.Index(s, "${env:")
		if start < 0 {
			return s
		}
		end := strings.Index(s[start:], "}")
		if end < 0 {
			return s
		}
		name := s[start+len("${env:") : start+end]
		s = s[:start] + os.Getenv(name) + s[start+end+1:]
	}
}
