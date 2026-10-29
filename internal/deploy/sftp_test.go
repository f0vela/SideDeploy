package deploy

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func testKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return signer.PublicKey()
}

func TestKnownHostsTrust(t *testing.T) {
	appDir := t.TempDir()
	host := "example.com:2222"
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}
	key := testKey(t)

	check, err := hostKeyCallback(appDir)
	if err != nil {
		t.Fatal(err)
	}
	err = check(host, addr, key)
	unknown, ok := AsUnknownHost(err)
	if !ok {
		t.Fatalf("unknown host error = %v", err)
	}
	if unknown.Host != host || unknown.Fingerprint != ssh.FingerprintSHA256(key) {
		t.Fatalf("unknown = %+v", unknown)
	}

	if err := TrustHost(appDir, unknown); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(appDir, knownHostsFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("known_hosts permissions = %o", info.Mode().Perm())
	}
	check, err = hostKeyCallback(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := check(host, addr, key); err != nil {
		t.Fatalf("trusted host still fails: %v", err)
	}

	// A different key on the same host is an attack, not a new host: it must
	// not offer to trust it.
	other := testKey(t)
	err = check(host, addr, other)
	if _, ok := AsUnknownHost(err); ok {
		t.Fatal("a changed host key was offered as a new host")
	}
	if err == nil {
		t.Fatal("a changed host key was accepted")
	}
}
