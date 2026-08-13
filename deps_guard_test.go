package main

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestNoForbiddenDependencies asserts that no banned package is actually compiled
// into the module. It inspects `go list -deps ./...` (the real compiled set, not
// just go.mod) so an indirect reintroduction is caught too.
//
// github.com/shoenig/go-m1cpu has a cgo init() that calls IOKit and segfaults at
// process start on some Apple Silicon machines — before main() runs, so it cannot
// be recovered (see PR #129). It is pulled in transitively by
// github.com/shirou/gopsutil/v3. We migrated to gopsutil/v4, which dropped the
// dependency. This guard turns a startup-crash regression into a deterministic,
// cross-platform test failure instead of a machine-specific SIGSEGV at runtime.
func TestNoForbiddenDependencies(t *testing.T) {
	// import paths that must never re-enter the compiled dependency graph.
	forbiddenDeps := []string{
		"github.com/shoenig/go-m1cpu",
		"github.com/shirou/gopsutil/v3",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "go", "list", "-deps", "./...").Output()
	if err != nil {
		t.Fatalf("go list -deps ./... failed: %v", err)
	}

	deps := strings.Split(string(out), "\n")
	for _, banned := range forbiddenDeps {
		for _, line := range deps {
			if line == banned || strings.HasPrefix(line, banned+"/") {
				t.Errorf("forbidden dependency %q is in the compiled graph (matched %q); "+
					"it must not be reintroduced — see deps_guard_test.go for why", banned, line)
				break
			}
		}
	}
}
