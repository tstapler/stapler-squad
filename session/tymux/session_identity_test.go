package tymux

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

func namedSession(id, name, dir string) *v1.Session {
	s := fakeSession(id, id+"-pane", dir, v1.Liveness_LIVENESS_LIVE)
	s.Name = name
	return s
}

func TestStart_NameCarriesSessionLabel(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{"": dir, "alpha": dir + " [alpha]"}
	for label, want := range cases {
		var gotName string
		transport := &fakeTransport{
			createSessionFn: func(_ context.Context, req *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
				gotName = req.Msg.GetName()
				return connect.NewResponse(fakeSession("id-"+label, "pane-"+label, dir, v1.Liveness_LIVENESS_LIVE)), nil
			},
		}
		sess := NewTymuxGRPCSession(transport)
		sess.SetSessionLabel(label)
		require.NoError(t, sess.Start(dir))
		t.Cleanup(func() { _ = sess.Close() })
		assert.Equal(t, want, gotName, "label %q", label)
	}
}

func TestRestoreWithWorkDir_NeverAdoptsSessionClaimedByAnotherOwner(t *testing.T) {
	dir := t.TempDir()
	legacy := namedSession("legacy-1", dir, dir)
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{legacy}}), nil
		},
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(namedSession("fresh-2", dir+" [b]", dir)), nil
		},
	}
	a := NewTymuxGRPCSession(transport)
	a.SetSessionLabel("a")
	require.NoError(t, a.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = a.Close() })
	require.Equal(t, "legacy-1", a.GetSessionIdentifier())

	b := NewTymuxGRPCSession(transport)
	b.SetSessionLabel("b")
	require.NoError(t, b.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = b.Close() })
	assert.Equal(t, "fresh-2", b.GetSessionIdentifier(), "b must not share a's legacy session")
}

func TestRestoreWithWorkDir_PrefersLabeledOverLegacy(t *testing.T) {
	dir := t.TempDir()
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{
				namedSession("legacy-p", dir, dir),
				namedSession("labeled-p", dir+" [c]", dir),
			}}), nil
		},
	}
	c := NewTymuxGRPCSession(transport)
	c.SetSessionLabel("c")
	require.NoError(t, c.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = c.Close() })
	assert.Equal(t, "labeled-p", c.GetSessionIdentifier())
}

func TestClose_ReleasesClaim(t *testing.T) {
	dir := t.TempDir()
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(namedSession("claim-r", dir, dir)), nil
		},
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))
	_, claimed := claimedSessions.Load("claim-r")
	require.True(t, claimed)
	require.NoError(t, sess.Close())
	_, claimed = claimedSessions.Load("claim-r")
	assert.False(t, claimed)
}

func TestClose_NotFoundFromKillIsSuccess(t *testing.T) {
	dir := t.TempDir()
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(namedSession("gone-k", dir, dir)), nil
		},
		killSessionFn: func(context.Context, *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("no such session"))
		},
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))
	assert.NoError(t, sess.Close())
}

func TestIsAlive_NotFoundMeansDead(t *testing.T) {
	dir := t.TempDir()
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(namedSession("gone-a", dir, dir)), nil
		},
		capturePaneFn: func(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("no such pane"))
		},
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))
	t.Cleanup(func() { _ = sess.Close() })
	assert.False(t, sess.IsAlive())
	assert.False(t, sess.IsAlive(), "liveness stays DEAD, not the stale cached LIVE")
}

func TestRestoreWithWorkDir_StoredIDSurvivesLabelChange(t *testing.T) {
	dir := t.TempDir()
	starts := 0
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{
				namedSession("renamed-1", dir+" [old-title]", dir),
			}}), nil
		},
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			starts++
			return connect.NewResponse(namedSession("dup-2", dir, dir)), nil
		},
	}
	sess := NewTymuxGRPCSession(transport)
	sess.SetSessionLabel("new-title") // Instance was renamed before the restart
	sess.SetStoredSessionID("renamed-1")
	require.NoError(t, sess.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = sess.Close() })

	assert.Equal(t, "renamed-1", sess.GetSessionIdentifier())
	assert.Zero(t, starts, "no second agent session may be created")
}

