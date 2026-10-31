package session

import (
	"sync"
	"time"
)

// terminalSweepSettleTTL is how long a terminal item that a safety-net sweep
// already fully processed is skipped. Without it the 60s sweeps re-read every
// done/archived item's sessions (and probe tmux per session) forever — ~88% of
// all read-pool queries on a live instance (see docs/reference/database-performance.md).
const terminalSweepSettleTTL = 10 * time.Minute

// terminalSweepSettledMax bounds the tracker; terminal items are bounded by the
// backlog size, this only guards against pathological growth.
const terminalSweepSettledMax = 16384

// settledTracker remembers keys processed successfully until a deadline. The
// zero value is ready to use. Keys embed the item's updated_at so any change to
// the item (reopen, status flip) makes it eligible for the sweep again at once.
type settledTracker struct {
	mu    sync.Mutex
	until map[string]time.Time
}

func (s *settledTracker) isSettled(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	deadline, ok := s.until[key]
	return ok && now.Before(deadline)
}

func (s *settledTracker) settle(key string, now time.Time, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.until == nil {
		s.until = make(map[string]time.Time)
	}
	if len(s.until) >= terminalSweepSettledMax {
		for k, d := range s.until {
			if !now.Before(d) {
				delete(s.until, k)
			}
		}
		if len(s.until) >= terminalSweepSettledMax {
			clear(s.until)
		}
	}
	s.until[key] = now.Add(ttl)
}
