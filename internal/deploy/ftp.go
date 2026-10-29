package deploy

import (
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"
)

type ftpSession struct {
	conn *ftp.ServerConn
	stop chan struct{}
	once sync.Once
	// The library is not safe for concurrent commands: the keepalive shares
	// the control connection with the transfers.
	mu sync.Mutex
}

func dialFTP(ctx context.Context, t Target) (Session, error) {
	addr := net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
	opts := []ftp.DialOption{
		ftp.DialWithContext(ctx),
		ftp.DialWithTimeout(15 * time.Second),
	}
	switch TLSOff(t) {
	case "implicit":
		opts = append(opts, ftp.DialWithTLS(tlsConfig(t)))
	case "explicit":
		opts = append(opts, ftp.DialWithExplicitTLS(tlsConfig(t)))
	}
	conn, err := ftp.Dial(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("conectar a %s: %w", addr, err)
	}
	if err := conn.Login(t.User, t.Password); err != nil {
		conn.Quit()
		return nil, fmt.Errorf("entrar a %s: %w", addr, err)
	}
	if strings.EqualFold(t.TransferDataType, "ascii") {
		if err := conn.Type(ftp.TransferTypeASCII); err != nil {
			conn.Quit()
			return nil, err
		}
	}
	s := &ftpSession{conn: conn, stop: make(chan struct{})}
	s.keepalive(time.Duration(t.KeepaliveIntervalMs) * time.Millisecond)
	return s, nil
}

func (s *ftpSession) keepalive(every time.Duration) {
	if every <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				s.mu.Lock()
				_ = s.conn.NoOp()
				s.mu.Unlock()
			}
		}
	}()
}

func (s *ftpSession) Upload(ctx context.Context, local, remote string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.conn.Stor(remote, f)
}

func (s *ftpSession) Delete(ctx context.Context, remote string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.Delete(remote)
}

func (s *ftpSession) MkdirAll(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir = path.Clean("/" + strings.Trim(strings.ReplaceAll(dir, "\\", "/"), "/"))
	if dir == "/" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	at := "/"
	for _, part := range strings.Split(strings.TrimPrefix(dir, "/"), "/") {
		at = path.Join(at, part)
		if err := s.conn.ChangeDir(at); err == nil {
			continue
		}
		if err := s.conn.MakeDir(at); err != nil {
			// Another worker may have created it, or the server made the
			// whole path at once.
			if again := s.conn.ChangeDir(at); again == nil {
				continue
			}
			return err
		}
	}
	return nil
}

// Close waits for the command in flight: an FTP transfer cannot be cut
// without killing the control connection the library keeps private.
func (s *ftpSession) Close() error {
	var err error
	s.once.Do(func() {
		close(s.stop)
		s.mu.Lock()
		err = s.conn.Quit()
		s.mu.Unlock()
	})
	return err
}
