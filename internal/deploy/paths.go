package deploy

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// RemotePath maps a repo-relative path to the absolute path it takes under
// the target's dir. A target may set baseDir to strip a local prefix, and a
// path that tries to leave dir is refused.
func RemotePath(t Target, rel string) (string, error) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	rel = strings.TrimPrefix(rel, "./")
	if rel == "" || rel == "." {
		return "", errors.New("ruta vacía")
	}
	if strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("la ruta %q es absoluta", rel)
	}
	if strings.ContainsRune(rel, 0) {
		return "", errors.New("la ruta tiene un byte nulo")
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			return "", fmt.Errorf("la ruta %q intenta salir del repo", rel)
		}
	}

	base := strings.Trim(filepath.ToSlash(t.BaseDir), "/")
	if base != "" {
		if rel == base {
			return "", fmt.Errorf("la ruta %q es la carpeta base", rel)
		}
		if strings.HasPrefix(rel, base+"/") {
			rel = rel[len(base)+1:]
		}
	}

	dir := path.Clean("/" + strings.Trim(filepath.ToSlash(t.Dir), "/"))
	remote := path.Join(dir, rel)
	if remote != dir && !strings.HasPrefix(remote, dir+"/") {
		return "", fmt.Errorf("la ruta %q queda fuera de %s", rel, dir)
	}
	return remote, nil
}
