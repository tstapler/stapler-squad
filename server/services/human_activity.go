package services

import (
	"context"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// human_activity.go is the signal behind the human_active guard (ADR-004
// residual risk, Task 3.4f): was a person typing in this pane just now.

// humanActiveWindow is how recent input must be to refuse a pane write.
const humanActiveWindow = 5 * time.Second

// humanActivityKeepAfter bounds the recorder: older entries cannot matter.
const humanActivityKeepAfter = time.Minute

// HumanActivity records the last browser-originated pane input per tmux session.
type HumanActivity struct {
	mu   sync.Mutex
	now  func() time.Time
	last map[string]time.Time
}

// NewHumanActivity builds a recorder on now (time.Now when nil).
func NewHumanActivity(now func() time.Time) *HumanActivity {
	if now == nil {
		now = time.Now
	}
	return &HumanActivity{now: now, last: map[string]time.Time{}}
}

// defaultHumanActivity is shared by every web-terminal stream, which each build
// their own paneWriter, and by the answer router's guard.
var defaultHumanActivity = NewHumanActivity(nil)

// Record notes input to tmuxSession now.
func (h *HumanActivity) Record(tmuxSession string) {
	if h == nil || tmuxSession == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	h.last[tmuxSession] = now
	if len(h.last) > 64 {
		for k, t := range h.last {
			if now.Sub(t) > humanActivityKeepAfter {
				delete(h.last, k)
			}
		}
	}
}

// Within reports whether tmuxSession received input in the last d.
func (h *HumanActivity) Within(tmuxSession string, d time.Duration) bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.last[tmuxSession]
	return ok && h.now().Sub(t) <= d
}

// tmuxClientLister is the list-clients seam; production uses tmux.ListClientActivity.
type tmuxClientLister func(ctx context.Context, serverSocket string) ([]tmux.ClientActivity, error)

// HumanActiveGuard answers "is a human active in this session's pane".
type HumanActiveGuard struct {
	activity *HumanActivity
	clients  tmuxClientLister
	// ours says a listed client is ssq's own pty attach; tests inject it.
	ours func(tmux.ClientActivity) bool
}

// NewHumanActiveGuard builds the guard. clients may be nil: signal (b) is then unavailable.
func NewHumanActiveGuard(activity *HumanActivity, clients tmuxClientLister) *HumanActiveGuard {
	return &HumanActiveGuard{activity: activity, clients: clients, ours: tmux.ClientActivity.IsOurs}
}

// ssq attaches a (non control-mode) pty client to every session it starts; that client's
// activity is the app resizing or redrawing, not a person, so it is skipped (client_pid is
// our child).
//
// Active is true when (a) the web terminal sent input to the session within the
// window or (b) a non-control-mode tmux client attached to it had input within
// the window (a person's own `tmux attach`; ssq's control-mode clients are
// always attached, so attachment alone says nothing). A list-clients failure
// drops (b) only.
func (g *HumanActiveGuard) Active(ctx context.Context, inst *session.Instance) bool {
	name := inst.GetTmuxSessionName()
	if g.activity.Within(name, humanActiveWindow) {
		return true
	}
	if g.clients == nil || name == "" {
		return false
	}
	clients, err := g.clients(ctx, inst.Snapshot().TmuxServerSocket)
	if err != nil {
		log.Warn("[HumanActivity] list-clients failed; using web-terminal input only", "session", name, "err", err)
		return false
	}
	now := g.activity.now()
	for _, c := range clients {
		if !c.ControlMode && c.Session == name && now.Sub(c.Activity) <= humanActiveWindow && !g.ours(c) {
			return true
		}
	}
	return false
}
