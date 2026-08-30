package tymux

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/log"
)

// reconnectMetrics backs tymux_attach_stream_reconnects_total (Task
// 2.5.2c, Observability Plan) — a process-wide hand-rolled counter
// matching session/tmux/fork_metrics.go's existing atomic.Int64
// convention (no metrics-library dependency), keyed by cause
// ("error" for a transport drop that triggered ReconnectLoop,
// "output_gap" for an in-stream resync that never reopened Attach) since
// a single aggregate count would conflate two different situations the
// Observability Plan wants distinguishable.
var reconnectMetrics = struct {
	mu     sync.Mutex
	counts map[string]*atomic.Int64
}{counts: make(map[string]*atomic.Int64)}

// recordReconnect increments tymux_attach_stream_reconnects_total{cause}.
func recordReconnect(cause string) {
	reconnectMetrics.mu.Lock()
	c, ok := reconnectMetrics.counts[cause]
	if !ok {
		c = new(atomic.Int64)
		reconnectMetrics.counts[cause] = c
	}
	reconnectMetrics.mu.Unlock()
	c.Add(1)
}

// ReconnectMetricsSnapshot returns a point-in-time copy of
// tymux_attach_stream_reconnects_total, keyed by cause — exported so a
// future Observability Plan wiring (e.g. an HTTP /metrics handler) can
// read it without reaching into package-private state, mirroring
// session/tmux's own ForkPressureSnapshot() convention.
func ReconnectMetricsSnapshot() map[string]int64 {
	reconnectMetrics.mu.Lock()
	defer reconnectMetrics.mu.Unlock()
	out := make(map[string]int64, len(reconnectMetrics.counts))
	for k, v := range reconnectMetrics.counts {
		out[k] = v.Load()
	}
	return out
}

// openStandingStream opens the one Attach stream that backs a
// tymuxGRPCSession for its whole lifetime (Story 2.3.1) — called once from
// cacheFromSession, the common tail of Start/RestoreWithWorkDir, never
// per-SendKeys-call. Tears down any previously open stream first, so a
// second Start/RestoreWithWorkDir call (e.g. a reconnect) never leaks the
// prior stream's reader goroutine.
//
// Deliberately fire-and-forget: it sends pane_id and starts the reader
// goroutine without waiting for the first AttachEvent, so Start()/
// RestoreWithWorkDir() never block on tymuxd's first reply. ReconnectLoop
// (below) uses a different, synchronous dial for its own reattach
// attempts, where blocking is safe because it always runs off the
// reader goroutine, never off a caller of Start/RestoreWithWorkDir.
func (s *tymuxGRPCSession) openStandingStream(paneID string) error {
	s.teardownStandingStream()

	ctx, cancel := context.WithCancel(context.Background())
	stream := s.transport.Attach(ctx)
	if stream == nil {
		cancel()
		return fmt.Errorf("tymux: Attach: transport returned a nil stream")
	}
	if err := stream.Send(&v1.AttachRequest{
		Payload: &v1.AttachRequest_PaneId{PaneId: paneID},
	}); err != nil {
		cancel()
		return classifyRPCError("Attach", err)
	}

	done := make(chan struct{})
	s.mu.Lock()
	s.stream = stream
	s.cancelAttach = cancel
	s.streamDone = done
	s.mu.Unlock()

	log.Info("tymux: standing Attach stream opened", "pane_id", paneID)

	go s.readAttachLoop(paneID, stream, done)
	return nil
}

