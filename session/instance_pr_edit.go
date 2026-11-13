package session

import (
	"context"
	"errors"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ErrPRPersisterUnavailable means a user PR edit needs a storage backend that
// can write link edits and none is wired.
var ErrPRPersisterUnavailable = errors.New("session: PR persister cannot edit links")

// prSetEditor is the storage seam for user-driven PR set edits (detach, pin,
// manual attach). *Storage implements it; it is separate from prSetPersister so
// automatic attach paths keep their narrower seam.
type prSetEditor interface {
	// DetachSessionPR deletes the session's link and writes its tombstone in one transaction.
	DetachSessionPR(ctx context.Context, sessionUUID string, ref domain.PRRef) error
	SetSessionPRPinned(ctx context.Context, sessionUUID string, ref domain.PRRef, pinned bool) error
	ClearSessionPRTombstone(ctx context.Context, sessionUUID string, ref domain.PRRef) error
}

// editorLocked returns the editor and the serialized persist section. The caller
// holds prPersistMu.
func (i *Instance) editorLocked() (prSetEditor, error) {
	ed, ok := i.prPersister.(prSetEditor)
	if !ok || ed == nil {
		return nil, ErrPRPersisterUnavailable
	}
	return ed, nil
}

// DetachPRPersisted is the user's "Not mine": the repository delete plus
// tombstone commit first, then the in-memory set and the legacy mirror follow.
// A repository failure changes nothing in memory. It reports whether ref was in
// the set; a ref that is not attached writes nothing.
func (i *Instance) DetachPRPersisted(ctx context.Context, ref domain.PRRef) (bool, error) {
	if ref.IsZero() {
		return false, ErrZeroPRRef
	}
	i.prPersistMu.Lock()
	defer i.prPersistMu.Unlock()
	ed, err := i.editorLocked()
	if err != nil {
		return false, err
	}
	snap := i.Snapshot()
	if indexOfPR(snap.GitHub.PRs, ref) < 0 {
		return false, nil
	}
	if err := ed.DetachSessionPR(ctx, snap.UUID, ref); err != nil {
		return false, err
	}
	found := i.DetachPR(ref)
	i.persistMirrorLocked()
	log.Info("session PR detached by user", "session", snap.Title, "pr", ref.String())
	return found, nil
}

// PinPRPersisted pins or unpins ref (pinning clears every other pin), repository
// first so a failure leaves the in-memory active PR untouched.
func (i *Instance) PinPRPersisted(ctx context.Context, ref domain.PRRef, pinned bool) error {
	if ref.IsZero() {
		return ErrZeroPRRef
	}
	i.prPersistMu.Lock()
	defer i.prPersistMu.Unlock()
	ed, err := i.editorLocked()
	if err != nil {
		return err
	}
	snap := i.Snapshot()
	if indexOfPR(snap.GitHub.PRs, ref) < 0 {
		return ErrPRNotAttached
	}
	if err := ed.SetSessionPRPinned(ctx, snap.UUID, ref, pinned); err != nil {
		return err
	}
	i.PinPR(ref, pinned)
	i.persistMirrorLocked()
	prMetrics.pin(pinned)
	log.Info("session pr pinned", "session", snap.Title, "pr", ref.String(), "pinned", pinned)
	return nil
}

// AttachPRManual attaches ref on the user's explicit request: it first clears
// this session's tombstone for ref (a manual attach overrides an earlier detach)
// and then attaches with source manual.
func (i *Instance) AttachPRManual(ctx context.Context, ref domain.PRRef, initial PRInitial) error {
	i.prPersistMu.Lock()
	ed, err := i.editorLocked()
	if err == nil {
		err = ed.ClearSessionPRTombstone(ctx, i.Snapshot().UUID, ref)
	}
	i.prPersistMu.Unlock() // AttachPR takes the same lock to persist
	if err != nil {
		return err
	}
	return i.AttachPR(ref, domain.AttachSourceManual, initial)
}

// ErrPRNotAttached means the PR is not in the session's set.
var ErrPRNotAttached = errors.New("session: pull request is not attached to this session")

// persistMirrorLocked writes the snapshot's legacy scalars. The caller holds
// prPersistMu. A failure is logged, not returned: the next save or reconciler
// pass repairs the mirror.
func (i *Instance) persistMirrorLocked() {
	snap := i.Snapshot()
	g := snap.GitHub
	if err := i.prPersister.PersistPRMirror(snap.UUID, g.GitHubPRURL, g.GitHubPRNumber, g.GitHubPRStatusTerminal); err != nil && !errors.Is(err, ErrSessionNotPersisted) {
		log.Warn("persist PR mirror after edit failed; the next pass retries", "session", snap.Title, "err", err)
	}
}
