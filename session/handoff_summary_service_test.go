package session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/handoffsummary"
	"github.com/tstapler/stapler-squad/session/ent/hook"
	"github.com/tstapler/stapler-squad/session/headless"
)

// newTestHandoffSummaryGenerator builds a HandoffSummaryGenerator backed by an
// in-memory ent client and a fakePoolClient (defined in
// session_summary_service_test.go, shared across this package's generator
// tests).
func newTestHandoffSummaryGenerator(t *testing.T) (*HandoffSummaryGenerator, *ent.Client, *fakePoolClient) {
	t.Helper()
	repo, cleanup := createTestEntRepository(t)
	t.Cleanup(cleanup)

	pool := &fakePoolClient{response: "This session made progress on the task.\n\n## Active Task\nImplement the remaining validation logic."}
	gen := NewHandoffSummaryGenerator(repo.client, pool)
	return gen, repo.client, pool
}

// getHandoffRow fetches the HandoffSummary row for sessionID, failing the test
// if it does not exist.
func getHandoffRow(t *testing.T, client *ent.Client, sessionID string) *ent.HandoffSummary {
	t.Helper()
	row, err := client.HandoffSummary.Query().Where(handoffsummary.SessionID(sessionID)).Only(context.Background())
	if err != nil {
		t.Fatalf("expected a handoff summary row for session %s: %v", sessionID, err)
	}
	return row
}

// writeHandoffConversationFixture points $HOME at a fresh temp directory and
// writes a per-session conversation JSONL file at the exact path
// findConversationFilePath (session/history.go) walks: $HOME/.claude/projects/<any-dir>/<file>.jsonl,
// with each line's "sessionId" field set to sessionID (matched via substring
// search on the file's first 5 lines). roles/contents must be the same length
// and alternate "user"/"assistant" to exercise buildTranscriptWindow's
// head/middle/tail split realistically.
//
// Callers must not also call t.Parallel() — t.Setenv forbids parallel tests.
func writeHandoffConversationFixture(t *testing.T, sessionID string, roles, contents []string) {
	t.Helper()
	if len(roles) != len(contents) {
		t.Fatalf("roles and contents must be the same length, got %d and %d", len(roles), len(contents))
	}

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	projectDir := filepath.Join(tempHome, ".claude", "projects", "test-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("failed to create fixture project dir: %v", err)
	}

	path := filepath.Join(projectDir, sessionID+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create fixture conversation file: %v", err)
	}
	defer f.Close()

	for i, role := range roles {
		entry := map[string]any{
			"type":      role,
			"uuid":      uuid.New().String(),
			"sessionId": sessionID,
			"timestamp": time.Now().Add(time.Duration(i) * time.Second).Format(time.RFC3339),
			"cwd":       tempHome,
			"message": map[string]any{
				"role":    role,
				"content": contents[i],
			},
		}
		line, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("failed to marshal fixture entry: %v", err)
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			t.Fatalf("failed to write fixture entry: %v", err)
		}
	}
}

// defaultFixtureMessages is a realistic 8-message transcript (4 exchanges),
// large enough to exercise buildTranscriptWindow's head(2)/middle(4)/tail(2)
// split rather than the short-conversation even-split path.
func defaultFixtureMessages() (roles, contents []string) {
	roles = []string{"user", "assistant", "user", "assistant", "user", "assistant", "user", "assistant"}
	contents = []string{
		"Please add input validation to the signup form.",
		"Sure — I'll start by looking at the existing form component.",
		"Also make sure email format is checked.",
		"Added a regex-based email check in validateSignup.go.",
		"What about the password strength requirement?",
		"Added a minimum-length check; working on the complexity rule now.",
		"Looks good, can you also handle empty submissions?",
		"Added an empty-field guard before the other validations run.",
	}
	return roles, contents
}

// ---- blockingPoolClient: a race-safe fake for the concurrent-dedup test ----

// blockingPoolClient records exactly how many times CallBlocking was entered
// (via an atomic counter, safe to read concurrently) and signals entered once
// the first call has begun, then blocks until ctx is done. Used instead of the
// shared fakePoolClient (whose plain int `calls` field is not race-safe to
// read from a different goroutine than the one blocked inside CallBlocking).
type blockingPoolClient struct {
	calls   atomic.Int32
	entered chan struct{}
	once    sync.Once
}

func newBlockingPoolClient() *blockingPoolClient {
	return &blockingPoolClient{entered: make(chan struct{})}
}

