package wakes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
)

// PeerEscalationTimeout is how long an open peer-help request waits for its
// answerer before the operator is pulled in.
const PeerEscalationTimeout = 10 * time.Minute

// Peer-help states counted in peer_help_total by the escalator.
const (
	peerStateEscalated = "escalated"
	peerStateNotSent   = "not_sent"
)

// PeerRequest is one open peer-help request.
type PeerRequest struct {
	ID        string
	Asker     string
	Answerer  string
	Question  string
	CreatedAt time.Time
	// AskerTitle and AnswererTitle name the sessions for the operator; empty
	// falls back to the uuid.
	AskerTitle    string
	AnswererTitle string
}

// PeerRequests is the durable store of open peer-help requests.
type PeerRequests interface {
	OpenPeerRequests(ctx context.Context) ([]PeerRequest, error)
	// MarkEscalated durably records that the operator was told about id. It
	// reports false when it already was, which is what keeps a restart or a
	// second pass from raising a second notification.
	MarkEscalated(ctx context.Context, id string) (bool, error)
}

// PeerFaultOf says why a request to answerer can no longer be answered or
// delivered, from the facts of both sessions. notSent marks a request that
// will never reach the peer ("Not sent: ..."); a stopped peer is only stalled
// (it would answer if resumed). ok is false when the peer is reachable.
func PeerFaultOf(answerer, asker OwnerFacts) (reason string, notSent, ok bool) {
	switch {
	case !answerer.Exists:
		return "peer no longer exists", true, true
	case answerer.Archived:
		return "peer is archived", true, true
	case answerer.WorkspaceKey != "" && asker.WorkspaceKey != "" && answerer.WorkspaceKey != asker.WorkspaceKey:
		return "peer works in a different repository", true, true
	case !answerer.Live:
		return "Peer is stopped. Not delivered unless resumed.", false, true
	}
	return "", false, false
}

// EscalatorConfig wires an Escalator.
type EscalatorConfig struct {
	Requests PeerRequests
	Owners   OwnerLookup
	Notifier Notifier
	// Events delivers session lifecycle events; nil leaves only the deadlines.
	Events  EventSource
	Clock   Clock
	Metrics *Metrics
	// Timeout overrides PeerEscalationTimeout (tests).
	Timeout time.Duration
}

// Escalator raises one needs-human notification per open peer-help request
// that nobody answered within the timeout, or at once when its answerer is
// stopped, archived or deleted. Deadlines are derived from the durable rows and
// armed on the Clock (one timer, no poll loop); a startup pass catches
// everything that expired while the process was down.
type Escalator struct {
	cfg   EscalatorConfig
	rearm chan struct{}

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}

	// escalated caches ids already handled so a pass does not re-hit the store.
	escalated map[string]struct{}
	// answerers maps an answerer uuid to the open requests that wait on it.
	answerers map[string]struct{}
}

// NewEscalator builds an Escalator; Start runs it.
func NewEscalator(cfg EscalatorConfig) *Escalator {
	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = PeerEscalationTimeout
	}
	if cfg.Metrics == nil {
		cfg.Metrics = NewMetrics()
	}
	return &Escalator{
		cfg: cfg, rearm: make(chan struct{}, 1),
		escalated: map[string]struct{}{}, answerers: map[string]struct{}{},
	}
}

// Rearm asks the loop to re-read the open requests (a request was created,
// answered or cancelled). It never blocks and coalesces bursts.
func (e *Escalator) Rearm() {
	select {
	case e.rearm <- struct{}{}:
	default:
	}
}

// Start runs the startup pass and launches the loop.
func (e *Escalator) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return errors.New("wakes: escalator already running")
	}
	loopCtx, cancel := context.WithCancel(ctx)
	var evCh <-chan *events.Event
	if e.cfg.Events != nil {
		evCh, _ = e.cfg.Events.Subscribe(loopCtx)
	}
	e.running, e.cancel, e.done = true, cancel, make(chan struct{})
	go e.loop(loopCtx, evCh, e.done)
	return nil
}

// Stop ends the loop and waits for it (or ctx).
func (e *Escalator) Stop(ctx context.Context) error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return nil
	}
	cancel, done := e.cancel, e.done
	e.running = false
	e.mu.Unlock()
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wakes: escalator did not stop: %w", ctx.Err())
	}
}

