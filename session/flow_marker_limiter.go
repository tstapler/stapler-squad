package session

import (
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

const (
	// TriggerRecordMinInterval is the per-(kind,item,PR) dedupe interval: an
	// identical value younger than this is not re-recorded.
	TriggerRecordMinInterval = 30 * time.Minute
	// MarkerMaxWritesPerHour caps all marker writes per fixed one-hour window.
	MarkerMaxWritesPerHour = 600

	markerWindow        = time.Hour
	markerSweepAbove    = 4096
	markerDropWarnEvery = time.Minute
)

type markerLast struct {
	value string
	at    time.Time
}

// MarkerLimiter bounds marker writes so a PR-fix thrash loop cannot flood the
// single-connection SQLite writer. Safe for concurrent use; the zero value is
// usable (clock defaults to time.Now).
type MarkerLimiter struct {
	now func() time.Time

	mu          sync.Mutex
	last        map[string]markerLast
	windowStart time.Time
	windowCount int
	dropped     int
	lastWarn    time.Time
}

// NewMarkerLimiter returns a limiter using now as its clock (nil => time.Now).
func NewMarkerLimiter(now func() time.Time) *MarkerLimiter {
	return &MarkerLimiter{now: now}
}

// Allow reports whether a marker for (key, value) should be written. The same
// value is deduped for TriggerRecordMinInterval (inclusive: exactly 30m later
// is allowed); a changed value bypasses that interval but never the global cap.
func (l *MarkerLimiter) Allow(key, value string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if l.now != nil {
		now = l.now()
	}
	if l.windowStart.IsZero() || !now.Before(l.windowStart.Add(markerWindow)) {
		l.windowStart, l.windowCount = now, 0
	}
	if l.last == nil {
		l.last = make(map[string]markerLast)
	}
	if prev, ok := l.last[key]; ok && prev.value == value && now.Sub(prev.at) < TriggerRecordMinInterval {
		return false
	}
	if l.windowCount >= MarkerMaxWritesPerHour {
		l.dropped++
		if l.lastWarn.IsZero() || now.Sub(l.lastWarn) >= markerDropWarnEvery {
			l.lastWarn = now
			log.WarningLog().Printf("flow_marker_dropped count=%d", l.dropped)
		}
		return false
	}
	l.windowCount++
	l.last[key] = markerLast{value: value, at: now}
	if len(l.last) > markerSweepAbove {
		for k, v := range l.last {
			if now.Sub(v.at) >= TriggerRecordMinInterval {
				delete(l.last, k)
			}
		}
	}
	return true
}

// Forget rolls back an Allow for (key, value) whose write failed, so the lost
// marker is not deduped for TriggerRecordMinInterval. It is a no-op if the key
// has since recorded a different value.
func (l *MarkerLimiter) Forget(key, value string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if prev, ok := l.last[key]; ok && prev.value == value {
		delete(l.last, key)
		if l.windowCount > 0 {
			l.windowCount--
		}
	}
}

// Dropped returns how many writes the global cap has dropped.
func (l *MarkerLimiter) Dropped() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.dropped
}
