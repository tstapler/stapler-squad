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
		storedIDHolders.reset()
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
	assert.Equal(t, "by-label", s.GetSessionIdentifier(), "a stale id must re-adopt the record's own label session, not spawn a duplicate agent")
	assert.Zero(t, starts)
}

// The label session another record stores as its id is not this record's to
// adopt, even when this record's own stored id has gone stale.
func TestRestoreWithWorkDir_StoredIDAbsent_DoesNotAdoptLabelSessionStoredByAnotherRecord(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	starts := 0
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{namedSession("theirs", dir, dir)}}), nil
		},
		createSessionFn: func(context.Context, *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			starts++
			return connect.NewResponse(namedSession("new-one", dir, dir)), nil
		},
	}
	other := NewTymuxGRPCSession(transport) // record that stores "theirs", not restored yet
	other.SetSessionLabel("o")
	other.SetStoredSessionID("theirs")
	s := NewTymuxGRPCSession(transport)
	s.SetSessionLabel("t")
	s.SetStoredSessionID("gone-id")
	require.NoError(t, s.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = s.Close() })
	assert.Equal(t, "new-one", s.GetSessionIdentifier())
	assert.Equal(t, 1, starts)
}

// A legacy record (no id) restoring before a record that stores its label
// session's id must not take that session: two holders would let the second
// Close kill the first's session.
func TestRestoreWithWorkDir_LegacyRecordDoesNotDoubleHoldAnotherRecordsStoredID(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	var killed []string
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{namedSession("held", dir, dir)}}), nil
		},
		createSessionFn: func(context.Context, *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(namedSession("legacy-fresh", dir, dir)), nil
		},
		killSessionFn: func(_ context.Context, r *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			killed = append(killed, r.Msg.GetSessionId())
			return connect.NewResponse(&v1.KillSessionResponse{}), nil
		},
	}
	stored := NewTymuxGRPCSession(transport)
	stored.SetSessionLabel("s")
	stored.SetStoredSessionID("held")
	legacy := NewTymuxGRPCSession(transport)
	legacy.SetSessionLabel("l")
	require.NoError(t, legacy.RestoreWithWorkDir(dir)) // restores first
	require.NoError(t, stored.RestoreWithWorkDir(dir))
	assert.Equal(t, "held", stored.GetSessionIdentifier())
	assert.NotEqual(t, "held", legacy.GetSessionIdentifier(), "two records must never hold one daemon session")
	require.NoError(t, legacy.Close())
	assert.NotContains(t, killed, "held", "closing the legacy record must not kill the other record's session")
	t.Cleanup(func() { _ = stored.Close() })
}

// With tymuxd unreachable the kill fails; the record must keep its id so the
// session is not orphaned and a retry can still kill it.
func TestClose_UnrestoredSessionKeepsStoredIDWhenKillFails(t *testing.T) {
	resetClaims(t)
	failing := true
	var killed []string
	transport := &fakeTransport{
		killSessionFn: func(_ context.Context, r *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			if failing {
				return nil, connect.NewError(connect.CodeUnavailable, errors.New("tymuxd unreachable"))
			}
			killed = append(killed, r.Msg.GetSessionId())
			return connect.NewResponse(&v1.KillSessionResponse{}), nil
		},
	}
	sess := NewTymuxGRPCSession(transport)
	sess.SetStoredSessionID("keep-1")
	require.Error(t, sess.Close())
	assert.Equal(t, "keep-1", sess.PersistedSessionID(), "a failed kill must not drop the persisted id")

	failing = false
	require.NoError(t, sess.Close())
	assert.Equal(t, []string{"keep-1"}, killed)
	assert.Empty(t, sess.PersistedSessionID())
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

func TestClose_UnrestoredSessionKillsItsStoredDaemonSession(t *testing.T) {
	resetClaims(t)
	var killed []string
	transport := &fakeTransport{
		killSessionFn: func(_ context.Context, r *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			killed = append(killed, r.Msg.GetSessionId())
			return connect.NewResponse(&v1.KillSessionResponse{}), nil
		},
	}
	sess := NewTymuxGRPCSession(transport)
	sess.SetStoredSessionID("stored-u1")
	require.NoError(t, sess.Close())
	assert.Equal(t, []string{"stored-u1"}, killed)
}

// A second Instance object rebuilt from the same persisted record (a registry
// throwaway, a liveness shadow) names the same stored id; it must attach to that
// session, never spawn a duplicate agent next to it.
func TestRestoreWithWorkDir_SameStoredRecordBorrowsInsteadOfSpawning(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	created := 0
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{namedSession("rec-1", dir+" [x]", dir)}}), nil
		},
		createSessionFn: func(context.Context, *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			created++
			return connect.NewResponse(namedSession("dup", dir, dir)), nil
		},
	}
	owner := NewTymuxGRPCSession(transport)
	owner.SetSessionLabel("x")
	owner.SetStoredSessionID("rec-1")
	require.NoError(t, owner.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = owner.Close() })

	view := NewTymuxGRPCSession(transport)
	view.SetSessionLabel("x")
	view.SetStoredSessionID("rec-1")
	require.NoError(t, view.RestoreWithWorkDir(dir))

	assert.Zero(t, created, "a view of the same record must not spawn a second session")
	assert.Equal(t, "rec-1", view.GetSessionIdentifier())
}

