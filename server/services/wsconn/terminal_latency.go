package wsconn

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services"

	"go.opentelemetry.io/otel/attribute"

	"github.com/tstapler/stapler-squad/internal/termlat"
	"github.com/tstapler/stapler-squad/session"
)

// streamLatency holds one stream's backend and listener labels. The listener is known at
// accept time and the backend once the session resolves, both before any keystroke or output
// batch is recorded; reads are lock-free because they sit on the typing path.
type streamLatency struct {
	mu sync.Mutex // serializes writers only
	v  atomic.Pointer[termlat.Labels]
	// lastInputNs is the arrival time of the newest keystroke not yet followed by an output
	// write; 0 when none is waiting.
	lastInputNs atomic.Int64
}

// maxEchoGap bounds what counts as an echo: a longer wait is output unrelated to the keystroke
// (a silent password prompt, a long-running command) and would only skew the percentiles.
const maxEchoGap = 2 * time.Second

func (s *streamLatency) noteInput(now time.Time) { s.lastInputNs.Store(now.UnixNano()) }

// noteOutputWritten records echo_server for the first output write after a keystroke: input
// received -> the browser-bound frame written. It is a heuristic (the first output after a
// keystroke need not be its echo) and an upper bound under bulk output, which is the point.
func (s *streamLatency) noteOutputWritten(now time.Time) {
	in := s.lastInputNs.Swap(0)
	if in == 0 {
		return
	}
	if d := now.Sub(time.Unix(0, in)); d >= 0 && d <= maxEchoGap {
		termlat.Default.Record(termlat.StageEchoServer, s.labels(), d)
	}
}

func (s *streamLatency) update(fn func(*termlat.Labels)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.labels()
	fn(&next)
	s.v.Store(&next)
}

func (s *streamLatency) setListener(listener string) {
	s.update(func(l *termlat.Labels) { l.Listener = termlat.NormalizeListener(listener) })
}

func (s *streamLatency) setBackend(b session.ProcessManagerBackend) {
	if b == "" {
		b = session.BackendTmux // an unset backend is the tmux default
	}
	s.update(func(l *termlat.Labels) { l.Backend = termlat.NormalizeBackend(string(b)) })
}

func (s *streamLatency) labels() termlat.Labels {
	if p := s.v.Load(); p != nil {
		return *p
	}
	return termlat.Labels{}
}

// LatencyLabels lets the stream hub label this transport's output observations.
func (t *WebSocketTransport) LatencyLabels() termlat.Labels {
	if t.stream == nil {
		return termlat.Labels{}
	}
	return t.stream.latency.labels()
}

// keystrokeDelivery carries one browser keystroke to a session pane and times each stage.
type keystrokeDelivery struct {
	instance    *session.Instance
	latency     *streamLatency
	bookkeeper  *inputBookkeeper
	logPrefix   string
	tmuxSession string
	tmuxSocket  string
}

// deliver queues the (deferred) keystroke bookkeeping, then sends to the backend, recording
// input_bookkeep, input_send and input_total.
func (k keystrokeDelivery) deliver(writer services.TerminalWriter, data []byte) {
	labels := k.latency.labels()
	start := time.Now()
	k.latency.noteInput(start)

	k.bookkeeper.note(data)
	sendStart := time.Now()

	// Try the control-mode path first (low latency, no subprocess); it falls back to a
	// subprocess send-keys. Errors are non-fatal: keystrokes may be lost under load but the
	// stream stays alive (sending TerminalError kills it).
	sendCtx, cancel := context.WithTimeout(termlat.WithKeystroke(context.Background()), 2*time.Second)
	_ = writer.SendInput(sendCtx, services.PaneInput{
		ControlMode: k.instance,
		TmuxSocket:  k.tmuxSocket,
		TmuxSession: k.tmuxSession,
		LogPrefix:   k.logPrefix,
		Data:        data,
	})
	cancel()

	end := time.Now()
	termlat.Default.Record(termlat.StageInputBookkeep, labels, sendStart.Sub(start))
	termlat.Default.Record(termlat.StageInputSend, labels, end.Sub(sendStart))
	termlat.Default.Record(termlat.StageInputTotal, labels, end.Sub(start))
	if c, slow := k.slowCall(labels, k.tmuxSession, start, sendStart, end); slow {
		termlat.EmitSlowSpan(context.Background(), c)
	}
}

// slowCall describes this keystroke as a slow-call span tree when it took longer than
// termlat.SlowCallThreshold, so Tempo holds the slow ones and nothing else. The backend call
// shows up beside it as a session.backend.* span for the same session.
func (k keystrokeDelivery) slowCall(labels termlat.Labels, sessionID string, start, sendStart, end time.Time) (termlat.SlowCall, bool) {
	if end.Sub(start) < termlat.SlowCallThreshold() {
		return termlat.SlowCall{}, false
	}
	return termlat.SlowCall{
		Name:     "terminal.input",
		Start:    start,
		Duration: end.Sub(start),
		Attrs: []attribute.KeyValue{
			attribute.String("backend", labels.Backend),
			attribute.String("listener", labels.Listener),
			attribute.String("session", sessionID),
		},
		Stages: []termlat.StageSpan{
			{Name: "terminal.input.bookkeep", Start: start, End: sendStart},
			{Name: "terminal.input.send", Start: sendStart, End: end},
		},
	}, true
}

// countServerResync counts one mid-stream CurrentPaneRequest. A request the client flagged as
// same-size (visibility/focus with unchanged dimensions) is the avoidable kind.
func countServerResync(stream *connectWebSocketStream, req *sessionv1.CurrentPaneRequest) {
	reason := termlat.ResyncServerRequest
	if req.GetStaleDimensions() {
		reason = termlat.ResyncServerStaleDims
	}
	var backend string
	if stream != nil {
		backend = stream.latency.labels().Backend
	}
	termlat.Default.CountResync(reason, backend)
}
