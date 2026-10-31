package tymux

import (
	"context"
	"errors"
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
