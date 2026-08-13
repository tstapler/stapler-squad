//go:build embed_tmux

package tmux

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// tmuxEmbedded holds the raw bytes of the platform-specific tmux binary.
// Populated at compile time from session/tmux/embed/tmux, which is built by:
//
//	make build-tmux-embed
//
// If the file doesn't exist, `go build -tags embed_tmux` will fail with a clear
// compile error — run the prerequisite first.
//
//go:embed embed/tmux
var tmuxEmbedded []byte

var (
	extractOnce sync.Once
	extractPath string
	extractErr  error
)

// Binary returns the path to the embedded tmux binary, extracting it to the
// user's cache directory on first call. TMUX_BIN env var still overrides this
// so tests and developers can point at a different binary when needed.
func Binary() string {
	if bin := os.Getenv("TMUX_BIN"); bin != "" {
		return bin
	}
	extractOnce.Do(func() {
		extractPath, extractErr = extractEmbeddedTmux()
	})
	if extractErr != nil {
		// Extraction failed: fall back to whatever "tmux" is on PATH.
		// This is better than a hard crash at startup.
		return "tmux"
	}
	return extractPath
}

func extractEmbeddedTmux() (string, error) {
	if len(tmuxEmbedded) == 0 {
		return "", fmt.Errorf("embedded tmux binary is empty (run: make build-tmux-embed)")
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	dir := filepath.Join(cacheDir, "stapler-squad", "tmux", runtime.GOOS+"_"+runtime.GOARCH)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating tmux cache dir: %w", err)
	}
	dst := filepath.Join(dir, "tmux")

	// Only rewrite if the content has changed (avoids churn on repeated starts).
	if existing, err := os.ReadFile(dst); err != nil || len(existing) != len(tmuxEmbedded) {
		if err := os.WriteFile(dst, tmuxEmbedded, 0755); err != nil {
			return "", fmt.Errorf("extracting embedded tmux: %w", err)
		}
	}
	return dst, nil
}