func (f *blockingPoolClient) CallBlocking(ctx context.Context, _ headless.FeatureKey, _, _ string, _ headless.CallOptions, _ headless.CostSink) (string, error) {
	f.calls.Add(1)
	f.once.Do(func() { close(f.entered) })
	<-ctx.Done()
	return "", ctx.Err()
}

// TestGenerateAndPersist_DedupsConcurrentCallsForSameSession covers the
// in-flight guard: a second concurrent call for a session already generating
// must be dropped — no row written by the second call, and no additional pool
// call — while the first call is genuinely still in flight (blocked mid-pool-call).
func TestGenerateAndPersist_DedupsConcurrentCallsForSameSession(t *testing.T) {
	sessionID := "sess-dedup"
	roles, contents := defaultFixtureMessages()
	writeHandoffConversationFixture(t, sessionID, roles, contents)

	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	pool := newBlockingPoolClient()
	gen := NewHandoffSummaryGenerator(repo.client, pool)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		gen.GenerateAndPersist(ctx, sessionID, "title")
	}()

	select {
	case <-pool.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first call never reached the pool")
	}

	// Second concurrent call: must return immediately without writing a row
	// or calling the pool, since the first call still holds the guard.
	gen.GenerateAndPersist(context.Background(), sessionID, "title")

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("first call did not finish after its context was canceled")
	}

	if got := pool.calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 pool call, got %d", got)
	}

	//nolint:entfullscan test assertion over a test-scoped in-memory DB; verifies exactly one row exists.
	rows, err := repo.client.HandoffSummary.Query().All(context.Background())
	if err != nil {
		t.Fatalf("failed to query handoff summary rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 row (from the first call only), got %d", len(rows))
	}
}

// TestGenerateAndPersist_SuccessTransitionsToReadyWithSummaryText covers the
// happy path: PENDING -> GENERATING -> READY, with summary_text/active_task
// populated from a successful pool call.
func TestGenerateAndPersist_SuccessTransitionsToReadyWithSummaryText(t *testing.T) {
	sessionID := "sess-success"
	roles, contents := defaultFixtureMessages()
	writeHandoffConversationFixture(t, sessionID, roles, contents)

	gen, client, _ := newTestHandoffSummaryGenerator(t)

	gen.GenerateAndPersist(context.Background(), sessionID, "title")

	row := getHandoffRow(t, client, sessionID)
	if row.Status != string(HandoffSummaryStatusReady) {
		t.Fatalf("expected READY, got %s", row.Status)
	}
	if row.SummaryText == "" {
		t.Fatal("expected non-empty summary_text")
	}
	if !strings.HasPrefix(row.SummaryText, "[CONTEXT COMPACTION") {
		t.Fatalf("expected summary_text to start with the reference-only prefix, got %q", row.SummaryText[:min(60, len(row.SummaryText))])
	}
	if row.ActiveTask == "" {
		t.Fatal("expected non-empty active_task")
	}
	if row.GeneratedAt == nil {
		t.Fatal("expected generated_at to be set")
	}
}

// TestGenerateAndPersist_PoolFailureTransitionsToError covers the pool-error
// path: the row must land in ERROR (stage "generation"), not stay stuck in
// GENERATING.
func TestGenerateAndPersist_PoolFailureTransitionsToError(t *testing.T) {
	sessionID := "sess-pool-failure"
	roles, contents := defaultFixtureMessages()
	writeHandoffConversationFixture(t, sessionID, roles, contents)

	gen, client, pool := newTestHandoffSummaryGenerator(t)
	poolErr := context.DeadlineExceeded
	pool.err = poolErr

	gen.GenerateAndPersist(context.Background(), sessionID, "title")

	row := getHandoffRow(t, client, sessionID)
	if row.Status != string(HandoffSummaryStatusError) {
		t.Fatalf("expected ERROR, got %s", row.Status)
	}
	if row.ErrorStage != "generation" {
		t.Fatalf("expected error_stage=generation, got %q", row.ErrorStage)
	}
	// headless.GenerateHandoffSummary wraps the pool error
	// ("GenerateHandoffSummary: %w") before returning it — the same wrapping
	// convention GenerateSessionCompletionNarrative uses in
	// session_summary_service.go. ErrorMessage is that wrapped err.Error(),
	// which still contains the pool error's text verbatim.
	if !strings.Contains(row.ErrorMessage, poolErr.Error()) {
		t.Fatalf("expected error_message to contain %q, got %q", poolErr.Error(), row.ErrorMessage)
	}
}