// Closing a stored-id view kills the record's session: the record owns it, so a
// stopped/deleted shadow Instance cannot leave the real session running.
func TestClose_StoredIDViewKillsTheRecordsSession(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	var killed []string
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{namedSession("rec-2", dir, dir)}}), nil
		},
		killSessionFn: func(_ context.Context, r *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			killed = append(killed, r.Msg.GetSessionId())
			return connect.NewResponse(&v1.KillSessionResponse{}), nil
		},
	}
	owner := NewTymuxGRPCSession(transport)
	owner.SetStoredSessionID("rec-2")
	require.NoError(t, owner.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = owner.Close() })
	view := NewTymuxGRPCSession(transport)
	view.SetStoredSessionID("rec-2")
	require.NoError(t, view.RestoreWithWorkDir(dir))

	require.NoError(t, view.Close())

	assert.Equal(t, []string{"rec-2"}, killed)
}

// A never-restored view can still answer "is the record's session live" from its
// stored id, without claiming it.
func TestStoredSessionLive(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	live := true
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			s := namedSession("rec-3", dir, dir)
			if !live {
				s.Liveness = v1.Liveness_LIVENESS_DEAD
			}
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{s}}), nil
		},
	}
	view := NewTymuxGRPCSession(transport).(*tymuxGRPCSession)
	assert.False(t, view.StoredSessionLive(), "no stored id")
	view.SetStoredSessionID("rec-3")
	assert.True(t, view.StoredSessionLive())
	_, claimed := claimedSessions.Load("rec-3")
	assert.False(t, claimed, "a liveness probe must not claim")
	live = false
	assert.False(t, view.StoredSessionLive())
	view.SetStoredSessionID("other")
	live = true
	assert.False(t, view.StoredSessionLive(), "unknown id")
}

// Startup restore of many Instances that share one directory: each adopts its own
// stored id, none is spawned, none shares (run with -race).
func TestRestoreWithWorkDir_ConcurrentStartupRestoreOfSharedDirRecords(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	const n = 24
	daemon := newModelDaemon()
	ids := make([]string, n)
	for i := range ids {
		resp, err := daemon.CreateSession(context.Background(), connect.NewRequest(&v1.CreateSessionRequest{Name: fmt.Sprintf("%s [t%d]", dir, i), Cwd: dir}))
		require.NoError(t, err)
		ids[i] = resp.Msg.GetId()
	}
	got := make([]string, n)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m := NewTymuxGRPCSession(daemon)
			m.SetSessionLabel(fmt.Sprintf("t%d", i))
			m.SetStoredSessionID(ids[i])
			assert.NoError(t, m.RestoreWithWorkDir(dir))
			got[i] = m.GetSessionIdentifier()
		}(i)
	}
	wg.Wait()
	assert.Equal(t, ids, got)
	assert.Len(t, daemon.ids(), n, "restore must not spawn extra sessions")
}

