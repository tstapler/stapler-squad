// Package reporoot finds the repository root from a test, under both `go test`
// and `bazel test`. It imports only the standard library so any package's tests
// (including internal tests of config and session) can use it without an import cycle.
package reporoot

import (
	"errors"
	"os"
	"path/filepath"
)

// Find returns a directory containing go.mod. Under Bazel it prefers the runfiles
// tree (TEST_SRCDIR/TEST_WORKSPACE), which has go.mod only when the test declares
// it in `data`; otherwise it walks up from the working directory.
func Find() (string, error) {
	if src, ws := os.Getenv("TEST_SRCDIR"), os.Getenv("TEST_WORKSPACE"); src != "" && ws != "" {
		if dir := filepath.Join(src, ws); hasGoMod(dir) {
			return dir, nil
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if hasGoMod(dir) {
			return dir, nil
		}
		if dir == filepath.Dir(dir) {
			return "", errors.New("reporoot: no go.mod found at or above " + wd + " (under Bazel, add //:go.mod to the test's data)")
		}
	}
}

func hasGoMod(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil && !info.IsDir()
}
