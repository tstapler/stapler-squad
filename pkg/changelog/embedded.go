package changelog

import "sync/atomic"

//nolint:gochecknoglobals // set once by package main (go:embed cannot reach the repo root from a subpackage).
var embedded atomic.Pointer[[]byte]

// SetEmbedded registers the CHANGELOG.md bytes compiled into the binary.
// //go:embed can't reach the repo root from a subpackage, so package main
// embeds the file and hands it over here before the server starts.
func SetEmbedded(content []byte) { embedded.Store(&content) }

// Embedded returns the registered CHANGELOG.md bytes, or nil if none.
func Embedded() []byte {
	if p := embedded.Load(); p != nil {
		return *p
	}
	return nil
}
