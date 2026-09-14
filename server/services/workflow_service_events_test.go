package services

// workflow_service_events_test.go — WatchWorkflows handler test suite,
// mirroring backlog_service_events_test.go's shape: drive
// svc.watchWorkflows directly through the workflowEventSender interface
// rather than a fake connect.ServerStream[T] (same vendored-connect
// constructor limitation documented there).

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// fakeWorkflowEventSender is a hand-rolled fake implementing
// workflowEventSender, capturing every sent message in a mutex-guarded
// slice (several tests run watchWorkflows in a goroutine while concurrently
// publishing to the real event bus).
type fakeWorkflowEventSender struct {
	mu   sync.Mutex
	sent []*sessionv1.WorkflowEvent
}

func (f *fakeWorkflowEventSender) Send(e *sessionv1.WorkflowEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, e)
	return nil
}

func (f *fakeWorkflowEventSender) Sent() []*sessionv1.WorkflowEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*sessionv1.WorkflowEvent, len(f.sent))
	copy(out, f.sent)
	return out
}

// newTestWorkflowServiceWithBus builds a WorkflowService backed by a real
// in-memory-SQLite-backed EntWorkflowRepository and a real *events.EventBus.
func newTestWorkflowServiceWithBus(t *testing.T) (*WorkflowService, *events.EventBus) {
	t.Helper()
	repo := createTestEntClient(t)
	workflowRepo := session.NewEntWorkflowRepository(repo.GetEntClient())
	svc := NewWorkflowService(workflowRepo, nil /* scheduler unused by these tests */, nil)
	bus := events.NewEventBus(100)
	svc.SetEventBus(bus)
	return svc, bus
}

func runWatchWorkflows(ctx context.Context, svc *WorkflowService, msg *sessionv1.WatchWorkflowsRequest, sender workflowEventSender) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- svc.watchWorkflows(ctx, msg, sender)
	}()
	return done
}

func requireWorkflowWatchCleanReturn(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("watchWorkflows did not return after context cancellation")
	}
}

func TestWatchWorkflows_should_sendSnapshotEventForEachWorkflow_When_AfterSeqIsZero(t *testing.T) {
	t.Parallel()
	svc, _ := newTestWorkflowServiceWithBus(t)
	ctx := context.Background()

	resp1, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug: "wf-1", Name: "WF 1", Command: "do it", TargetDirectory: "/tmp/wf1",
	}))
	require.NoError(t, err)
	resp2, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug: "wf-2", Name: "WF 2", Command: "do it", TargetDirectory: "/tmp/wf2",
	}))
	require.NoError(t, err)

	sender := &fakeWorkflowEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	// CreateWorkflow above already published live events on the shared bus
	// before this stream subscribes, so only the fresh-snapshot branch's
	// sends (2, one per existing workflow) are expected here.
	done := runWatchWorkflows(runCtx, svc, &sessionv1.WatchWorkflowsRequest{}, sender)

	wait.RequireEventually(t, func() bool { return len(sender.Sent()) >= 2 }, 2*time.Second, 10*time.Millisecond)
	requireWorkflowWatchCleanReturn(t, cancel, done)

	sent := sender.Sent()
	require.Len(t, sent, 2, "exactly one snapshot event per existing workflow, no more")

	gotIDs := map[string]bool{}
	for _, ev := range sent {
		created := ev.GetWorkflowCreated()
		require.NotNil(t, created, "fresh-connection snapshot events are always the workflow_created variant")
		assert.True(t, created.GetIsSnapshot(), "every fresh-snapshot event must be is_snapshot: true")
		gotIDs[created.GetWorkflow().GetId()] = true
	}
	assert.True(t, gotIDs[resp1.Msg.Workflow.Id], "missing snapshot event for wf-1")
	assert.True(t, gotIDs[resp2.Msg.Workflow.Id], "missing snapshot event for wf-2")
}

func TestWatchWorkflows_should_sendSnapshotCompleteMarker_When_NoWorkflowsExist(t *testing.T) {
	t.Parallel()
	svc, _ := newTestWorkflowServiceWithBus(t)
	ctx := context.Background()

	sender := &fakeWorkflowEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	done := runWatchWorkflows(runCtx, svc, &sessionv1.WatchWorkflowsRequest{}, sender)

	wait.RequireEventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)
	requireWorkflowWatchCleanReturn(t, cancel, done)

	sent := sender.Sent()
	require.Len(t, sent, 1)
	assert.NotNil(t, sent[0].GetSnapshotComplete(), "zero workflows must still send exactly one snapshot_complete marker")
}

