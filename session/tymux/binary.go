//go:build !embed_tymux

package tymux

import "os"

// TymuxdBinary returns the tymuxd executable path.
// TYMUXD_BIN env var overrides the default "tymuxd" — set it to use a specific
// binary (e.g. TYMUXD_BIN=$(pwd)/bin/tymuxd go test or the fetched embed copy).
//
// To bundle tymuxd directly into the stapler-squad binary instead, build with:
//
//	go build -tags embed_tymux .
//
// after running: make build-tymuxd-embed
func TymuxdBinary() string {
	if bin := os.Getenv("TYMUXD_BIN"); bin != "" {
		return bin
	}
	return "tymuxd"
}
