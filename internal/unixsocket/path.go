// Package unixsocket resolves short, private Unix-domain socket paths.
package unixsocket

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const privateDirMode = 0o700

// Path returns a deterministic, kernel-safe socket path for namespace. The
// socket parent is private to the current user; an existing symlink or
// non-directory is rejected rather than followed.
func Path(prefix, socketName, namespace string) (string, error) {
	if prefix == "" || socketName == "" || namespace == "" {
		return "", errors.New("unixsocket: prefix, socket name, and namespace are required")
	}
	digest := sha256.Sum256([]byte(namespace))
	dir := filepath.Join(os.TempDir(), prefix+"-"+hex.EncodeToString(digest[:8]))

	info, err := os.Lstat(dir)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("unixsocket: private path is not a directory: %s", dir)
		}
	case os.IsNotExist(err):
		if err := os.Mkdir(dir, privateDirMode); err != nil {
			return "", fmt.Errorf("unixsocket: create private directory: %w", err)
		}
	case err != nil:
		return "", fmt.Errorf("unixsocket: inspect private directory: %w", err)
	}
	if err := os.Chmod(dir, privateDirMode); err != nil {
		return "", fmt.Errorf("unixsocket: secure private directory: %w", err)
	}
	return filepath.Join(dir, socketName), nil
}
