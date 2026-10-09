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
		return tok, writeHeaderFile(path, tok)
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
			tok, readErr := Read(path)
			if readErr != nil {
				return "", readErr
			}
			return tok, writeHeaderFile(path, tok)
		}
		return "", fmt.Errorf("create local token file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(tok + "\n"); err != nil {
		return "", fmt.Errorf("write local token file: %w", err)
	}
	return tok, writeHeaderFile(path, tok)
}

// writeHeaderFile (re)writes the header file for tok with mode 0600 if it is
// missing or stale.
func writeHeaderFile(tokenPath, tok string) error {
	hp := tokenPath + ".header"
	want := "Authorization: Bearer " + tok + "\n"
	if cur, err := os.ReadFile(hp); err == nil && string(cur) == want { // #nosec G304 -- derived from the token path
		return nil
	}
	if err := os.WriteFile(hp, []byte(want), 0600); err != nil {
		return fmt.Errorf("write local token header file: %w", err)
	}
	return os.Chmod(hp, 0600)
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

// HeaderPath is the 0600 file holding the complete "Authorization: Bearer <token>"
// header line, written next to the token. curl reads it with `-H @file`, so hook
// commands carry only a path in argv, never the secret.
func HeaderPath(configDir string) string {
	return Path(configDir) + ".header"
}

// CurlHeaderArg returns a curl argument fragment (`-H @'<header file>'`) that
// supplies the Authorization header from HeaderPath, or "" if the header file
// does not exist yet. The server creates it at startup, before any session.
func CurlHeaderArg(configDir string) string {
	hp := HeaderPath(configDir)
	if _, err := os.Stat(hp); err != nil {
		return ""
	}
	return fmt.Sprintf("-H @'%s'", strings.ReplaceAll(hp, "'", `'\''`))
}

// FromConfigDir best-effort reads the token for a client process; "" if unavailable.
func FromConfigDir(configDir string) string {
	tok, err := Read(Path(configDir))
	if err != nil {
		return ""
	}
	return tok
}
