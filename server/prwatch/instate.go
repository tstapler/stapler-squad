package prwatch

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

// StateFromError maps an inbox client or token error onto the InboxState the
// operator should be told about. A nil error is InboxStateOK. Errors that say
// nothing about reachability or configuration (decode, rejected) map to OK so
// they do not raise a misleading notice.
func StateFromError(err error) InboxState {
	switch {
	case err == nil:
		return InboxStateOK
	case errors.Is(err, inbox.ErrUnauthorized):
		return InboxStateUnauthorized
	case errors.Is(err, inbox.ErrTokenMissing):
		return InboxStateTokenMissing
	case errors.Is(err, inbox.ErrTokenPermissions):
		return InboxStateTokenPerms
	case errors.Is(err, inbox.ErrTransport), errors.Is(err, inbox.Err5xx):
		return InboxStateUnreachable
	}
	return InboxStateOK
}

// graceState reports whether the state waits StateNotifyGrace before it is
// raised: a restart of the inbox or the other Squad is normal and brief.
func graceState(st InboxState) bool {
	return st == InboxStateUnreachable || st == InboxStateLeaderNotHeld
}

// stateTracker implements InboxStateTracker. Inbox-health states replace one
// another; leader_not_held coexists with them until an inbox-health state is
// observed (which implies this process leads) or OK is observed.
type stateTracker struct {
	notes *Notifications
	clock Clock
	log   Logger
	s     Settings

	mu      sync.Mutex
	current InboxState
	since   map[InboxState]time.Time
	raised  map[InboxState]bool
}

var _ InboxStateTracker = (*stateTracker)(nil)

// NewInboxStateTracker builds the tracker. Notification dedupe and text live
// in notes (task 4.5); this adds grace timing, recovery and the file mode.
func NewInboxStateTracker(notes *Notifications, clock Clock, log Logger, s Settings) InboxStateTracker {
	return &stateTracker{
		notes: notes, clock: clock, log: log, s: s, current: InboxStateOK,
		since: map[InboxState]time.Time{}, raised: map[InboxState]bool{},
	}
}

func (t *stateTracker) Current() InboxState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.current
}

// Observe records the state seen this tick, raises it once when due, and
// publishes a recovery for every raised state that is no longer present.
func (t *stateTracker) Observe(st InboxState) {
	now := t.clock.Now()
	t.mu.Lock()
	t.current = st
	var recovered []InboxState
	for old := range t.since {
		if !t.stillPresent(old, st) {
			if t.raised[old] {
				recovered = append(recovered, old)
			}
			delete(t.since, old)
			delete(t.raised, old)
		}
	}
	raise := false
	if st != InboxStateOK {
		if _, ok := t.since[st]; !ok {
			t.since[st] = now
		}
		grace := time.Duration(0)
		if graceState(st) {
			grace = t.s.StateNotifyGrace
		}
		if !t.raised[st] && now.Sub(t.since[st]) >= grace {
			t.raised[st] = true
			raise = true
		}
	}
	t.mu.Unlock()

	for _, old := range recovered {
		if err := t.notes.InboxStateRecovered(old); err != nil {
			t.log.Warn("prwatch.notify_failed", "kind", string(NotifyInboxState), "state", string(old))
		}
	}
	if raise {
		if err := t.notes.InboxState(st, t.tokenMode(st)); err != nil {
			t.mu.Lock()
			t.raised[st] = false // retry on the next tick
			t.mu.Unlock()
			t.log.Warn("prwatch.notify_failed", "kind", string(NotifyInboxState), "state", string(st))
		}
	}
}

// stillPresent says whether a previously seen state survives observing now.
func (t *stateTracker) stillPresent(old, now InboxState) bool {
	switch {
	case now == InboxStateOK:
		return false
	case old == now:
		return true
	case old == InboxStateLeaderNotHeld:
		return now == InboxStateLeaderNotHeld
	}
	return false
}

// tokenMode returns the octal mode of the token file for token_permissions.
func (t *stateTracker) tokenMode(st InboxState) string {
	if st != InboxStateTokenPerms {
		return ""
	}
	info, err := os.Lstat(t.s.TokenPath)
	if err != nil {
		return "unknown"
	}
	return fmt.Sprintf("%04o", info.Mode().Perm())
}
