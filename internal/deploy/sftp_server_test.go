package deploy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// TestSFTPAgainstLocalServer checks the whole path: a new host asks to be
// trusted, and then files go up, folders are made and files are removed.
func TestSFTPAgainstLocalServer(t *testing.T) {
	host, port := startSFTPServer(t)
	appDir := t.TempDir()
	target := Target{
		Name: "local", Type: "sftp", Host: host, Port: port,
		User: "test", Password: "secret", Dir: "/", BaseDir: "/",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := NewOpener(target, appDir)(ctx)
	unknown, ok := AsUnknownHost(err)
	if !ok {
		t.Fatalf("first connection = %v, want an unknown host", err)
	}
	if err := TrustHost(appDir, unknown); err != nil {
		t.Fatal(err)
	}

	session, err := NewOpener(target, appDir)(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	local := filepath.Join(t.TempDir(), "local.txt")
	if err := os.WriteFile(local, []byte("hola desde SideDeploy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	remoteDir := filepath.Join(t.TempDir(), "www", "app")
	remote := filepath.Join(remoteDir, "x.txt")
	if err := session.MkdirAll(ctx, remoteDir); err != nil {
		t.Fatal(err)
	}
	if err := session.Upload(ctx, local, remote); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(remote)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hola desde SideDeploy\n" {
		t.Fatalf("remote file = %q", got)
	}
	if err := session.Delete(ctx, remote); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(remote); !os.IsNotExist(err) {
		t.Fatalf("remote file still there: %v", err)
	}
}

func startSFTPServer(t *testing.T) (string, int) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "test" && string(pass) == "secret" {
				return nil, nil
			}
			return nil, errors.New("credenciales incorrectas")
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSFTPConn(conn, cfg)
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

func serveSFTPConn(conn net.Conn, cfg *ssh.ServerConfig) {
	server, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		conn.Close()
		return
	}
	defer server.Close()
	go ssh.DiscardRequests(reqs)
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "solo sesiones")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go func() {
			for req := range requests {
				req.Reply(true, nil)
			}
		}()
		go func() {
			srv, err := sftp.NewServer(channel)
			if err == nil {
				srv.Serve()
				srv.Close()
			}
			channel.Close()
		}()
	}
}
