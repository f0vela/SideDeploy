package deploy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
)

// NewOpener returns the function that connects to t for a deploy. FTP opens a
// connection per call; SFTP shares one SSH connection across them.
func NewOpener(t Target, appDir string) Opener {
	if strings.EqualFold(t.Type, "ftp") {
		return func(ctx context.Context) (Session, error) { return dialFTP(ctx, t) }
	}
	pool := &sftpPool{target: t, appDir: appDir}
	return pool.open
}

// InsecureTLS reports whether the target asks to skip certificate checks.
func InsecureTLS(t Target) bool {
	return t.SecureOptions != nil && t.SecureOptions.RejectUnauthorized != nil && !*t.SecureOptions.RejectUnauthorized
}

// TLSOff tells how the target secures FTP: "", "explicit" or "implicit".
func TLSOff(t Target) string {
	switch v := t.Secure.(type) {
	case bool:
		if v {
			return "explicit"
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "implicit":
			return "implicit"
		case "true", "yes", "explicit":
			return "explicit"
		}
	}
	return ""
}

func tlsConfig(t Target) *tls.Config {
	cfg := &tls.Config{ServerName: t.Host, MinVersion: tls.VersionTLS12}
	if InsecureTLS(t) {
		cfg.InsecureSkipVerify = true
	}
	return cfg
}

// AsUnknownHost reports the error that asks the user to trust a new SSH host.
func AsUnknownHost(err error) (*UnknownHostError, bool) {
	var u *UnknownHostError
	if errors.As(err, &u) {
		return u, true
	}
	return nil, false
}

// CheckTarget rejects a target that cannot be dialed at all.
func CheckTarget(t Target) error {
	if strings.TrimSpace(t.Host) == "" {
		return errors.New("el destino no tiene host")
	}
	switch strings.ToLower(t.Type) {
	case "ftp", "sftp":
	default:
		return fmt.Errorf("tipo de destino desconocido: %q", t.Type)
	}
	return nil
}
