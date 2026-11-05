package prwatch

import (
	"sync"
	"time"
)

const (
	// MaxHeldPerKey bounds the items held per (session, PR, class) (D12).
	MaxHeldPerKey = 50
	// maxMinSteerInterval mirrors the config clamp so no window outlasts the
	// HeldLong threshold even if a caller skips the adapter.
	maxMinSteerInterval = 30 * time.Minute
)

type prSlot struct {
	session string
	repo    string
	pr      int
}

type prWindow struct {
	feedbackAt time.Time
	ciSHA      string
	ciAt       time.Time
}

// WindowCoalescer is the real D12 Coalescer: class-keyed hold-and-fold with a
// per-key cap and CI-per-head-SHA. State is in memory, grouped by session so
// Evict is O(PRs of that session).
type WindowCoalescer struct {
	clock    Clock
	interval time.Duration
	metrics  Metrics // may be nil

	mu     sync.Mutex
	state  map[string]map[prSlot]*prWindow
	nslots int
}

var _ Coalescer = (*WindowCoalescer)(nil)

// NewWindowCoalescer builds the coalescer from Settings.MinSteerInterval
// (0 disables coalescing; values above 30 minutes are clamped). metrics may
// be nil.
func NewWindowCoalescer(clock Clock, s Settings, metrics Metrics) *WindowCoalescer {
	iv := s.MinSteerInterval
	if iv > maxMinSteerInterval {
		iv = maxMinSteerInterval
	}
	if iv < 0 {
		iv = 0
	}
	return &WindowCoalescer{clock: clock, interval: iv, metrics: metrics, state: map[string]map[prSlot]*prWindow{}}
}

func slotOf(k CoalesceKey) prSlot { return prSlot{session: k.SessionUUID, repo: k.Repo, pr: k.PR} }

// window returns the state for key or nil. Caller holds mu.
func (c *WindowCoalescer) window(k CoalesceKey) *prWindow {
	return c.state[k.SessionUUID][slotOf(k)]
}

// classStart is the time of the class's last steer; zero when none.
func (w *prWindow) classStart(class KindClass) time.Time {
	if w == nil {
		return time.Time{}
	}
	if class == ClassCI {
		return w.ciAt
	}
	return w.feedbackAt
}

func (c *WindowCoalescer) windowOpen(w *prWindow, class KindClass, now time.Time) bool {
	start := w.classStart(class)
	return c.interval > 0 && !start.IsZero() && now.Before(start.Add(c.interval))
}

// Decide has no side effects. held is the number of items already held for
// key before this one, so the 51st item of a key is folded over the cap.
func (c *WindowCoalescer) Decide(key CoalesceKey, headSHA string, held int) CoalesceAction {
	if c.interval == 0 {
		return CoalesceSteer
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.window(key)
	if key.Class == ClassCI && headSHA != "" {
		if w != nil && w.ciSHA == headSHA {
			return CoalesceCoveredSameSHA
		}
		return CoalesceSteer // a new head SHA always steers
	}
	if !c.windowOpen(w, key.Class, c.clock.Now()) {
		return CoalesceSteer
	}
	if held >= MaxHeldPerKey {
		return CoalesceFoldOverCap
	}
	return CoalesceHold
}

// RecordSteer starts the class window; for CI it also records the head SHA
// (a SHA-less steer keeps the previous SHA).
func (c *WindowCoalescer) RecordSteer(key CoalesceKey, headSHA string) {
	now := c.clock.Now()
	c.mu.Lock()
	w := c.window(key)
	if w == nil {
		if c.state[key.SessionUUID] == nil {
			c.state[key.SessionUUID] = map[prSlot]*prWindow{}
		}
		w = &prWindow{}
		c.state[key.SessionUUID][slotOf(key)] = w
		c.nslots++
	}
	insideFeedback := key.Class == ClassCI && c.windowOpen(w, ClassHumanFeedback, now)
	if key.Class == ClassCI {
		if headSHA != "" {
			w.ciSHA = headSHA
		}
		w.ciAt = now
	} else {
		w.feedbackAt = now
	}
	c.mu.Unlock()
	if insideFeedback && c.metrics != nil {
		c.metrics.Inc(MetricCISteerInsideFeedbackWindow, nil)
	}
}

// Evict forgets every window of the session.
func (c *WindowCoalescer) Evict(sessionUUID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nslots -= len(c.state[sessionUUID])
	delete(c.state, sessionUUID)
}

// NextEligible is read-only: the close of the class window while it is open,
// ok=false (steer now) otherwise.
func (c *WindowCoalescer) NextEligible(key CoalesceKey) (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.window(key)
	if !c.windowOpen(w, key.Class, c.clock.Now()) {
		return time.Time{}, false
	}
	return w.classStart(key.Class).Add(c.interval), true
}

// size is the number of tracked (session, PR) slots; tests assert it stays bounded.
func (c *WindowCoalescer) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nslots
}
