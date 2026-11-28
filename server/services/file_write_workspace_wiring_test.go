package services

// These tests drive the composed SessionService so that the hub's wiring of the fileops and
// pathcomplete leaves (guard, status invalidator, flag registry, delegators) cannot drop silently.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	instancepkg "github.com/tstapler/stapler-squad/session/instance"
)

// staticFinder is a LiveInstanceFinder over one instance (any id resolves to it).
type staticFinder struct{ inst *instancepkg.Instance }

func (f staticFinder) FindLiveInstance(string) *instancepkg.Instance { return f.inst }

func newArchivedInstanceForTest(t *testing.T) *instancepkg.Instance {
	t.Helper()
	inst := hiddenInstance(t, false)
	now := time.Now()
	inst.SetArchivedAt(&now)
	return inst
}

// writeThroughHub saves a.go in inst's directory through SessionService.WriteFile with editing on.
func writeThroughHub(t *testing.T, inst *instancepkg.Instance) (*SessionService, error) {
	t.Helper()
	svc := newGatedSessionService(svctest.CreateTestStorage(t), flippedDefaults(flagsvc.FilesEditingFlagName))
	svc.workspaceSvc.SetLiveFinder(staticFinder{inst})
	path := filepath.Join(inst.Workspace().ActiveDir, "a.go")
	require.NoError(t, os.WriteFile(path, []byte("one\n"), 0o644))
	stat, err := svc.StatFile(context.Background(), connect.NewRequest(&sessionv1.StatFileRequest{SessionId: inst.Title, Path: "a.go"}))
	require.NoError(t, err)
	svc.workspaceSvc.vcs.StoreForTest(inst.Workspace().ActiveDir, nil, time.Now())
	_, err = svc.WriteFile(context.Background(), connect.NewRequest(&sessionv1.WriteFileRequest{
		SessionId: inst.Title, Path: "a.go", ExpectedToken: stat.Msg.VersionToken, Content: []byte("mine\n"),
	}))
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	if err != nil {
		assert.Equal(t, "one\n", string(got), "a refused save writes nothing")
	} else {
		assert.Equal(t, "mine\n", string(got))
	}
	return svc, err
}

func requireWriteRefusal(t *testing.T, err error, code connect.Code, reason sessionv1.ReadOnlyReason) {
	t.Helper()
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, code, ce.Code())
	require.Len(t, ce.Details(), 1)
	detail, derr := ce.Details()[0].Value()
	require.NoError(t, derr)
	rd, ok := detail.(*sessionv1.ReadOnlyReasonDetail)
	require.True(t, ok)
	assert.Equal(t, reason, rd.Reason)
}

func TestSessionServiceWriteFile_ShouldRefuseHiddenSession_ThroughTheWorkspaceGuard(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	inst := hiddenInstance(t, true)

	_, err := writeThroughHub(t, inst)

	requireWriteRefusal(t, err, connect.CodePermissionDenied, sessionv1.ReadOnlyReason_READ_ONLY_REASON_HIDDEN_SESSION)

	// The same guard allows once the live guards flag is off, and allows a visible session.
	ws := &WorkspaceService{liveFinder: staticFinder{inst}}
	off := &access.UnaryGuardsFlag{}
	off.SetEnabled(false)
	ws.SetGuardsFlag(off)
	assert.NoError(t, ws.CheckWriteAccess(inst.Title))
	assert.NoError(t, (&WorkspaceService{liveFinder: staticFinder{hiddenInstance(t, false)}}).CheckWriteAccess(inst.Title))
}

func TestSessionServiceWriteFile_ShouldRefuseArchivedSession_And_NeverReviveIt(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	inst := newArchivedInstanceForTest(t)
	before := inst.Snapshot().ArchivedAt

	_, err := writeThroughHub(t, inst)

	requireWriteRefusal(t, err, connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_WORKTREE_MISSING)
	assert.Equal(t, before, inst.Snapshot().ArchivedAt, "ArchivedAt unchanged")
	assert.True(t, inst.IsArchived())
}

func TestWorkspaceService_ShouldEvictVcsStatusCache_When_InvalidateVcsStatusCalledBySave(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	ws := &WorkspaceService{}
	ws.vcs.StoreForTest("/work/a", nil, time.Now())
	ws.vcs.StoreForTest("/work/b", nil, time.Now())

	ws.InvalidateVCSStatus("/work/a")

	_, _, aCached := ws.vcs.PeekForTest("/work/a")
	_, _, bCached := ws.vcs.PeekForTest("/work/b")
	assert.False(t, aCached, "keyed like GetVCSStatus: the entry for the saved session's dir is gone")
	assert.True(t, bCached, "other sessions' entries stay")

	// A save reaches the cache through the real wiring: WorkspaceService is the StatusInvalidator.
	inst := hiddenInstance(t, false)
	svc, err := writeThroughHub(t, inst)
	require.NoError(t, err)
	_, _, still := svc.workspaceSvc.vcs.PeekForTest(inst.Workspace().ActiveDir)
	assert.False(t, still, "the next GetVCSStatus recomputes instead of waiting out the 15 s TTL")
}

func TestSessionService_ShouldServePathCompletionsAndWorktrees_ThroughPathComplete(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := NewSessionService(svctest.CreateTestStorage(t), nil)
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "alpha"), 0o755))

	resp, err := svc.ListPathCompletions(context.Background(), connect.NewRequest(&sessionv1.ListPathCompletionsRequest{PathPrefix: dir + "/"}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Entries, 1)
	assert.Equal(t, "alpha", resp.Msg.Entries[0].Name)

	trees, err := svc.ListWorktrees(context.Background(), connect.NewRequest(&sessionv1.ListWorktreesRequest{RepoPath: dir}))
	require.NoError(t, err)
	assert.Empty(t, trees.Msg.Worktrees)
}