func (e *Escalator) loop(ctx context.Context, evCh <-chan *events.Event, done chan struct{}) {
	defer close(done)
	for {
		next, ok := e.Pass(ctx)
		var timer Timer
		var fire <-chan time.Time
		if ok {
			timer = e.cfg.Clock.NewTimer(max(0, next.Sub(e.cfg.Clock.Now())))
			fire = timer.C()
		}
		alive := e.wait(ctx, evCh, fire)
		if timer != nil {
			timer.Stop()
		}
		if !alive {
			return
		}
	}
}

// wait blocks until something worth a new pass happens: the deadline, a
// rearm, or a lifecycle event of a session some request waits on. It reports
// false once ctx ends.
func (e *Escalator) wait(ctx context.Context, evCh <-chan *events.Event, fire <-chan time.Time) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case <-fire:
			return true
		case <-e.rearm:
			return true
		case ev, open := <-evCh:
			if !open {
				evCh = nil
				continue
			}
			if e.waitsOn(sessionEventID(ev)) {
				return true
			}
		}
	}
}

func (e *Escalator) waitsOn(sessionUUID string) bool {
	if sessionUUID == "" {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.answerers[sessionUUID]
	return ok
}

// Pass escalates every open request that is overdue or whose answerer is gone
// and returns the earliest deadline among the rest.
func (e *Escalator) Pass(ctx context.Context) (next time.Time, ok bool) {
	reqs, err := e.cfg.Requests.OpenPeerRequests(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Warn("peer help escalator: reading open requests", errKey, err)
		}
		return e.cfg.Clock.Now().Add(storeRetryDelay), true
	}
	now := e.cfg.Clock.Now()
	waiting := map[string]struct{}{}
	for _, r := range reqs {
		if e.handled(r.ID) {
			continue
		}
		if reason, state := e.reasonToEscalate(ctx, r, now); reason != "" {
			e.escalate(ctx, r, reason, state)
			continue
		}
		waiting[r.Answerer] = struct{}{}
		if deadline := r.CreatedAt.Add(e.cfg.Timeout); !ok || deadline.Before(next) {
			next, ok = deadline, true
		}
	}
	e.mu.Lock()
	e.answerers = waiting
	e.mu.Unlock()
	return next, ok
}

func (e *Escalator) handled(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, done := e.escalated[id]
	return done
}

// reasonToEscalate says why r needs the operator now ("" when it does not) and
// which peer_help_total state that is: a request that can never reach its peer
// counts as not_sent, a merely silent one as escalated.
func (e *Escalator) reasonToEscalate(ctx context.Context, r PeerRequest, now time.Time) (reason, state string) {
	if fault, notSent := e.answererFault(ctx, r); fault != "" {
		if notSent {
			return fault, peerStateNotSent
		}
		return fault, peerStateEscalated
	}
	if !now.Before(r.CreatedAt.Add(e.cfg.Timeout)) {
		return fmt.Sprintf("no reply in %dm", int(e.cfg.Timeout/time.Minute)), peerStateEscalated
	}
	return "", ""
}

func (e *Escalator) answererFault(ctx context.Context, r PeerRequest) (reason string, notSent bool) {
	if e.cfg.Owners == nil {
		return "", false
	}
	peer, err := e.cfg.Owners.Owner(ctx, r.Answerer)
	if err != nil {
		return "", false
	}
	asker, err := e.cfg.Owners.Owner(ctx, r.Asker)
	if err != nil {
		return "", false
	}
	reason, notSent, _ = PeerFaultOf(peer, asker)
	return reason, notSent
}

func (e *Escalator) escalate(ctx context.Context, r PeerRequest, reason, state string) {
	first, err := e.cfg.Requests.MarkEscalated(ctx, r.ID)
	if err != nil {
		if ctx.Err() == nil {
			log.Warn("peer help escalator: recording escalation", "request_id", r.ID, errKey, err)
		}
		return
	}
	e.mu.Lock()
	e.escalated[r.ID] = struct{}{}
	e.mu.Unlock()
	if !first {
		return
	}
	e.cfg.Metrics.Inc(MetricPeerHelp, state)
	e.cfg.Notifier.Notify(Notification{
		Kind:              NotifyPeerStalled,
		OwnerSessionUUID:  r.Asker,
		GuidanceRequestID: r.ID,
		PeerTitle:         orID(r.AnswererTitle, r.Answerer),
		Title:             "Help request stalled",
		Message:           fmt.Sprintf("%s asked %s: %s. %s", orID(r.AskerTitle, r.Asker), orID(r.AnswererTitle, r.Answerer), reason, snippet(r.Question)),
	})
}

func orID(title, id string) string {
	if title != "" {
		return title
	}
	return id
}
