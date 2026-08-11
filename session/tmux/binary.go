//go:build !embed_tmux

package tmux

import "os"

// Binary returns the tmux executable path.
// TMUX_BIN env var overrides the default "tmux" — set it to use a specific
// binary (e.g. TMUX_BIN=$(pwd)/bin/tmux go test or the pinned submodule build).
//
// To bundle tmux directly into the stapler-squad binary instead, build with:
//
//	go build -tags embed_tmux .
//
// after running: make build-tmux-embed
func Binary() string {
	if bin := os.Getenv("TMUX_BIN"); bin != "" {
		return bin
	}
	return "tmux"
}
