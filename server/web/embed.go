package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// GetDistFS returns the embedded web UI filesystem.
// The files are copied into dist/ from web-app/out (Next export or Bun build, see WEB_BUILD).
func GetDistFS() (fs.FS, error) {
	// Return the dist subdirectory from the embedded filesystem
	return fs.Sub(distFS, "dist")
}
