package search

import "time"

// CurrentSyncMetadataVersion is the schema version for sync metadata.
// Increment this when making breaking changes to the metadata format.
const CurrentSyncMetadataVersion = 1

// SessionIndexMetadata tracks the indexing state for a single conversation session.
// Used to detect changes since last index build.
type SessionIndexMetadata struct {
	// SessionID is the unique conversation identifier
	SessionID string `json:"session_id"`
	// UpdatedAt is the last known update time of the conversation
	UpdatedAt time.Time `json:"updated_at"`
	// MessageCount is the number of messages in the conversation when indexed
	MessageCount int `json:"message_count"`
	// LastIndexedAt is when we last indexed this session
	LastIndexedAt time.Time `json:"last_indexed_at"`
	// DocCount is the number of documents we created for this session
	DocCount int `json:"doc_count"`
}

// IndexSyncMetadata tracks the overall index state for incremental updates.
// This metadata is persisted alongside the inverted index and document store.
type IndexSyncMetadata struct {
	// Version is the schema version of the sync metadata
	Version int `json:"version"`
	// LastFullSync is when the index was last fully rebuilt
	LastFullSync time.Time `json:"last_full_sync"`
	// LastIncrementalSync is when the index was last incrementally updated
	LastIncrementalSync time.Time `json:"last_incremental_sync"`
	// Sessions maps session IDs to their index metadata
	Sessions map[string]*SessionIndexMetadata `json:"sessions"`
	// TotalSessions is the count of indexed sessions
	TotalSessions int `json:"total_sessions"`
	// TotalDocuments is the total indexed document count
	TotalDocuments int `json:"total_documents"`
}

// NewIndexSyncMetadata creates a new IndexSyncMetadata with initialized maps.
func NewIndexSyncMetadata() *IndexSyncMetadata {
	return &IndexSyncMetadata{
		Version:  CurrentSyncMetadataVersion,
		Sessions: make(map[string]*SessionIndexMetadata),
	}
}

// SyncResult contains statistics about an incremental sync operation.
type SyncResult struct {
	// SessionsAdded is the count of newly indexed sessions
	SessionsAdded int
	// SessionsUpdated is the count of sessions with new messages re-indexed
	SessionsUpdated int
	// SessionsRemoved is the count of deleted sessions removed from index
	SessionsRemoved int
	// DocumentsAdded is the count of new documents indexed
	DocumentsAdded int
	// DocumentsRemoved is the count of documents removed
	DocumentsRemoved int
	// SyncDuration is how long the sync took
	SyncDuration time.Duration
	// WasFullRebuild is true if a full rebuild was performed instead of incremental
	WasFullRebuild bool
	// Errors contains any non-fatal errors encountered during sync
	Errors []error
}

// HasChanges returns true if any sessions were added, updated, or removed.
func (r *SyncResult) HasChanges() bool {
	return r.SessionsAdded > 0 || r.SessionsUpdated > 0 || r.SessionsRemoved > 0
}

// String returns a human-readable summary of the sync result.
func (r *SyncResult) String() string {
	if r.WasFullRebuild {
		return "full rebuild: " + r.summary()
	}
	if !r.HasChanges() {
		return "no changes"
	}
	return "incremental: " + r.summary()
}

func (r *SyncResult) summary() string {
	return "+" + itoa(r.SessionsAdded) + " sessions, ~" + itoa(r.SessionsUpdated) + " updated, -" + itoa(r.SessionsRemoved) + " removed (" + r.SyncDuration.String() + ")"
}

// itoa is a simple int to string conversion without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	var b [20]byte
	bp := len(b) - 1
	for i > 0 {
		b[bp] = byte('0' + i%10)
		bp--
		i /= 10
	}
	return string(b[bp+1:])
}