// TestGenerateAndPersist_TranscriptFailureTransitionsToError directly covers
// the transcript-read failure path: $HOME points at a fresh temp dir with no
// conversation-file fixture written for sourceSessionID, so
// findConversationFilePath (session/history.go) fails and GenerateAndPersist
// must land the row in ERROR with error_stage="transcript" rather than
// leaving it stuck in GENERATING.
func TestGenerateAndPersist_TranscriptFailureTransitionsToError(t *testing.T) {
	sessionID := "sess-transcript-failure"
	t.Setenv("HOME", t.TempDir())

	gen, client, _ := newTestHandoffSummaryGenerator(t)

	gen.GenerateAndPersist(context.Background(), sessionID, "title")

	row := getHandoffRow(t, client, sessionID)
	if row.Status != string(HandoffSummaryStatusError) {
		t.Fatalf("expected ERROR, got %s", row.Status)
	}
	if row.ErrorStage != "transcript" {
		t.Fatalf("expected error_stage=transcript, got %q", row.ErrorStage)
	}
}

// TestGenerateAndPersist_PersistFailureTransitionsToError covers the
// highest-consequence failure path flagged by GenerateAndPersist's own
// "persist" stage comment: the LLM call succeeds (cost already spent) but the
// final READY write fails. An ent mutation hook forces exactly that write to
// error (matched by status=="ready", so the interim GENERATING upsert and the
// upsertHandoffSummaryError fallback write are unaffected) -- the row must
// still land in ERROR with error_stage="persist" via failStage's best-effort
// fallback write, not stay stuck in GENERATING.
func TestGenerateAndPersist_PersistFailureTransitionsToError(t *testing.T) {
	sessionID := "sess-persist-failure"
	roles, contents := defaultFixtureMessages()
	writeHandoffConversationFixture(t, sessionID, roles, contents)

	gen, client, _ := newTestHandoffSummaryGenerator(t)

	persistErr := errors.New("simulated persist failure")
	client.HandoffSummary.Use(func(next ent.Mutator) ent.Mutator {
		return hook.HandoffSummaryFunc(func(ctx context.Context, m *ent.HandoffSummaryMutation) (ent.Value, error) {
			if status, ok := m.Status(); ok && status == string(HandoffSummaryStatusReady) {
				return nil, persistErr
			}
			return next.Mutate(ctx, m)
		})
	})

	gen.GenerateAndPersist(context.Background(), sessionID, "title")

	row := getHandoffRow(t, client, sessionID)
	if row.Status != string(HandoffSummaryStatusError) {
		t.Fatalf("expected ERROR, got %s", row.Status)
	}
	if row.ErrorStage != "persist" {
		t.Fatalf("expected error_stage=persist, got %q", row.ErrorStage)
	}
	if !strings.Contains(row.ErrorMessage, persistErr.Error()) {
		t.Fatalf("expected error_message to contain %q, got %q", persistErr.Error(), row.ErrorMessage)
	}
}

// TestGenerateAndPersist_PanicIsRecoveredAndGuardIsReleased covers panic
// safety: a panic inside the pipeline (here, inside the pool call) must be
// recovered without crashing the test binary, and must not leave the
// in-flight guard permanently held.
func TestGenerateAndPersist_PanicIsRecoveredAndGuardIsReleased(t *testing.T) {
	sessionID := "sess-panic"
	roles, contents := defaultFixtureMessages()
	writeHandoffConversationFixture(t, sessionID, roles, contents)

	gen, _, pool := newTestHandoffSummaryGenerator(t)
	pool.panics = true

	done := make(chan struct{})
	go func() {
		defer close(done)
		gen.GenerateAndPersist(context.Background(), sessionID, "title")
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("GenerateAndPersist did not return after a panic — recover() did not catch it")
	}

	release, ok := gen.tryAcquire(sessionID)
	if !ok {
		t.Fatal("expected the in-flight guard to be released after the panic")
	}
	release()
}

