package session

import "time"

// touchLocked advances LastMeaningfulChangeAt monotonically. Caller must hold i.mu
// and publish the snapshot afterwards; never call the locking Touch from there.
func (i *Instance) touchLocked(_ SessionEventKind, at time.Time) {
	if at.After(i.LastMeaningfulChangeAt) {
		i.LastMeaningfulChangeAt = at
	}
}

// Touch advances LastMeaningfulChangeAt for callers that do not hold i.mu.
func (i *Instance) Touch(kind SessionEventKind, at time.Time) {
	_ = i.sendSyncErr(func(s *instanceState) error {
		s.inst.mu.Lock()
		s.inst.touchLocked(kind, at)
		snap := buildSnapshot(s.inst)
		s.inst.mu.Unlock()
		s.inst.snapshot.Store(snap)
		return nil
	})
}
