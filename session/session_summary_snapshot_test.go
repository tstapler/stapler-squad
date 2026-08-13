package session

import (
	"context"
	"testing"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/tokens"
)

// ---- BuildDiffSnapshot ----

func TestBuildDiffSnapshot_should_ReturnEmptySnapshot_When_DiffStatsIsNil(t *testing.T) {
	snapshot := BuildDiffSnapshot(nil)
	if !snapshot.IsEmpty() {
		t.Fatalf("expected empty snapshot, got %+v", snapshot)
	}
	if snapshot != (DiffSnapshot{}) {
		t.Fatalf("expected zero-value snapshot, got %+v", snapshot)
	}
}

func TestBuildDiffSnapshot_should_ReturnPopulatedSnapshot_When_DiffStatsProvided(t *testing.T) {
	stats := &git.DiffStats{
		Added:   42,
		Removed: 7,
		Content: "diff --git a/foo.go b/foo.go\n@@ -1,1 +1,1 @@\n-old\n+new\ndiff --git a/bar.go b/bar.go\n@@ -1,1 +1,1 @@\n-old\n+new\n",
	}
	snapshot := BuildDiffSnapshot(stats)
	if snapshot.Added != 42 || snapshot.Removed != 7 || snapshot.FilesChanged != 2 {
		t.Fatalf("expected {Added:42 Removed:7 FilesChanged:2}, got %+v", snapshot)
	}
}

// ---- BuildTimelineSnapshot ----

func TestBuildTimelineSnapshot_should_ComputeDuration_When_CreatedAtAndStoppedAtProvided(t *testing.T) {
	start := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	stop := start.Add(5 * time.Minute)
	snapshot := BuildTimelineSnapshot(start, stop)
	if snapshot.Duration() != 5*time.Minute {
		t.Fatalf("expected 5m duration, got %v", snapshot.Duration())
	}
}

// ---- BuildDecisionsSnapshot ----

type fakeNotificationDecisionLister struct {
	records []DecisionRecord
	err     error
}

func (f *fakeNotificationDecisionLister) ListDecisionRecords(_ context.Context, _ string) ([]DecisionRecord, error) {
	return f.records, f.err
}

type fakeReviewQueueLookup struct {
	resolved, stillOpen int
	err                 error
	block               bool
}

func (f *fakeReviewQueueLookup) ReviewQueueResolvedCount(ctx context.Context, _ string) (int, int, error) {
	if f.block {
		<-ctx.Done()
		return 0, 0, ctx.Err()
	}
	return f.resolved, f.stillOpen, f.err
}

