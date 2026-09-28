package services

// diagnose_dispatch_session_creator_test.go covers Phase 7's concrete
// HeadlessDiagnosticSessionCreator implementation: the synchronous
// ItemSession-row recording (and its failure/pool-unavailable paths), plus
// the detached goroutine's end-of-call bookkeeping.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
)

// fakeDiagnoseHeadlessCaller is a fake diagnoseHeadlessCaller. block, when
// non-nil, is read from before CallBlocking returns, letting a test observe
// state while the call is still "running".
type fakeDiagnoseHeadlessCaller struct {
	mu    sync.Mutex
	calls []headless.CallOptions
	err   error
	block chan struct{}
}

func (f *fakeDiagnoseHeadlessCaller) CallBlocking(_ context.Context, _ headless.FeatureKey, _, _ string, opts headless.CallOptions, _ headless.CostSink) (string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, opts)
	f.mu.Unlock()
	if f.block != nil {
		<-f.block
	}
	return "", f.err
}

func (f *fakeDiagnoseHeadlessCaller) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// endCall records one UpdateItemSessionEndedWithReason invocation.
type endCall struct {
	id     string
	reason string
}

// fakeDiagnoseSessionPersistence is a fake diagnoseSessionPersistence.
// ended is a buffered channel a test can receive from to know the detached
// goroutine reached its end-of-call bookkeeping, without a sleep.
type fakeDiagnoseSessionPersistence struct {
	mu        sync.Mutex
	created   []session.ItemSessionData
	createErr error
	createID  string
	endCalls  []endCall
	ended     chan struct{}
}

func newFakeDiagnoseSessionPersistence() *fakeDiagnoseSessionPersistence {
	return &fakeDiagnoseSessionPersistence{createID: "item-session-1", ended: make(chan struct{}, 1)}
}

func (f *fakeDiagnoseSessionPersistence) CreateItemSession(_ context.Context, data session.ItemSessionData) (session.ItemSessionSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, data)
	if f.createErr != nil {
		return session.ItemSessionSummary{}, f.createErr
	}
	return session.ItemSessionSummary{ID: f.createID}, nil
}

func (f *fakeDiagnoseSessionPersistence) UpdateItemSessionEndedWithReason(_ context.Context, id string, _ time.Time, reason string) error {
	f.mu.Lock()
	f.endCalls = append(f.endCalls, endCall{id: id, reason: reason})
	f.mu.Unlock()
	select {
	case f.ended <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakeDiagnoseSessionPersistence) endCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.endCalls)
}

func sampleHeadlessDiagnosticSessionRequest() HeadlessDiagnosticSessionRequest {
	return HeadlessDiagnosticSessionRequest{
		ItemID:                "e6c2a88e",
		DiagnosticSessionUUID: HeadlessDiagnosticSessionIDPrefix + "e6c2a88e-test",
		RepoPath:              "/repo",
		Prompt:                "diagnose this stuck item",
	}
}

// TestHeadlessDiagnosticSessionCreator_CreateHeadlessDiagnosticSession_ShouldRecordItemSessionAndReturnNil_WhenPoolAvailable
// covers the synchronous half: the ItemSession row is created with
// SessionRoleDiagnose before the method returns, and no error is returned
// just because dispatch is still running in the background.
func TestHeadlessDiagnosticSessionCreator_CreateHeadlessDiagnosticSession_ShouldRecordItemSessionAndReturnNil_WhenPoolAvailable(t *testing.T) {
	t.Parallel()
	pool := &fakeDiagnoseHeadlessCaller{block: make(chan struct{})}
	storage := newFakeDiagnoseSessionPersistence()
	creator := NewDiagnoseDispatchSessionCreator(pool, storage)

	err := creator.CreateHeadlessDiagnosticSession(context.Background(), sampleHeadlessDiagnosticSessionRequest())
	require.NoError(t, err)

	require.Len(t, storage.created, 1)
	require.Equal(t, "e6c2a88e", storage.created[0].ItemID)
	require.Equal(t, session.SessionRoleDiagnose, storage.created[0].SessionRole)

	close(pool.block)
	<-storage.ended
}

// TestHeadlessDiagnosticSessionCreator_CreateHeadlessDiagnosticSession_ShouldReturnError_WhenPoolNil
// covers the guard: a nil pool fails synchronously rather than silently
// no-oping, so DiagnoseDispatcher.dispatchAndRecord can translate it into a
// DispatchFailed outcome (Story 5.1.3).
func TestHeadlessDiagnosticSessionCreator_CreateHeadlessDiagnosticSession_ShouldReturnError_WhenPoolNil(t *testing.T) {
	t.Parallel()
	storage := newFakeDiagnoseSessionPersistence()
	creator := NewDiagnoseDispatchSessionCreator(nil, storage)

	err := creator.CreateHeadlessDiagnosticSession(context.Background(), sampleHeadlessDiagnosticSessionRequest())
	require.Error(t, err)
	require.Empty(t, storage.created, "no ItemSession row should be created when the pool is unavailable")
}

