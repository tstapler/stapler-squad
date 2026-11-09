package streamhub

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/internal/qos"
	"github.com/tstapler/stapler-squad/log"
)

// BackgroundBatchWindow is the output coalescing ceiling for a hub whose session is in the
// background while another session is interactive: a few more milliseconds of latency on an
// idle pane in exchange for fewer, larger writes competing with the typist's echo. Bounded, so
// background output is never held longer than this.
const BackgroundBatchWindow = 3 * MaxBatchWindow

// ActivityHinter is implemented by session controllers whose backend can use a per-pane priority
// hint (tymuxd's SetPaneActivity). A nil return means the hint is settled: delivered, or
// permanently impossible (backend or daemon version cannot take it). A non-nil return means it
// was not delivered and the hub should retry.
type ActivityHinter interface {
	SetPaneActivity(ctx context.Context, activity qos.Activity) error
}

// activityRefreshInterval is how often a non-Normal hint is re-sent. The daemon keeps hints until
// it restarts and forgets them then, so a refresh bounds how long a restarted daemon runs a
// boosted or demoted pane at Normal.
const activityRefreshInterval = time.Minute

// activityRetryInterval is how soon an undelivered hint (of any class) is retried, and
// activityMaxClosedRetries bounds those retries once the hub is closed: a failed final Normal
// would otherwise leave the pane demoted, but a dead hub must not retry forever.
const (
	activityRetryInterval    = 2 * time.Second
	activityMaxClosedRetries = 5
)

// activityHintTimeout bounds one hint RPC; hints are advisory, so a slow daemon just misses one.
const activityHintTimeout = 2 * time.Second

// WithQoSTracker replaces the process-wide interactivity tracker, e.g. with a fake-clock one.
func WithQoSTracker(t *qos.Tracker) HubOption {
	return func(h *StreamHub) { h.qos = t }
}

// WithActivitySink makes the hub report its session's activity class to fn on every change. fn
// runs on a hub-owned goroutine, never on the input path, and calls are serialized with
// latest-wins coalescing. A non-nil error from fn means the hint was not delivered and is
// retried. Supplying a sink also enables background deprioritization of the hub's own output.
func WithActivitySink(fn func(qos.Activity) error) HubOption {
	return func(h *StreamHub) { h.activitySink = fn }
}

// activityState drives the hub's activity reporting. Zero value is inert.
type activityState struct {
	desired atomic.Int32 // qos.Activity the hub should be in
	pending atomic.Bool  // a pump goroutine owns lastSent
	// lastSent, confirmed and failures are touched only by the pump goroutine holding pending.
	// lastSent is the last value the sink confirmed; a fresh hub is unconfirmed so it sends an
	// initial hint unconditionally instead of assuming the daemon already holds Normal.
	lastSent  qos.Activity
	confirmed bool
	failures  int

	unwatch  func()
	expiryMu sync.Mutex
	expiry   Timer
	refresh  Timer // guarded by expiryMu
	force    atomic.Bool
	closed   atomic.Bool
}

// initQoS wires the hub into the tracker. Called once from NewStreamHub after options apply.
func (h *StreamHub) initQoS() {
	if h.qos == nil {
		h.qos = qos.Default
	}
	if h.activitySink == nil {
		if hinter, ok := h.controller.(ActivityHinter); ok {
			h.activitySink = func(a qos.Activity) error {
				ctx, cancel := context.WithTimeout(context.Background(), activityHintTimeout)
				defer cancel()
				err := hinter.SetPaneActivity(ctx, a)
				if err != nil {
					log.Debug("streamhub: pane activity hint failed", "session", h.sessionName, "activity", a.String(), "error", err)
				}
				return err
			}
		}
	}
	if h.activitySink == nil {
		return
	}
	h.activity.unwatch = h.qos.Watch(h.refreshActivity)
	h.refreshActivity()
}

// Activity is the hub's current priority class.
func (h *StreamHub) Activity() qos.Activity { return h.qos.ActivityOf(h.sessionName) }

// backgrounded reports whether this hub should yield to a typing session elsewhere.
func (h *StreamHub) backgrounded() bool {
	return h.activitySink != nil && h.Activity() == qos.ActivityBackground
}

