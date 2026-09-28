package services

// diagnose_service_test.go covers Phase 7, Story 7.1.1's ConnectRPC handler:
// DiagnoseBacklogItem (success, already-in-flight rejection) and
// ListDiagnoseDispatches (empty history, the page-refresh-mid-dispatch fix
// for architecture-review Blocker 1 at the RPC layer, and a Stalled row's
// round trip). Reuses diagnose_dispatcher_test.go's fakes/helpers
// (fakeHeadlessDiagnosticSessionCreator, sampleDiagnosticDataSource,
// newTestDiagnoseDispatcher) since this file lives in the same package and
// exercises the same DiagnoseDispatcher underneath a thin RPC translation
// layer.

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// TestDiagnoseService_DiagnoseBacklogItem_ShouldReturnDispatchIDAndSessionID_WhenNoInFlightDispatchExists
// is Story 7.1.1's happy-path AC: {dispatch_id, diagnostic_session_id} comes
// back populated and the session ID carries the headless-diagnose- prefix.
func TestDiagnoseService_DiagnoseBacklogItem_ShouldReturnDispatchIDAndSessionID_WhenNoInFlightDispatchExists(t *testing.T) {
	t.Parallel()
	dataSource := sampleDiagnosticDataSource()
	creator := &fakeHeadlessDiagnosticSessionCreator{}
	dispatcher, store := newTestDiagnoseDispatcher(t, dataSource, creator, &fakeDiagnoseNotifier{})
	svc := NewDiagnoseService(dispatcher, store)

	resp, err := svc.DiagnoseBacklogItem(context.Background(), connect.NewRequest(&sessionv1.DiagnoseBacklogItemRequest{ItemId: "e6c2a88e"}))
	require.NoError(t, err)
	require.NotEmpty(t, resp.Msg.DispatchId)
	require.Contains(t, resp.Msg.DiagnosticSessionId, HeadlessDiagnosticSessionIDPrefix)
}

// TestDiagnoseService_DiagnoseBacklogItem_ShouldReturnFailedPrecondition_WhenAlreadyInFlight
// is Story 7.1.1's rejection AC: a concurrent duplicate dispatch surfaces as a
// gRPC error, not a normal response with an AlreadyInFlight field.
func TestDiagnoseService_DiagnoseBacklogItem_ShouldReturnFailedPrecondition_WhenAlreadyInFlight(t *testing.T) {
	t.Parallel()
	dataSource := sampleDiagnosticDataSource()
	release := make(chan struct{})
	creator := &fakeHeadlessDiagnosticSessionCreator{onCreate: func() { <-release }}
	dispatcher, store := newTestDiagnoseDispatcher(t, dataSource, creator, &fakeDiagnoseNotifier{})
	svc := NewDiagnoseService(dispatcher, store)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _ = svc.DiagnoseBacklogItem(context.Background(), connect.NewRequest(&sessionv1.DiagnoseBacklogItemRequest{ItemId: "e6c2a88e"}))
	}()
	require.Eventually(t, func() bool { return creator.callCount() == 1 }, time.Second, time.Millisecond,
		"first call must reach dispatch before the second call is issued")

	_, err := svc.DiagnoseBacklogItem(context.Background(), connect.NewRequest(&sessionv1.DiagnoseBacklogItemRequest{ItemId: "e6c2a88e"}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr))
	require.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())

	close(release)
	<-firstDone
}

// TestDiagnoseService_ListDiagnoseDispatches_ShouldReturnEmptyList_WhenItemHasNoDispatches
// covers Story 7.1.1's third handler test: an item with no dispatch history
// returns an empty list, not an error.
func TestDiagnoseService_ListDiagnoseDispatches_ShouldReturnEmptyList_WhenItemHasNoDispatches(t *testing.T) {
	t.Parallel()
	dataSource := sampleDiagnosticDataSource()
	dispatcher, store := newTestDiagnoseDispatcher(t, dataSource, &fakeHeadlessDiagnosticSessionCreator{}, &fakeDiagnoseNotifier{})
	svc := NewDiagnoseService(dispatcher, store)

	resp, err := svc.ListDiagnoseDispatches(context.Background(), connect.NewRequest(&sessionv1.ListDiagnoseDispatchesRequest{ItemId: "no-such-item"}))
	require.NoError(t, err)
	require.Empty(t, resp.Msg.Dispatches)
}

