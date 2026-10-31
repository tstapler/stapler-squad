package session

import (
	"bytes"
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
)

// FeatureFlagDriverPaneActivityGate is the kill switch for paneActivityGate. Default on: with
// the flag off every driver tick captures the pane, as before.
const FeatureFlagDriverPaneActivityGate = "driver_pane_activity_gate"

const (
	// paneSnapshotMaxAgeDriver is how stale a shared pane-activity snapshot a driver tick
	// accepts: one driver tick (2 s), so detection latency stays within the existing class.
	paneSnapshotMaxAgeDriver = 2 * time.Second
	// paneGateMaxCaptureAge forces a recapture even when activity is unchanged, covering tmux-side
	// screen changes (resize, clear) that do not move pane_last_activity.
	paneGateMaxCaptureAge = 30 * time.Second
)

// paneActivityCache shares one `list-panes -a` result across every consumer, so N sessions cost
// one spawn per max-age window instead of one capture-pane each per tick.
type paneActivityCache struct {
	fetch func() map[string]time.Time
	now   func() time.Time

	mu        sync.Mutex
	snap      map[string]time.Time
	fetchedAt time.Time
	fetched   bool
}

func newPaneActivityCache(fetch func() map[string]time.Time, now func() time.Time) *paneActivityCache {
	return &paneActivityCache{fetch: fetch, now: now}
}

// get returns a snapshot no older than maxAge, fetching under the lock so concurrent callers
// coalesce into one spawn. A failed fetch (nil) is cached for the same window: callers then take
// their uncached path rather than every one of them re-spawning against an unavailable tmux.
func (c *paneActivityCache) get(maxAge time.Duration) map[string]time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now := c.now(); c.fetched && now.Sub(c.fetchedAt) < maxAge {
		return c.snap
	}
	c.snap = c.fetch()
	c.fetchedAt = c.now()
	c.fetched = true
	return c.snap
}

var sharedPaneActivity = newPaneActivityCache(func() map[string]time.Time { return batchWindowActivity("") }, time.Now)

func driverPaneActivityGateEnabled() bool {
	return config.LoadConfig().GetFeatureFlagWithDefault(FeatureFlagDriverPaneActivityGate, true)
}

// paneActivityGate lets one driver skip a capture-pane when its pane has produced no output since
// the last capture. One gate belongs to one driver goroutine; it is not safe for concurrent use.
//
// A cached capture is reused only when all hold: the session is in the snapshot with the same
// pane_last_activity as at the last capture, that capture began in a later second than the
// activity (so any output after the capture moves the activity to a later second), and the capture
// is under paneGateMaxCaptureAge old. Otherwise it captures, exactly as before.
type paneActivityGate struct {
	snapshot func() map[string]time.Time
	now      func() time.Time
	enabled  func() bool

	valid      bool
	activity   time.Time
	capturedAt time.Time
	content    string
}

func newPaneActivityGate(snapshot func() map[string]time.Time, now func() time.Time, enabled func() bool) *paneActivityGate {
	return &paneActivityGate{snapshot: snapshot, now: now, enabled: enabled}
}

// preview returns the pane content for tmuxName, calling capture only when the cache cannot be
// proven current. Capture errors are returned unchanged and never cached.
func (g *paneActivityGate) preview(ctx context.Context, tmuxName string, capture func(context.Context) (string, error)) (string, error) {
	if !g.enabled() {
		g.valid = false
		return capture(ctx)
	}
	activity, inSnapshot := g.snapshot()[tmuxName]
	start := g.now()
	if inSnapshot && g.valid && activity.Equal(g.activity) &&
		g.capturedAt.Unix() > activity.Unix() && start.Sub(g.capturedAt) < paneGateMaxCaptureAge {
		return g.content, nil
	}
	content, err := capture(ctx)
	if err != nil || !inSnapshot {
		g.valid = false
		return content, err
	}
	g.valid, g.activity, g.capturedAt, g.content = true, activity, start, content
	return content, nil
}

// batchWindowActivity returns, per tmux session, #{window_activity} (Unix seconds of the last
// output in the window) for the session's active pane, from one `list-panes -a`. That is the pane
// `capture-pane -t <session>` reads. window_activity covers every pane in the window, so it can
// only over-report change, never miss it. Returns nil when no slot is free or tmux failed.
func batchWindowActivity(socket string) map[string]time.Time {
	out, ok := listAllPanes(socket, "#{session_name} #{window_active}#{pane_active} #{window_activity}")
	if !ok {
		return nil
	}
	return parseWindowActivity(out)
}

// parseWindowActivity parses "<session> <window_active><pane_active> <unix>" lines. The active
// pane's line wins; a session with no active pane line keeps its first.
func parseWindowActivity(out []byte) map[string]time.Time {
	result := make(map[string]time.Time)
	active := make(map[string]bool)
	for _, line := range bytes.Split(out, []byte{'\n'}) {
		fields := bytes.Fields(line)
		if len(fields) != 3 {
			continue
		}
		ts, err := strconv.ParseInt(string(fields[2]), 10, 64)
		if err != nil || ts <= 0 {
			continue
		}
		name := string(fields[0])
		isActive := string(fields[1]) == "11"
		if _, seen := result[name]; seen && (active[name] || !isActive) {
			continue
		}
		result[name], active[name] = time.Unix(ts, 0), isActive
	}
	return result
}