func TestWatchWorkflows_should_broadcastLiveEvents_When_CreateUpdateDeleteRunSucceed(t *testing.T) {
	t.Parallel()
	svc, _ := newTestWorkflowServiceWithBus(t)
	ctx := context.Background()

	sender := &fakeWorkflowEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	// Start watching before any mutation, on an already-empty backlog, so
	// the first message is the snapshot_complete marker and every mutation
	// below is delivered exclusively through the live fan-out loop.
	done := runWatchWorkflows(runCtx, svc, &sessionv1.WatchWorkflowsRequest{}, sender)
	wait.RequireEventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)

	createResp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug: "live-wf", Name: "Live WF", Command: "do it", TargetDirectory: "/tmp/live-wf",
	}))
	require.NoError(t, err)
	wfID := createResp.Msg.Workflow.Id

	newName := "Renamed"
	_, err = svc.UpdateWorkflow(ctx, connect.NewRequest(&sessionv1.UpdateWorkflowRequest{
		Id: wfID, Name: &newName,
	}))
	require.NoError(t, err)

	_, err = svc.DeleteWorkflow(ctx, connect.NewRequest(&sessionv1.DeleteWorkflowRequest{Id: wfID}))
	require.NoError(t, err)

	wait.RequireEventually(t, func() bool { return len(sender.Sent()) >= 4 }, 2*time.Second, 10*time.Millisecond)
	requireWorkflowWatchCleanReturn(t, cancel, done)

	sent := sender.Sent()
	require.Len(t, sent, 4, "snapshot_complete + created + updated + deleted")

	require.NotNil(t, sent[0].GetSnapshotComplete())

	created := sent[1].GetWorkflowCreated()
	require.NotNil(t, created)
	assert.Equal(t, "live-wf", created.GetWorkflow().GetSlug())
	assert.False(t, created.GetIsSnapshot(), "a live create must not be marked is_snapshot")

	updated := sent[2].GetWorkflowUpdated()
	require.NotNil(t, updated)
	assert.Equal(t, newName, updated.GetWorkflow().GetName())

	deleted := sent[3].GetWorkflowDeleted()
	require.NotNil(t, deleted)
	assert.Equal(t, wfID, deleted.GetId())
}

func TestWatchWorkflows_should_broadcastRunEvent_When_RunWorkflowSucceeds(t *testing.T) {
	t.Parallel()
	repo := createTestEntClient(t)
	workflowRepo := session.NewEntWorkflowRepository(repo.GetEntClient())
	scheduler := &mockScheduler{}
	svc := NewWorkflowService(workflowRepo, scheduler, nil)
	bus := events.NewEventBus(100)
	svc.SetEventBus(bus)
	ctx := context.Background()

	createResp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug: "runnable", Name: "Runnable", Command: "do it", TargetDirectory: "/tmp/runnable",
	}))
	require.NoError(t, err)
	wfID := createResp.Msg.Workflow.Id

	sender := &fakeWorkflowEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	done := runWatchWorkflows(runCtx, svc, &sessionv1.WatchWorkflowsRequest{}, sender)
	// Fresh-snapshot branch sends one workflow_created event for the
	// already-existing "runnable" row before the live loop starts.
	wait.RequireEventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)

	runResp, err := svc.RunWorkflow(ctx, connect.NewRequest(&sessionv1.RunWorkflowRequest{Id: wfID}))
	require.NoError(t, err)

	wait.RequireEventually(t, func() bool { return len(sender.Sent()) >= 2 }, 2*time.Second, 10*time.Millisecond)
	requireWorkflowWatchCleanReturn(t, cancel, done)

	sent := sender.Sent()
	require.Len(t, sent, 2)
	run := sent[1].GetWorkflowRun()
	require.NotNil(t, run)
	assert.Equal(t, wfID, run.GetWorkflowId())
	assert.Equal(t, runResp.Msg.SessionId, run.GetSessionId())
}

func TestWatchWorkflows_should_returnUnavailable_When_EventBusNotWired(t *testing.T) {
	t.Parallel()
	repo := createTestEntClient(t)
	workflowRepo := session.NewEntWorkflowRepository(repo.GetEntClient())
	svc := NewWorkflowService(workflowRepo, nil, nil) // no SetEventBus call

	err := svc.watchWorkflows(context.Background(), &sessionv1.WatchWorkflowsRequest{}, &fakeWorkflowEventSender{})
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
}
