package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const knownHostsFile = "known_hosts"

type sftpSession struct {
	cli *sftp.Client
	ssh *ssh.Client
	mu  sync.Mutex
	one sync.Once
}

func dialSFTP(ctx context.Context, t Target, appDir string) (*sftpSession, error) {
	addr := net.JoinHostPort(t.Host, fmt.Sprint(t.Port))
	auth, cleanup, err := authMethods(t)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	check, err := hostKeyCallback(appDir)
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            t.User,
		Auth:            auth,
		HostKeyCallback: check,
		Timeout:         15 * time.Second,
	}
	dialer := net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("conectar a %s: %w", addr, err)
	}
	// The SSH handshake has no context of its own: a deadline frees it.
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SSH a %s: %w", addr, err)
	}
	_ = conn.SetDeadline(time.Time{})
	sshClient := ssh.NewClient(clientConn, chans, reqs)
	cli, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("SFTP a %s: %w", addr, err)
	}
	return &sftpSession{cli: cli, ssh: sshClient}, nil
}

func authMethods(t Target) ([]ssh.AuthMethod, func(), error) {
	noop := func() {}
	var methods []ssh.AuthMethod
	if t.Password != "" {
		methods = append(methods, ssh.Password(t.Password))
	}
	if t.PrivateKey != nil && strings.TrimSpace(*t.PrivateKey) != "" {
		data, err := os.ReadFile(expandHome(*t.PrivateKey))
		if err != nil {
			return nil, noop, fmt.Errorf("leer la clave privada: %w", err)
		}
		var signer ssh.Signer
		if t.Passphrase != nil && *t.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(*t.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(data)
		}
		if err != nil {
			return nil, noop, fmt.Errorf("clave privada: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if t.UseAuthAgent {
		socket := os.Getenv("SSH_AUTH_SOCK")
		if t.AuthAgentPath != nil && *t.AuthAgentPath != "" {
			socket = expandHome(*t.AuthAgentPath)
		}
		if socket == "" {
			return nil, noop, errors.New("useAuthAgent está activo pero no hay SSH_AUTH_SOCK")
		}
		conn, err := net.Dial("unix", socket)
		if err != nil {
			return nil, noop, fmt.Errorf("agente SSH: %w", err)
		}
		methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		// The agent only signs during the handshake.
		cleanup := func() { conn.Close() }
		noop = cleanup
	}
	if len(methods) == 0 {
		return nil, noop, errors.New("el destino no tiene contraseña, clave privada ni agente SSH")
	}
	return methods, noop, nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// UnknownHostError says the SSH server presented a key nobody trusted yet..
// The app asks the user and appends it to its own known_hosts.
type UnknownHostError struct {
	Host        string
	Fingerprint string
	key         ssh.PublicKey
}

func (e *UnknownHostError) Error() string {
	return fmt.Sprintf("el host %s no está en known_hosts (huella %s)", e.Host, e.Fingerprint)
}

func hostKeyCallback(appDir string) (ssh.HostKeyCallback, error) {
	appFile, err := ensureKnownHosts(appDir)
	if err != nil {
		return nil, err
	}
	files := []string{appFile}
	if home, err := os.UserHomeDir(); err == nil {
		if user := filepath.Join(home, ".ssh", "known_hosts"); fileExists(user) {
			files = append(files, user)
		}
	}
	check, err := knownhosts.New(files...)
	if err != nil {
		return nil, fmt.Errorf("leer known_hosts: %w", err)
	}
	return func(host string, remote net.Addr, key ssh.PublicKey) error {
		err := check(host, remote, key)
		if err == nil {
			return nil
		}
		var missing *knownhosts.KeyError
		if errors.As(err, &missing) {
			if len(missing.Want) == 0 {
				return &UnknownHostError{
					Host:        host,
					Fingerprint: ssh.FingerprintSHA256(key),
					key:         key,
				}
			}
			return fmt.Errorf("la clave SSH de %s cambió y no coincide con known_hosts", host)
		}
		return err
	}, nil
}

// TrustHost saves a key the user accepted, in the app's own known_hosts.
func TrustHost(appDir string, u *UnknownHostError) error {
	if u == nil || u.key == nil {
		return errors.New("no hay una clave que confiar")
	}
	file, err := ensureKnownHosts(appDir)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, knownhosts.Line([]string{u.Host}, u.key))
	return err
}

func ensureKnownHosts(appDir string) (string, error) {
	if appDir == "" {
		return "", errors.New("no hay carpeta de datos para known_hosts")
	}
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		return "", err
	}
	file := filepath.Join(appDir, knownHostsFile)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	f.Close()
	return file, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *sftpSession) Upload(ctx context.Context, local, remote string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	src, err := os.Open(local)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := s.cli.Create(remote)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

func (s *sftpSession) Delete(ctx context.Context, remote string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.cli.Remove(remote)
}

func (s *sftpSession) MkdirAll(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// MkdirAll races when two workers create the same folder.
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cli.MkdirAll(dir)
}

func (s *sftpSession) Close() error {
	s.one.Do(func() {
		s.cli.Close()
		s.ssh.Close()
	})
	return nil
}

// sftpPool shares the one SSH connection the plan asks for.
type sftpPool struct {
	target Target
	appDir string

	mu      sync.Mutex
	session *sftpSession
	err     error
	refs    int
}

func (p *sftpPool) open(ctx context.Context) (Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return nil, p.err
	}
	if p.session == nil {
		s, err := dialSFTP(ctx, p.target, p.appDir)
		if err != nil {
			p.err = err
			return nil, err
		}
		p.session = s
		// Canceling the deploy cuts the one connection all workers share.
		go func() {
			<-ctx.Done()
			s.Close()
		}()
	}
	p.refs++
	return &sharedSFTP{sftpSession: p.session, release: p.release}, nil
}

func (p *sftpPool) release() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refs--
	if p.refs <= 0 && p.session != nil {
		p.session.Close()
		p.session = nil
	}
}

type sharedSFTP struct {
	*sftpSession
	once    sync.Once
	release func()
}

func (s *sharedSFTP) Close() error {
	s.once.Do(s.release)
	return nil
}
