package wakes

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session/domain"
)

// registerSessionSource installs the session_state source: any session event
// is a nudge to re-read the peer's state, and the predicate (also used by
// reconcile and by the check at schedule time) decides whether it reached the
// target. Idle is not a lifecycle event of its own, so the state is re-read
// rather than inferred from the event.
func (d *Dispatcher) registerSessionSource() {
	d.RegisterMatcher(func(ev *events.Event) []EventMatch {
		if id := sessionEventID(ev); id != "" {
			return []EventMatch{{Type: domain.WakeTriggerSessionState, Key: id, Check: true}}
		}
		return nil
	})
	d.RegisterPredicate(domain.WakeTriggerSessionState, func(ctx context.Context, spec domain.WakeSpec) (bool, error) {
		s, ok := spec.(domain.SessionStateSpec)
		if !ok || d.cfg.Sessions == nil {
			return false, nil
		}
		f, err := d.cfg.Sessions.SessionState(ctx, s.SessionUUID)
		if err != nil {
			return false, fmt.Errorf("session_state predicate %s: %w", s.SessionUUID, err)
		}
		return f.Exists && f.State == s.State, nil
	})
}

func sessionEventID(ev *events.Event) string {
	if ev == nil {
		return ""
	}
	switch ev.Type {
	case events.EventSessionUpdated, events.EventSessionCreated:
		if ev.Session != nil {
			return ev.Session.Snapshot().UUID
		}
	case events.EventSessionDeleted, events.EventSessionArchived:
		return ev.SessionID
	}
	return ""
}