// TestListDiagnoseDispatches_ShouldReturnSamePendingRowAcrossRepeatedCalls_WhenDispatchStillInFlight
// is validation.md REQ-8's row: two successive RPC calls mid-dispatch return
// the identical Pending row (same ID, no outcome_kind) -- the concrete
// API-level fix for architecture-review Blocker 1.
func TestListDiagnoseDispatches_ShouldReturnSamePendingRowAcrossRepeatedCalls_WhenDispatchStillInFlight(t *testing.T) {
	t.Parallel()
	dataSource := sampleDiagnosticDataSource()
	release := make(chan struct{})
	creator := &fakeHeadlessDiagnosticSessionCreator{onCreate: func() { <-release }}
	dispatcher, store := newTestDiagnoseDispatcher(t, dataSource, creator, &fakeDiagnoseNotifier{})
	svc := NewDiagnoseService(dispatcher, store)

	dispatchDone := make(chan struct{})
	go func() {
		defer close(dispatchDone)
		_, _ = svc.DiagnoseBacklogItem(context.Background(), connect.NewRequest(&sessionv1.DiagnoseBacklogItemRequest{ItemId: "e6c2a88e"}))
	}()
	require.Eventually(t, func() bool { return creator.callCount() == 1 }, time.Second, time.Millisecond)

	first, err := svc.ListDiagnoseDispatches(context.Background(), connect.NewRequest(&sessionv1.ListDiagnoseDispatchesRequest{ItemId: "e6c2a88e"}))
	require.NoError(t, err)
	require.Len(t, first.Msg.Dispatches, 1)
	require.Equal(t, sessionv1.DiagnoseDispatchStatus_DIAGNOSE_DISPATCH_STATUS_PENDING, first.Msg.Dispatches[0].Status)
	require.Nil(t, first.Msg.Dispatches[0].OutcomeKind)

	second, err := svc.ListDiagnoseDispatches(context.Background(), connect.NewRequest(&sessionv1.ListDiagnoseDispatchesRequest{ItemId: "e6c2a88e"}))
	require.NoError(t, err)
	require.Len(t, second.Msg.Dispatches, 1)
	require.Equal(t, first.Msg.Dispatches[0].Id, second.Msg.Dispatches[0].Id, "the same Pending row must be returned, not a new one")
	require.Equal(t, sessionv1.DiagnoseDispatchStatus_DIAGNOSE_DISPATCH_STATUS_PENDING, second.Msg.Dispatches[0].Status)
	require.Nil(t, second.Msg.Dispatches[0].OutcomeKind)

	close(release)
	<-dispatchDone
}

// TestDiagnoseService_ListDiagnoseDispatches_ShouldRoundTripStalledStatusWithOutcomeKindUnset_WhenDispatchMarkedStalled
// covers design/ux.md Surface 15's concrete API-level surface: a dispatch
// Story 6.1.5's reconciler marked Stalled since the last call comes back with
// status STALLED and outcome_kind still unset.
func TestDiagnoseService_ListDiagnoseDispatches_ShouldRoundTripStalledStatusWithOutcomeKindUnset_WhenDispatchMarkedStalled(t *testing.T) {
	t.Parallel()
	dataSource := sampleDiagnosticDataSource()
	dispatcher, store := newTestDiagnoseDispatcher(t, dataSource, &fakeHeadlessDiagnosticSessionCreator{}, &fakeDiagnoseNotifier{})
	svc := NewDiagnoseService(dispatcher, store)

	result, err := dispatcher.RequestDiagnosis(context.Background(), "e6c2a88e")
	require.NoError(t, err)
	require.NoError(t, store.MarkStalled(context.Background(), result.DispatchID))

	resp, err := svc.ListDiagnoseDispatches(context.Background(), connect.NewRequest(&sessionv1.ListDiagnoseDispatchesRequest{ItemId: "e6c2a88e"}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Dispatches, 1)
	require.Equal(t, sessionv1.DiagnoseDispatchStatus_DIAGNOSE_DISPATCH_STATUS_STALLED, resp.Msg.Dispatches[0].Status)
	require.Nil(t, resp.Msg.Dispatches[0].OutcomeKind, "a Stalled row must never carry an outcome_kind")
	require.NotNil(t, resp.Msg.Dispatches[0].CompletedAt)
}