func TestRestoreWithWorkDir_ConcurrentRestoresNeverShareASession(t *testing.T) {
	dir := t.TempDir()
	const n = 16
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{namedSession("only-1", dir, dir)}}), nil
		},
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(namedSession(fmt.Sprintf("fresh-%d", atomic.AddInt32(&freshCounter, 1)), dir, dir)), nil
		},
	}
	sessions := make([]TymuxManager, n)
	var wg sync.WaitGroup
	for i := range sessions {
		sessions[i] = NewTymuxGRPCSession(transport)
		wg.Add(1)
		go func(s TymuxManager) {
			defer wg.Done()
			_ = s.RestoreWithWorkDir(dir)
		}(sessions[i])
	}
	wg.Wait()
	seen := map[string]int{}
	for _, s := range sessions {
		seen[s.GetSessionIdentifier()]++
	}
	for id, c := range seen {
		assert.Equal(t, 1, c, "session %s adopted by %d owners", id, c)
	}
	for _, s := range sessions {
		_ = s.Close()
	}
}

var freshCounter int32

func TestClose_DoesNotKillSessionClaimedByAnotherOwner(t *testing.T) {
	dir := t.TempDir()
	killed := 0
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(namedSession("shared-k", dir, dir)), nil
		},
		killSessionFn: func(context.Context, *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			killed++
			return connect.NewResponse(&v1.KillSessionResponse{}), nil
		},
	}
	winner := NewTymuxGRPCSession(transport)
	require.NoError(t, winner.Start(dir))
	t.Cleanup(func() { _ = winner.Close() })
	loser := NewTymuxGRPCSession(transport)
	require.NoError(t, loser.Start(dir)) // daemon handed back the same id; loser never holds the claim
	require.NoError(t, loser.Close())
	assert.Zero(t, killed, "a non-owner's Close must not kill the owner's session")
}

func TestIsAlive_NotFoundButSessionListed_ReportsDeadThenStartKillsStale(t *testing.T) {
	dir := t.TempDir()
	var killedIDs []string
	created := 0
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			created++
			return connect.NewResponse(namedSession(fmt.Sprintf("gen-%d", created), dir, dir)), nil
		},
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{deadSession("gen-1", dir)}}), nil
		},
		capturePaneFn: func(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("no such pane"))
		},
		killSessionFn: func(_ context.Context, r *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			killedIDs = append(killedIDs, r.Msg.GetSessionId())
			return connect.NewResponse(&v1.KillSessionResponse{}), nil
		},
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))
	require.False(t, sess.IsAlive())
	require.NoError(t, sess.Start(dir)) // health-check recovery
	t.Cleanup(func() { _ = sess.Close() })
	assert.Equal(t, []string{"gen-1"}, killedIDs, "recovery must not leak the stale same-owner session")
	assert.Equal(t, "gen-2", sess.GetSessionIdentifier())
}

func TestIsAlive_NotFoundAndListUnreachable_FallsBackToCached(t *testing.T) {
	dir := t.TempDir()
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(namedSession("flaky-1", dir, dir)), nil
		},
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("down"))
		},
		capturePaneFn: func(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("no such pane"))
		},
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))
	t.Cleanup(func() { _ = sess.Close() })
	assert.True(t, sess.IsAlive(), "NotFound is not definitive unless a daemon round trip confirms it")
}

// resetClaims gives a test a fresh claim registry: tests reuse fixed daemon
// session ids, and claimedSessions is process-global.
func resetClaims(t *testing.T) {
	t.Helper()
	wipe := func() {
		claimedSessions.Range(func(k, _ any) bool { claimedSessions.Delete(k); return true })
	}
	wipe()
	t.Cleanup(wipe)
}

func deadSession(id, dir string) *v1.Session {
	s := namedSession(id, dir, dir)
	s.Liveness = v1.Liveness_LIVENESS_DEAD
	return s
}

