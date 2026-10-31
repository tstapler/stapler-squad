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

// driverPaneActivityGateFlagTTL bounds how often the flag is re-read from config; LoadConfig reads
// and parses the config file, and every driver asks on every tick.
const driverPaneActivityGateFlagTTL = 5 * time.Second

// DriverPaneActivityGateSetting reports the live (uncached) state of the driver pane-activity gate flag.
func DriverPaneActivityGateSetting() bool {
	return config.LoadConfig().GetFeatureFlagWithDefault(FeatureFlagDriverPaneActivityGate, config.DriverPaneActivityGateDefault)
}

var driverGateFlag = &cachedFlag{read: DriverPaneActivityGateSetting, ttl: driverPaneActivityGateFlagTTL, now: time.Now}

func driverPaneActivityGateEnabled() bool { return driverGateFlag.get() }

// cachedFlag memoizes a boolean read for ttl.
type cachedFlag struct {
	read func() bool
	ttl  time.Duration
	now  func() time.Time

	mu      sync.Mutex
	value   bool
	readAt  time.Time
	hasRead bool
}

func (f *cachedFlag) get() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if now := f.now(); !f.hasRead || now.Sub(f.readAt) >= f.ttl {
		f.value, f.readAt, f.hasRead = f.read(), now, true
	}
	return f.value
}

// driverPaneSnapshot is the snapshot source a driver hands its gate: the shared local snapshot,
// or nil for a remote instance (the snapshot is the local tmux server's).
func driverPaneSnapshot(remote bool) map[string]time.Time {
	if remote {
		return nil
	}
	return sharedPaneActivity.get(paneSnapshotMaxAgeDriver)
}

// captureStaleness bounds how old the content behind a capture can be: the process manager serves
// a cached capture for up to capturePaneCacheTTL, stamped at completion, so a capture that began at T
// may show the pane as of T minus the TTL minus the capture latency.
const captureStaleness = capturePaneCacheTTL + captureLatencyAllowance

// captureLatencyAllowance covers the cache being stamped when a capture finishes while its content is
// as of when tmux ran it; 3 s is the control-mode capture timeout, the longest a capture runs.
const captureLatencyAllowance = 3 * time.Second

// paneActivityGate lets one driver skip a capture-pane when its pane has produced no output since
// the last capture. One gate belongs to one driver goroutine; it is not safe for concurrent use.
//
// A cached capture is reused only when all hold: the session is in the snapshot with the same
// pane_last_activity as at the last capture, that capture's content is provably newer than the
// activity second (capture start minus captureStaleness falls in a later whole second, so any later
// output moves the activity), and the capture is under paneGateMaxCaptureAge old. Otherwise it captures, exactly as before.
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

// reset drops the cached capture; the driver calls it when the instance's preview is blocked.
func (g *paneActivityGate) reset() { g.valid = false }

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
		g.capturedAt.Add(-captureStaleness).Unix() > activity.Unix() && start.Sub(g.capturedAt) < paneGateMaxCaptureAge {
		return g.content, nil
	}
	content, err := capture(ctx)
	// "" is also what previewBlocked returns for a paused/stopped/not-started instance; caching it
	// would serve a blank pane after the instance resumes.
	if err != nil || !inSnapshot || content == "" {
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

// driverPreview is the driver's per-tick preview: gated, except that a blocked instance (paused,
// stopped, hibernated, not started) always goes through PreviewContext so it gets its "" and the
// gate forgets any cached content.
func driverPreview(ctx context.Context, gate *paneActivityGate, inst *Instance) (string, error) {
	if inst.previewBlocked() {
		gate.reset()
		return inst.PreviewContext(ctx)
	}
	return gate.preview(ctx, inst.GetTmuxSessionName(), inst.PreviewContext)
}