// seedDaemonSession plants a session the way an earlier process left it.
func seedDaemonSession(t *testing.T, d *modelDaemon, name, dir string, live bool) string {
	t.Helper()
	resp, err := d.CreateSession(context.Background(), connect.NewRequest(&v1.CreateSessionRequest{Name: name, Cwd: dir}))
	require.NoError(t, err)
	id := resp.Msg.GetId()
	d.mu.Lock()
	d.sessions[id].live = live
	d.mu.Unlock()
	return id
}

func labeledView(d *modelDaemon, label, storedID string) TymuxManager {
	v := NewTymuxGRPCSession(d)
	v.SetSessionLabel(label)
	v.SetStoredSessionID(storedID)
	return v
}

// A live view re-entering restore already holds the claim for its session. Throwaway
// copies of the same record store that id too (health-tick loads, liveness shadows);
// they are the same record, never "another" one, so the view must keep its session
// instead of spawning a second agent and orphaning the first.
func TestRestoreWithWorkDir_ReEntryKeepsOwnSessionWhileCopyOfRecordStoresIt(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	view := NewTymuxGRPCSession(d)
	view.SetSessionLabel("x")
	require.NoError(t, view.Start(dir))
	t.Cleanup(func() { _ = view.Close() })
	own := view.GetSessionIdentifier()
	_ = labeledView(d, "x", own) // the copy, still reachable

	require.NoError(t, view.RestoreWithWorkDir(dir))

	assert.Equal(t, own, view.GetSessionIdentifier())
	assert.Equal(t, map[string]bool{own: true}, d.ids(), "re-entering restore must not spawn a second agent")
}

// Same, for a stale stored id: recovery created d001 under the record's label, and a
// copy of the record (saved after recovery) stores d001. The view's own label session
// must not be skipped as "stored by another record".
func TestRestoreWithWorkDir_StaleStoredID_CopyOfSameRecordDoesNotBlockLabelAdoption(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	recovered := seedDaemonSession(t, d, dir+" [x]", dir, true)
	_ = labeledView(d, "x", recovered) // copy saved after recovery
	view := labeledView(d, "x", "gone-id")

	require.NoError(t, view.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = view.Close() })

	assert.Equal(t, recovered, view.GetSessionIdentifier())
	assert.Len(t, d.ids(), 1, "the record's own session must be re-adopted, not duplicated")
}

// A session another record stores by id is still never adopted by name.
func TestRestoreWithWorkDir_OtherRecordsStoredSessionIsStillSkipped(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	theirs := seedDaemonSession(t, d, dir, dir, true) // legacy name
	_ = labeledView(d, "other", theirs)
	view := labeledView(d, "x", "")

	require.NoError(t, view.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = view.Close() })

	assert.NotEqual(t, theirs, view.GetSessionIdentifier())
}

// A view that holds the claim for the session it drives re-adopts it before any
// stored-id lookup, even if its stored id was changed to name another session.
func TestRestoreWithWorkDir_HeldClaimWinsOverStoredID(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	view := NewTymuxGRPCSession(d)
	view.SetSessionLabel("x")
	require.NoError(t, view.Start(dir))
	t.Cleanup(func() { _ = view.Close() })
	own := view.GetSessionIdentifier()
	other := seedDaemonSession(t, d, "unrelated", dir, true)
	view.SetStoredSessionID(other)

	require.NoError(t, view.RestoreWithWorkDir(dir))

	assert.Equal(t, own, view.GetSessionIdentifier())
}

// The registry holds one entry per live record, not one per view ever built: it
// shrinks when a record clears or changes its stored id.
func TestStoredIDHolders_UnregisteredOnCloseAndIDChange(t *testing.T) {
	resetClaims(t)
	d := newModelDaemon()
	a := labeledView(d, "x", "id-1")
	a.SetStoredSessionID("id-2")
	storedIDHolders.mu.Lock()
	_, stale := storedIDHolders.labels["id-1"]
	storedIDHolders.mu.Unlock()
	assert.False(t, stale, "a changed stored id must be unregistered")

	require.NoError(t, a.Close())
	storedIDHolders.mu.Lock()
	n := len(storedIDHolders.labels)
	storedIDHolders.mu.Unlock()
	assert.Zero(t, n, "a closed record must not stay registered")
}

