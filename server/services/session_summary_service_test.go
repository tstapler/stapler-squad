package services

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/sessionsummary"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
)

// newTestSessionSummaryEntClient constructs a fresh in-memory-on-disk sqlite-backed
// *ent.Client for tests, mirroring session package's own createTestEntRepository
// (session/ent_repository_test.go) — that helper is unexported and lives in a
// different package, so this is a thin, test-local re-implementation using the
// exported session.NewEntRepository/session.WithDatabasePath constructors.
func newTestSessionSummaryEntClient(t *testing.T) *ent.Client {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), fmt.Sprintf("session-summary-test-%d.db", time.Now().UnixNano()))
	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo.GetEntClient()
}

// fakePoolClient is a minimal headless.PoolClient test double, mirroring the shape
// of session package's own fakePoolClient (session/session_summary_service_test.go)
// which cannot be reused directly across the package boundary. started fires once
// CallBlocking has been entered (used by tests that need to know the in-flight guard
// is now held before proceeding), block gates return until ctx is cancelled.
type fakePoolClient struct {
	mu       sync.Mutex
	response string
	err      error
	block    bool
	calls    int
	lastUser string

	started chan struct{}
}

func (f *fakePoolClient) CallBlocking(ctx context.Context, _ headless.FeatureKey, _, userPrompt string, _ headless.CallOptions) (string, float64, error) {
	f.mu.Lock()
	f.calls++
	f.lastUser = userPrompt
	f.mu.Unlock()

	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
	}
	if f.block {
		<-ctx.Done()
		return "", 0, ctx.Err()
	}
	if f.err != nil {
		return "", 0, f.err
	}
	return f.response, 0, nil
}

func (f *fakePoolClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakePoolClient) lastUserPrompt() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastUser
}

// fakeLiveInstanceFinder is a trivial liveInstanceFinder test double.
type fakeLiveInstanceFinder struct {
	instances map[string]*session.Instance
}

func (f *fakeLiveInstanceFinder) FindLiveInstance(sessionID string) *session.Instance {
	if f.instances == nil {
		return nil
	}
	return f.instances[sessionID]
}

// getRow fetches the current SessionSummary row for sessionID, failing the test if
// it doesn't exist.
func getRow(t *testing.T, client *ent.Client, sessionID string) *ent.SessionSummary {
	t.Helper()
	row, err := client.SessionSummary.Query().Where(sessionsummary.SessionID(sessionID)).Only(context.Background())
	require.NoError(t, err)
	return row
}

