package session

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/sessionsummary"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/tokens"
)

// fakePoolClient is a minimal headless.PoolClient test double for
// SessionSummaryGenerator tests — records calls and can simulate an error,
// a panic, or blocking past a context deadline (for timeout tests).
type fakePoolClient struct {
	response string
	err      error
	panics   bool
	block    bool

	calls int
}

func (f *fakePoolClient) CallBlocking(ctx context.Context, _ headless.FeatureKey, _, _ string, _ headless.CallOptions, sink headless.CostSink) (string, error) {
	f.calls++
	if f.panics {
		panic("fakePoolClient: simulated panic")
	}
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	sink(0)
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

func newTestSessionSummaryGenerator(t *testing.T) (*SessionSummaryGenerator, *ent.Client, *fakePoolClient) {
	t.Helper()
	repo, cleanup := createTestEntRepository(t)
	t.Cleanup(cleanup)

	pool := &fakePoolClient{response: "A narrative."}
	gen := NewSessionSummaryGenerator(
		repo.client,
		pool,
		&fakeNotificationDecisionLister{},
		nil, // no token store — BuildCostSnapshot handles nil
		&fakeReviewQueueLookup{},
	)
	return gen, repo.client, pool
}

func getRow(t *testing.T, client *ent.Client, sessionUUID string) *ent.SessionSummary {
	t.Helper()
	row, err := client.SessionSummary.Query().Where(sessionsummary.SessionID(sessionUUID)).Only(context.Background())
	if err != nil {
		t.Fatalf("expected a row for session %s: %v", sessionUUID, err)
	}
	return row
}

// ---- Task 1.5.2e: guard / pre-check / cooldown / panic scenarios ----

func TestGenerateAndPersist_should_RejectSecondCall_When_FirstCallHoldsInFlightGuard(t *testing.T) {
	t.Parallel()
	gen, _, _ := newTestSessionSummaryGenerator(t)

	release, ok := gen.tryAcquire("sess-guard")
	if !ok {
		t.Fatal("expected first acquire to succeed")
	}
	defer release()

	_, ok2 := gen.tryAcquire("sess-guard")
	if ok2 {
		t.Fatal("expected second acquire to fail while the guard is held")
	}
}

// TestTryAcquire_should_RemoveInFlightMapEntry_When_ReleaseCalled guards against
// unbounded growth of g.inFlight: every session that ever triggers generation
// previously left a permanent *sync.Mutex entry in the map for the process's
// lifetime, since tryAcquire's release func only ever called Unlock and never
// removed the entry. release() must now also remove it (via CompareAndDelete)
// once it's done, and a session with a released guard must not still be
// reported in-flight afterward.
func TestTryAcquire_should_RemoveInFlightMapEntry_When_ReleaseCalled(t *testing.T) {
	t.Parallel()
	gen, _, _ := newTestSessionSummaryGenerator(t)
	sessionUUID := "sess-inflight-cleanup"

	release, ok := gen.tryAcquire(sessionUUID)
	if !ok {
		t.Fatal("expected first acquire to succeed")
	}

	if _, stillPresent := gen.inFlight.Load(sessionUUID); !stillPresent {
		t.Fatal("expected the map entry to exist while the guard is held")
	}

	release()

	if _, stillPresent := gen.inFlight.Load(sessionUUID); stillPresent {
		t.Fatal("expected release() to remove the inFlight map entry for the session")
	}
	if gen.isInFlight(sessionUUID) {
		t.Fatal("expected isInFlight to report false for a session with no map entry")
	}

	// A fresh acquire for the same session must succeed cleanly (not reuse stale
	// locked state left over from before release()).
	release2, ok2 := gen.tryAcquire(sessionUUID)
	if !ok2 {
		t.Fatal("expected a follow-up acquire to succeed after release()")
	}
	release2()
}

func TestGenerateAndPersist_should_SkipRegeneration_When_ReadyRowAndDiffCountsUnchanged(t *testing.T) {
	t.Parallel()
	gen, client, pool := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-dup"

	diffStats := &git.DiffStats{Added: 42, Removed: 7, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")
	row := getRow(t, client, sessionUUID)
	if row.Status != string(SessionSummaryStatusReady) {
		t.Fatalf("expected READY after first call, got %s", row.Status)
	}
	firstCalls := pool.calls

	// Second call with the exact same diff counts, non-manual reason: must short-circuit.
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")

	if pool.calls != firstCalls {
		t.Fatalf("expected no additional LLM call, got %d (was %d)", pool.calls, firstCalls)
	}
}

func TestGenerateAndPersist_should_ProceedWithRegeneration_When_ReadyRowButDiffCountsDiffer(t *testing.T) {
	t.Parallel()
	gen, client, pool := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-resume"

	first := &git.DiffStats{Added: 42, Removed: 7, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(first), first.Content, nil, "pty-eof")
	firstCalls := pool.calls

	// Resumed session did substantially more work — larger diff, same non-manual reason.
	second := &git.DiffStats{Added: 90, Removed: 12, Content: strings.Repeat("diff --git a/x.go b/x.go\n", 5)}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(second), second.Content, nil, "pty-eof")

	if pool.calls != firstCalls+1 {
		t.Fatalf("expected regeneration to call the LLM again, calls=%d (was %d)", pool.calls, firstCalls)
	}
	row := getRow(t, client, sessionUUID)
	if row.DiffAdded != 90 || row.DiffRemoved != 12 {
		t.Fatalf("expected row to reflect the new larger diff, got Added=%d Removed=%d", row.DiffAdded, row.DiffRemoved)
	}
}

func TestGenerateAndPersist_should_ProceedRegardlessOfDiffChange_When_ReasonIsManualRegenerate(t *testing.T) {
	t.Parallel()
	gen, client, pool := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-manual"

	diffStats := &git.DiffStats{Added: 5, Removed: 1}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")
	row := getRow(t, client, sessionUUID)
	// Push generated_at back beyond the cooldown so the manual-regenerate call proceeds.
	_, err := client.SessionSummary.UpdateOne(row).SetGeneratedAt(time.Now().Add(-time.Hour)).Save(ctx)
	if err != nil {
		t.Fatalf("failed to backdate generated_at: %v", err)
	}
	firstCalls := pool.calls

	// Same diff counts, but reason=manual-regenerate must still proceed (no short-circuit).
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, reasonManualRegenerate)

	if pool.calls != firstCalls+1 {
		t.Fatalf("expected manual-regenerate to always call the LLM, calls=%d (was %d)", pool.calls, firstCalls)
	}
}

