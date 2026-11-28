//go:build !windows

package services

// GetSessionDiff's branches and GetDetectionEvents' limits and lookup.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/claudectl"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/external"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/instance"
)

// changedRepo is a repo with one committed 40-line file and one edited line.
func changedRepo(t *testing.T) (dir, base string) {
	t.Helper()
	dir = fanoutGitRepo(t)
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "line"
	}
	file := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	fanoutGitRun(t, dir, "add", ".")
	fanoutGitRun(t, dir, "commit", "-q", "-m", "base")
	out, err := safeexec.CommandContext(context.Background(), "git", "-C", dir, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	base = strings.TrimSpace(string(out))
	lines[20] = "changed"
	require.NoError(t, os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return dir, base
}

func (e *charEnv) diff(id string, contextLines int32) (*sessionv1.DiffStats, error) {
	resp, err := e.svc.GetSessionDiff(context.Background(), connect.NewRequest(&sessionv1.GetSessionDiffRequest{Id: id, ContextLines: contextLines}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.DiffStats, nil
}

// Not parallel: fanoutGitRepo sets git identity env.
func TestGetSessionDiff_ShouldServeTheCacheByDefaultAndRecomputeForCustomContext_When_Live(t *testing.T) {
	dir, base := changedRepo(t)
	e := newCharEnv(t, true)
	p := newCharPane(t)
	p.inst.SetGitWorktree(git.NewGitWorktreeFromStorage(dir, dir, p.inst.Title, "main", base))
	e.poller.AddInstance(p.inst)
	id := p.inst.Title

	def, err := e.diff(id, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, def.Added, "the default path computes stats that were never cached")
	one, err := e.diff(id, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, one.Added)
	assert.Less(t, len(one.Content), len(def.Content), "custom context bypasses the default-context cache")

	require.NoError(t, os.RemoveAll(dir))
	_, err = e.diff(id, 1)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.ErrorContains(t, err, "failed to compute diff")
	_, err = e.diff(id, 0)
	assert.NoError(t, err, "the default path does not fail the call")
}

// Characterizes today's wiring, not the intent: the hub lists stored sessions
// without their worktree rows, so a finished worktree session is diffed at its
// Path. The worktree branch itself is covered in sessionstream.
//
// Not parallel: fanoutGitRepo sets git identity env.
func TestGetSessionDiff_ShouldDiffThePathNotTheStoredWorktree_When_NoLiveInstance(t *testing.T) {
	dir, base := changedRepo(t)
	e := newCharEnv(t, true)
	stored := instance.NewPausedTestInstance("stored-worktree")
	stored.Path = t.TempDir() // not a repo, so only the worktree could produce a diff
	stored.SetGitWorktree(git.NewGitWorktreeFromStorage(dir, dir, "stored-worktree", "main", base))
	require.NoError(t, e.storage.AddInstance(stored))

	got, err := e.diff("stored-worktree", 0)
	require.NoError(t, err)
	assert.Zero(t, got.Added)
	assert.Empty(t, got.Content)
}

func TestGetDetectionEvents_ShouldReturnTheNewestUpToTheCappedLimit_When_ControllerHasEvents(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	e.poller.AddInstance(p.inst)
	detector := detection.NewStatusDetector()
	detector.SetSessionID(p.inst.Title)
	for range 150 {
		detector.Detect([]byte("some output"))
	}
	controller := claudectl.NewBareForTest(p.inst.Title, nil)
	controller.SetDetectorsForTest(detector, nil)
	statusMgr := instance.NewInstanceStatusManager()
	statusMgr.RegisterController(p.inst.Title, controller)
	e.svc.SetStatusManager(statusMgr)

	get := func(limit int32) []*sessionv1.DetectionEventProto {
		resp, err := e.svc.GetDetectionEvents(context.Background(), connect.NewRequest(&sessionv1.GetDetectionEventsRequest{SessionId: p.inst.Title, Limit: limit}))
		require.NoError(t, err)
		return resp.Msg.Events
	}
	assert.Len(t, get(0), 20)
	assert.Len(t, get(-1), 20)
	assert.Len(t, get(5), 5)
	assert.Len(t, get(100), 100)
	capped := get(500)
	require.Len(t, capped, 100)
	assert.Equal(t, p.inst.Title, capped[0].SessionId)
	assert.False(t, capped[0].Timestamp.AsTime().Before(capped[99].Timestamp.AsTime()), "newest first")
}

// A session known only to external discovery is found by the stream reads.
func TestStreamReads_ShouldFindExternalSessions_When_OnlyDiscoveryKnowsThem(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	ext := newCharPane(t)
	discovery := external.NewExternalSessionDiscovery()
	discovery.AddSessionForTest("/tmp/reads.sock", ext.inst)
	e.svc.SetExternalDiscovery(discovery)

	assert.Same(t, ext.inst, e.svc.streamPorts().FindInstance("/tmp/reads.sock"))
	_, err := e.svc.GetDetectionEvents(context.Background(), connect.NewRequest(&sessionv1.GetDetectionEventsRequest{SessionId: "/tmp/reads.sock"}))
	assert.NoError(t, err)
}
