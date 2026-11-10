package backend

import (
	"os"
	"path/filepath"
)

func openFileCreate(path string) (*os.File, error) {
	return os.Create(path)
}

// callerDir is the directory of a runtime.Caller file. Bazel reports it relative to the workspace
// root while the test runs in the package directory, so anchor it in the runfiles tree.
func callerDir(file string) string {
	if src, ws := os.Getenv("TEST_SRCDIR"), os.Getenv("TEST_WORKSPACE"); src != "" && ws != "" && !filepath.IsAbs(file) {
		return filepath.Join(src, ws, filepath.Dir(file))
	}
	return filepath.Dir(file)
}
