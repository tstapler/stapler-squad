package workspacerpc

// Pins the fields GetVCSStatus hands to vcsstatus.Cache; the cache itself is tested there.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/vc"
)

func wiredVCSService(t *testing.T) (*WorkspaceService, string) {
	t.Helper()
	dir, _, _ := svctest.NewVCSStatusTestRepo(t)
	ws := NewWorkspaceService(nil, nil)
	ws.SetLiveFinder(&svctest.StubLiveFinder{Inst: &instance.Instance{Title: "wired", Path: dir, Status: domain.Paused, Program: "claude"}})
	return ws, dir
}

func wiredVCSRequest(waitForFresh bool) *connect.Request[sessionv1.GetVCSStatusRequest] {
	return connect.NewRequest(&sessionv1.GetVCSStatusRequest{Id: "wired", WaitForFresh: waitForFresh})
}

func TestGetVCSStatus_ShouldSkipTheStaleEntry_OnlyWhenWaitForFresh(t *testing.T) {
	t.Parallel()
	ws, dir := wiredVCSService(t)
	ws.vcs.StoreForTest(dir, &vc.VCSStatus{Branch: "stale"}, time.Now().Add(-30*time.Second))

	resp, err := ws.GetVCSStatus(context.Background(), wiredVCSRequest(true))

	require.NoError(t, err)
	assert.Equal(t, "main", resp.Msg.VcsStatus.Branch)
}

// Not parallel: swaps the process-wide git backend.
func TestGetVCSStatus_ShouldNotJoinAnOlderFlight_When_WaitForFresh(t *testing.T) {
	spy := svctest.InstallStatusSpy(t, true)
	t.Cleanup(spy.Release)
	ws, _ := wiredVCSService(t)
	done := make(chan error, 2)
	get := func(waitForFresh bool) {
		_, err := ws.GetVCSStatus(context.Background(), wiredVCSRequest(waitForFresh))
		done <- err
	}
	go get(false)
	require.Eventually(t, func() bool { return spy.Calls.Load() == 1 }, 10*time.Second, 2*time.Millisecond)

	go get(true)

	require.Eventually(t, func() bool { return spy.Calls.Load() == 2 }, 10*time.Second, 2*time.Millisecond,
		"the request's start time must reach the cache, or the caller joins the older run")
	spy.Release()
	for range 2 {
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(10 * time.Second):
			t.Fatal("GetVCSStatus did not return")
		}
	}
}
