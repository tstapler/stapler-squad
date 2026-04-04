package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var distFS embed.FS

// GetDistFS returns the embedded web UI filesystem.
// The files are embedded from dist/ directory (Next.js static export).
func GetDistFS() (fs.FS, error) {
	return distFS, nil
}