// Instance.Start(false) on an unrestored record runs Start then RestoreWithWorkDir.
// The record's live session must be attached by Start, never shadowed by a fresh
// agent that Restore then drops (verified identical on main before the fix).
func TestStartThenRestore_UnrestoredRecordWithLiveStoredSessionHoldsOneSession(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	stored := seedDaemonSession(t, d, dir+" [x]", dir, true)
	view := labeledView(d, "x", stored)
	t.Cleanup(func() { _ = view.Close() })

	require.NoError(t, view.Start(dir))
	require.NoError(t, view.RestoreWithWorkDir(dir))

	assert.Equal(t, stored, view.GetSessionIdentifier())
	assert.Equal(t, map[string]bool{stored: true}, d.ids(), "the daemon must hold exactly the record's session")
}

// With the stored id gone, Start re-adopts the record's still-running label session.
func TestStart_UnrestoredRecordWithGoneStoredIDAdoptsLabelSession(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	labeled := seedDaemonSession(t, d, dir+" [x]", dir, true)
	view := labeledView(d, "x", "gone-id")
	t.Cleanup(func() { _ = view.Close() })

	require.NoError(t, view.Start(dir))

	assert.Equal(t, labeled, view.GetSessionIdentifier())
	assert.Len(t, d.ids(), 1)
}

// A dead stored session is replaced (the caller wants a fresh launch command) and
// the dead one is removed, leaving exactly one session.
func TestStart_UnrestoredRecordWithDeadStoredSessionReplacesIt(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	stored := seedDaemonSession(t, d, dir+" [x]", dir, false)
	view := labeledView(d, "x", stored)
	t.Cleanup(func() { _ = view.Close() })

	require.NoError(t, view.Start(dir))
	require.NoError(t, view.RestoreWithWorkDir(dir))

	assert.NotEqual(t, stored, view.GetSessionIdentifier())
	assert.Equal(t, map[string]bool{view.GetSessionIdentifier(): true}, d.ids())
	assert.Equal(t, view.GetSessionIdentifier(), view.PersistedSessionID())
}

// An unanswered ListSessions must not be read as "no stored session": creating
// blind would put a second agent next to the live one.
func TestStart_UnrestoredRecordDoesNotCreateWhenDaemonListFails(t *testing.T) {
	resetClaims(t)
	creates := 0
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("tymuxd unreachable"))
		},
		createSessionFn: func(context.Context, *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			creates++
			return connect.NewResponse(namedSession("dup", "d", "d")), nil
		},
	}
	view := NewTymuxGRPCSession(transport)
	view.SetStoredSessionID("live-1")

	require.Error(t, view.Start(t.TempDir()))

	assert.Zero(t, creates)
	assert.Equal(t, "live-1", view.PersistedSessionID())
}

// A restored view whose kill fails (tymuxd unreachable) keeps persisting the id,
// or the next save writes "" and orphans the running session; the failed kill is
// still not retried by the next Close.
func TestClose_RestoredViewKeepsIDWhenKillFails(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	kills := 0
	var live *v1.Session
	transport := &fakeTransport{
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{live}}), nil
		},
		killSessionFn: func(context.Context, *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			kills++
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("tymuxd unreachable"))
		},
	}
	live = namedSession("run-1", dir, dir)
	view := NewTymuxGRPCSession(transport)
	view.SetStoredSessionID("run-1")
	require.NoError(t, view.RestoreWithWorkDir(dir))

	require.Error(t, view.Close())
	assert.Equal(t, "run-1", view.PersistedSessionID(), "a failed kill must not drop the persisted id")
	require.ErrorIs(t, view.Close(), ErrTymuxdUnreachable)
	assert.Equal(t, 2, kills, "failed cleanup must remain retryable")
}