// noteTypingForQoS marks this session interactive and keeps an expiry timer armed so the
// "typing stopped" transition is reported promptly. O(1) on the keystroke path.
func (h *StreamHub) noteTypingForQoS() {
	if h.activity.closed.Load() {
		return // closeQoS already forgot this session; Note would resurrect it in the tracker
	}
	h.qos.Note(h.sessionName)
	if h.activitySink == nil {
		return
	}
	h.armActivityExpiry(qos.InteractiveWindow)
}

func (h *StreamHub) armActivityExpiry(d time.Duration) {
	h.activity.expiryMu.Lock()
	defer h.activity.expiryMu.Unlock()
	if h.activity.expiry != nil || h.activity.closed.Load() {
		return
	}
	h.activity.expiry = h.clock.AfterFunc(d, h.onActivityExpiry)
}

func (h *StreamHub) onActivityExpiry() {
	h.activity.expiryMu.Lock()
	h.activity.expiry = nil
	h.activity.expiryMu.Unlock()
	if left := h.qos.Expire(h.sessionName); left > 0 {
		h.armActivityExpiry(left)
	}
}

// refreshActivity recomputes the class and hands it to the pump. It never blocks: the tracker
// calls it from whichever goroutine just typed.
func (h *StreamHub) refreshActivity() {
	if h.activity.closed.Load() {
		return
	}
	a := h.qos.ActivityOf(h.sessionName)
	h.activity.desired.Store(int32(a))
	if h.activity.closed.Load() {
		// closeQoS raced past the check above; its final Normal must win over this stale store.
		h.activity.desired.Store(int32(qos.ActivityNormal))
		return
	}
	if a == qos.ActivityBackground {
		h.batchWindow.SetMaxWindow(BackgroundBatchWindow)
	} else {
		h.batchWindow.SetMaxWindow(h.batchMaxWindow)
	}
	h.kickActivityPump()
}

// armActivityRefresh schedules the next send after one attempt: a retry soon if it was not
// confirmed, else a periodic refresh of a non-Normal hint, else nothing.
func (h *StreamHub) armActivityRefresh(sent qos.Activity, confirmed bool, failures int) {
	h.activity.expiryMu.Lock()
	defer h.activity.expiryMu.Unlock()
	if h.activity.refresh != nil {
		h.activity.refresh.Stop()
		h.activity.refresh = nil
	}
	closed := h.activity.closed.Load()
	var delay time.Duration
	switch {
	case !confirmed && (!closed || failures < activityMaxClosedRetries):
		delay = activityRetryInterval
	case confirmed && sent != qos.ActivityNormal && !closed:
		delay = activityRefreshInterval
	default:
		return
	}
	h.activity.refresh = h.clock.AfterFunc(delay, func() {
		h.activity.force.Store(true)
		h.kickActivityPump()
	})
}

func (h *StreamHub) kickActivityPump() {
	if !h.activity.pending.CompareAndSwap(false, true) {
		return
	}
	go h.pumpActivity()
}

func (h *StreamHub) pumpActivity() {
	for {
		want := qos.Activity(h.activity.desired.Load())
		refresh := h.activity.force.Swap(false)
		a := &h.activity
		if !a.confirmed || want != a.lastSent || (refresh && want != qos.ActivityNormal) {
			if err := h.activitySink(want); err == nil {
				a.lastSent, a.confirmed, a.failures = want, true, 0
			} else {
				a.confirmed = false
				a.failures++
			}
			h.armActivityRefresh(want, a.confirmed, a.failures)
		}
		// Retrying a failure is the timer's job; looping on it here would spin.
		a.pending.Store(false) // releases ownership of lastSent/confirmed/failures
		if qos.Activity(a.desired.Load()) == want || !a.pending.CompareAndSwap(false, true) {
			return
		}
	}
}

// closeQoS ends the hub's participation: it stops watching, clears the session from the tracker
// and sends a final Normal so a dead hub never leaves its pane boosted or demoted.
func (h *StreamHub) closeQoS() {
	if h.activitySink == nil || !h.activity.closed.CompareAndSwap(false, true) {
		return
	}
	h.activity.expiryMu.Lock()
	if h.activity.expiry != nil {
		h.activity.expiry.Stop()
		h.activity.expiry = nil
	}
	if h.activity.refresh != nil {
		h.activity.refresh.Stop()
		h.activity.refresh = nil
	}
	h.activity.expiryMu.Unlock()
	if h.activity.unwatch != nil {
		h.activity.unwatch()
	}
	h.qos.Forget(h.sessionName)
	h.activity.desired.Store(int32(qos.ActivityNormal))
	h.kickActivityPump()
}
