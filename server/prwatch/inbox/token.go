package inbox

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DefaultTokenPath is where gh-signal-inbox writes its bearer token.
const DefaultTokenPath = "~/.local/state/gh-signal/inbox.token"

var (
	// ErrTokenMissing means the token file is absent, unreadable or empty.
	ErrTokenMissing = errors.New("inbox token file missing or empty")
	// ErrTokenPermissions means the path is not a regular 0600 file.
	ErrTokenPermissions = errors.New("inbox token file must be a regular file with mode 0600")
)

// ReadToken reads the bearer token fresh on every call so a rotated token is
// picked up without a restart. Errors name the path and mode, never contents.
func ReadToken(path string) (string, error) {
	if path == "" {
		path = DefaultTokenPath
	}
	path, err := expandTilde(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrTokenMissing, path)
	}

	info, err := checkTokenFile(path)
	// kibitzer:ignore go-error-context -- checkTokenFile already wraps with the path
	if err != nil {
		return "", err
	}

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrTokenMissing, path)
	}
	defer f.Close()

	// Re-check the opened file so a swap between Lstat and Open is caught.
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", fmt.Errorf("%w: %s changed while opening", ErrTokenPermissions, path)
	}

	raw, err := io.ReadAll(io.LimitReader(f, 64*1024))
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrTokenMissing, path)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", fmt.Errorf("%w: %s", ErrTokenMissing, path)
	}
	return token, nil
}

// checkTokenFile Lstats first: a FIFO or symlink must be refused before any
// open can block or follow.
func checkTokenFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrTokenMissing, path)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("%w: %s has mode %s", ErrTokenPermissions, path, info.Mode())
	}
	if !ownedByCurrentUser(info) {
		return nil, fmt.Errorf("%w: %s is not owned by the current user", ErrTokenPermissions, path)
	}
	return info, nil
}

func expandTilde(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path, err
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}