// Close clears the record-owned kill right, so a repeat Close never kills again
// (the first kill may be long gone and its id reused by another owner).
func TestClose_RecordOwnedViewKillsOnce(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	stored := seedDaemonSession(t, d, dir+" [x]", dir, true)
	holder := labeledView(d, "x", stored)
	require.NoError(t, holder.RestoreWithWorkDir(dir)) // holds the claim
	t.Cleanup(func() { _ = holder.Close() })
	view := labeledView(d, "x", stored)
	require.NoError(t, view.RestoreWithWorkDir(dir)) // record-owned, claim held by holder

	require.NoError(t, view.Close())
	require.NoError(t, view.Close())

	assert.Equal(t, []string{stored}, d.kills)
}

// A recovery Start replaces a record-owned session with one this view created, so the
// record-owned kill right must not carry over to a session another owner now claims.
func TestStart_RecoveryClearsRecordOwnedKillRight(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	stored := seedDaemonSession(t, d, dir+" [x]", dir, true)
	holder := labeledView(d, "x", stored)
	require.NoError(t, holder.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = holder.Close() })
	view := labeledView(d, "x", stored)
	require.NoError(t, view.RestoreWithWorkDir(dir)) // record-owned
	d.mu.Lock()
	delete(d.sessions, stored)
	d.mu.Unlock()
	require.NoError(t, view.Start(dir))
	fresh := view.GetSessionIdentifier()
	claimedSessions.Store(fresh, any(holder)) // another owner took the replacement

	require.NoError(t, view.Close())

	assert.NotContains(t, d.kills, fresh)
}

// failKillOnce is a daemon whose next KillSession fails as if tymuxd were unreachable.
type failKillOnce struct {
	*modelDaemon
	fail bool
}

func (f *failKillOnce) KillSession(ctx context.Context, req *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
	if f.fail {
		f.fail = false
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("tymuxd unreachable"))
	}
	return f.modelDaemon.KillSession(ctx, req)
}

// A Close whose kill failed leaves the session running and the view closed; a later Start on
// the same view (pause/restart) must re-adopt that session, not create a second agent.
func TestStart_AfterFailedCloseReadoptsTheStillRunningSession(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := &failKillOnce{modelDaemon: newModelDaemon()}
	stored := seedDaemonSession(t, d.modelDaemon, dir+" [x]", dir, true)
	view := NewTymuxGRPCSession(d)
	view.SetSessionLabel("x")
	view.SetStoredSessionID(stored)
	require.NoError(t, view.RestoreWithWorkDir(dir))
	t.Cleanup(func() { _ = view.Close() })
	d.fail = true
	require.Error(t, view.Close())

	require.NoError(t, view.Start(dir))

	assert.Equal(t, stored, view.GetSessionIdentifier())
	assert.Equal(t, map[string]bool{stored: true}, d.ids(), "the daemon must hold exactly one session")
}

// With the stored session dead, a live session under the record's label is the running agent:
// adopt it, never replace the dead one with a second live agent.
func TestStart_DeadStoredSessionAdoptsLiveLabelSession(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	dead := seedDaemonSession(t, d, "old-name", dir, false)
	live := seedDaemonSession(t, d, dir+" [x]", dir, true)
	view := labeledView(d, "x", dead)
	t.Cleanup(func() { _ = view.Close() })

	require.NoError(t, view.Start(dir))

	assert.Equal(t, live, view.GetSessionIdentifier())
	assert.Equal(t, map[string]bool{live: true}, d.ids(), "one live agent; the dead session is removed")
}

// A failed Start releases a session it adopted (it ran before the attempt) without killing it, and
// keeps the record's id; a session the attempt created is left to the caller to kill.
func TestReleaseAdoptedSession_KeepsAdoptedSessionRunning(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	stored := seedDaemonSession(t, d, dir+" [x]", dir, true)
	view := labeledView(d, "x", stored)
	require.NoError(t, view.Start(dir))

	released := view.(interface{ ReleaseAdoptedSession() bool }).ReleaseAdoptedSession()

	assert.True(t, released)
	assert.Empty(t, d.kills)
	assert.Equal(t, map[string]bool{stored: true}, d.ids())
	assert.Equal(t, stored, view.PersistedSessionID())
	assert.False(t, view.HasSession())
	require.NoError(t, view.Close()) // the record still owns it
	assert.Equal(t, []string{stored}, d.kills)
}