// teardownStandingStream cancels the currently open standing stream (if
// any) and waits for its reader goroutine to exit. Safe to call when no
// stream is open (no-op). Callers that want this to actually terminate
// the reader goroutine (rather than let ReconnectLoop transparently
// reattach) must call beginClosing() first — see Close()/DetachSafely().
func (s *tymuxGRPCSession) teardownStandingStream() {
	s.mu.Lock()
	cancel := s.cancelAttach
	done := s.streamDone
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// readAttachLoop is the standing stream's reader goroutine (Task 2.3.1b,
// restructured by Story 2.5.1/2.5.2). On every AttachEvent it delegates to
// handleAttachEvent. On a Receive() error it checks s.closing and
// s.exited: if a deliberate Close()/DetachSafely() set closing first, or
// the pane already delivered a clean Exited event (deliverExit already
// ran via handleAttachEvent's Exited case — tymuxd closes the stream
// itself right after sending Exited, per crates/tymuxd/src/main.rs, so
// the very next Receive() is expected to error, typically io.EOF), this
// is an intentional/known end — close done and return, unblocking
// Attach()'s callers and teardownStandingStream's <-done wait. Without
// the exited check, a clean pane exit gets misclassified as a transport
// drop: ReconnectLoop's reattach dial fails with errPaneDead (the pane
// really is dead, just not because tymuxd restarted), which
// daemonRestarted's errors.Is check can't tell apart from an actual
// daemon restart, so it spawns a needless replacement process via
// ReviveSession and falsely reports BackendRestarted() == true.
// Otherwise it's a genuine unexpected drop (network blip, daemon
// restart) — ReconnectLoop (Story 2.5.1's acceptance criteria: "any
// other stream-end triggers ReconnectLoop") either splices in a freshly
// reattached stream, which this same goroutine resumes reading (so
// s.streamDone/Attach()'s channel never fires for a transient drop that
// successfully reconnects — the whole point is transparency), or
// exhausts its retry budget, in which case it closes done itself and
// this goroutine returns.
func (s *tymuxGRPCSession) readAttachLoop(paneID string, stream attachStream, done chan struct{}) {
	for {
		event, err := stream.Receive()
		if err != nil {
			s.mu.RLock()
			exited := s.exited
			s.mu.RUnlock()
			if s.closing.Load() || exited {
				close(done)
				return
			}
			newStream, first, ok := s.ReconnectLoop(paneID, "error")
			if !ok {
				close(done)
				return
			}
			stream = newStream
			if first != nil {
				// The reattach's own priming event was already consumed
				// by ReconnectLoop's resync call (resync.go); nothing
				// further to do with it here except keep reading.
				_ = first
			}
			continue
		}
		s.handleAttachEvent(event)
	}
}

// handleAttachEvent is readAttachLoop's per-event switch, pulled out into
// its own method so ReconnectLoop's resync path (resync.go) can be driven
// from the same logic without duplicating it: on Snapshot (Epic 1.3's
// priming event, also the first event of a fresh reconnect per ADR-003),
// seed local liveness state; on Output, forward to ClientFanout; on
// Exited, update cached liveness and fire the registered exit callback;
// on OutputGap, run the shared resync path (Task 2.5.2b) — the proto's
// own "not a fatal condition" contract, so a resync failure here is
// swallowed rather than tearing down the stream.
func (s *tymuxGRPCSession) handleAttachEvent(event *v1.AttachEvent) {
	switch p := event.GetPayload().(type) {
	case *v1.AttachEvent_Snapshot:
		s.mu.Lock()
		s.liveness = p.Snapshot.GetLiveness()
		s.mu.Unlock()
	case *v1.AttachEvent_Output:
		s.fanout.Broadcast(p.Output)
	case *v1.AttachEvent_Exited:
		s.mu.Lock()
		s.liveness = v1.Liveness_LIVENESS_DEAD
		s.mu.Unlock()
		s.deliverExit(exitReason(p.Exited))
	case *v1.AttachEvent_OutputGap:
		recordReconnect("output_gap")
		count := s.outputGapCount.Add(1)
		log.Warn("tymux: output_gap received, resyncing via CapturePane", "session_id", s.GetSessionIdentifier(), "count", count)
		ctx, cancel := s.interruptibleContext()
		_ = s.resyncViaCapturePane(ctx)
		cancel()
	}
}

// exitReason renders an ExitStatus (ADR-001: an absent code is a
// first-class "exited, but no code known" state, never fabricated as 0)
// into the reason string SetOnExitCallback's callers expect, matching
// TmuxSession.SetOnExitCallback's func(reason string) shape.
func exitReason(status *v1.ExitStatus) string {
	if status == nil || status.Code == nil {
		return "exited: code=unknown"
	}
	return fmt.Sprintf("exited: code=%d", status.GetCode())
}

// deliverExit is the "before" half of Story 2.4.2's check-before-and-after
// fire-once pattern: readAttachLoop calls this the moment Exited arrives,
// which may be well before any SetOnExitCallback registration. It records
// the exit and, if a callback is already registered and hasn't fired yet,
// claims the fire-once slot and invokes it. If no callback is registered
// yet, this only records exited/exitReason — SetOnExitCallback's own
// check (session.go) is the "after" half that fires it once one shows up.
//
// Also ReconnectLoop's give-up path (Task 2.5.2a): rather than invent a
// second failure-notification mechanism for "backend permanently
// unreachable," it reuses this exact plumbing (pitfalls.md §5's "one
// mechanism, not two") with a distinguishable reason string.
func (s *tymuxGRPCSession) deliverExit(reason string) {
	s.mu.Lock()
	s.exited = true
	s.exitReason = reason
	cb := s.exitCallback
	fire := cb != nil && !s.exitFired
	if fire {
		s.exitFired = true
	}
	s.mu.Unlock()
	if fire {
		log.Info("tymux: exit callback fired", "session_id", s.GetSessionIdentifier(), "reason", reason, "registered_after_exit", false)
		cb(reason)
	} else {
		log.Debug("tymux: exit observed, no callback fired yet", "session_id", s.GetSessionIdentifier(), "reason", reason, "callback_registered", cb != nil)
	}
}

// sendOnStream writes req to the standing stream's input side. Every
// input method (SendKeys/TapEnter/SendPromptWithEnter/
// SendInputViaControlMode, Story 2.3.3) and SetWindowSize/SetDetachedSize
// (Epic 2.4) funnel through here rather than opening a new RPC.
func (s *tymuxGRPCSession) sendOnStream(req *v1.AttachRequest) error {
	s.mu.RLock()
	stream := s.stream
	s.mu.RUnlock()
	if stream == nil {
		return errSessionNotStarted
	}
	if err := stream.Send(req); err != nil {
		return classifyRPCError("Attach.Send", err)
	}
	return nil
}

// sendResize sends a Resize AttachRequest on the standing stream — the one
// helper SetWindowSize and SetDetachedSize both call (Task 2.4.1a).
func (s *tymuxGRPCSession) sendResize(cols, rows int) error {
	return s.sendOnStream(&v1.AttachRequest{
		Payload: &v1.AttachRequest_Resize{Resize: &v1.Resize{
			Cols: uint32(cols),
			Rows: uint32(rows),
		}},
	})
}

// --- Epic 2.5: reconnect loop, backoff, daemon-restart detection ---

// errPaneDead is the sentinel dialAttach wraps its returned error with
// when tymuxd rejects Attach with connect.CodeFailedPrecondition — Task
// 2.5.3a's signal that the pane is known (persisted) but has no live
// process behind it right now. Confirmed against
// crates/tymuxd/src/main.rs's resolve_live_pane: PaneLookup::Dead maps to
// Status::failed_precondition specifically (PaneLookup::Unknown maps to
// NotFound instead — genuinely never heard of, not the daemon-restart
// case). The daemon-restart contract (Story 2.5.3) is exactly this case:
// tymuxd's persistence layer remembers the pane existed but never
// persists an OS PID, so on restart every pane it reloads starts in this
// Dead state until ReviveSession respawns it — a fresh process, never a
// reattach to the original (engine.rs's revive_session unconditionally
// calls Pane::spawn_with_id).
var errPaneDead = errors.New("tymux: pane not live (daemon restart or process exit)")

// dialAttach performs one Attach{pane_id} dial and synchronously waits for
// the first AttachEvent, unlike openStandingStream's fire-and-forget
// initial open — safe here because every caller of dialAttach is
// ReconnectLoop, always running off the reader goroutine, never off a
// caller of Start/RestoreWithWorkDir. ADR-003 guarantees a successful
// Attach's first message is always a priming Snapshot; a failure here
// (Send or the first Receive) is classified via classifyRPCError, with
// connect.CodeFailedPrecondition specifically wrapped in errPaneDead per
// Task 2.5.3a.
func (s *tymuxGRPCSession) dialAttach(ctx context.Context, paneID string) (attachStream, *v1.AttachEvent, error) {
	stream := s.transport.Attach(ctx)
	if stream == nil {
		return nil, nil, fmt.Errorf("tymux: Attach: transport returned a nil stream")
	}
	if err := stream.Send(&v1.AttachRequest{
		Payload: &v1.AttachRequest_PaneId{PaneId: paneID},
	}); err != nil {
		return nil, nil, classifyRPCError("Attach", err)
	}
	first, err := stream.Receive()
	if err != nil {
		if connect.CodeOf(err) == connect.CodeFailedPrecondition {
			return nil, nil, fmt.Errorf("%w: %w", errPaneDead, classifyRPCError("Attach", err))
		}
		return nil, nil, classifyRPCError("Attach", err)
	}
	return stream, first, nil
}

// dialAttachInterruptible wraps dialAttach with a context that
// abortReconnect can cancel mid-dial — without this, a reconnect attempt
// blocked inside Send/Receive against an unresponsive (not merely
// refused) endpoint could make Close()/DetachSafely() wait indefinitely.
// The watcher goroutine it starts always exits by the time this function
// returns (via closing the local stop channel), so it never outlives the
// call.
func (s *tymuxGRPCSession) dialAttachInterruptible(paneID string) (attachStream, *v1.AttachEvent, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.RLock()
	abort := s.abortReconnect
	s.mu.RUnlock()
	stop := make(chan struct{})
	go func() {
		select {
		case <-abort:
			cancel()
		case <-stop:
		}
	}()
	stream, first, err := s.dialAttach(ctx, paneID)
	close(stop)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	return stream, first, cancel, nil
}

// interruptibleContext returns a context.Context derived from
// context.Background() that s.abortReconnect cancels — the same
// abort-on-close watcher pattern dialAttachInterruptible uses above,
// factored out for the other reconnect-path RPCs (the OutputGap resync in
// handleAttachEvent, and ReconnectLoop's reviveAfterRestart/
// resyncAfterReconnect calls) that previously ran on context.Background()
// directly. Without this, a slow-but-not-dropped tymuxd could block the
// reader goroutine (and therefore teardownStandingStream's <-done wait)
// indefinitely even after a deliberate Close()/DetachSafely() fired.
// Unlike dialAttachInterruptible — which hands its CancelFunc to the
// caller for the life of a whole stream — every caller here is a single
// short-lived RPC, so the returned CancelFunc must be invoked (e.g. via
// defer) as soon as that RPC returns; it both releases ctx and stops the
// watcher goroutine so it never outlives the call.
func (s *tymuxGRPCSession) interruptibleContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.RLock()
	abort := s.abortReconnect
	s.mu.RUnlock()
	stop := make(chan struct{})
	go func() {
		select {
		case <-abort:
			cancel()
		case <-stop:
		}
	}()
	return ctx, func() {
		close(stop)
		cancel()
	}
}

