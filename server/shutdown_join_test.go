package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/testutil/wait"
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

// newStapleStacks returns the stacks of stapler-squad goroutines that did not
// exist in before, excluding the calling test's own goroutine.
func newStapleStacks(before map[string]string) []string {
	var out []string
	for id, stack := range goroutineStacks() {
		if _, existed := before[id]; existed {
			continue
		}
		if !strings.Contains(stack, "stapler-squad/") || strings.Contains(stack, "TestShutdown_JoinsAllWireDepsGoroutines") {
			continue
		}
		out = append(out, stack)
	}
	return out
}

// TestShutdown_JoinsAllWireDepsGoroutines guards the "directory not empty"
// TempDir flake: no goroutine wireDepsIntoServer starts may outlive Shutdown(),
// or it can write into the state dir afterwards. Goroutines present before
// NewServerWithDeps (BuildDependencies' own, which have their own lifetime) are
// excluded. Two checks, because each alone has a blind spot:
//
//  1. Registration (deterministic, see TestWireDepsIntoServer_HasNoBareGoStatements):
//     wireDepsIntoServer must not contain a bare `go` statement.
//  2. Liveness (best effort, this test): no new goroutine may still be running
//     after Shutdown returns. This catches an unjoined goroutine inside a
//     component's own Start, but only if it is still alive when the check runs.
//     The check polls briefly because a joined helper (e.g. StatsWriter.FlushFinal's
//     save goroutine) closes its done channel a few instructions before it exits;
//     a goroutine that is truly leaked outlives the window and still fails.
func TestShutdown_JoinsAllWireDepsGoroutines(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	deps, err := BuildDependencies()
	require.NoError(t, err)

	before := goroutineStacks()
	srv := NewServerWithDeps("localhost:0", deps)
	require.NoError(t, srv.Shutdown())

	var stragglers []string
	_ = wait.WaitForCondition(func() bool {
		stragglers = newStapleStacks(before)
		return len(stragglers) == 0
	}, wait.WaitConfig{Timeout: 2 * time.Second, PollInterval: 10 * time.Millisecond, Description: "wireDepsIntoServer goroutines to exit"})
	require.Empty(t, stragglers, "goroutines started by wireDepsIntoServer outlived Shutdown():\n%s", strings.Join(stragglers, "\n\n"))
}

// TestWireDepsIntoServer_HasNoBareGoStatements fails if wireDepsIntoServer
// launches a goroutine with a bare `go`. Every one must go through
// Server.goBackground or joinAfterCancel (or a Start* helper taking
// &srv.backgroundTasksWG) so Shutdown joins it. Checked on the AST because a
// goroutine that exits instantly on cancel is invisible to a runtime check.
func TestWireDepsIntoServer_HasNoBareGoStatements(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "server.go", nil, 0)
	require.NoError(t, err)

	var found bool
	var bare []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "wireDepsIntoServer" {
			continue
		}
		found = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if g, ok := n.(*ast.GoStmt); ok {
				bare = append(bare, fset.Position(g.Pos()).String())
			}
			return true
		})
	}
	require.True(t, found, "wireDepsIntoServer not found in server.go; update this test if it was renamed")
	require.Empty(t, bare, "bare `go` in wireDepsIntoServer; use srv.goBackground or srv.joinAfterCancel so Shutdown joins it")
}