func TestReleaseAdoptedSession_RefusesASessionTheViewCreated(t *testing.T) {
	resetClaims(t)
	d := newModelDaemon()
	view := NewTymuxGRPCSession(d)
	require.NoError(t, view.Start(t.TempDir()))
	t.Cleanup(func() { _ = view.Close() })

	assert.False(t, view.(interface{ ReleaseAdoptedSession() bool }).ReleaseAdoptedSession())
	assert.Len(t, d.ids(), 1)
}

// A dead pane tymuxd is restarting (BACKOFF/RESTARTING) is still the record's agent: Start adopts
// it. Every other state is replaced by a fresh session.
func TestStart_UnrestoredRecordAdoptsOnlyLiveOrRestartingPanes(t *testing.T) {
	for _, p := range matrixStates {
		t.Run(p.name, func(t *testing.T) {
			resetClaims(t)
			dir := t.TempDir()
			calls := &matrixCalls{}
			ft := matrixTransport(dir, p, calls)
			view := NewTymuxGRPCSession(ft)
			view.SetStoredSessionID(testSessionID)
			t.Cleanup(func() { _ = view.Close() })

			require.NoError(t, view.Start(dir))

			wantAdopt := p.liveness == v1.Liveness_LIVENESS_LIVE || p.restartPending()
			if wantAdopt {
				assert.Zero(t, calls.create.Load(), "must adopt, not create")
			} else {
				assert.Equal(t, int32(1), calls.create.Load(), "must replace")
			}
		})
	}
}

// A stored id the daemon lists is attached by the restore path without taking the claim early;
// the claim is the view's once attached.
func TestRestoreWithWorkDir_StoredIDAttachTakesTheClaim(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	stored := seedDaemonSession(t, d, dir, dir, true)
	view := NewTymuxGRPCSession(d).(*tymuxGRPCSession)
	view.SetStoredSessionID(stored)
	t.Cleanup(func() { _ = view.Close() })

	require.NoError(t, view.RestoreWithWorkDir(dir))

	assert.True(t, claimedSessions.holds(stored, view))
}

// A renamed record re-registers under its new label, or the old one stays registered.
func TestSetSessionLabel_ReregistersStoredIDUnderNewLabel(t *testing.T) {
	resetClaims(t)
	d := newModelDaemon()
	view := labeledView(d, "old", "id-1")

	view.SetSessionLabel("new")

	storedIDHolders.mu.Lock()
	_, oldHeld := storedIDHolders.labels["id-1"]["old"]
	_, newHeld := storedIDHolders.labels["id-1"]["new"]
	storedIDHolders.mu.Unlock()
	assert.False(t, oldHeld)
	assert.True(t, newHeld)
}

// A record with no stored id that is renamed before restore still finds the daemon session
// created under its old label, instead of spawning a second agent.
func TestRestoreWithWorkDir_RenamedRecordWithoutStoredIDFindsSessionByPriorLabel(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	old := seedDaemonSession(t, d, dir+" [old]", dir, true)
	view := labeledView(d, "old", "")
	view.SetSessionLabel("new")
	t.Cleanup(func() { _ = view.Close() })

	require.NoError(t, view.RestoreWithWorkDir(dir))

	assert.Equal(t, old, view.GetSessionIdentifier())
	assert.Len(t, d.ids(), 1)
}

// A record whose stored session is dead must not take another legacy record's live
// Name == workDir session; only its own labeled name counts.
func TestStart_DeadStoredSessionDoesNotAdoptLegacyDirNamedSession(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	d := newModelDaemon()
	dead := seedDaemonSession(t, d, dir+" [x]", dir, false)
	legacy := seedDaemonSession(t, d, dir, dir, true) // someone else's, never stored by id
	view := labeledView(d, "x", dead)
	t.Cleanup(func() { _ = view.Close() })

	require.NoError(t, view.Start(dir))

	assert.NotEqual(t, legacy, view.GetSessionIdentifier())
	assert.True(t, d.ids()[legacy], "the other record's session must be left alone")
}