func TestBuildDecisionsSnapshot_should_ReturnAllZero_When_NoRecords(t *testing.T) {
	snapshot, err := BuildDecisionsSnapshot(context.Background(), "sess-1", &fakeNotificationDecisionLister{}, &fakeReviewQueueLookup{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot.Total() != 0 {
		t.Fatalf("expected all-zero snapshot, got %+v", snapshot)
	}
}

func TestBuildDecisionsSnapshot_should_QueryNotificationHistoryStore_When_SessionHasFiveAutoApprovedRecords(t *testing.T) {
	autoApproved := int32(sessionv1.NotificationType_NOTIFICATION_TYPE_AUTO_APPROVED)
	approvalNeeded := int32(sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED)

	var records []DecisionRecord
	for i := 0; i < 5; i++ {
		records = append(records, DecisionRecord{NotificationType: autoApproved, ApprovalDecision: "allow"})
	}
	records = append(records, DecisionRecord{NotificationType: approvalNeeded, ApprovalDecision: "allow"})

	lister := &fakeNotificationDecisionLister{records: records}
	snapshot, err := BuildDecisionsSnapshot(context.Background(), "sess-123", lister, &fakeReviewQueueLookup{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot.AutoApproved != 5 || snapshot.ManuallyApproved != 1 || snapshot.Denied != 0 {
		t.Fatalf("expected {AutoApproved:5 ManuallyApproved:1 Denied:0}, got %+v", snapshot)
	}
}

func TestBuildDecisionsSnapshot_should_ReturnError_When_ReviewQueueLookupExceedsTimeout(t *testing.T) {
	start := time.Now()
	_, err := BuildDecisionsSnapshot(context.Background(), "sess-1", &fakeNotificationDecisionLister{}, &fakeReviewQueueLookup{block: true})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error from a blocked ReviewQueueLookup")
	}
	if elapsed < reviewQueueLookupTimeout {
		t.Fatalf("expected the call to respect reviewQueueLookupTimeout (%v), returned after %v", reviewQueueLookupTimeout, elapsed)
	}
}

// ---- BuildCostSnapshot ----

// fakeTokenStoreReader is a lightweight tokens.TokenStoreReader test double —
// avoids constructing a real *tokens.TokenStore (which requires parsing a JSONL
// fixture off disk) for the fast unit-test cases below.
type fakeTokenStoreReader struct {
	byUUID map[string]*tokens.ParseResult
}

func (f *fakeTokenStoreReader) GetAll() []*tokens.ParseResult { return nil }
func (f *fakeTokenStoreReader) GetByUUID(uuid string) *tokens.ParseResult {
	return f.byUUID[uuid]
}
func (f *fakeTokenStoreReader) IsLoading() bool                { return false }
func (f *fakeTokenStoreReader) Subscribe() <-chan struct{}     { return nil }
func (f *fakeTokenStoreReader) Unsubscribe(ch <-chan struct{}) {}

func TestBuildCostSnapshot_should_ReturnDataUnavailable_When_TokenStoreReturnsNil(t *testing.T) {
	store := &fakeTokenStoreReader{}
	snapshot := BuildCostSnapshot("no-such-session", store)
	if !snapshot.DataUnavailable {
		t.Fatalf("expected DataUnavailable, got %+v", snapshot)
	}
}

func TestBuildCostSnapshot_should_ReturnPopulatedSnapshot_When_TokenStoreReturnsParseResult(t *testing.T) {
	store := &fakeTokenStoreReader{byUUID: map[string]*tokens.ParseResult{
		"sess-123": {
			SessionUUID:  "sess-123",
			PrimaryModel: "claude-sonnet-4-5",
			TotalInput:   100000,
			TotalOutput:  28000,
		},
	}}
	snapshot := BuildCostSnapshot("sess-123", store)
	if snapshot.DataUnavailable {
		t.Fatal("expected data to be available")
	}
	if snapshot.TotalTokens != 128000 {
		t.Fatalf("expected TotalTokens=128000, got %d", snapshot.TotalTokens)
	}
}

func TestBuildCostSnapshot_should_ReturnTotalTokensAndCost_When_RealTranscriptParsedByTokenStore(t *testing.T) {
	store := tokens.NewTokenStore("")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Start(ctx)
	// TokenStore derives SessionUUID from the filename (session/tokens/parser.go's
	// ParseFile), not from the JSONL content's "sessionId" field — the fixture's
	// resulting UUID is "valid_session" (its basename minus .jsonl).
	store.OnHistoryFileChanged("tokens/testdata/valid_session.jsonl")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if store.GetByUUID("valid_session") != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	snapshot := BuildCostSnapshot("valid_session", store)
	if snapshot.DataUnavailable {
		t.Fatal("expected cost data to be available")
	}
	if snapshot.TotalTokens <= 0 {
		t.Fatalf("expected positive TotalTokens, got %d", snapshot.TotalTokens)
	}
}

// ---- isTrivialSession ----

func TestIsTrivialSession_should_ReturnTrue_When_EmptyDiffZeroDecisionsAndShortDuration(t *testing.T) {
	if !isTrivialSession(DiffSnapshot{}, DecisionsSnapshot{}, 5*time.Second) {
		t.Fatal("expected trivial session to be detected")
	}
}

func TestIsTrivialSession_should_ReturnFalse_When_DurationEqualsExactly30Seconds(t *testing.T) {
	if isTrivialSession(DiffSnapshot{}, DecisionsSnapshot{}, trivialSessionMaxDuration) {
		t.Fatal("expected duration == trivialSessionMaxDuration to NOT be trivial (strict less-than boundary)")
	}
}
