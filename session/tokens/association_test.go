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
	storage := &stubStorage{records: []SessionRecord{}}
	a := NewAssociator(storage)
	result := &ParseResult{SessionUUID: "no-match"}

	sessionID, isOrphan := a.Associate(result)
	assert.Equal(t, "", sessionID)
	assert.True(t, isOrphan)
}

func TestAssociator_WhenTimestampProximityMatch_ExpectSessionIDReturned(t *testing.T) {
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