// reconnectBackoffDelay computes Task 2.5.2a's jittered exponential
// backoff for the given 1-indexed attempt number: base * 2^(attempt-1),
// capped at max, with up to 50% jitter subtracted so many sessions
// reconnecting simultaneously (pre-mortem.md P1 #2's mass-reconnect
// scenario) don't all retry in lockstep.
func reconnectBackoffDelay(attempt int, base, max time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := base
	for i := 1; i < attempt && d < max; i++ {
		d *= 2
		if d <= 0 { // overflow guard
			d = max
			break
		}
	}
	if d > max {
		d = max
	}
	jitter := time.Duration(rand.Int63n(int64(d)/2 + 1))
	return d - jitter
}

// waitBackoff sleeps for reconnectBackoffDelay(attempt, ...), returning
// false early (without completing the sleep) if abortReconnect fires
// first — Close()/DetachSafely() calling beginClosing() mid-backoff.
func (s *tymuxGRPCSession) waitBackoff(attempt int) bool {
	s.mu.RLock()
	base, max := s.reconnectBaseDelay, s.reconnectMaxDelay
	abort := s.abortReconnect
	s.mu.RUnlock()
	select {
	case <-time.After(reconnectBackoffDelay(attempt, base, max)):
		return true
	case <-abort:
		return false
	}
}

