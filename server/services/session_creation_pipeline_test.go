package services

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// TestBackgroundResolutionPipeline_should_ContinueRunning_When_RPCContextIsCanceled
// is the regression test for research/pitfalls.md §2 (Story 2.2.1): the RPC's
// own context being canceled -- e.g. the client disconnecting right after
// CreateSession returns -- must not cancel the in-progress background clone,
// since the pipeline runs against context.WithTimeout(context.WithoutCancel(rpcCtx), ...),
// never a direct derivative of rpcCtx.
func TestBackgroundResolutionPipeline_should_ContinueRunning_When_RPCContextIsCanceled(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	resolvedPath := t.TempDir()
	initGitRepoWithCommit(t, resolvedPath)
	resolved := make(chan struct{})
	fix.svc.githubResolver = func(ctx context.Context, input string, enterpriseHosts []string) (string, *session.GitHubRef, error) {
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
			return "", nil, ctx.Err()
		}
		defer close(resolved)
		return resolvedPath, &session.GitHubRef{Owner: "acme", Repo: "widgets", Branch: "main"}, nil
	}

	rpcCtx, rpcCancel := context.WithCancel(context.Background())
	resp, err := fix.svc.CreateSession(rpcCtx, connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "epic22-rpc-ctx-canceled",
		Path:  "https://github.com/acme/widgets",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, fix.svc, resp.Msg.Session.Id) })

	// Simulate the client disconnecting immediately after the RPC returns.
	rpcCancel()

	select {
	case <-resolved:
		// The deferred clone ran to completion despite the RPC context above
		// already being canceled -- exactly the property this test exists to
		// verify.
	case <-time.After(2 * time.Second):
		t.Fatal("background resolution pipeline did not continue running after the RPC context was canceled")
	}

	require.Eventually(t, func() bool {
		inst := fix.svc.FindLiveInstance(resp.Msg.Session.Id)
		return inst != nil && session.Status(inst.GetStatus()) == session.Active
	}, 3*time.Second, 20*time.Millisecond, "pipeline must still reach Active after the RPC context cancellation")
}

// TestBackgroundResolutionPipeline_should_WriteFailed_When_ResolutionExceedsTimeout
// covers Story 2.2.1's other acceptance criterion: once the Background
// Resolution Context's own timeout elapses, the pipeline's context is done,
// any in-flight resolution call observes ctx.Done(), and the pipeline makes a
// terminal Failed write rather than hanging forever.
func TestBackgroundResolutionPipeline_should_WriteFailed_When_ResolutionExceedsTimeout(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)
	fix.svc.creationResolutionTimeout = 50 * time.Millisecond

	sawTimeout := make(chan struct{})
	fix.svc.githubResolver = func(ctx context.Context, input string, enterpriseHosts []string) (string, *session.GitHubRef, error) {
		<-ctx.Done()
		close(sawTimeout)
		return "", nil, ctx.Err()
	}

	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "epic22-resolution-timeout",
		Path:  "https://github.com/acme/widgets",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, fix.svc, resp.Msg.Session.Id) })

	select {
	case <-sawTimeout:
	case <-time.After(2 * time.Second):
		t.Fatal("githubResolver's context never became Done within creationResolutionTimeout")
	}

	require.Eventually(t, func() bool {
		inst := fix.svc.FindLiveInstance(resp.Msg.Session.Id)
		return inst != nil && session.Status(inst.GetStatus()) == session.Failed
	}, 2*time.Second, 20*time.Millisecond, "a resolution that exceeds the Background Resolution Context's timeout must produce a terminal Failed write")

	inst := fix.svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst)
	assert.Equal(t, "GitHubResolutionError", inst.FailureReason())
}

