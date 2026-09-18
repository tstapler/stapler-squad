//go:build embed_tymux

package tymux

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// tymuxdEmbedded holds the raw bytes of the platform-specific tymuxd binary.
// Populated at compile time from session/tymux/embed/tymuxd, which is fetched by:
//
//	make build-tymuxd-embed
//
// If the file doesn't exist, `go build -tags embed_tymux` will fail with a clear
// compile error — run the prerequisite first.
//
//go:embed embed/tymuxd
var tymuxdEmbedded []byte

var (
	extractOnce sync.Once
	extractPath string
	extractErr  error
)

// TymuxdBinary returns the path to the embedded tymuxd binary, extracting it to
// the user's cache directory on first call. TYMUXD_BIN env var still overrides
// this so tests and developers can point at a different binary when needed —
// like TMUX_BIN, this is a deliberate, unvalidated escape hatch: anyone who can
// set env vars for this process can point it at an arbitrary binary. That's an
// accepted risk, mirroring TMUX_BIN's identical shape.
func TymuxdBinary() string {
	if bin := os.Getenv("TYMUXD_BIN"); bin != "" {
		return bin
	}
	extractOnce.Do(func() {
		extractPath, extractErr = extractEmbeddedTymuxd()
	})
	if extractErr != nil {
		// Extraction failed: fall back to whatever "tymuxd" is on PATH.
		// This is better than a hard crash at startup.
		return "tymuxd"
	}
	return extractPath
}

func extractEmbeddedTymuxd() (string, error) {
	if len(tymuxdEmbedded) == 0 {
		return "", fmt.Errorf("embedded tymuxd binary is empty (run: make build-tymuxd-embed)")
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	dir := filepath.Join(cacheDir, "stapler-squad", "tymux", runtime.GOOS+"_"+runtime.GOARCH)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating tymuxd cache dir: %w", err)
	}
	dst := filepath.Join(dir, "tymuxd")

	// Unlike tmux's length-only comparison, hash both the embedded bytes and
	// the existing cache file: a fetched binary needs a real integrity check,
	// not just a same-length heuristic, so a corrupted or tampered cache-dir
	// copy is never silently executed. Only skip the rewrite when both
	// hashes match.
	embeddedSum := sha256.Sum256(tymuxdEmbedded)
	if existing, err := os.ReadFile(dst); err == nil {
		if sha256.Sum256(existing) == embeddedSum {
			return dst, nil
		}
	}
	if err := os.WriteFile(dst, tymuxdEmbedded, 0755); err != nil {
		return "", fmt.Errorf("extracting embedded tymuxd: %w", err)
	}
	return dst, nil
}