// reviveAfterRestart calls ReviveSession for this session's owning
// session_id after Task 2.5.3a's FailedPrecondition detection — the
// achievable daemon-restart contract (Story 2.5.3): tymuxd never
// persisted an OS PID, so there is nothing to reattach to; this respawns
// a fresh replacement process running the same command/cwd.
func (s *tymuxGRPCSession) reviveAfterRestart(ctx context.Context) error {
	s.mu.RLock()
	sessionID := s.sessionID
	s.mu.RUnlock()
	_, err := s.transport.ReviveSession(ctx, connect.NewRequest(&v1.ReviveSessionRequest{
		SessionId: sessionID,
	}))
	if err != nil {
		return classifyRPCError("ReviveSession", err)
	}
	return nil
}

// ReconnectLoop re-opens Attach{pane_id} with jittered exponential
// backoff after readAttachLoop observes a non-deliberate stream end
// (Story 2.5.1/2.5.2). Runs entirely within readAttachLoop's own reader
// goroutine — blocking here is fine since nothing else waits on this
// goroutine except via s.streamDone, which only a permanent outcome
// (successful handoff, or exhaustion) touches.
//
// Each attempt: dial Attach{pane_id} (Task 2.5.2a); if the pane is
// reported Dead (errPaneDead, Task 2.5.3a — the daemon-restart case),
// call ReviveSession (Story 2.5.3) and dial once more before giving up on
// this attempt. On any successful dial, run the shared resync path (Task
// 2.5.2b, resync.go) exactly once, clear ReconnectState (Task 2.5.2e),
// and return the new stream for readAttachLoop to resume reading from —
// s.streamDone is never touched on a successful reconnect, so a
// transient drop that recovers is invisible to Attach()'s callers.
//
// If every attempt up to reconnectMaxAttempts fails (or a deliberate
// Close()/DetachSafely() interrupts a backoff wait), it returns
// (nil, nil, false); on true exhaustion (not a deliberate interrupt) it
// also delivers a distinguishable failure via the existing exit-callback
// plumbing (deliverExit, Epic 2.4) — reusing "one mechanism, not two"
// (pitfalls.md §5) rather than inventing a second notification path.
func (s *tymuxGRPCSession) ReconnectLoop(paneID string, cause string) (attachStream, *v1.AttachEvent, bool) {
	recordReconnect(cause)
	sessionID := s.GetSessionIdentifier()
	log.Info("tymux: standing Attach stream reconnect starting", "session_id", sessionID, "pane_id", paneID, "cause", cause)

	s.mu.Lock()
	maxAttempts := s.reconnectMaxAttempts
	s.reconnecting = true
	s.reconnectAttempt = 0
	s.reconnectCause = cause
	s.reconnectSince = time.Now()
	s.mu.Unlock()

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if s.closing.Load() {
			break
		}
		s.mu.Lock()
		s.reconnectAttempt = attempt
		s.mu.Unlock()
		log.Info("tymux: standing Attach stream reconnect attempt", "session_id", sessionID, "pane_id", paneID, "cause", cause, "attempt", attempt, "max_attempts", maxAttempts)

		if attempt > 1 && !s.waitBackoff(attempt) {
			break // interrupted by a deliberate Close()/DetachSafely()
		}
		if s.closing.Load() {
			break
		}

		stream, first, cancel, err := s.dialAttachInterruptible(paneID)
		daemonRestarted := errors.Is(err, errPaneDead)
		if err != nil && daemonRestarted {
			log.Warn("tymux: daemon restart detected, reviving session", "session_id", sessionID, "pane_id", paneID)
			s.mu.Lock()
			s.backendRestarted = true
			s.backendRestartedAt = time.Now()
			s.mu.Unlock()
			reviveCtx, reviveCancel := s.interruptibleContext()
			reviveErr := s.reviveAfterRestart(reviveCtx)
			reviveCancel()
			if reviveErr != nil {
				log.Warn("tymux: ReviveSession failed after daemon restart", "session_id", sessionID, "pane_id", paneID, "err", reviveErr)
				continue
			}
			stream, first, cancel, err = s.dialAttachInterruptible(paneID)
		}
		if err != nil {
			continue
		}

		resyncCtx, resyncCancel := s.interruptibleContext()
		resyncErr := s.resyncAfterReconnect(resyncCtx, first)
		resyncCancel()
		if resyncErr != nil {
			// Best-effort, matching OutputGap's own "not a fatal
			// condition" contract — the stream itself is live again and
			// will keep delivering ordinary Output events regardless.
			_ = resyncErr
		}

		s.mu.Lock()
		s.stream = stream
		s.cancelAttach = cancel
		s.reconnecting = false
		s.reconnectAttempt = 0
		s.reconnectCause = ""
		s.mu.Unlock()

		log.Info("tymux: standing Attach stream reconnect succeeded", "session_id", sessionID, "pane_id", paneID, "cause", cause, "attempt", attempt)
		return stream, first, true
	}

	s.mu.Lock()
	s.reconnecting = false
	s.mu.Unlock()

	if s.closing.Load() {
		log.Info("tymux: standing Attach stream reconnect abandoned (deliberate close)", "session_id", sessionID, "pane_id", paneID, "cause", cause)
		return nil, nil, false
	}

	log.Warn("tymux: standing Attach stream reconnect exhausted, giving up", "session_id", sessionID, "pane_id", paneID, "cause", cause, "max_attempts", maxAttempts)
	s.deliverExit(fmt.Sprintf("reconnect failed: tymuxd unreachable after %d attempts", maxAttempts))
	return nil, nil, false
}
