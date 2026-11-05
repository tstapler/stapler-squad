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
	"time"

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
	// The command is still blocked, yet the caller already has its answer. The
	// caller-level race regression is TestSetArchivedAtIfNilCtx_should_NotRace...
	assert.ErrorIs(t, r.err, context.Canceled)
	assert.Zero(t, r.val)

	close(cmd.release)
	<-cmd.ran // outcome-unknown semantics: the cancelled command still completes
}

// TestSetArchivedAtIfNilCtx_should_NotRaceWithRunningCommand_When_CtxCancelled is
// the caller-level red/green test: with the old closure-captured `set`, the
// caller's read after the early return raced with the command's later write, and
// -race flagged it.
func TestSetArchivedAtIfNilCtx_should_NotRaceWithRunningCommand_When_CtxCancelled(t *testing.T) {
	t.Parallel()
	inst := newActorTestInstance(t)
	li := NewLiveInstance(inst)
	t.Cleanup(li.Stop)
	gate := newBlockedCommand()
	go func() { _ = inst.sendSyncErr(func(*instanceState) error { _, err := gate.run(nil); return err }) }()
	<-gate.started // actor is now busy; the next command queues behind it

	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan struct {
		set bool
		err error
	}, 1)
	go func() {
		set, err := inst.SetArchivedAtIfNilCtx(ctx, time.Now())
		got <- struct {
			set bool
			err error
		}{set, err}
	}()
	// Cancel only once the caller's command is enqueued behind the busy actor.
	require.Eventually(t, func() bool { return len(li.mailbox) == 1 }, 10*time.Second, time.Millisecond)
	cancel()
	// Release before receiving the caller's result: the caller's read of its
	// return value then has no happens-before edge to the command's write, which
	// is exactly what -race needs to see to flag a shared variable.
	close(gate.release)
	r := <-got
	<-gate.ran

	assert.ErrorIs(t, r.err, context.Canceled)
	assert.False(t, r.set)
	require.NoError(t, li.sendSync(func(*Instance) {}))
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

// TestActorCommandsDoNotWriteCapturedVariables is the gate for the early-return
// race: a closure passed to an actor helper must not write to anything reachable
// from a variable declared outside it (x = ..., x.f = ..., x[k] = ..., *x = ...,
// x++), because the caller may read that variable after an early ctx return while
// the command is still writing it. Return results through sendSyncValue or
// sendCtxValue instead. The gate is deliberately stricter than "the caller reads
// it": it cannot see the caller. It also cannot see mutation via method calls
// (append, x.Set(...)) on captured values.
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
		af, err := parser.ParseFile(fset, f, nil, 0) // mode 0 keeps object resolution: scoping comes from ast.Ident.Obj
		require.NoError(t, err)
		ast.Inspect(af, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 || !isActorHelper(call.Fun) || isActorHelperDefinitionSite(f) {
				return true
			}
			lit, ok := call.Args[len(call.Args)-1].(*ast.FuncLit)
			if !ok {
				offenders = append(offenders, fset.Position(call.Pos()).String()+" passes a non-literal command (gate cannot inspect it)")
				return true
			}
			for _, name := range outerWrites(lit) {
				offenders = append(offenders, fset.Position(call.Pos()).String()+" writes "+name)
			}
			return true
		})
	}
	assert.Empty(t, offenders, "use sendSyncValue/sendCtxValue to return results from an actor command")
}

// isActorHelperDefinitionSite exempts actor.go, whose wrappers forward a
// parameter closure rather than defining a command.
func isActorHelperDefinitionSite(file string) bool { return file == "actor.go" }

func isActorHelper(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		return f.Sel.Name == "sendSyncErr" || f.Sel.Name == "sendCtx"
	case *ast.IndexExpr: // explicit instantiation: sendSyncValue[T](...)
		return isActorHelper(f.X)
	case *ast.Ident:
		return f.Name == "sendSyncValue" || f.Name == "sendCtxValue"
	}
	return false
}

// outerWrites lists the root identifiers of write targets inside lit whose
// declaration lies outside lit. Scoping uses the parser's identifier resolution,
// so shadowing, range variables, named results and nested func params are exact.
func outerWrites(lit *ast.FuncLit) []string {
	var outer []string
	note := func(target ast.Expr) {
		id := rootIdent(target)
		if id == nil || id.Name == "_" || id.Obj == nil {
			return // blank, package-level, or not a variable
		}
		if pos := id.Obj.Pos(); pos < lit.Pos() || pos >= lit.End() {
			outer = append(outer, id.Name)
		}
	}
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok != token.DEFINE {
				for _, l := range x.Lhs {
					note(l)
				}
			}
		case *ast.RangeStmt:
			if x.Tok == token.ASSIGN {
				note(x.Key)
				note(x.Value)
			}
		case *ast.IncDecStmt:
			note(x.X)
		}
		return true
	})
	return outer
}

// rootIdent peels selectors, indexing, slicing, derefs and parens off a write target.
func rootIdent(e ast.Expr) *ast.Ident {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x
		case *ast.SelectorExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		default:
			return nil
		}
	}
}
