package diagnose

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeLinkedTranscriptSummaryGenerator is a fake LinkedTranscriptSummaryGenerator
// used in place of a real ent-backed *session.HandoffSummaryGenerator: that
// type is concrete and lives in package session, which package diagnose
// cannot import (see bundle_transcript.go's import-cycle note), and it has no
// existing lightweight in-memory test double to reuse. The fake models just
// enough of the real generator's behavior -- BeginGeneration reserves a
// dedup guard and records the started row, GenerateAndPersist transitions it
// to ready/error after a caller-controlled delay, FindRowBySessionID reads it
// back -- to exercise assembleLinkedTranscript's verbatim/ready/error/timeout
// paths without a database.
type fakeLinkedTranscriptSummaryGenerator struct {
	mu   sync.Mutex
	rows map[string]*HandoffSummaryRow

	// generateDelay is how long GenerateAndPersist sleeps before writing its
	// terminal row, simulating the real pipeline's async LLM call.
	generateDelay time.Duration
	// generateResult is the terminal status/summary GenerateAndPersist writes.
	generateResult HandoffSummaryRow
	// beginGenerationErr, when non-nil, makes BeginGeneration fail as if the
	// interim GENERATING upsert failed.
	beginGenerationErr error
	// neverPersist, when true, makes GenerateAndPersist a no-op -- used to
	// exercise the poll-until-timeout path (row stays pending forever).
	neverPersist bool

	beginGenerationCalls    int
	generateAndPersistCalls int
}

func newFakeLinkedTranscriptSummaryGenerator() *fakeLinkedTranscriptSummaryGenerator {
	return &fakeLinkedTranscriptSummaryGenerator{rows: map[string]*HandoffSummaryRow{}}
}

func (f *fakeLinkedTranscriptSummaryGenerator) BeginGeneration(_ context.Context, sourceSessionID, _ string) (func(), time.Time, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beginGenerationCalls++
	if f.beginGenerationErr != nil {
		return nil, time.Time{}, false, f.beginGenerationErr
	}
	f.rows[sourceSessionID] = &HandoffSummaryRow{Status: "generating"}
	return func() {}, time.Now(), true, nil
}

