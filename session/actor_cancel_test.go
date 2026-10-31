package session

// actor_cancel_test.go pins the cancellation contract of callActor: when the
// caller (or the actor) is cancelled while a command is still running, the call
// returns early without handing the caller any memory the command still writes.

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type blockedCommand struct {
	started chan struct{}
	release chan struct{}
	ran     chan struct{}
}

func newBlockedCommand() blockedCommand {
	return blockedCommand{
		started: make(chan struct{}),
		release: make(chan struct{}),
		ran:     make(chan struct{}),
	}
}

// run blocks the actor goroutine until release is closed, then returns 42.
func (b blockedCommand) run(*instanceState) (int, error) {
	close(b.started)
	<-b.release
	defer close(b.ran)
	return 42, nil
}

type valueResult struct {
	val int
	err error
}

func TestSendCtxValue_should_ReturnCtxErrAndZero_When_CancelledWhileCommandRuns(t *testing.T) {
	t.Parallel()
	li := NewLiveInstance(newActorTestInstance(t))
	t.Cleanup(li.Stop)
	cmd := newBlockedCommand()
	ctx, cancel := context.WithCancel(context.Background())

	got := make(chan valueResult, 1)
	go func() {
		v, err := sendCtxValue(ctx, li.Instance, cmd.run)
		got <- valueResult{v, err}
	}()
	<-cmd.started
	cancel()
	r := <-got
	// The command is still blocked: the caller already has its answer, and
	// nothing it holds aliases the command's result. Under -race this fails if
	// the helper ever shares state with the running command.
	assert.ErrorIs(t, r.err, context.Canceled)
	assert.Zero(t, r.val)

	close(cmd.release)
	<-cmd.ran // outcome-unknown semantics: the cancelled command still completes
}

func TestSendSyncValue_should_ReturnActorErrAndZero_When_ActorStopsWhileCommandRuns(t *testing.T) {
	t.Parallel()
	li := NewLiveInstance(newActorTestInstance(t))
	cmd := newBlockedCommand()

	got := make(chan valueResult, 1)
	go func() {
		v, err := sendSyncValue(li.Instance, cmd.run)
		got <- valueResult{v, err}
	}()
	<-cmd.started
	li.cancel() // Stop() would wait for the blocked command; cancel only signals
	r := <-got
	assert.ErrorIs(t, r.err, context.Canceled)
	assert.Zero(t, r.val)

	close(cmd.release)
	<-cmd.ran
	li.Stop()
}

func TestSendCtxValue_should_NotEnqueue_When_CallerCtxAlreadyCancelled(t *testing.T) {
	t.Parallel()
	li := NewLiveInstance(newActorTestInstance(t))
	t.Cleanup(li.Stop)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ran := false
	_, err := sendCtxValue(ctx, li.Instance, func(*instanceState) (int, error) {
		ran = true
		return 1, nil
	})

	assert.ErrorIs(t, err, context.Canceled)
	require.NoError(t, li.sendSync(func(*Instance) {})) // drain: any enqueued command has run
	assert.False(t, ran, "a command rejected before enqueue must never run")
}

func TestSendSyncValue_should_RunInline_When_NoLiveInstance(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "no-actor"}

	v, err := sendSyncValue(inst, func(*instanceState) (int, error) { return 7, nil })

	require.NoError(t, err)
	assert.Equal(t, 7, v)
}

// TestActorCommandsDoNotWriteCapturedVariables is the gate for the
// sendSyncErr/sendCtx early-return race: a closure passed to either helper must
// not assign to a variable declared outside it, because the caller may read that
// variable after an early ctx return while the command is still writing it.
// Return results through sendSyncValue/sendCtxValue instead.
func TestActorCommandsDoNotWriteCapturedVariables(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	var offenders []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		require.NoError(t, err)
		ast.Inspect(af, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "sendSyncErr" && sel.Sel.Name != "sendCtx") {
				return true
			}
			if lit, ok := call.Args[len(call.Args)-1].(*ast.FuncLit); ok {
				for _, name := range outerWrites(lit) {
					offenders = append(offenders, fset.Position(call.Pos()).String()+" writes "+name)
				}
			}
			return true
		})
	}
	assert.Empty(t, offenders, "use sendSyncValue/sendCtxValue to return results from an actor command")
}

// outerWrites lists identifiers assigned inside lit that it did not declare.
func outerWrites(lit *ast.FuncLit) []string {
	declared := map[string]bool{}
	for _, p := range lit.Type.Params.List {
		for _, nm := range p.Names {
			declared[nm.Name] = true
		}
	}
	var outer []string
	note := func(e ast.Expr) {
		if id, ok := e.(*ast.Ident); ok && id.Name != "_" && !declared[id.Name] {
			outer = append(outer, id.Name)
		}
	}
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, l := range x.Lhs {
				if id, ok := l.(*ast.Ident); ok && x.Tok == token.DEFINE {
					declared[id.Name] = true
				} else {
					note(l)
				}
			}
		case *ast.IncDecStmt:
			note(x.X)
		case *ast.ValueSpec:
			for _, nm := range x.Names {
				declared[nm.Name] = true
			}
		}
		return true
	})
	return outer
}
