package tokens

import (
	"strings"
	"time"
)

// SessionRecord is a minimal snapshot of a stapler-squad session used for
// matching against ParseResult values. This avoids importing the full session
// package and prevents circular dependencies.
type SessionRecord struct {
	SessionID      string
	ConversationID string // matches ParseResult.SessionUUID
	Path           string // working directory
	CreatedAt      time.Time
}

// SessionStorage is the interface Associator uses to look up sessions.
// Implemented by session.Storage (or a test stub).
type SessionStorage interface {
	// ListSessionRecords returns a snapshot of all sessions for association.
	ListSessionRecords() []SessionRecord
}

// Associator links ParseResult values to stapler-squad sessions.
type Associator struct {
	storage SessionStorage
}

// NewAssociator creates a new Associator backed by the given storage.
func NewAssociator(storage SessionStorage) *Associator {
	return &Associator{storage: storage}
}

// Associate returns the stapler-squad session ID that best matches the given
// ParseResult, and whether the result is an orphan (no match found).
//
// Lookup priority:
//  1. Exact conversation UUID match (ParseResult.SessionUUID == session.ConversationID)
//  2. Project path prefix match (ParseResult.ProjectPath is a prefix of session.Path)
//  3. Timestamp proximity (file mod time within ±5 minutes of session.CreatedAt)
func (a *Associator) Associate(result *ParseResult) (sessionID string, isOrphan bool) {
	if a == nil || a.storage == nil {
		return "", true
	}
	return associate(result, a.storage.ListSessionRecords())
}

// Snapshot fetches the current session records once, for callers that need to
// call AssociateWithSnapshot for many results without re-querying storage per
// call. Both ListInstancesFiltered-style loops in InsightsService previously
// called Associate per result, each paying a fresh ListSessionRecords() ->
// ListInstanceData() full-repository scan.
func (a *Associator) Snapshot() []SessionRecord {
	if a == nil || a.storage == nil {
		return nil
	}
	return a.storage.ListSessionRecords()
}

// AssociateWithSnapshot is Associate against a pre-fetched session snapshot
// (see Snapshot), instead of re-querying storage on every call.
func (a *Associator) AssociateWithSnapshot(result *ParseResult, sessions []SessionRecord) (sessionID string, isOrphan bool) {
	if a == nil {
		return "", true
	}
	return associate(result, sessions)
}

func associate(result *ParseResult, sessions []SessionRecord) (sessionID string, isOrphan bool) {
	// Strategy 1: exact conversation UUID match.
	if result.SessionUUID != "" {
		for _, s := range sessions {
			if s.ConversationID == result.SessionUUID {
				return s.SessionID, false
			}
		}
	}

	// Strategy 2: path prefix match.
	if result.ProjectPath != "" {
		for _, s := range sessions {
			if s.Path != "" && isPathPrefixMatch(result.ProjectPath, s.Path) {
				return s.SessionID, false
			}
		}
	}

	// Strategy 3: timestamp proximity (±5 minutes).
	if !result.FileModTime.IsZero() {
		const window = 5 * time.Minute
		for _, s := range sessions {
			if s.CreatedAt.IsZero() {
				continue
			}
			diff := result.FileModTime.Sub(s.CreatedAt)
			if diff < 0 {
				diff = -diff
			}
			if diff <= window {
				return s.SessionID, false
			}
		}
	}

	return "", true
}

// isPathPrefixMatch returns true if resultPath is a path-component prefix of sessionPath,
// or if sessionPath is a path-component prefix of resultPath.
func isPathPrefixMatch(resultPath, sessionPath string) bool {
	if resultPath == sessionPath {
		return true
	}
	// Ensure we match on path component boundaries.
	if strings.HasPrefix(resultPath, sessionPath) {
		rest := resultPath[len(sessionPath):]
		return rest == "" || strings.HasPrefix(rest, "/")
	}
	if strings.HasPrefix(sessionPath, resultPath) {
		rest := sessionPath[len(resultPath):]
		return rest == "" || strings.HasPrefix(rest, "/")
	}
	return false
}
