package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sync"
	"testing"
)

// T-OB-21: the stats flush runs after the shutdown hooks and the background
// join, so it sees the last publishes those steps can still produce.
func TestShutdown_ShouldRunFinalStatsFlushAfterHooksAndBackgroundJoin_WhenServerStopped(t *testing.T) {
	srv := newTestServer("localhost:0")
	var mu sync.Mutex
	var order []string
	note := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}
	hookRan := make(chan struct{})
	srv.shutdownHooks = append(srv.shutdownHooks, func() { note("hook"); close(hookRan) })
	srv.backgroundTasksWG.Add(1)
	go func() {
		<-hookRan
		note("background-joined")
		srv.backgroundTasksWG.Done()
	}()
	srv.finalStatsFlush = func() { note("flush") }

	if err := srv.Shutdown(); err != nil {
		t.Fatalf("Shutdown() = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 || order[2] != "flush" {
		t.Fatalf("order = %v, want the flush last", order)
	}
}

// The flush must be a defer registered before httpServer.Shutdown's early
// `return err`, otherwise an error from it would skip the flush.
func TestShutdown_ShouldRegisterTheFinalStatsFlushWithDeferBeforeHttpShutdown_WhenSourceIsParsed(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "server.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var deferPos, httpShutdownPos token.Pos
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Shutdown" || fn.Recv == nil {
			return true
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.DeferStmt:
				if sel, ok := x.Call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "finalStatsFlush" {
					deferPos = x.Pos()
				}
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Shutdown" {
					if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "httpServer" {
						httpShutdownPos = x.Pos()
					}
				}
			}
			return true
		})
		return false
	})
	if deferPos == token.NoPos || httpShutdownPos == token.NoPos || deferPos > httpShutdownPos {
		t.Fatalf("defer s.finalStatsFlush() at %v must precede httpServer.Shutdown at %v", deferPos, httpShutdownPos)
	}
}
