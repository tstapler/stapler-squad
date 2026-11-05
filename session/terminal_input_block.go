package session

// TerminalInputBlock reports whether a UI terminal stream attached to snap must be
// read-only, and whether that can end without a new attach. It is the one predicate
// behind both the stream's writer decision and Session.terminal_read_only, so the
// server and the UI cannot disagree.
//
// recoverable is true only for a non-hidden tymux instance that is not Active yet
// (any non-Active status, not just PermanentlyFailed); archived and hidden never recover.
func TerminalInputBlock(snap *InstanceSnapshot) (blocked, recoverable bool) {
	if snap.ArchivedAt != nil {
		return true, false
	}
	if snap.Hidden {
		return true, false
	}
	if snap.Backend == BackendTymux && snap.Status != Active {
		return true, true
	}
	return false, false
}
