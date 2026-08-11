//go:build darwin && !cgo

package procinfo

// openFilesCgo is a no-op stub used when CGO is unavailable (e.g. cross-compilation).
// The real CGO implementation in openfiles_darwin.go is used on native darwin builds.
func openFilesCgo(_ int32) ([]string, error) {
	return []string{}, nil
}