func (f *fakeLinkedTranscriptSummaryGenerator) GenerateAndPersist(_ context.Context, sourceSessionID, _ string, release func(), _ time.Time) {
	defer release()
	f.mu.Lock()
	f.generateAndPersistCalls++
	delay := f.generateDelay
	result := f.generateResult
	skip := f.neverPersist
	f.mu.Unlock()

	if skip {
		return
	}
	if delay > 0 {
		time.Sleep(delay)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	row := result
	f.rows[sourceSessionID] = &row
}

func (f *fakeLinkedTranscriptSummaryGenerator) FindRowBySessionID(_ context.Context, sessionID string) (*HandoffSummaryRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[sessionID]
	if !ok {
		//nolint:nilnil // mirrors LinkedTranscriptSummaryGenerator's documented not-found contract (nil, nil), see bundle_transcript.go's FindRowBySessionID doc comment.
		return nil, nil
	}
	copyRow := *row
	return &copyRow, nil
}

func withFastHandoffSummaryPolling(t *testing.T, timeout, interval time.Duration) {
	t.Helper()
	origTimeout, origInterval := handoffSummaryTimeout, handoffSummaryPollInterval
	handoffSummaryTimeout, handoffSummaryPollInterval = timeout, interval
	t.Cleanup(func() {
		handoffSummaryTimeout, handoffSummaryPollInterval = origTimeout, origInterval
	})
}

func TestAssembleLinkedTranscript_ShouldReturnVerbatimContent_WhenUnderSectionByteBudget(t *testing.T) {
	generator := newFakeLinkedTranscriptSummaryGenerator()
	linked := LinkedSessionTranscript{SessionID: "sess-1", SessionTitle: "title", Content: "short transcript"}
	budget := SectionBudget{Section: BundleSectionLinkedTranscript, MaxBytes: 1000}

	got := assembleLinkedTranscript(context.Background(), generator, linked, budget)

	if got != linked.Content {
		t.Fatalf("expected verbatim content %q, got %q", linked.Content, got)
	}
	if generator.beginGenerationCalls != 0 {
		t.Fatalf("expected BeginGeneration not to be called for under-budget content, got %d calls", generator.beginGenerationCalls)
	}
}

func TestAssembleLinkedTranscript_ShouldPollUntilReadyAndSubstituteSummary_WhenTranscriptExceedsBudget(t *testing.T) {
	withFastHandoffSummaryPolling(t, 5*time.Second, 10*time.Millisecond)

	generator := newFakeLinkedTranscriptSummaryGenerator()
	generator.generateDelay = 50 * time.Millisecond
	generator.generateResult = HandoffSummaryRow{Status: handoffSummaryRowStatusReady, SummaryText: "generated summary"}

	content := strings.Repeat("x", 300_000)
	linked := LinkedSessionTranscript{SessionID: "sess-2", SessionTitle: "title", Content: content}
	budget := SectionBudget{Section: BundleSectionLinkedTranscript, MaxBytes: 200_000}

	got := assembleLinkedTranscript(context.Background(), generator, linked, budget)

	if got != "generated summary" {
		t.Fatalf("expected generated summary text, got %q", got)
	}
	if generator.beginGenerationCalls != 1 {
		t.Fatalf("expected BeginGeneration to be called once, got %d", generator.beginGenerationCalls)
	}
	if generator.generateAndPersistCalls != 1 {
		t.Fatalf("expected GenerateAndPersist to be called once, got %d", generator.generateAndPersistCalls)
	}
}

func TestAssembleLinkedTranscript_ShouldReturnFallbackNote_WhenGenerationResolvesToError(t *testing.T) {
	withFastHandoffSummaryPolling(t, 5*time.Second, 10*time.Millisecond)

	generator := newFakeLinkedTranscriptSummaryGenerator()
	generator.generateDelay = 20 * time.Millisecond
	generator.generateResult = HandoffSummaryRow{Status: handoffSummaryRowStatusError}

	content := strings.Repeat("x", 300_000)
	linked := LinkedSessionTranscript{SessionID: "sess-3", SessionTitle: "title", Content: content}
	budget := SectionBudget{Section: BundleSectionLinkedTranscript, MaxBytes: 200_000}

	got := assembleLinkedTranscript(context.Background(), generator, linked, budget)

	if got != linkedTranscriptTooLargeNote {
		t.Fatalf("expected fallback note %q, got %q", linkedTranscriptTooLargeNote, got)
	}
}

func TestAssembleLinkedTranscript_ShouldReturnFallbackNote_WhenPollingTimesOut(t *testing.T) {
	// Row never resolves (neverPersist) -- the poll loop must give up at
	// handoffSummaryTimeout rather than blocking forever.
	withFastHandoffSummaryPolling(t, 50*time.Millisecond, 10*time.Millisecond)

	generator := newFakeLinkedTranscriptSummaryGenerator()
	generator.neverPersist = true

	content := strings.Repeat("x", 300_000)
	linked := LinkedSessionTranscript{SessionID: "sess-4", SessionTitle: "title", Content: content}
	budget := SectionBudget{Section: BundleSectionLinkedTranscript, MaxBytes: 200_000}

	start := time.Now()
	got := assembleLinkedTranscript(context.Background(), generator, linked, budget)
	elapsed := time.Since(start)

	if got != linkedTranscriptTooLargeNote {
		t.Fatalf("expected fallback note %q, got %q", linkedTranscriptTooLargeNote, got)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected poll loop to give up near the configured timeout, took %s", elapsed)
	}
}

func TestAssembleLinkedTranscript_ShouldReturnFallbackNote_WhenBeginGenerationFails(t *testing.T) {
	generator := newFakeLinkedTranscriptSummaryGenerator()
	generator.beginGenerationErr = context.DeadlineExceeded

	content := strings.Repeat("x", 300_000)
	linked := LinkedSessionTranscript{SessionID: "sess-5", SessionTitle: "title", Content: content}
	budget := SectionBudget{Section: BundleSectionLinkedTranscript, MaxBytes: 200_000}

	got := assembleLinkedTranscript(context.Background(), generator, linked, budget)

	if got != linkedTranscriptTooLargeNote {
		t.Fatalf("expected fallback note %q, got %q", linkedTranscriptTooLargeNote, got)
	}
	if generator.generateAndPersistCalls != 0 {
		t.Fatalf("expected GenerateAndPersist not to be called when BeginGeneration fails, got %d calls", generator.generateAndPersistCalls)
	}
}