func TestGenerateAndPersist_should_SkipWrite_When_ManualRegenerateWithinCooldown(t *testing.T) {
	t.Parallel()
	gen, client, pool := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-cooldown"

	diffStats := &git.DiffStats{Added: 5, Removed: 1}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")
	firstCalls := pool.calls
	beforeRow := getRow(t, client, sessionUUID)

	// Immediately click regenerate again — within regenerateCooldown of generated_at.
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, reasonManualRegenerate)

	if pool.calls != firstCalls {
		t.Fatalf("expected no LLM call within the cooldown window, calls=%d (was %d)", pool.calls, firstCalls)
	}
	afterRow := getRow(t, client, sessionUUID)
	if !afterRow.GeneratedAt.Equal(*beforeRow.GeneratedAt) {
		t.Fatalf("expected no write during cooldown, generated_at changed: %v -> %v", beforeRow.GeneratedAt, afterRow.GeneratedAt)
	}
}

// TestReconcileStaleness_should_NotStompFreshReadyRow_When_ConcurrentGenerationCompletesBetweenCheckAndWrite
// covers the TOCTOU fix: ReconcileStaleness reads a row snapshot, checks
// isInFlight, then writes ERROR — if a fresh GenerateAndPersist call
// completes (writing READY) in the window between the read and the write,
// an unconditional UpdateOne(row) would stomp that fresh READY row back to
// ERROR. The predicated bulk update must instead no-op when the row no
// longer matches the snapshot it was read from.
func TestReconcileStaleness_should_NotStompFreshReadyRow_When_ConcurrentGenerationCompletesBetweenCheckAndWrite(t *testing.T) {
	t.Parallel()
	gen, client, _ := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-toctou"

	staleStart := time.Now().Add(-10 * time.Minute)
	_, err := client.SessionSummary.Create().
		SetID("row-toctou").
		SetSessionID(sessionUUID).
		SetSessionTitle("title").
		SetStatus(string(SessionSummaryStatusGenerating)).
		SetGenerationStartedAt(staleStart).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to seed row: %v", err)
	}
	staleRow := getRow(t, client, sessionUUID)

	// Simulate a fresh GenerateAndPersist call completing concurrently, in the
	// window between ReconcileStaleness reading staleRow and its write landing:
	// the row transitions to READY with a new generation_started_at.
	freshNow := time.Now()
	if _, err := client.SessionSummary.Update().
		Where(sessionsummary.SessionID(sessionUUID)).
		SetStatus(string(SessionSummaryStatusReady)).
		SetGenerationStartedAt(freshNow).
		SetGeneratedAt(freshNow).
		Save(ctx); err != nil {
		t.Fatalf("failed to simulate concurrent fresh completion: %v", err)
	}

	// ReconcileStaleness operates on the stale snapshot, as GetSessionSummary's
	// read-then-reconcile path would have read it before the fresh completion
	// landed.
	result := gen.ReconcileStaleness(ctx, staleRow)

	// The predicated update must match 0 rows (generation_started_at no longer
	// equals staleStart) — a no-op, not an overwrite. The returned value is the
	// row as originally read, not a freshly-error'd one.
	if result.Status != string(SessionSummaryStatusGenerating) {
		t.Fatalf("expected ReconcileStaleness to return the stale snapshot unchanged, got status=%s", result.Status)
	}

	// Most importantly: the actual DB row must still be READY — the fresh
	// completion must survive, not get stomped back to ERROR.
	dbRow := getRow(t, client, sessionUUID)
	if dbRow.Status != string(SessionSummaryStatusReady) {
		t.Fatalf("expected the fresh READY row to survive ReconcileStaleness, got %s", dbRow.Status)
	}
}

