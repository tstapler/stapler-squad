package session

import "github.com/tstapler/stapler-squad/session/domain"

// TerminalInputBlock reports whether a UI terminal stream attached to snap must be
// read-only, and whether that can end without a new attach. It is the one predicate
// behind both the stream's writer decision and Session.terminal_read_only, so the
// server and the UI cannot disagree.
//
// recoverable is true for an archived instance (until unarchived) and a tymux instance
// that is not Active yet (any non-Active status); a hidden one never recovers.
func TerminalInputBlock(snap *InstanceSnapshot) (blocked, recoverable bool) {
	if snap.Hidden {
		return true, false
	}
	if snap.ArchivedAt != nil {
		return true, true // clears on unarchive
	}
	if snap.Backend == BackendTymux && snap.Status != domain.Active {
		return true, true
	}
	return false, false
}