// TestGenerateAndPersist_TimesOutAndTransitionsToError_When_PoolCallExceedsDeadline
// covers Adversarial-review Blocker #1: the summarization call has a hard
// deadline (handoffSummaryTimeout), and a pool call blocking past it must
// still resolve the row to ERROR (stage "generation") rather than hang
// forever, releasing the dedup guard.
func TestGenerateAndPersist_TimesOutAndTransitionsToError_When_PoolCallExceedsDeadline(t *testing.T) {
	sessionID := "sess-timeout"
	roles, contents := defaultFixtureMessages()
	writeHandoffConversationFixture(t, sessionID, roles, contents)

	original := handoffSummaryTimeout
	handoffSummaryTimeout = 200 * time.Millisecond
	defer func() { handoffSummaryTimeout = original }()

	gen, client, pool := newTestHandoffSummaryGenerator(t)
	pool.block = true

	done := make(chan struct{})
	go func() {
		defer close(done)
		gen.GenerateAndPersist(context.Background(), sessionID, "title")
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("GenerateAndPersist did not return after the pool call exceeded its deadline")
	}

	row := getHandoffRow(t, client, sessionID)
	if row.Status != string(HandoffSummaryStatusError) {
		t.Fatalf("expected ERROR after timeout, got %s", row.Status)
	}
	if row.ErrorStage != "generation" {
		t.Fatalf("expected error_stage=generation, got %q", row.ErrorStage)
	}

	release, ok := gen.tryAcquire(sessionID)
	if !ok {
		t.Fatal("expected the in-flight guard to be released after the timeout")
	}
	release()
}

// capturingPoolClient wraps fakePoolClient to additionally record the last
// user prompt CallBlocking was invoked with, so a test can inspect exactly
// what GenerateHandoffSummary sent to the LLM (e.g. to confirm an oversized
// message was truncated before it ever reached the prompt).
type capturingPoolClient struct {
	fakePoolClient
	lastUserPrompt string
}

func (f *capturingPoolClient) CallBlocking(ctx context.Context, key headless.FeatureKey, system, user string, opts headless.CallOptions, sink headless.CostSink) (string, error) {
	f.lastUserPrompt = user
	return f.fakePoolClient.CallBlocking(ctx, key, system, user, opts, sink)
}

// TestGenerateAndPersist_TruncatesOversizedHeadAndTailMessages covers the fix
// for Head/Tail messages bypassing the per-message byte cap: an oversized
// message in either Head or Tail (e.g. a large pasted file in the first
// turn) must be truncated via pruneMessages before it reaches the prompt
// sent to the LLM, exactly like Middle already is via applySummaryBudget.
func TestGenerateAndPersist_TruncatesOversizedHeadAndTailMessages(t *testing.T) {
	sessionID := "sess-oversized-head-tail"
	oversizedHead := strings.Repeat("h", 20000)
	oversizedTail := strings.Repeat("t", 20000)
	roles := []string{"user", "assistant", "user", "assistant", "user", "assistant", "user", "assistant"}
	contents := []string{
		oversizedHead, // Head[0] -- oversized
		"ack",         // Head[1]
		"middle 1",
		"middle 2",
		"middle 3",
		"middle 4",
		"second to last", // Tail[0]
		oversizedTail,    // Tail[1] -- oversized
	}
	writeHandoffConversationFixture(t, sessionID, roles, contents)

	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	pool := &capturingPoolClient{fakePoolClient: fakePoolClient{response: "A narrative.\n\n## Active Task\nnext step"}}
	gen := NewHandoffSummaryGenerator(repo.client, pool)

	gen.GenerateAndPersist(context.Background(), sessionID, "title")

	row := getHandoffRow(t, repo.client, sessionID)
	if row.Status != string(HandoffSummaryStatusReady) {
		t.Fatalf("expected READY, got %s", row.Status)
	}

	if strings.Contains(pool.lastUserPrompt, oversizedHead) {
		t.Fatal("expected the oversized Head message to be truncated, but it appeared verbatim in the prompt")
	}
	if strings.Contains(pool.lastUserPrompt, oversizedTail) {
		t.Fatal("expected the oversized Tail message to be truncated, but it appeared verbatim in the prompt")
	}
	if got := strings.Count(pool.lastUserPrompt, "... [truncated]"); got != 2 {
		t.Fatalf("expected exactly 2 truncation markers (one for Head, one for Tail), got %d in prompt:\n%s", got, pool.lastUserPrompt)
	}
}