func TestGenerateAndPersist_should_RecoverFromPanicAndReleaseGuard_When_BuilderPanics(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	sessionUUID := "sess-panic"

	gen := NewSessionSummaryGenerator(
		repo.client,
		&fakePoolClient{response: "narrative"},
		&fakeNotificationDecisionLister{err: nil, records: nil}, // replaced below with a panicking lister
		nil,
		&fakeReviewQueueLookup{},
	)
	gen.notifLister = &panickingNotificationDecisionLister{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		gen.GenerateAndPersist(context.Background(), sessionUUID, "title", time.Now(), BuildDiffSnapshot(&git.DiffStats{Added: 1}), "", nil, "pty-eof")
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("GenerateAndPersist did not return after a panic — recover() did not catch it")
	}

	// The guard must have been released — a follow-up call for the same session succeeds.
	release, ok := gen.tryAcquire(sessionUUID)
	if !ok {
		t.Fatal("expected the in-flight guard to be released after the panic")
	}
	release()
}

type panickingNotificationDecisionLister struct{}

func (panickingNotificationDecisionLister) ListDecisionRecords(context.Context, string) ([]DecisionRecord, error) {
	panic("panickingNotificationDecisionLister: simulated panic")
}

// ---- Task 1.5.2f: build/narrative/persist/fallback scenarios ----

func TestGenerateAndPersist_should_ReachReadyWithFallbackNarrative_When_TrivialSession(t *testing.T) {
	t.Parallel()
	gen, client, pool := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-trivial"

	createdAt := time.Now()
	gen.GenerateAndPersist(ctx, sessionUUID, "title", createdAt, DiffSnapshot{}, "", nil, "pty-eof")

	if pool.calls != 0 {
		t.Fatalf("expected no LLM call for a trivial session, got %d calls", pool.calls)
	}
	row := getRow(t, client, sessionUUID)
	if row.Status != string(SessionSummaryStatusReady) {
		t.Fatalf("expected READY, got %s", row.Status)
	}
	if !row.NarrativeFallbackUsed {
		t.Fatal("expected narrative_fallback_used=true for a trivial session")
	}
	if row.Narrative != narrativeFallbackTrivial {
		t.Fatalf("expected trivial fallback narrative, got %q", row.Narrative)
	}
}

