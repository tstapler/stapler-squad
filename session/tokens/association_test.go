package tokens

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// stubStorage implements SessionStorage for tests.
type stubStorage struct {
	records []SessionRecord
}

func (s *stubStorage) ListSessionRecords() []SessionRecord {
	return s.records
}

func TestAssociator_WhenExactConversationIDMatch_ExpectSessionIDReturned(t *testing.T) {
	t.Parallel()
	storage := &stubStorage{
		records: []SessionRecord{
			{SessionID: "sess-123", ConversationID: "abc-123", Path: "/some/path"},
		},
	}
	a := NewAssociator(storage)
	result := &ParseResult{SessionUUID: "abc-123"}

	sessionID, isOrphan := a.Associate(result)
	assert.Equal(t, "sess-123", sessionID)
	assert.False(t, isOrphan)
}

func TestAssociator_WhenPathPrefixMatch_ExpectSessionIDReturned(t *testing.T) {
	t.Parallel()
	storage := &stubStorage{
		records: []SessionRecord{
			{SessionID: "sess-456", Path: "/home/user/projects/myapp"},
		},
	}
	a := NewAssociator(storage)
	result := &ParseResult{ProjectPath: "/home/user/projects/myapp/subdir"}

	sessionID, isOrphan := a.Associate(result)
	assert.Equal(t, "sess-456", sessionID)
	assert.False(t, isOrphan)
}

func TestAssociator_WhenNoMatch_ExpectOrphan(t *testing.T) {
	t.Parallel()
	storage := &stubStorage{records: []SessionRecord{}}
	a := NewAssociator(storage)
	result := &ParseResult{SessionUUID: "no-match"}

	sessionID, isOrphan := a.Associate(result)
	assert.Equal(t, "", sessionID)
	assert.True(t, isOrphan)
}

func TestAssociator_WhenTimestampProximityMatch_ExpectSessionIDReturned(t *testing.T) {
	t.Parallel()
	now := time.Now()
	storage := &stubStorage{
		records: []SessionRecord{
			{SessionID: "sess-789", CreatedAt: now.Add(-2 * time.Minute)},
		},
	}
	a := NewAssociator(storage)
	result := &ParseResult{FileModTime: now}

	sessionID, isOrphan := a.Associate(result)
	assert.Equal(t, "sess-789", sessionID)
	assert.False(t, isOrphan)
}

// countingStorage counts ListSessionRecords calls, so a test can assert a
// caller resolving many results only pays for one storage round-trip.
type countingStorage struct {
	records []SessionRecord
	calls   int
}

func (s *countingStorage) ListSessionRecords() []SessionRecord {
	s.calls++
	return s.records
}

// TestAssociator_WhenAssociatingManyResultsViaSnapshot_ExpectStorageQueriedOnce
// is the PerfFix-3 regression test: insights_service.go's per-result loops
// previously called Associate for every ParseResult, each paying a fresh
// ListSessionRecords() -> Storage.ListInstanceData() full-repository query.
// Snapshot + AssociateWithSnapshot must resolve N results from one query.
func TestAssociator_WhenAssociatingManyResultsViaSnapshot_ExpectStorageQueriedOnce(t *testing.T) {
	t.Parallel()
	storage := &countingStorage{
		records: []SessionRecord{
			{SessionID: "sess-1", ConversationID: "conv-1"},
			{SessionID: "sess-2", ConversationID: "conv-2"},
		},
	}
	a := NewAssociator(storage)
	results := []*ParseResult{
		{SessionUUID: "conv-1"},
		{SessionUUID: "conv-2"},
		{SessionUUID: "no-match"},
	}

	snapshot := a.Snapshot()
	for i, r := range results {
		sessionID, isOrphan := a.AssociateWithSnapshot(r, snapshot)
		switch i {
		case 0:
			assert.Equal(t, "sess-1", sessionID)
			assert.False(t, isOrphan)
		case 1:
			assert.Equal(t, "sess-2", sessionID)
			assert.False(t, isOrphan)
		case 2:
			assert.True(t, isOrphan)
		}
	}

	assert.Equal(t, 1, storage.calls, "expected exactly one ListSessionRecords call for %d results", len(results))
}
