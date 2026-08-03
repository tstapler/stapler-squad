package session

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/sessionsummary"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
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

func (f *fakePoolClient) CallBlocking(ctx context.Context, _ headless.FeatureKey, _, _ string, _ headless.CallOptions) (string, float64, error) {
	f.calls++
	if f.panics {
		panic("fakePoolClient: simulated panic")
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

func TestGenerateAndPersist_should_SkipRegeneration_When_ReadyRowAndDiffCountsUnchanged(t *testing.T) {
	gen, client, pool := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-dup"

	diffStats := &git.DiffStats{Added: 42, Removed: 7, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), diffStats, nil, "pty-eof")
	row := getRow(t, client, sessionUUID)
	if row.Status != string(SessionSummaryStatusReady) {
		t.Fatalf("expected READY after first call, got %s", row.Status)
	}
	firstCalls := pool.calls

	// Second call with the exact same diff counts, non-manual reason: must short-circuit.
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), diffStats, nil, "pty-eof")

	if pool.calls != firstCalls {
		t.Fatalf("expected no additional LLM call, got %d (was %d)", pool.calls, firstCalls)
	}
}

func TestGenerateAndPersist_should_ProceedWithRegeneration_When_ReadyRowButDiffCountsDiffer(t *testing.T) {
	gen, client, pool := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-resume"

	first := &git.DiffStats{Added: 42, Removed: 7, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), first, nil, "pty-eof")
	firstCalls := pool.calls

	// Resumed session did substantially more work — larger diff, same non-manual reason.
	second := &git.DiffStats{Added: 90, Removed: 12, Content: strings.Repeat("diff --git a/x.go b/x.go\n", 5)}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), second, nil, "pty-eof")

	if pool.calls != firstCalls+1 {
		t.Fatalf("expected regeneration to call the LLM again, calls=%d (was %d)", pool.calls, firstCalls)
	}
	row := getRow(t, client, sessionUUID)
	if row.DiffAdded != 90 || row.DiffRemoved != 12 {
		t.Fatalf("expected row to reflect the new larger diff, got Added=%d Removed=%d", row.DiffAdded, row.DiffRemoved)
	}
}

func TestGenerateAndPersist_should_ProceedRegardlessOfDiffChange_When_ReasonIsManualRegenerate(t *testing.T) {
	gen, client, pool := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-manual"

	diffStats := &git.DiffStats{Added: 5, Removed: 1}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), diffStats, nil, "pty-eof")
	row := getRow(t, client, sessionUUID)
	// Push generated_at back beyond the cooldown so the manual-regenerate call proceeds.
	_, err := client.SessionSummary.UpdateOne(row).SetGeneratedAt(time.Now().Add(-time.Hour)).Save(ctx)
	if err != nil {
		t.Fatalf("failed to backdate generated_at: %v", err)
	}
	firstCalls := pool.calls

	// Same diff counts, but reason=manual-regenerate must still proceed (no short-circuit).
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), diffStats, nil, reasonManualRegenerate)

	if pool.calls != firstCalls+1 {
		t.Fatalf("expected manual-regenerate to always call the LLM, calls=%d (was %d)", pool.calls, firstCalls)
	}
}

func TestGenerateAndPersist_should_SkipWrite_When_ManualRegenerateWithinCooldown(t *testing.T) {
	gen, client, pool := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-cooldown"

	diffStats := &git.DiffStats{Added: 5, Removed: 1}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), diffStats, nil, "pty-eof")
	firstCalls := pool.calls
	beforeRow := getRow(t, client, sessionUUID)

	// Immediately click regenerate again — within regenerateCooldown of generated_at.
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), diffStats, nil, reasonManualRegenerate)

	if pool.calls != firstCalls {
		t.Fatalf("expected no LLM call within the cooldown window, calls=%d (was %d)", pool.calls, firstCalls)
	}
	afterRow := getRow(t, client, sessionUUID)
	if !afterRow.GeneratedAt.Equal(*beforeRow.GeneratedAt) {
		t.Fatalf("expected no write during cooldown, generated_at changed: %v -> %v", beforeRow.GeneratedAt, afterRow.GeneratedAt)
	}
}

