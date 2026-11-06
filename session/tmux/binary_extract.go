package tmux

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// extractBinary writes data into dir as an executable named after its content
// hash and returns the path. Never overwrite an existing tmux file in place:
// with --tmux-keep-server an older tmux server keeps running from it, and macOS
// SIGKILLs every later launch of a signed binary rewritten under a running
// process. A new version gets a new filename; a new file is renamed into place
// so a concurrent start never sees a partial write.
func extractBinary(dir string, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	dst := filepath.Join(dir, "tmux-"+hex.EncodeToString(sum[:])[:12])

	if info, err := os.Stat(dst); err == nil && info.Mode().IsRegular() && info.Size() == int64(len(data)) {
		return dst, nil
	}

	tmp, err := os.CreateTemp(dir, ".tmux-extract-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file for tmux: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("writing tmux: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("closing tmux: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return "", fmt.Errorf("chmod tmux: %w", err)
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return "", fmt.Errorf("installing tmux: %w", err)
	}
	return dst, nil
}
