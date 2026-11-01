package services

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/log"
)

// errSessionGone marks a stream failure where the daemon reports the
// session/pane no longer exists, so reconnecting cannot help.
var errSessionGone = errors.New("session ended: backing terminal session no longer exists")

// goneLogInterval bounds how often a still-reconnecting client may log the
// same "session is gone" condition at ERROR.
const goneLogInterval = 30 * time.Second

// isSessionGone reports whether err is the daemon's definitive NotFound.
func isSessionGone(err error) bool {
	return err != nil && connect.CodeOf(err) == connect.CodeNotFound
}

// definitivelyGone is isSessionGone plus a fresh backend liveness check. IsAlive
// confirms a pane NotFound with a ListSessions round trip, and treats an
// unreachable daemon (Unavailable, retriable) as not-gone, so a daemon restart
// or mid-restore blip never reaches the SPA as the permanent not_found.
func definitivelyGone(err error, backendAlive func() bool) bool {
	return isSessionGone(err) && !backendAlive()
}

// wrapSessionGone tags a NotFound capture error as errSessionGone, keeping the cause.
func wrapSessionGone(err error) error {
	if !isSessionGone(err) {
		return err
	}
	return fmt.Errorf("%w: %w", errSessionGone, err)
}

// logThrottle admits one event per key per interval. Callers pass the clock
// reading so tests stay deterministic.
type logThrottle struct {
	mu       sync.Mutex
	interval time.Duration
	last     map[string]time.Time
}

func newLogThrottle(interval time.Duration) *logThrottle {
	return &logThrottle{interval: interval, last: make(map[string]time.Time)}
}

// logThrottleSweepAt is the map size past which allow drops expired keys.
const logThrottleSweepAt = 256

func (t *logThrottle) allow(key string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.last) >= logThrottleSweepAt {
		for k, prev := range t.last {
			if now.Sub(prev) >= t.interval {
				delete(t.last, k)
			}
		}
	}
	if prev, ok := t.last[key]; ok && now.Sub(prev) < t.interval {
		return false
	}
	t.last[key] = now
	return true
}

// goneLogThrottle dedupes per-session "gone" ERROR logs across reconnects.
var goneLogThrottle = newLogThrottle(goneLogInterval)

// logSessionGone logs a gone-session failure at ERROR at most once per
// interval per session, and at Debug otherwise.
func logSessionGone(site, sessionID string, err error) {
	if goneLogThrottle.allow(sessionID, time.Now()) {
		log.Error(site+": session gone", "session", sessionID, "err", err)
		return
	}
	log.Debug(site+": session gone (suppressed repeat)", "session", sessionID, "err", err)
}
