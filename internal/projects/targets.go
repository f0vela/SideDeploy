package projects

import (
	"errors"
	"strconv"
	"strings"

	"sidedeploy/internal/deploy"
)

// NewTarget is the draft a form starts from.
func NewTarget() deploy.Target {
	return deploy.Target{Type: "sftp", Dir: "/", BaseDir: "/"}
}

// DraftFields gives the port and the private key of t as the text a form
// shows: empty when there is none.
func DraftFields(t deploy.Target) (port, key string) {
	if t.Port > 0 {
		port = strconv.Itoa(t.Port)
	}
	if t.PrivateKey != nil {
		key = *t.PrivateKey
	}
	return port, key
}

// SaveTarget validates a draft and stores it at index, or appends it when
// index is out of range. port and key are the texts of the form; the
// project's active target follows a rename and falls on the first target
// when it had none.
func (p *Project) SaveTarget(index int, d deploy.Target, port, key string) error {
	isNew := index < 0 || index >= len(p.Targets)
	d.Name = strings.TrimSpace(d.Name)
	d.Host = strings.TrimSpace(d.Host)
	if d.Name == "" {
		return errors.New("Ponle un nombre al destino.")
	}
	if d.Host == "" {
		return errors.New("Falta el host.")
	}
	d.Port = 0
	if s := strings.TrimSpace(port); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 65535 {
			return errors.New("El puerto va de 1 a 65535, o vacío para el de su tipo.")
		}
		d.Port = n
	}
	d.PrivateKey = nil
	if key = strings.TrimSpace(key); key != "" {
		d.PrivateKey = &key
	}
	for i := range p.Targets {
		if (isNew || i != index) && p.Targets[i].Name == d.Name {
			return errors.New("Ya hay un destino con ese nombre.")
		}
	}

	one := []deploy.Target{d}
	deploy.Normalize(one)
	d = one[0]

	if isNew {
		p.Targets = append(p.Targets, d)
	} else {
		if p.ActiveTarget == p.Targets[index].Name {
			p.ActiveTarget = d.Name
		}
		p.Targets[index] = d
	}
	if p.ActiveTarget == "" {
		p.ActiveTarget = d.Name
	}
	return nil
}

// DeleteTarget removes the target at i from the list, not from the server.
func (p *Project) DeleteTarget(i int) {
	if i < 0 || i >= len(p.Targets) {
		return
	}
	name := p.Targets[i].Name
	p.Targets = append(p.Targets[:i], p.Targets[i+1:]...)
	if p.ActiveTarget == name {
		p.ActiveTarget = ""
		if len(p.Targets) > 0 {
			p.ActiveTarget = p.Targets[0].Name
		}
	}
}

// Adopt copies the targets of pro-deployer.json the first time, so they can
// be edited in the app. It reports whether it copied anything.
func (p *Project) Adopt(cfg *deploy.Config) bool {
	if len(p.Targets) > 0 || cfg == nil || len(cfg.Targets) == 0 {
		return false
	}
	p.Targets = append([]deploy.Target(nil), cfg.Targets...)
	if len(cfg.ActiveTargets) > 0 {
		p.ActiveTarget = cfg.ActiveTargets[0]
	}
	if p.ActiveTarget == "" {
		p.ActiveTarget = p.Targets[0].Name
	}
	return true
}