// TestHandoffReconcileStaleness_should_NotStompFreshReadyRow_When_ConcurrentGenerationCompletesBetweenCheckAndWrite
// is the HandoffSummaryGenerator counterpart to SessionSummaryGenerator's
// identically-shaped
// TestReconcileStaleness_should_NotStompFreshReadyRow_When_ConcurrentGenerationCompletesBetweenCheckAndWrite
// (session_summary_service_test.go) -- named with a Handoff prefix since both
// live in package session and would otherwise collide. It covers the same
// TOCTOU fix: ReconcileStaleness reads a row snapshot, checks isInFlight,
// then writes ERROR via a predicated update. If a fresh GenerateAndPersist
// call completes (writing READY) in the window between the read and the
// write, the predicated update must no-op (0 rows affected, since
// generation_started_at no longer matches the stale snapshot) rather than
// stomping the fresh READY row back to ERROR.
func TestHandoffReconcileStaleness_should_NotStompFreshReadyRow_When_ConcurrentGenerationCompletesBetweenCheckAndWrite(t *testing.T) {
	t.Parallel()
	gen, client, _ := newTestHandoffSummaryGenerator(t)
	ctx := context.Background()
	sessionID := "sess-handoff-toctou"

	staleStart := time.Now().Add(-10 * time.Minute)
	_, err := client.HandoffSummary.Create().
		SetID(uuid.New().String()).
		SetSessionID(sessionID).
		SetSessionTitle("title").
		SetStatus(string(HandoffSummaryStatusGenerating)).
		SetGenerationStartedAt(staleStart).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to seed row: %v", err)
	}
	staleRow := getHandoffRow(t, client, sessionID)

	// Simulate a fresh GenerateAndPersist call completing concurrently, in the
	// window between ReconcileStaleness reading staleRow and its write
	// landing: the row transitions to READY with a new
	// generation_started_at.
	freshNow := time.Now()
	if _, err := client.HandoffSummary.Update().
		Where(handoffsummary.SessionID(sessionID)).
		SetStatus(string(HandoffSummaryStatusReady)).
		SetGenerationStartedAt(freshNow).
		SetGeneratedAt(freshNow).
		SetSummaryText("fresh summary").
		Save(ctx); err != nil {
		t.Fatalf("failed to simulate concurrent fresh completion: %v", err)
	}

	// ReconcileStaleness operates on the stale snapshot, as
	// GetHandoffSummary's read-then-reconcile path would have read it before
	// the fresh completion landed.
	result := gen.ReconcileStaleness(ctx, fromEntHandoffSummary(staleRow))

	// The predicated update must match 0 rows (generation_started_at no
	// longer equals staleStart) -- a no-op, not an overwrite. The returned
	// value is the row as originally read, not a freshly-error'd one.
	if result.Status != string(HandoffSummaryStatusGenerating) {
		t.Fatalf("expected ReconcileStaleness to return the stale snapshot unchanged, got status=%s", result.Status)
	}

	// Most importantly: the actual DB row must still be READY -- the fresh
	// completion must survive, not get stomped back to ERROR.
	dbRow := getHandoffRow(t, client, sessionID)
	if dbRow.Status != string(HandoffSummaryStatusReady) {
		t.Fatalf("expected the fresh READY row to survive ReconcileStaleness, got %s", dbRow.Status)
	}
	if dbRow.SummaryText != "fresh summary" {
		t.Fatalf("expected the fresh summary_text to survive ReconcileStaleness, got %q", dbRow.SummaryText)
	}
}

// ---- Simple ent schema smoke tests ----

func TestHandoffSummarySchema_should_DefaultStatusToPending_When_CreatedWithoutExplicitStatus(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	created, err := repo.client.HandoffSummary.Create().
		SetID(uuid.New().String()).
		SetSessionID("sess-default-status").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create row: %v", err)
	}
	if created.Status != string(HandoffSummaryStatusPending) {
		t.Fatalf("expected default status %q, got %q", HandoffSummaryStatusPending, created.Status)
	}

	reread, err := repo.client.HandoffSummary.Query().Where(handoffsummary.SessionID("sess-default-status")).Only(ctx)
	if err != nil {
		t.Fatalf("failed to reread row: %v", err)
	}
	if reread.Status != string(HandoffSummaryStatusPending) {
		t.Fatalf("expected reread status %q, got %q", HandoffSummaryStatusPending, reread.Status)
	}
}

func TestHandoffSummarySchema_should_SurviveSessionDeletion_When_NoEdgeExists(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	sessionData := createTestSession("handoff-fk-test-session")
	if err := repo.Create(ctx, sessionData); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if _, err := repo.client.HandoffSummary.Create().
		SetID(uuid.New().String()).
		SetSessionID(sessionData.Title).
		SetStatus(string(HandoffSummaryStatusReady)).
		SetSummaryText("some summary").
		Save(ctx); err != nil {
		t.Fatalf("failed to create handoff summary row: %v", err)
	}

	if err := repo.Delete(ctx, sessionData.Title); err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}

	row, err := repo.client.HandoffSummary.Query().Where(handoffsummary.SessionID(sessionData.Title)).Only(ctx)
	if err != nil {
		t.Fatalf("expected handoff summary row to survive session deletion, got err: %v", err)
	}
	if row.SummaryText != "some summary" {
		t.Fatalf("expected summary_text to be untouched, got %q", row.SummaryText)
	}
}
