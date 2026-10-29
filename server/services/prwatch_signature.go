package services

import githubpkg "github.com/tstapler/stapler-squad/github"

// PRWatchKind is the services-level kind of PR activity the PR watch steers on.
// server/prwatch has its own Kind enum; the server adapter maps it onto this one
// so prwatch never imports services (ADR-001 decision 6).
type PRWatchKind int

const (
	PRWatchKindUnspecified PRWatchKind = iota
	PRWatchKindCI
	PRWatchKindComment
	PRWatchKindReview
)

// reasonName maps the kind onto the canonical reason names the manual nudge and
// PR-fix auto-steer already key on, so all three paths dedupe each other.
func (k PRWatchKind) reasonName() (string, bool) {
	switch k {
	case PRWatchKindCI:
		return reasonNameFailingChecks, true
	case PRWatchKindComment:
		return reasonNameUnresolvedThreads, true
	case PRWatchKindReview:
		return reasonNameChangesRequested, true
	}
	return "", false
}

// PRWatchGuardSignature is the duplicate-window key for a PR watch steer. It
// equals the PR-fix auto-steer signature for the same PR and problem set,
// including dropping CHANGES_REQUESTED when threads are also present. Kinds
// with no canonical reason are ignored.
func PRWatchGuardSignature(pr githubpkg.LinkKey, kinds ...PRWatchKind) string {
	names := make([]string, 0, len(kinds))
	for _, k := range kinds {
		if n, ok := k.reasonName(); ok {
			names = append(names, n)
		}
	}
	return guardSignature(pr, reasonSetSignature(dropSubsumedChangesRequested(names)...))
}