// TestHeadlessDiagnosticSessionCreator_CreateHeadlessDiagnosticSession_ShouldReturnError_WhenCreateItemSessionFails
// covers the other synchronous failure path.
func TestHeadlessDiagnosticSessionCreator_CreateHeadlessDiagnosticSession_ShouldReturnError_WhenCreateItemSessionFails(t *testing.T) {
	t.Parallel()
	pool := &fakeDiagnoseHeadlessCaller{}
	storage := newFakeDiagnoseSessionPersistence()
	storage.createErr = errors.New("db unavailable")
	creator := NewDiagnoseDispatchSessionCreator(pool, storage)

	err := creator.CreateHeadlessDiagnosticSession(context.Background(), sampleHeadlessDiagnosticSessionRequest())
	require.Error(t, err)
	require.Equal(t, 0, pool.callCount(), "the headless call must never run when the ItemSession row failed to record")
}

// TestHeadlessDiagnosticSessionCreator_CreateHeadlessDiagnosticSession_ShouldEndItemSessionWithCompletedReason_WhenCallSucceeds
// covers the detached goroutine's happy path: WorkDir is forwarded from
// RepoPath and the ItemSession row is ended with reason "completed".
func TestHeadlessDiagnosticSessionCreator_CreateHeadlessDiagnosticSession_ShouldEndItemSessionWithCompletedReason_WhenCallSucceeds(t *testing.T) {
	t.Parallel()
	pool := &fakeDiagnoseHeadlessCaller{}
	storage := newFakeDiagnoseSessionPersistence()
	creator := NewDiagnoseDispatchSessionCreator(pool, storage)

	req := sampleHeadlessDiagnosticSessionRequest()
	require.NoError(t, creator.CreateHeadlessDiagnosticSession(context.Background(), req))

	select {
	case <-storage.ended:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the diagnostic call's goroutine to end the item session")
	}

	require.Len(t, pool.calls, 1)
	require.Equal(t, req.RepoPath, pool.calls[0].WorkDir)

	require.Equal(t, 1, storage.endCallCount())
	require.Equal(t, "completed", storage.endCalls[0].reason)
	require.Equal(t, "item-session-1", storage.endCalls[0].id)
}

// TestHeadlessDiagnosticSessionCreator_CreateHeadlessDiagnosticSession_ShouldRestrictToolSurface_ExcludingRunCommand
// is a regression test: the dispatched diagnostic agent must never run with
// an unrestricted tool surface. Previously, CallOptions carried no
// AllowedTools/DisallowedTools at all, so the agent had access to
// run_command (server/mcp/tools_terminal.go) -- a write-capable MCP tool with
// no NudgeGate check, no identity reverification, and no cap/cooldown,
// completely bypassing ADR-002/ADR-003. AllowedTools must now name exactly
// the tool surface session/diagnose/prompt.go's instructionBlock() documents,
// and must never contain "run_command" in any form.
func TestHeadlessDiagnosticSessionCreator_CreateHeadlessDiagnosticSession_ShouldRestrictToolSurface_ExcludingRunCommand(t *testing.T) {
	t.Parallel()
	pool := &fakeDiagnoseHeadlessCaller{}
	storage := newFakeDiagnoseSessionPersistence()
	creator := NewDiagnoseDispatchSessionCreator(pool, storage)

	req := sampleHeadlessDiagnosticSessionRequest()
	require.NoError(t, creator.CreateHeadlessDiagnosticSession(context.Background(), req))

	select {
	case <-storage.ended:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the diagnostic call's goroutine to end the item session")
	}

	require.Len(t, pool.calls, 1)
	allowed := pool.calls[0].AllowedTools
	require.NotEmpty(t, allowed, "the dispatched diagnostic agent must have an explicit AllowedTools scope, not an unrestricted tool surface")
	require.NotContains(t, allowed, "run_command", "run_command must never be reachable by the dispatched diagnostic agent")

	for _, want := range []string{
		"mcp__stapler-squad__create_backlog_item",
		"mcp__stapler-squad__post_backlog_update",
		"mcp__stapler-squad__resume_session",
		"mcp__stapler-squad__steer_session",
		"mcp__stapler-squad__write_to_session",
	} {
		require.Contains(t, allowed, want)
	}
}

// TestHeadlessDiagnosticSessionCreator_CreateHeadlessDiagnosticSession_ShouldEndItemSessionWithFailureReason_WhenCallErrors
// covers the detached goroutine's failure path: a CallBlocking error still
// ends the ItemSession row (never leaves it open forever), with a
// distinguishable reason -- the underlying DiagnoseDispatch row is left
// Pending for Story 6.1.5's stale-dispatch sweeper to eventually mark
// Stalled, since no outcome was ever recorded.
func TestHeadlessDiagnosticSessionCreator_CreateHeadlessDiagnosticSession_ShouldEndItemSessionWithFailureReason_WhenCallErrors(t *testing.T) {
	t.Parallel()
	pool := &fakeDiagnoseHeadlessCaller{err: errors.New("claude: exit status 1")}
	storage := newFakeDiagnoseSessionPersistence()
	creator := NewDiagnoseDispatchSessionCreator(pool, storage)

	require.NoError(t, creator.CreateHeadlessDiagnosticSession(context.Background(), sampleHeadlessDiagnosticSessionRequest()))

	select {
	case <-storage.ended:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the diagnostic call's goroutine to end the item session")
	}

	require.Equal(t, 1, storage.endCallCount())
	require.Equal(t, "diagnostic_call_failed", storage.endCalls[0].reason)
}
