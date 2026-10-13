package server

import (
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newShutdownOnCleanupServer builds a Server from deps and registers its
// Shutdown with t.Cleanup. Cleanups run LIFO, so Shutdown runs before a
// t.TempDir registered earlier is removed: background goroutines must have
// stopped touching the state dir by then.
func newShutdownOnCleanupServer(t *testing.T, addr string, deps *ServerDependencies) *Server {
	t.Helper()
	srv := NewServerWithDeps(addr, deps)
	t.Cleanup(func() {
		if err := srv.Shutdown(); err != nil {
			t.Logf("srv.Shutdown: %v", err)
		}
	})
	return srv
}

var goroutineHeader = regexp.MustCompile(`(?m)^goroutine (\d+) \[`)

// goroutineStacks returns every goroutine's stack keyed by goroutine ID.
func goroutineStacks() map[string]string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	out := map[string]string{}
	for _, block := range strings.Split(string(buf), "\n\n") {
		if m := goroutineHeader.FindStringSubmatch(block); m != nil {
			out[m[1]] = block
		}
	}
	return out
}

// TestShutdown_JoinsAllWireDepsGoroutines pins that every goroutine
// wireDepsIntoServer starts has exited by the time Shutdown returns, so none
// can write into the state dir afterwards (the cause of the "TempDir RemoveAll
// cleanup: directory not empty" flake). It deliberately does not retry the way
// goleak.VerifyNone does: a retry would wait out the very straggler we are
// asserting cannot exist. Goroutines present before NewServerWithDeps
// (BuildDependencies' own, which have their own lifetime) are excluded.
func TestShutdown_JoinsAllWireDepsGoroutines(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	deps, err := BuildDependencies()
	require.NoError(t, err)

	before := goroutineStacks()
	srv := NewServerWithDeps("localhost:0", deps)
	require.NoError(t, srv.Shutdown())

	var stragglers []string
	for id, stack := range goroutineStacks() {
		if _, existed := before[id]; existed {
			continue
		}
		if !strings.Contains(stack, "stapler-squad/") || strings.Contains(stack, "TestShutdown_JoinsAllWireDepsGoroutines") {
			continue
		}
		stragglers = append(stragglers, stack)
	}
	require.Empty(t, stragglers, "goroutines started by wireDepsIntoServer outlived Shutdown():\n%s", strings.Join(stragglers, "\n\n"))
}