func TestGenerateAndPersist_should_RecoverFromPanicAndReleaseGuard_When_BuilderPanics(t *testing.T) {
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
		gen.GenerateAndPersist(context.Background(), sessionUUID, "title", time.Now(), &git.DiffStats{Added: 1}, nil, "pty-eof")
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
	gen, client, pool := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-trivial"

	createdAt := time.Now()
	gen.GenerateAndPersist(ctx, sessionUUID, "title", createdAt, nil, nil, "pty-eof")

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
	gen, client, _ := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-decisions-err"

	// Seed a prior successful generation so we can assert its narrative/markdown
	// are left untouched by the error-path upsert.
	diffStats := &git.DiffStats{Added: 42, Removed: 7, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), diffStats, nil, "pty-eof")
	priorRow := getRow(t, client, sessionUUID)
	if priorRow.Status != string(SessionSummaryStatusReady) {
		t.Fatalf("expected prior generation to succeed, got %s", priorRow.Status)
	}

	// Force the review-queue lookup to fail on the next call.
	gen.reviewLookup = &fakeReviewQueueLookup{err: context.DeadlineExceeded}
	newDiff := &git.DiffStats{Added: 90, Removed: 12, Content: strings.Repeat("diff --git a/x.go b/x.go\n", 5)}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), newDiff, nil, "pty-eof")

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

func TestGenerateAndPersist_should_UseFallbackNarrative_When_LLMCallFails(t *testing.T) {
	gen, client, pool := newTestSessionSummaryGenerator(t)
	pool.err = context.DeadlineExceeded
	ctx := context.Background()
	sessionUUID := "sess-llm-fail"

	// Non-trivial: needs a diff so isTrivialSession is false and the LLM path is taken.
	diffStats := &git.DiffStats{Added: 5, Removed: 1, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), diffStats, nil, "pty-eof")

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
	original := llmNarrativeTimeout
	llmNarrativeTimeout = 200 * time.Millisecond
	defer func() { llmNarrativeTimeout = original }()

	gen, client, pool := newTestSessionSummaryGenerator(t)
	pool.block = true
	ctx := context.Background()
	sessionUUID := "sess-llm-timeout"

	diffStats := &git.DiffStats{Added: 5, Removed: 1, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), diffStats, nil, "pty-eof")

	row := getRow(t, client, sessionUUID)
	if row.Status != string(SessionSummaryStatusReady) {
		t.Fatalf("expected READY after narrative timeout (graceful degradation), got %s", row.Status)
	}
	if !row.NarrativeFallbackUsed {
		t.Fatal("expected narrative_fallback_used=true after a timed-out narrative call")
	}
}

func TestGenerateAndPersist_should_PersistDiffFieldsFromCapturedGitDiffStats_When_HappyPath(t *testing.T) {
	gen, client, _ := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-happy-diff"

	diffStats := &git.DiffStats{
		Added:   42,
		Removed: 7,
		Content: "diff --git a/foo.go b/foo.go\ndiff --git a/bar.go b/bar.go\n",
	}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), diffStats, nil, "pty-eof")

	row := getRow(t, client, sessionUUID)
	if row.DiffAdded != 42 || row.DiffRemoved != 7 || row.DiffFilesChanged != 2 {
		t.Fatalf("expected diff fields {42,7,2}, got Added=%d Removed=%d FilesChanged=%d", row.DiffAdded, row.DiffRemoved, row.DiffFilesChanged)
	}
	if !strings.Contains(row.Markdown, "[View full diff](") {
		t.Fatalf("expected rendered markdown to contain a diff link, got:\n%s", row.Markdown)
	}
}

func TestGenerateAndPersist_should_PersistTimelineFromInstanceCreatedAtAndDispatchTime_When_HappyPath(t *testing.T) {
	gen, client, _ := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-happy-timeline"

	createdAt := time.Now().Add(-10 * time.Minute)
	diffStats := &git.DiffStats{Added: 1, Removed: 1, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", createdAt, diffStats, nil, "pty-eof")

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
	gen, client, _ := newTestSessionSummaryGenerator(t)
	ctx := context.Background()
	sessionUUID := "sess-no-session-row-exists"

	// No Session ent row for this session_id exists anywhere — if SessionSummary
	// had an edge/FK to Session, this would fail. It succeeds because session_id
	// is a plain unique string field (session/ent/schema/session_summary.go has
	// no Edges() method).
	diffStats := &git.DiffStats{Added: 1, Removed: 1, Content: "diff --git a/foo.go b/foo.go\n"}
	gen.GenerateAndPersist(ctx, sessionUUID, "title", time.Now(), diffStats, nil, "pty-eof")

	row := getRow(t, client, sessionUUID)
	if row.Status != string(SessionSummaryStatusReady) {
		t.Fatalf("expected READY, got %s", row.Status)
	}
}
