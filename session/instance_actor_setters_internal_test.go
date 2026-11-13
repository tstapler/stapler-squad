package session

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// TestAllTagRelevantSetters_should_CallReclassifyTagsLocked_When_SourceScanned re-runs Task
// 3.3.1d's grep/AST enumeration of every xxxLocked setter that mutates
// Branch/Path/Program/Title in instance_actor_setters.go, and asserts each one's function
// body calls reclassifyTagsLocked — a structural safeguard against a future setter being
// wired incorrectly (architecture-review.md Blocker 3). Deliberately scoped to
// instance_actor_setters.go only; instance_worktree.go's unlocked setupFirstTimeWorktree
// path is covered separately by Story 3.3.2's ReclassifyTagsAfterCreate tests.
func TestAllTagRelevantSetters_should_CallReclassifyTagsLocked_When_SourceScanned(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "instance_actor_setters.go", nil, 0)
	require.NoError(t, err, "expected to parse instance_actor_setters.go")

	tagRelevantFields := map[string]bool{"Branch": true, "Path": true, "Program": true, "Title": true}

	checked := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !strings.HasSuffix(fn.Name.Name, "Locked") {
			continue
		}
		if !mutatesTagRelevantFieldOnInst(fn.Body, tagRelevantFields) {
			continue
		}
		checked++
		assert.True(t, callsReclassifyTagsLocked(fn.Body),
			"%s mutates a tag-relevant field (Branch/Path/Program/Title) but never calls reclassifyTagsLocked", fn.Name.Name)
	}
	assert.GreaterOrEqual(t, checked, 3, "expected at least the 3 setters enumerated by Task 3.3.1d (setProgramLocked, setTitleDirectLocked, setGitHubResolutionLocked)")
}

// mutatesTagRelevantFieldOnInst reports whether body contains an assignment to
// s.inst.<Field> for one of fields — an AST-based replacement for the former
// `s\.inst\.(Branch|Path|Program|Title)\s*=` regex, which broke on reformatting.
func mutatesTagRelevantFieldOnInst(body ast.Node, fields map[string]bool) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if !ok || !fields[sel.Sel.Name] {
				continue
			}
			inst, ok := sel.X.(*ast.SelectorExpr)
			if !ok || inst.Sel.Name != "inst" {
				continue
			}
			if recv, ok := inst.X.(*ast.Ident); ok && recv.Name == "s" {
				found = true
			}
		}
		return true
	})
	return found
}

// callsReclassifyTagsLocked reports whether body contains a call to reclassifyTagsLocked
// anywhere in its statement tree (including inside nested blocks/closures).
func callsReclassifyTagsLocked(body ast.Node) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "reclassifyTagsLocked" {
			found = true
		}
		return true
	})
	return found
}

// If this fails, a caller on a request goroutine can be parked indefinitely by
// one busy actor: sendSyncErr (which plain SetArchivedAtIfNil uses) has no
// deadline at all, so ArchiveWorkflowSessions' per-instance loop would stall the
// whole RPC on a single instance mid-Start.
func TestSetArchivedAtIfNilCtx_should_ReturnCtxError_When_ActorIsBusy(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "archive-ctx-busy", Status: domain.Stopped}
	li := NewLiveInstance(inst)
	t.Cleanup(li.cancel)

	// Occupy the actor goroutine so no further command can be executed.
	occupied := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	inst.send(func(*instanceState) {
		close(occupied)
		<-release
	})
	<-occupied

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := inst.SetArchivedAtIfNilCtx(ctx, time.Now())
		errCh <- err
	}()
	cancel() // stands in for the caller's deadline firing

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("SetArchivedAtIfNilCtx did not return once its context was cancelled")
	}
}

// If this fails, the bounded variant has diverged from SetArchivedAtIfNil's CAS
// semantics on the normal (uncontended) path.
func TestSetArchivedAtIfNilCtx_should_ApplyCASOnce_When_ActorIsIdle(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "archive-ctx-idle", Status: domain.Stopped}
	li := NewLiveInstance(inst)
	t.Cleanup(li.cancel)

	ctx := context.Background()
	set, err := inst.SetArchivedAtIfNilCtx(ctx, time.Now())
	require.NoError(t, err)
	assert.True(t, set, "first archive should apply (CAS)")
	assert.True(t, inst.IsArchived(), "IsArchived() must see the published snapshot")

	set, err = inst.SetArchivedAtIfNilCtx(ctx, time.Now())
	require.NoError(t, err)
	assert.False(t, set, "second archive is a no-op (CAS)")
}