func TestGenerateAndPersist_should_SetStatusErrorWithDecisionsStage_When_DecisionsBuilderFails(t *testing.T) {
	t.Parallel()
	gen, client, _ := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-decisions-err"

	// Seed a prior successful generation so we can assert its narrative/markdown
	// are left untouched by the error-path upsert.
	diffStats := &git.DiffStats{Added: 42, Removed: 7, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")
	priorRow := getRow(t, client, sessionUUID)
	if priorRow.Status != string(SessionSummaryStatusReady) {
		t.Fatalf("expected prior generation to succeed, got %s", priorRow.Status)
	}

	// Force the review-queue lookup to fail on the next call.
	gen.reviewLookup = &fakeReviewQueueLookup{err: context.DeadlineExceeded}
	newDiff := &git.DiffStats{Added: 90, Removed: 12, Content: strings.Repeat("diff --git a/x.go b/x.go\n", 5)}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(newDiff), newDiff.Content, nil, "pty-eof")

	row := getRow(t, client, sessionUUID)
	if row.Status != string(SessionSummaryStatusError) {
		t.Fatalf("expected ERROR, got %s", row.Status)
	}
	if row.ErrorStage != "decisions" {
		t.Fatalf("expected error_stage=decisions, got %q", row.ErrorStage)
	}
	// The already-computed (new) diff/timeline fields ARE persisted.
	if row.DiffAdded != 90 || row.DiffRemoved != 12 {
		t.Fatalf("expected the new diff to be persisted even on error, got Added=%d Removed=%d", row.DiffAdded, row.DiffRemoved)
	}
	// The prior successful generation's narrative/markdown are untouched.
	if row.Narrative != priorRow.Narrative {
		t.Fatalf("expected prior narrative to be preserved, got %q (was %q)", row.Narrative, priorRow.Narrative)
	}
	if row.Markdown != priorRow.Markdown {
		t.Fatal("expected prior markdown to be preserved on the decisions-stage error path")
	}
}

// TestGenerateAndPersist_should_ClearStaleErrorFields_When_SubsequentGenerationSucceeds
// covers the fix for a row left with error_message/error_stage set by a prior
// failed generation surviving unchanged into a later successful (READY) row.
// error_message/error_stage are Optional() ent fields with no Default(), so
// UpdateNewValues() alone (which only sets columns explicitly Set() on the
// create mutation) never clears them on the final success-path upsert unless
// it's done explicitly.
func TestGenerateAndPersist_should_ClearStaleErrorFields_When_SubsequentGenerationSucceeds(t *testing.T) {
	t.Parallel()
	gen, client, _ := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-clear-stale-error"

	// First call fails at the decisions stage, leaving error_message/error_stage set.
	gen.reviewLookup = &fakeReviewQueueLookup{err: context.DeadlineExceeded}
	firstDiff := &git.DiffStats{Added: 1, Removed: 1, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(firstDiff), firstDiff.Content, nil, "pty-eof")

	failedRow := getRow(t, client, sessionUUID)
	if failedRow.Status != string(SessionSummaryStatusError) {
		t.Fatalf("expected first generation to fail, got %s", failedRow.Status)
	}
	if failedRow.ErrorStage == "" || failedRow.ErrorMessage == "" {
		t.Fatalf("expected error_stage/error_message to be set after the first failure, got stage=%q message=%q", failedRow.ErrorStage, failedRow.ErrorMessage)
	}

	// Second call, with a working review-queue lookup, succeeds.
	gen.reviewLookup = &fakeReviewQueueLookup{}
	secondDiff := &git.DiffStats{Added: 2, Removed: 2, Content: strings.Repeat("diff --git a/x.go b/x.go\n", 3)}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(secondDiff), secondDiff.Content, nil, "pty-eof")

	row := getRow(t, client, sessionUUID)
	if row.Status != string(SessionSummaryStatusReady) {
		t.Fatalf("expected READY after the successful second generation, got %s", row.Status)
	}
	if row.ErrorStage != "" {
		t.Fatalf("expected error_stage to be cleared after a successful generation, got %q", row.ErrorStage)
	}
	if row.ErrorMessage != "" {
		t.Fatalf("expected error_message to be cleared after a successful generation, got %q", row.ErrorMessage)
	}
}