// TestBackgroundResolutionPipeline_should_PublishProgressPerPhase_When_GithubURLSession
// covers Story 2.2.2's acceptance criterion: a GitHub-URL session's pipeline
// transitions through every named Creation Phase, in order, each backed by a
// SessionUpdatedEvent{"creation_progress"} publish.
//
// Phase text is captured via creationPhaseHook (a test-only seam), not by
// re-reading the live session.Instance.CreationProgress field off a
// subsequently-received eventBus event -- the pipeline's own goroutine can
// race ahead to a later phase (or terminal completion, clearing the field to
// "") before a slower test consumer goroutine gets scheduled to dequeue and
// read an earlier event, which would make an event-bus-only version of this
// test flaky by construction.
func TestBackgroundResolutionPipeline_should_PublishProgressPerPhase_When_GithubURLSession(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	resolvedPath := t.TempDir()
	initGitRepoWithCommit(t, resolvedPath)
	fix.svc.githubResolver = func(ctx context.Context, input string, enterpriseHosts []string) (string, *session.GitHubRef, error) {
		return resolvedPath, &session.GitHubRef{Owner: "acme", Repo: "widgets", Branch: "main"}, nil
	}

	var mu sync.Mutex
	var phases []string
	fix.svc.creationPhaseHook = func(msg string) {
		mu.Lock()
		phases = append(phases, msg)
		mu.Unlock()
	}

	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "epic22-progress-phases",
		Path:  "https://github.com/acme/widgets",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, fix.svc, resp.Msg.Session.Id) })

	require.Eventually(t, func() bool {
		inst := fix.svc.FindLiveInstance(resp.Msg.Session.Id)
		return inst != nil && session.Status(inst.GetStatus()) == session.Active
	}, 3*time.Second, 20*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{
		"Resolving GitHub URL...",
		"Cloning repository...",
		"Resolving defaults...",
		"Setting up worktree...",
		"Starting session...",
	}, phases)
}

// TestBackgroundResolutionPipeline_should_CompleteWithoutNetworkIO_When_PlainDirectorySession
// covers Story 2.2.2's third acceptance criterion: a plain directory session
// (no GitHub URL) never calls the network-resolving githubResolver and
// completes in low-single-digit milliseconds.
func TestBackgroundResolutionPipeline_should_CompleteWithoutNetworkIO_When_PlainDirectorySession(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	var resolverCalled atomic.Bool
	fix.svc.githubResolver = func(ctx context.Context, input string, enterpriseHosts []string) (string, *session.GitHubRef, error) {
		resolverCalled.Store(true)
		return "", nil, fmt.Errorf("githubResolver must not be called for a plain directory session")
	}

	start := time.Now()
	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "epic22-plain-directory",
		Path:  t.TempDir(),
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, fix.svc, resp.Msg.Session.Id) })

	require.Eventually(t, func() bool {
		inst := fix.svc.FindLiveInstance(resp.Msg.Session.Id)
		return inst != nil && session.Status(inst.GetStatus()) == session.Active
	}, 3*time.Second, 5*time.Millisecond)
	elapsed := time.Since(start)

	assert.False(t, resolverCalled.Load(), "githubResolver must never be invoked for a plain directory session")
	assert.Less(t, elapsed, 500*time.Millisecond, "a plain directory session's pipeline must complete with no network I/O latency")
}

// TestBackgroundResolutionPipeline_should_WriteFailedAndNotCrashProcess_When_PhaseFuncPanics
// covers Task 2.2.2d: a panic inside any phase of the pipeline goroutine must
// be recovered, logged, and turned into a terminal Failed write -- not an
// unrecovered panic that would crash the whole process (this test's own
// survival to its final assertion is part of that proof).
func TestBackgroundResolutionPipeline_should_WriteFailedAndNotCrashProcess_When_PhaseFuncPanics(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	fix.svc.githubResolver = func(ctx context.Context, input string, enterpriseHosts []string) (string, *session.GitHubRef, error) {
		panic("simulated phase panic")
	}

	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "epic22-phase-panic",
		Path:  "https://github.com/acme/widgets",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, fix.svc, resp.Msg.Session.Id) })

	require.Eventually(t, func() bool {
		inst := fix.svc.FindLiveInstance(resp.Msg.Session.Id)
		return inst != nil && session.Status(inst.GetStatus()) == session.Failed
	}, 2*time.Second, 20*time.Millisecond, "a panic inside the pipeline must be recovered into a terminal Failed write")

	// The process (and this test) is still running to check this at all --
	// the panic did not crash it.
	inst := fix.svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst)
	assert.Equal(t, "StartupError", inst.FailureReason())
}