func TestStart_DoesNotKillStaleSessionReadoptedByAnotherOwner(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	var killed []string
	var listedID string // the id a's Start created; the daemon lists it dead
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(namedSession(fmt.Sprintf("m6-%d", atomic.AddInt32(&freshCounter, 1)), dir, dir)), nil
		},
		killSessionFn: func(_ context.Context, r *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			killed = append(killed, r.Msg.GetSessionId())
			return connect.NewResponse(&v1.KillSessionResponse{}), nil
		},
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{deadSession(listedID, dir)}}), nil
		},
	}
	a := NewTymuxGRPCSession(transport)
	require.NoError(t, a.Start(dir))
	oldID := a.GetSessionIdentifier()
	listedID = oldID
	other := NewTymuxGRPCSession(transport)
	claimedSessions.Store(oldID, any(other)) // another owner re-adopted a's cached id
	require.NoError(t, a.Start(dir))
	t.Cleanup(func() { _ = a.Close() })
	assert.NotContains(t, killed, oldID)
}

func TestRestoreWithWorkDir_StoredIDAbsent_FallsBackToLabel(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	starts := 0
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{namedSession("by-label", dir+" [t]", dir)}}), nil
		},
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			starts++
			return connect.NewResponse(namedSession("new-one", dir, dir)), nil
		},
	}
	s := NewTymuxGRPCSession(transport)
	s.SetSessionLabel("t")
	s.SetStoredSessionID("gone-id")
	require.NoError(t, s.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = s.Close() })
	assert.Equal(t, "by-label", s.GetSessionIdentifier())
	assert.Zero(t, starts)
}

func TestRestoreWithWorkDir_StoredIDClaimedByOther_DoesNotFallBack(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{
				namedSession("held", dir+" [t]", dir),
			}}), nil
		},
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(namedSession("fresh-held", dir+" [t]", dir)), nil
		},
	}
	claimedSessions.Store("held", any(NewTymuxGRPCSession(transport)))
	s := NewTymuxGRPCSession(transport)
	s.SetSessionLabel("t")
	s.SetStoredSessionID("held")
	require.NoError(t, s.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = s.Close() })
	assert.Equal(t, "fresh-held", s.GetSessionIdentifier())
}

func TestIsAlive_NotFoundButSessionListedLiveUnderNewPane_StaysAliveAndRecachesPane(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	respawned := namedSession("resp-1", dir, dir)
	respawned.Windows = fakeSession("resp-1", "new-pane", dir, v1.Liveness_LIVENESS_LIVE).Windows
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(namedSession("resp-1", dir, dir)), nil
		},
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{respawned}}), nil
		},
		capturePaneFn: func(_ context.Context, r *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			if r.Msg.GetPaneId() == "new-pane" {
				return connect.NewResponse(&v1.PaneSnapshot{PaneId: "new-pane", Liveness: v1.Liveness_LIVENESS_LIVE}), nil
			}
			return nil, connect.NewError(connect.CodeNotFound, errors.New("no such pane"))
		},
	}
	s := NewTymuxGRPCSession(transport)
	require.NoError(t, s.Start(dir))
	t.Cleanup(func() { _ = s.Close() })
	assert.True(t, s.IsAlive())
	assert.True(t, s.IsAlive(), "second call uses the re-cached pane")
}

func TestPersistedSessionID_StoredBeforeRestore_ClearedOnClose(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{namedSession("p-1", dir, dir)}}), nil
		},
	}
	s := NewTymuxGRPCSession(transport)
	s.SetStoredSessionID("p-1")
	assert.Equal(t, "p-1", s.PersistedSessionID(), "not-yet-restored instance keeps its loaded id")
	require.NoError(t, s.RestoreWithWorkDir(dir))
	assert.Equal(t, "p-1", s.PersistedSessionID())
	require.NoError(t, s.Close())
	assert.Empty(t, s.PersistedSessionID(), "a killed session must not be persisted")
}
