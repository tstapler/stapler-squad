package instance

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// recordingIOPM records which pane calls got past a status gate.
type recordingIOPM struct {
	hotRestorablePM
	calls map[string]int
}

func (p recordingIOPM) HasUpdated() (bool, bool, string) {
	p.calls["HasUpdated"]++
	return false, false, ""
}
func (p recordingIOPM) CapturePaneContent() (string, error) {
	p.calls["CapturePaneContent"]++
	return "", nil
}
func (p recordingIOPM) SetDetachedSize(int, int, string) error {
	p.calls["SetDetachedSize"]++
	return nil
}
func (p recordingIOPM) SetWindowSize(int, int) error { p.calls["SetWindowSize"]++; return nil }
func (p recordingIOPM) SendKeys(string) (int, error) { p.calls["SendKeys"]++; return 0, nil }
func (p recordingIOPM) SendInputViaControlMode(context.Context, []byte) error {
	p.calls["SendInputViaControlMode"]++
	return nil
}

// A publisher that built its snapshot before a transition and stored it afterwards must not
// leave a live session refusing input.
func TestInstanceIOGates_should_FollowStatus_When_OlderSnapshotStoredLast(t *testing.T) {
	t.Parallel()
	gates := map[string]struct {
		reaches string
		call    func(*Instance) error
	}{
		"SendKeys":  {"SendKeys", func(i *Instance) error { return i.SendKeys("x") }},
		"SendKeysN": {"SendKeys", func(i *Instance) error { _, err := i.SendKeysN("x"); return err }},
		"SendInputViaControlMode": {"SendInputViaControlMode", func(i *Instance) error {
			return i.SendInputViaControlMode(context.Background(), []byte("x"))
		}},
		"CapturePaneContent": {"CapturePaneContent", func(i *Instance) error { _, err := i.CapturePaneContent(); return err }},
		"CapturePaneContentPriority": {"CapturePaneContent", func(i *Instance) error {
			_, err := i.CapturePaneContentPriority()
			return err
		}},
		"SetWindowSize":  {"SetWindowSize", func(i *Instance) error { return i.SetWindowSize(80, 24) }},
		"SetPreviewSize": {"SetDetachedSize", func(i *Instance) error { return i.SetPreviewSize(80, 24) }},
		"HasUpdated":     {"HasUpdated", func(i *Instance) error { i.HasUpdated(); return nil }},
	}
	for name, gate := range gates {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pm := recordingIOPM{calls: map[string]int{}}
			inst := newStaleSnapshotInstance(t, pm, domain.Paused, domain.Active)

			require.NoError(t, gate.call(inst))
			assert.Equal(t, 1, pm.calls[gate.reaches], "call must reach the pane")
		})
	}
}

func TestInstanceStatusQueries_should_FollowStatus_When_OlderSnapshotStoredLast(t *testing.T) {
	t.Parallel()
	inst := newStaleSnapshotInstance(t, recordingIOPM{calls: map[string]int{}}, domain.Paused, domain.Active)

	assert.True(t, inst.TmuxAlive())
	assert.True(t, inst.terminalScreenReady())
	assert.False(t, inst.PreviewBlocked())
	assert.Equal(t, "●", inst.GetStatusIconForType())
	_, err := inst.RepoName()
	assert.ErrorContains(t, err, "gitWorktree is nil", "must get past the paused gate")
}

// newStaleSnapshotInstance replays the lost update: a snapshot of old is built, the instance
// moves to current, and only then is the old snapshot stored.
func newStaleSnapshotInstance(t *testing.T, pm ProcessManager, old, current domain.Status) *Instance {
	t.Helper()
	inst := &Instance{Title: "stale-snapshot", Status: old, Path: t.TempDir(), IsManaged: true}
	inst.processManager = pm
	inst.started.Store(true)
	FinishInstanceConstruction(inst)
	delayed := inst.Snapshot()
	inst.ForceStatus(current)
	inst.snapshot.Store(delayed)
	return inst
}

func TestGateStatus_should_MatchStatus_When_BuiltByEachConstructionPath(t *testing.T) {
	t.Parallel()
	literal := &Instance{Title: "literal", Status: domain.Paused}
	assert.Equal(t, domain.Paused, literal.gateStatus(), "no write yet: falls back to the literal")
	literal.ForceStatus(domain.Creating)
	assert.Equal(t, domain.Creating, literal.gateStatus(), "the zero Status value is not 'unset'")

	finished := &Instance{Title: "finished", Status: domain.Hibernated}
	FinishInstanceConstruction(finished)
	assert.Equal(t, int32(domain.Hibernated)+1, finished.statusMirror.Load())

	loaded, err := FromInstanceData(domain.InstanceData{Title: "loaded", Status: domain.Paused, Path: t.TempDir()})
	require.NoError(t, err)
	assert.Equal(t, int32(domain.Paused)+1, loaded.statusMirror.Load())
}

// Status writes outside loadStatus would leave the mirror the I/O gates read behind. The
// allow-listed expressions are Status fields of other types.
func TestStatusWrites_should_GoThroughLoadStatus(t *testing.T) {
	t.Parallel()
	otherTypes := map[string]bool{"tasks[i]": true, "newShell": true, "copied": true}
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	var writes []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		file, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				assign, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for _, lhs := range assign.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if ok && sel.Sel.Name == "Status" && !otherTypes[types.ExprString(sel.X)] {
						writes = append(writes, name+":"+fn.Name.Name)
					}
				}
				return true
			})
		}
	}
	assert.Equal(t, []string{"instance_state.go:loadStatus"}, writes)
}