// TestBackgroundResolutionPipeline_should_TransitionToActive_When_ResolutionSucceeds
// covers Story 2.2.3's happy path: a successful pipeline run commits a terminal
// Active write (with no failure reason) via commitTerminalStatus, carrying the
// resolved GitHub path/branch through to the final instance state.
func TestBackgroundResolutionPipeline_should_TransitionToActive_When_ResolutionSucceeds(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	resolvedPath := t.TempDir()
	initGitRepoWithCommit(t, resolvedPath)
	fix.svc.githubResolver = func(ctx context.Context, input string, enterpriseHosts []string) (string, *session.GitHubRef, error) {
		return resolvedPath, &session.GitHubRef{Owner: "acme", Repo: "widgets", Branch: "feature-x"}, nil
	}

	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "epic22-resolution-success",
		Path:  "https://github.com/acme/widgets",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, fix.svc, resp.Msg.Session.Id) })

	require.Eventually(t, func() bool {
		inst := fix.svc.FindLiveInstance(resp.Msg.Session.Id)
		return inst != nil && session.Status(inst.GetStatus()) == session.Active
	}, 3*time.Second, 20*time.Millisecond)

	inst := fix.svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst)
	assert.Empty(t, inst.FailureReason())
	// Read via Snapshot(), not the raw Path/Branch fields directly -- those
	// are actor-owned and can still be concurrently touched by the
	// session driver/controller goroutines Start() wires up even after
	// Status reads Active (caught by -race).
	snap := inst.Snapshot()
	assert.Equal(t, resolvedPath, snap.Path)
	assert.Equal(t, "feature-x", snap.Branch)

	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	found := false
	for _, l := range loaded {
		if l.GetStableID() == inst.GetStableID() {
			found = true
			assert.Equal(t, session.Active, l.Status, "persisted row must reflect the terminal Active write")
		}
	}
	assert.True(t, found)
}

// TestBackgroundResolutionPipeline_should_SkipTerminalWrite_When_EpochAlreadyBumped
// covers Story 2.2.3's fencing acceptance criterion: if the captured epoch a
// pipeline invocation presents no longer matches the persisted row's
// creation_epoch (as if a cancel/retry had already bumped it, per ADR-002),
// the pipeline's terminal write must be a silent no-op -- no in-memory state
// change. Presents a deliberately stale epoch directly to
// runBackgroundResolutionPipeline (white-box, same package) rather than
// exercising a live epoch-bump, mirroring
// TestUpdateInstanceIfEpoch_should_ReturnFalse_When_EpochIsStale's technique:
// a freshly-created row's persisted creation_epoch defaults to 0, so any
// non-zero captured epoch reproduces "already superseded" without needing
// Epic 3.2's not-yet-implemented Cancel RPC to actually bump it.
func TestBackgroundResolutionPipeline_should_SkipTerminalWrite_When_EpochAlreadyBumped(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	dir := t.TempDir()
	instance, err := session.CreateManagedInstance(context.Background(), session.CreateManagedInstanceParams{
		Options: session.InstanceOptions{
			Title:            "epic22-stale-epoch",
			Path:             dir,
			Program:          "claude",
			SessionType:      session.SessionTypeDirectory,
			TmuxServerSocket: fix.svc.testTmuxServerSocket,
		},
		Storage: fix.storage,
	})
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, fix.svc, instance.GetStableID()) })

	fix.svc.runBackgroundResolutionPipeline(context.Background(), creationPipelineParams{
		instance:        instance,
		epoch:           1, // stale: the persisted row's creation_epoch defaults to 0
		instanceTitle:   instance.Title,
		instanceRootDir: instance.GetEffectiveRootDir(),
	})

	assert.Equal(t, session.Creating, session.Status(instance.GetStatus()),
		"in-memory status must be untouched when the pipeline's captured epoch is already stale")

	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	found := false
	for _, l := range loaded {
		if l.GetStableID() == instance.GetStableID() {
			found = true
			assert.Equal(t, session.Creating, l.Status,
				"persisted row must be unchanged when the pipeline's captured epoch is already stale")
		}
	}
	assert.True(t, found)
}