// waitForStatus polls until sessionID's row reaches one of the given statuses (or
// the timeout elapses), returning the final observed row. Used to wait for the
// detached goroutine dispatched by RegenerateSessionSummary to finish without
// coupling the test to internal timing. Only safe to use when no row already has
// one of the wanted statuses before the goroutine being waited on runs — otherwise
// it can match a stale prior row instead of the new write; use
// waitForNewGeneratedRow for the "regenerating an already-READY row" case.
func waitForStatus(t *testing.T, client *ent.Client, sessionID string, timeout time.Duration, want ...session.SessionSummaryStatus) *ent.SessionSummary {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		row, err := client.SessionSummary.Query().Where(sessionsummary.SessionID(sessionID)).Only(context.Background())
		lastErr = err
		if err == nil {
			for _, w := range want {
				if row.Status == string(w) {
					return row
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for session %s to reach status %v (last err=%v)", sessionID, want, lastErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitForNewGeneratedRow polls until sessionID's row reaches READY with a
// generated_at strictly after the given marker time, returning it. Needed when a
// row is already READY before the goroutine under test runs (e.g. regenerating an
// already-generated summary) — waitForStatus would otherwise match the stale row
// immediately instead of waiting for the new write.
func waitForNewGeneratedRow(t *testing.T, client *ent.Client, sessionID string, after time.Time, timeout time.Duration) *ent.SessionSummary {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		row, err := client.SessionSummary.Query().Where(sessionsummary.SessionID(sessionID)).Only(context.Background())
		if err == nil && row.Status == string(session.SessionSummaryStatusReady) && row.GeneratedAt != nil && row.GeneratedAt.After(after) {
			return row
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for session %s to produce a new generated row after %v (last err=%v)", sessionID, after, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ---- GetSessionSummary ----

func TestGetSessionSummary_should_ReturnNilSummary_When_NoRowExists(t *testing.T) {
	entClient := newTestSessionSummaryEntClient(t)
	gen := session.NewSessionSummaryGenerator(entClient, &fakePoolClient{}, nil, nil, nil)
	svc := NewSessionSummaryService(gen, nil)

	resp, err := svc.GetSessionSummary(context.Background(), connect.NewRequest(&sessionv1.GetSessionSummaryRequest{SessionId: "does-not-exist"}))
	require.NoError(t, err)
	require.Nil(t, resp.Msg.Summary)
}

func TestGetSessionSummary_should_ReturnRow_When_Found(t *testing.T) {
	entClient := newTestSessionSummaryEntClient(t)
	pool := &fakePoolClient{response: "A narrative."}
	gen := session.NewSessionSummaryGenerator(entClient, pool, nil, nil, nil)
	svc := NewSessionSummaryService(gen, nil)

	sessionID := "sess-found"
	diffStats := &git.DiffStats{Added: 5, Removed: 1, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(context.Background(), sessionID, "my title", time.Now(), session.BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")

	resp, err := svc.GetSessionSummary(context.Background(), connect.NewRequest(&sessionv1.GetSessionSummaryRequest{SessionId: sessionID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)
	require.Equal(t, sessionID, resp.Msg.Summary.SessionId)
	require.Equal(t, "my title", resp.Msg.Summary.SessionTitle)
	require.Equal(t, sessionv1.SessionSummaryStatus_SESSION_SUMMARY_STATUS_READY, resp.Msg.Summary.Status)
	require.Equal(t, int32(5), resp.Msg.Summary.Diff.Added)
	require.Equal(t, int32(1), resp.Msg.Summary.Diff.Removed)
}

func TestGetSessionSummary_should_ReconcileStaleGeneratingRowToError_When_GuardNotHeld(t *testing.T) {
	entClient := newTestSessionSummaryEntClient(t)
	gen := session.NewSessionSummaryGenerator(entClient, &fakePoolClient{}, nil, nil, nil)
	svc := NewSessionSummaryService(gen, nil)

	sessionID := "sess-stale"
	staleStart := time.Now().Add(-10 * time.Minute)
	_, err := entClient.SessionSummary.Create().
		SetID("row-stale").
		SetSessionID(sessionID).
		SetSessionTitle("stale title").
		SetStatus(string(session.SessionSummaryStatusGenerating)).
		SetGenerationStartedAt(staleStart).
		Save(context.Background())
	require.NoError(t, err)

	resp, err := svc.GetSessionSummary(context.Background(), connect.NewRequest(&sessionv1.GetSessionSummaryRequest{SessionId: sessionID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)
	require.Equal(t, sessionv1.SessionSummaryStatus_SESSION_SUMMARY_STATUS_ERROR, resp.Msg.Summary.Status)
	require.Equal(t, "restart-interrupted", resp.Msg.Summary.ErrorStage)

	// Confirm the write actually landed, not just the in-memory response.
	row := getRow(t, entClient, sessionID)
	require.Equal(t, string(session.SessionSummaryStatusError), row.Status)
}

func TestGetSessionSummary_should_LeaveStaleGeneratingRowUnchanged_When_GuardHeld(t *testing.T) {
	entClient := newTestSessionSummaryEntClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := &fakePoolClient{block: true, started: make(chan struct{}, 1)}
	gen := session.NewSessionSummaryGenerator(entClient, pool, nil, nil, nil)
	svc := NewSessionSummaryService(gen, nil)

	sessionID := "sess-in-flight"
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Non-trivial + non-empty diff so the LLM (blocking) path is taken, and an
		// old CreatedAt so isTrivialSession's duration check doesn't skip the LLM call.
		diffStats := &git.DiffStats{Added: 1, Removed: 0, Content: "diff --git a/foo.go b/foo.go\n"}
		gen.GenerateAndPersist(ctx, sessionID, "title", time.Now().Add(-time.Hour), session.BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")
	}()

	select {
	case <-pool.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the in-flight generation to reach the LLM call")
	}

	// The interim GENERATING row GenerateAndPersist wrote has a fresh
	// generation_started_at; backdate it so ReconcileStaleness's time check alone
	// wouldn't already leave it unchanged — isolating the isInFlight guard as the
	// reason the row stays GENERATING.
	row := getRow(t, entClient, sessionID)
	_, err := entClient.SessionSummary.UpdateOne(row).SetGenerationStartedAt(time.Now().Add(-10 * time.Minute)).Save(context.Background())
	require.NoError(t, err)

	resp, err := svc.GetSessionSummary(context.Background(), connect.NewRequest(&sessionv1.GetSessionSummaryRequest{SessionId: sessionID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)
	require.Equal(t, sessionv1.SessionSummaryStatus_SESSION_SUMMARY_STATUS_GENERATING, resp.Msg.Summary.Status)

	cancel()
	<-done
}

// ---- RegenerateSessionSummary ----

func TestRegenerateSessionSummary_should_ReturnNotFound_When_NoRowAndNoLiveInstance(t *testing.T) {
	entClient := newTestSessionSummaryEntClient(t)
	gen := session.NewSessionSummaryGenerator(entClient, &fakePoolClient{}, nil, nil, nil)
	svc := NewSessionSummaryService(gen, &fakeLiveInstanceFinder{})

	_, err := svc.RegenerateSessionSummary(context.Background(), connect.NewRequest(&sessionv1.RegenerateSessionSummaryRequest{SessionId: "nope"}))
	require.Error(t, err)
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestRegenerateSessionSummary_should_NotTriggerSecondPipeline_When_AlreadyGenerating(t *testing.T) {
	entClient := newTestSessionSummaryEntClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := &fakePoolClient{block: true, started: make(chan struct{}, 1)}
	gen := session.NewSessionSummaryGenerator(entClient, pool, nil, nil, nil)
	svc := NewSessionSummaryService(gen, &fakeLiveInstanceFinder{})

	sessionID := "sess-dedup"
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		diffStats := &git.DiffStats{Added: 1, Content: "diff --git a/foo.go b/foo.go\n"}
		gen.GenerateAndPersist(ctx, sessionID, "title", time.Now().Add(-time.Hour), session.BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")
	}()

	select {
	case <-pool.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first generation to reach the LLM call")
	}
	require.Equal(t, 1, pool.callCount())

	resp, err := svc.RegenerateSessionSummary(context.Background(), connect.NewRequest(&sessionv1.RegenerateSessionSummaryRequest{SessionId: sessionID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)
	require.Equal(t, sessionv1.SessionSummaryStatus_SESSION_SUMMARY_STATUS_GENERATING, resp.Msg.Summary.Status)

	// Give the dispatched (and, per the dedup guard, immediately-rejected) second
	// goroutine a moment to run and confirm it never reached the LLM call.
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, 1, pool.callCount(), "expected the dedup guard to reject the second GenerateAndPersist call")

	cancel()
	<-firstDone
}

func TestRegenerateSessionSummary_should_RefreshDiffAndGoalFromLiveInstance_When_LiveInstancePresent(t *testing.T) {
	entClient := newTestSessionSummaryEntClient(t)
	pool := &fakePoolClient{response: "A narrative."}
	gen := session.NewSessionSummaryGenerator(entClient, pool, nil, nil, nil)

	sessionID := "sess-live"
	// Seed a prior generation with a different title, well past the regenerate cooldown.
	priorDiffStats := &git.DiffStats{Added: 42, Removed: 7}
	gen.GenerateAndPersist(context.Background(), sessionID, "stale persisted title", time.Now(), session.BuildDiffSnapshot(priorDiffStats), priorDiffStats.Content, nil, "pty-eof")
	priorRow := getRow(t, entClient, sessionID)
	_, err := entClient.SessionSummary.UpdateOne(priorRow).SetGeneratedAt(time.Now().Add(-time.Hour)).Save(context.Background())
	require.NoError(t, err)

	liveInst := &session.Instance{
		Title:     "fresh live title",
		UUID:      sessionID,
		CreatedAt: time.Now().Add(-time.Hour), // old enough that isTrivialSession's duration check doesn't skip the LLM call
	}
	liveInst.SetSessionGoalCached(&session.SessionGoalData{Goal: "a very particular live goal"})

	svc := NewSessionSummaryService(gen, &fakeLiveInstanceFinder{instances: map[string]*session.Instance{sessionID: liveInst}})

	before := time.Now()
	resp, err := svc.RegenerateSessionSummary(context.Background(), connect.NewRequest(&sessionv1.RegenerateSessionSummaryRequest{SessionId: sessionID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)

	finalRow := waitForNewGeneratedRow(t, entClient, sessionID, before, 2*time.Second)
	require.Equal(t, "fresh live title", finalRow.SessionTitle, "expected the live instance's title, not the previously persisted one")
	require.Contains(t, pool.lastUserPrompt(), "a very particular live goal", "expected the live instance's session goal to reach the narrative prompt")
}

func TestRegenerateSessionSummary_should_FallBackToPersistedFieldsWithNilGoal_When_NoLiveInstance(t *testing.T) {
	entClient := newTestSessionSummaryEntClient(t)
	pool := &fakePoolClient{response: "A narrative."}
	gen := session.NewSessionSummaryGenerator(entClient, pool, nil, nil, nil)

	sessionID := "sess-no-live"
	// Two "diff --git" header lines so DiffFilesChanged is unambiguously non-zero
	// (and distinguishable from Added/Removed) — regression coverage for the bug
	// where the no-live-instance fallback re-derived FilesChanged from an empty
	// Content string instead of carrying over the persisted row's count.
	priorDiffStats := &git.DiffStats{Added: 42, Removed: 7, Content: "diff --git a/foo.go b/foo.go\ndiff --git a/bar.go b/bar.go\n"}
	gen.GenerateAndPersist(context.Background(), sessionID, "persisted title", time.Now(), session.BuildDiffSnapshot(priorDiffStats), priorDiffStats.Content, nil, "pty-eof")
	priorRow := getRow(t, entClient, sessionID)
	require.Equal(t, 1, pool.callCount())
	require.Equal(t, 2, priorRow.DiffFilesChanged, "sanity check: prior generation persisted a non-zero files-changed count")
	_, err := entClient.SessionSummary.UpdateOne(priorRow).SetGeneratedAt(time.Now().Add(-time.Hour)).Save(context.Background())
	require.NoError(t, err)

	svc := NewSessionSummaryService(gen, &fakeLiveInstanceFinder{}) // no live instance for sessionID

	before := time.Now()
	resp, err := svc.RegenerateSessionSummary(context.Background(), connect.NewRequest(&sessionv1.RegenerateSessionSummaryRequest{SessionId: sessionID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)

	finalRow := waitForNewGeneratedRow(t, entClient, sessionID, before, 2*time.Second)
	require.Equal(t, "persisted title", finalRow.SessionTitle)
	require.Equal(t, 42, finalRow.DiffAdded)
	require.Equal(t, 7, finalRow.DiffRemoved)
	require.Equal(t, 2, finalRow.DiffFilesChanged, "expected the no-live-instance fallback to carry over the persisted row's diff_files_changed rather than re-deriving it from an empty diff Content and zeroing it")
	require.Equal(t, 2, pool.callCount(), "expected regeneration to call the LLM again")
	require.False(t, strings.Contains(pool.lastUserPrompt(), "Session goal:"), "expected no session goal line when no live instance is available")
}

func TestRegenerateSessionSummary_should_DispatchWithBackgroundContext_When_RequestContextIsCancelledImmediately(t *testing.T) {
	entClient := newTestSessionSummaryEntClient(t)
	pool := &fakePoolClient{response: "A narrative."}
	gen := session.NewSessionSummaryGenerator(entClient, pool, nil, nil, nil)

	sessionID := "sess-detached-ctx"
	liveInst := &session.Instance{Title: "t", UUID: sessionID, CreatedAt: time.Now()}
	svc := NewSessionSummaryService(gen, &fakeLiveInstanceFinder{instances: map[string]*session.Instance{sessionID: liveInst}})

	reqCtx, reqCancel := context.WithCancel(context.Background())
	resp, err := svc.RegenerateSessionSummary(reqCtx, connect.NewRequest(&sessionv1.RegenerateSessionSummaryRequest{SessionId: sessionID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)

	// Cancel the request context the instant the handler returns, simulating
	// ConnectRPC tearing down the request as soon as the RPC completes.
	reqCancel()

	// GenerateAndPersist's ent writes (the interim GENERATING upsert and the final
	// READY upsert) are both made with whatever ctx it was dispatched with. If the
	// handler had wired reqCtx through instead of context.Background(), those
	// writes would fail against the now-cancelled reqCtx and the row would never
	// reach READY — so reaching READY here is proof the pipeline is running on an
	// independent, uncancelled context.
	finalRow := waitForStatus(t, entClient, sessionID, 2*time.Second, session.SessionSummaryStatusReady, session.SessionSummaryStatusError)
	require.Equal(t, string(session.SessionSummaryStatusReady), finalRow.Status, "expected the async pipeline to complete successfully despite the request ctx being cancelled")
}
