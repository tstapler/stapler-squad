// Package markerstore persists server-decided flow markers and backlog stuck
// episodes. session.EntRepository embeds *Markers so existing callers keep
// their method set.
package markerstore

import (
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/session/ent"
)

// Markers owns the backlog_flow_markers table and its write-rate limiter.
type Markers struct {
	client *ent.Client
	// limiter is built lazily (limiterOnce) from clock, an injectable test clock (nil => time.Now).
	limiter     *MarkerLimiter
	limiterOnce sync.Once
	clock       func() time.Time
}

// New returns Markers over client, which must already hold the migrated schema.
func New(client *ent.Client) *Markers {
	return &Markers{client: client}
}

// Limiter returns the shared write limiter, building it on first use.
func (m *Markers) Limiter() *MarkerLimiter {
	m.limiterOnce.Do(func() {
		m.limiter = NewMarkerLimiter(m.clock)
	})
	return m.limiter
}

func (m *Markers) now() time.Time {
	if m.clock != nil {
		return m.clock().UTC()
	}
	return time.Now().UTC()
}

// SetMarkerClock overrides the clock behind marker timestamps and rate limiting; call it
// before the first marker is recorded (tests).
func (m *Markers) SetMarkerClock(now func() time.Time) { m.clock = now }