// TestCreateSession_should_PublishExactlyOneTerminalEvent_When_CancelRacesSuccess
// is Task 2.2.3c's explicit race test (research/pitfalls.md §4): a cancel-path
// terminal write and a success-path terminal write, racing for the same
// instance/epoch, must let exactly one terminal SessionUpdatedEvent publish --
// commitTerminalStatus's durable-write-then-in-memory-CAS ordering (ADR-002)
// is what this test's outcome depends on.
//
// Deliberately exercises commitTerminalStatus directly (both racers, each
// publishing on its own win exactly as the pipeline's terminal() closure
// does) against a plain addCreatingSession fixture, rather than racing a live
// CreateSession pipeline's real p.instance.Start(true) call: Start() contains
// its own pre-existing, epoch-unaware transitionTo(Active) side effect
// (session/instance.go's startLocked) that fires unconditionally once
// worktree/tmux setup succeeds -- entirely independent of, and not
// synchronized against, the epoch-fenced commitTerminalStatus mechanism this
// test exists to verify. Racing a real Start() call would make this test's
// outcome depend on that unrelated, out-of-scope interaction (which
// Instance.Start() callers everywhere else rely on) rather than on the
// specific guarantee Epic 2.2/1.2 introduced. commitTerminalStatus is exactly
// the shared primitive both the pipeline's real success path and Epic 3.2's
// (not-yet-implemented) Cancel RPC will call, so exercising it directly here
// is a faithful, deterministic test of the guarantee itself.
//
// Run with `go test -race -count=50 -run
// TestCreateSession_should_PublishExactlyOneTerminalEvent_When_CancelRacesSuccess`
// per validation.md.
func TestCreateSession_should_PublishExactlyOneTerminalEvent_When_CancelRacesSuccess(t *testing.T) {
	storage := createTestStorage(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	t.Cleanup(svc.Shutdown)

	title := fmt.Sprintf("epic22-cancel-race-%d", time.Now().UnixNano())
	inst := addCreatingSession(t, storage, title, title)
	// Actor-backed (matching every real production instance, which is always
	// constructed via NewLiveInstance): TryForceStatusIfEpoch's Status read
	// ahead of its i.mu-guarded write is only race-free when the actor
	// mailbox serializes concurrent callers onto one goroutine -- see
	// sendSyncErr's doc comment ("If liveInstance is nil ... fn runs
	// directly on the calling goroutine without locking"), and
	// TestTryForceStatusIfEpoch_should_ProduceExactlyOneWinner_When_CalledConcurrently
	// (session/instance_epoch_test.go) for the same requirement on the
	// lower-level primitive this test races at the commitTerminalStatus layer.
	li := session.NewLiveInstance(inst)
	defer li.Stop()

	ch, _ := bus.Subscribe(context.Background())

	var wg sync.WaitGroup
	results := make([]bool, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		results[0] = commitTerminalStatus(context.Background(), storage, inst, 0, session.Active, "")
		if results[0] {
			bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status", "creation_progress"}))
		}
	}()
	go func() {
		defer wg.Done()
		results[1] = commitTerminalStatus(context.Background(), storage, inst, 0, session.Failed, "Cancelled")
		if results[1] {
			bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status", "creation_progress"}))
		}
	}()
	wg.Wait()

	assert.NotEqual(t, results[0], results[1], "exactly one of the two racing terminal writes must apply")

	terminalCount := 0
	timeout := time.After(500 * time.Millisecond)
drain:
	for {
		select {
		case ev := <-ch:
			for _, f := range ev.UpdatedFields {
				if f == "status" {
					terminalCount++
					break
				}
			}
		case <-timeout:
			break drain
		}
	}

	assert.Equal(t, 1, terminalCount, "exactly one terminal SessionUpdatedEvent must publish under a cancel/success race")
}
