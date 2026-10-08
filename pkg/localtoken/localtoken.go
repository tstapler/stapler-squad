// Package localtoken owns the bearer token that non-browser local clients
// (ssq-hooks, the MCP proxy, hook commands, --open-url) present to the :8543
// listener when require_local_auth is on. It has no server dependencies so
// every client can import it.
package localtoken

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	dirName  = "auth"
	fileName = "local-api-token"
)

// Path returns where the token lives under configDir.
func Path(configDir string) string {
	return filepath.Join(configDir, dirName, fileName)
}

// LoadOrCreate returns the token at path, creating it with mode 0600 if absent.
func LoadOrCreate(path string) (string, error) {
	tok, err := Read(path)
	if err == nil {
		return tok, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("load local token: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("create local token dir: %w", err)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate local token: %w", err)
	}
	tok = hex.EncodeToString(raw)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- path is built from the config dir and constants
	if err != nil {
		if errors.Is(err, fs.ErrExist) { // lost a creation race; use the winner's token
			return Read(path)
		}
		return "", fmt.Errorf("create local token file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(tok + "\n"); err != nil {
		return "", fmt.Errorf("write local token file: %w", err)
	}
	return tok, nil
}

// Read returns the token at path. A file with group/other permission bits is
// rejected rather than trusted. A missing file wraps fs.ErrNotExist.
func Read(path string) (string, error) {
	// #nosec G304 -- path is Path(configDir), never request input.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read local token: %w", err)
	}
	if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("local token file %s is accessible by other users (mode %o); chmod 600 it", path, info.Mode().Perm())
	}
	tok := strings.TrimSpace(string(data))
	if tok == "" {
		return "", fmt.Errorf("local token file %s is empty", path)
	}
	return tok, nil
}

// CurlHeaderArg returns a shell fragment for a curl Authorization header that
// reads the token from path at run time, so the secret is never written into a
// generated settings/hooks file. Expands to an empty bearer when the file is
// absent (harmless when require_local_auth is off). The expanded token is
// visible in the process's argv for the life of the curl call.
func CurlHeaderArg(path string) string {
	return fmt.Sprintf(`-H "Authorization: Bearer $(cat '%s' 2>/dev/null)"`, strings.ReplaceAll(path, "'", `'\''`))
}

// FromConfigDir best-effort reads the token for a client process; "" if unavailable.
func FromConfigDir(configDir string) string {
	tok, err := Read(Path(configDir))
	if err != nil {
		return ""
	}
	return tok
}
