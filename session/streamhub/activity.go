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
// hint (tymuxd's SetPaneActivity). Implementations must no-op, not fail, when the backend or
// daemon version cannot take the hint.
type ActivityHinter interface {
	SetPaneActivity(ctx context.Context, activity qos.Activity) error
}

// activityHintTimeout bounds one hint RPC; hints are advisory, so a slow daemon just misses one.
const activityHintTimeout = 2 * time.Second

// WithQoSTracker replaces the process-wide interactivity tracker, e.g. with a fake-clock one.
func WithQoSTracker(t *qos.Tracker) HubOption {
	return func(h *StreamHub) { h.qos = t }
}

// WithActivitySink makes the hub report its session's activity class to fn on every change. fn
// runs on a hub-owned goroutine, never on the input path, and calls are serialized with
// latest-wins coalescing. Supplying a sink also enables background deprioritization of the
// hub's own output.
func WithActivitySink(fn func(qos.Activity)) HubOption {
	return func(h *StreamHub) { h.activitySink = fn }
}

// activityState drives the hub's activity reporting. Zero value is inert.
type activityState struct {
	desired atomic.Int32 // qos.Activity the hub should be in
	pending atomic.Bool  // a pump goroutine owns lastSent
	// lastSent is touched only by the pump goroutine holding pending.
	lastSent qos.Activity

	unwatch  func()
	expiryMu sync.Mutex
	expiry   Timer
	closed   atomic.Bool
}

// initQoS wires the hub into the tracker. Called once from NewStreamHub after options apply.
func (h *StreamHub) initQoS() {
	if h.qos == nil {
		h.qos = qos.Default
	}
	if h.activitySink == nil {
		if hinter, ok := h.controller.(ActivityHinter); ok {
			h.activitySink = func(a qos.Activity) {
				ctx, cancel := context.WithTimeout(context.Background(), activityHintTimeout)
				defer cancel()
				if err := hinter.SetPaneActivity(ctx, a); err != nil {
					log.Debug("streamhub: pane activity hint failed", "session", h.sessionName, "activity", a.String(), "error", err)
				}
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
	if a == qos.ActivityBackground {
		h.batchWindow.SetMaxWindow(BackgroundBatchWindow)
	} else {
		h.batchWindow.SetMaxWindow(h.batchMaxWindow)
	}
	h.kickActivityPump()
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
		if want != h.activity.lastSent {
			h.activity.lastSent = want
			h.activitySink(want)
		}
		sent := h.activity.lastSent // read before releasing ownership of lastSent
		h.activity.pending.Store(false)
		if qos.Activity(h.activity.desired.Load()) == sent ||
			!h.activity.pending.CompareAndSwap(false, true) {
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
	h.activity.expiryMu.Unlock()
	if h.activity.unwatch != nil {
		h.activity.unwatch()
	}
	h.qos.Forget(h.sessionName)
	h.activity.desired.Store(int32(qos.ActivityNormal))
	h.kickActivityPump()
}
