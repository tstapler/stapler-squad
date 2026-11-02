// Package gotool puts the Go toolchain on PATH for analysistest, which shells out to `go list`.
// Under Bazel the test sandbox has no `go`; without this analysistest silently SKIPS.
package gotool

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const reexecMarker = "GOTOOL_REEXEC"

// Setup makes the rules_go SDK (from runfiles) usable by analysistest and fails, never skips,
// when Bazel runfiles are present but no SDK was found. Outside Bazel it does nothing.
//
// analysistest requires `go env GOROOT` to equal runtime.GOROOT(), which is the literal
// "GOROOT" in a trimmed Bazel binary and is read from the process environment at start, so
// setting GOROOT needs a one-time re-exec of the test binary.
func Setup(t *testing.T) {
	t.Helper()
	src := os.Getenv("TEST_SRCDIR")
	if src == "" {
		return
	}
	matches, _ := filepath.Glob(filepath.Join(src, "*go_sdk*", "bin", "go"))
	if len(matches) == 0 {
		t.Fatalf("under Bazel but no Go SDK in runfiles %s: add @rules_go//go to the test's data", src)
	}
	bin := filepath.Dir(matches[0])
	if os.Getenv(reexecMarker) == "" {
		exe, err := os.Executable()
		if err != nil {
			t.Fatalf("os.Executable: %v", err)
		}
		override := map[string]string{
			reexecMarker: "1",
			"GOROOT":     filepath.Dir(bin),
			"PATH":       bin + string(os.PathListSeparator) + os.Getenv("PATH"),
			"GOCACHE":    filepath.Join(os.Getenv("TEST_TMPDIR"), "gocache"),
			"GOFLAGS":    "-mod=mod",
		}
		var env []string
		for _, kv := range os.Environ() { // first duplicate wins in Go, so drop the originals
			if k, _, _ := strings.Cut(kv, "="); override[k] == "" {
				env = append(env, kv)
			}
		}
		for k, v := range override {
			env = append(env, k+"="+v)
		}
		t.Fatalf("re-exec failed: %v", syscall.Exec(exe, os.Args, env))
	}
}