func TestGenerateAndPersist_should_UseFallbackNarrative_When_LLMCallFails(t *testing.T) {
	t.Parallel()
	gen, client, pool := newTestSessionSummaryGenerator(t)
	pool.err = context.DeadlineExceeded
	ctx := context.Background()
	sessionUUID := "sess-llm-fail"

	// Non-trivial: needs a diff so isTrivialSession is false and the LLM path is taken.
	diffStats := &git.DiffStats{Added: 5, Removed: 1, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")

	row := getRow(t, client, sessionUUID)
	if row.Status != string(SessionSummaryStatusReady) {
		t.Fatalf("expected READY despite LLM failure (graceful degradation), got %s", row.Status)
	}
	if !row.NarrativeFallbackUsed {
		t.Fatal("expected narrative_fallback_used=true")
	}
	if row.Narrative != narrativeFallbackLLMFailure {
		t.Fatalf("expected LLM-failure fallback narrative, got %q", row.Narrative)
	}
}

func TestGenerateAndPersist_should_TimeoutNarrativeCall_When_PoolBlocksLongerThanLlmNarrativeTimeout(t *testing.T) {
	t.Parallel()
	original := llmNarrativeTimeout
	llmNarrativeTimeout = 200 * time.Millisecond
	defer func() { llmNarrativeTimeout = original }()

	gen, client, pool := newTestSessionSummaryGenerator(t)
	pool.block = true
	ctx := context.Background()
	sessionUUID := "sess-llm-timeout"

	diffStats := &git.DiffStats{Added: 5, Removed: 1, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")

	row := getRow(t, client, sessionUUID)
	if row.Status != string(SessionSummaryStatusReady) {
		t.Fatalf("expected READY after narrative timeout (graceful degradation), got %s", row.Status)
	}
	if !row.NarrativeFallbackUsed {
		t.Fatal("expected narrative_fallback_used=true after a timed-out narrative call")
	}
}

func TestGenerateAndPersist_should_PersistDiffFieldsFromCapturedGitDiffStats_When_HappyPath(t *testing.T) {
	t.Parallel()
	gen, client, _ := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-happy-diff"

	diffStats := &git.DiffStats{
		Added:   42,
		Removed: 7,
		Content: "diff --git a/foo.go b/foo.go\ndiff --git a/bar.go b/bar.go\n",
	}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")

	row := getRow(t, client, sessionUUID)
	if row.DiffAdded != 42 || row.DiffRemoved != 7 || row.DiffFilesChanged != 2 {
		t.Fatalf("expected diff fields {42,7,2}, got Added=%d Removed=%d FilesChanged=%d", row.DiffAdded, row.DiffRemoved, row.DiffFilesChanged)
	}
	if !strings.Contains(row.Markdown, "[View full diff](") {
		t.Fatalf("expected rendered markdown to contain a diff link, got:\n%s", row.Markdown)
	}
}

func TestGenerateAndPersist_should_PersistTimelineFromInstanceCreatedAtAndDispatchTime_When_HappyPath(t *testing.T) {
	t.Parallel()
	gen, client, _ := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-happy-timeline"

	createdAt := time.Now().Add(-10 * time.Minute)
	diffStats := &git.DiffStats{Added: 1, Removed: 1, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", createdAt, BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")

	row := getRow(t, client, sessionUUID)
	if row.SessionStartedAt == nil || !row.SessionStartedAt.Equal(createdAt) {
		t.Fatalf("expected session_started_at=%v, got %v", createdAt, row.SessionStartedAt)
	}
	if row.SessionStoppedAt == nil {
		t.Fatal("expected session_stopped_at to be set")
	}
	if row.DurationMs == nil || *row.DurationMs < 9*60*1000 {
		t.Fatalf("expected duration_ms to reflect ~10 minutes, got %v", row.DurationMs)
	}
}

func TestGenerateAndPersist_should_UpsertBySessionIDNotEdge_When_HappyPath(t *testing.T) {
	t.Parallel()
	gen, client, _ := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-no-session-row-exists"

	// No Session ent row for this session_id exists anywhere — if SessionSummary
	// had an edge/FK to Session, this would fail. It succeeds because session_id
	// is a plain unique string field (session/ent/schema/session_summary.go has
	// no Edges() method).
	diffStats := &git.DiffStats{Added: 1, Removed: 1, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")

	row := getRow(t, client, sessionUUID)
	if row.Status != string(SessionSummaryStatusReady) {
		t.Fatalf("expected READY, got %s", row.Status)
	}
}

// TestSetNotificationListerAndSetTokenStore_should_NotRace_When_CalledConcurrentlyWithGenerateAndPersist
// reproduces the production ordering from server/dependencies.go: WireSessionSummaryListener
// runs at construction time (so a lifecycle event can dispatch GenerateAndPersist as a
// goroutine at any point afterwards), while SetNotificationLister/SetTokenStore are called
// later during server startup — a lifecycle event firing in that window races the bare
// field writes without lateBindMu. Run with `-race` to verify the guard closes that race;
// without it, `go test -race` flags a data race on notifLister/tokenStore.
func TestSetNotificationListerAndSetTokenStore_should_NotRace_When_CalledConcurrentlyWithGenerateAndPersist(t *testing.T) {
	t.Parallel()
	gen, _, _ := newTestSessionSummaryGenerator(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		gen.SetNotificationLister(&fakeNotificationDecisionLister{})
	}()
	go func() {
		defer wg.Done()
		gen.SetTokenStore(nil)
	}()
	go func() {
		defer wg.Done()
		diffStats := &git.DiffStats{Added: 1, Removed: 1, Content: "diff --git a/foo.go b/foo.go\n"}
		gen.GenerateAndPersist(ctx, "sess-race", "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")
	}()

	wg.Wait()
}

// TestGenerateAndPersist_should_ClearStaleTokenAndCostFields_When_SubsequentGenerationHasCostDataUnavailable
// covers the bonus fix: a generation N with real token-store data persists
// total_tokens/estimated_cost_usd, then generation N+1 (cost_data_unavailable=true,
// e.g. the token store has no data for this session anymore) must clear those two
// columns rather than leaving generation N's stale values in place alongside
// cost_data_unavailable=true.
func TestGenerateAndPersist_should_ClearStaleTokenAndCostFields_When_SubsequentGenerationHasCostDataUnavailable(t *testing.T) {
	t.Parallel()
	gen, client, _ := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-stale-cost"

	gen.tokenStore = &fakeTokenStoreReader{byUUID: map[string]*tokens.ParseResult{
		sessionUUID: {SessionUUID: sessionUUID, PrimaryModel: "claude-sonnet-4-5", TotalInput: 1000, TotalOutput: 200},
	}}
	diffStats := &git.DiffStats{Added: 1, Removed: 1, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(diffStats), diffStats.Content, nil, "pty-eof")

	priorRow := getRow(t, client, sessionUUID)
	if priorRow.CostDataUnavailable {
		t.Fatal("expected first generation to have cost data available")
	}
	if priorRow.TotalTokens == nil || *priorRow.TotalTokens == 0 {
		t.Fatalf("expected first generation to persist non-zero total_tokens, got %v", priorRow.TotalTokens)
	}
	if priorRow.EstimatedCostUsd == nil {
		t.Fatal("expected first generation to persist a non-nil estimated_cost_usd")
	}

	// Second generation: token store no longer has data for this session (e.g. the
	// underlying JSONL was rotated/cleared) — cost becomes unavailable. Uses a
	// different (larger) diff so the sequential-duplicate short-circuit for a READY
	// row doesn't skip this generation (reason stays "pty-eof", not
	// reasonManualRegenerate, to avoid the independent cooldown check rejecting a
	// same-second second call).
	gen.tokenStore = &fakeTokenStoreReader{}
	newDiff := &git.DiffStats{Added: 2, Removed: 2, Content: "diff --git a/bar.go b/bar.go\ndiff --git a/baz.go b/baz.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), BuildDiffSnapshot(newDiff), newDiff.Content, nil, "pty-eof")

	row := getRow(t, client, sessionUUID)
	if !row.CostDataUnavailable {
		t.Fatal("expected second generation to have cost_data_unavailable=true")
	}
	if row.TotalTokens != nil {
		t.Fatalf("expected total_tokens to be cleared (nil) when cost data is unavailable, got %v", *row.TotalTokens)
	}
	if row.EstimatedCostUsd != nil {
		t.Fatalf("expected estimated_cost_usd to be cleared (nil) when cost data is unavailable, got %v", *row.EstimatedCostUsd)
	}
}
