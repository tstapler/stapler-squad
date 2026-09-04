package streamhub_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/streamhub"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// stormSessionCount is the number of real tmux sessions Story 3.3.3's storm
// test spins up — plan.md's AC requires "at least 5".
const stormSessionCount = 5

// tmuxSessionController adapts a real *tmux.TmuxSession to
// streamhub.SessionController, the same six-method structural contract
// *session.Instance satisfies in production (Task 1.3.2a). This test
// deliberately uses *tmux.TmuxSession directly rather than *session.Instance
// (which additionally drags in git worktree/session-lifecycle machinery
// unrelated to what this test is proving) — package session/tmux exposes
// every method the interface needs directly, and package session/streamhub
// never imports package session (ADR-003), so this adapter is what lets the
// storm test exercise real tmux control-mode plumbing without either
// package session or a fake SessionController (pre-mortem P1 #1).
type tmuxSessionController struct {
	session *tmux.TmuxSession
}

func (c *tmuxSessionController) SetWindowSizeContext(_ context.Context, cols, rows int) error {
	return c.session.SetWindowSize(cols, rows)
}

// ResizePTY mirrors SetWindowSize: *tmux.TmuxSession has no separate
// ResizePTY method (that nudge lives one layer up, in
// *session.Instance.ResizePTY), and the hub's own resize pipeline never
// calls ResizePTY (see SessionController's doc comment) — only
// applyNegotiatedSize's SetWindowSize call is load-bearing for this test.
func (c *tmuxSessionController) ResizePTY(cols, rows int) error {
	return c.session.SetWindowSize(cols, rows)
}

func (c *tmuxSessionController) CapturePaneContentRawContext(_ context.Context) (streamhub.RawPaneContent, error) {
	content, err := c.session.CapturePaneContentRaw()
	return streamhub.RawPaneContent(content), err
}

func (c *tmuxSessionController) GetPaneCursorPosition() (x, y int, err error) {
	return c.session.GetCursorPosition()
}

func (c *tmuxSessionController) StartControlMode() error {
	return c.session.StartControlMode()
}

func (c *tmuxSessionController) StopControlMode() error {
	return c.session.StopControlMode()
}

func (c *tmuxSessionController) SubscribeControlModeUpdates() (string, <-chan []byte) {
	id, ch := c.session.SubscribeToControlModeUpdates()
	return id, ch
}

func (c *tmuxSessionController) UnsubscribeControlModeUpdates(id string) {
	c.session.UnsubscribeFromControlModeUpdates(id)
}

// reconnectStormSession bundles one real tmux session's storm-test plumbing:
// the real tmux process, its SessionController adapter, its StreamHub, and
// the currently-attached subscriber's ID (so the storm phase can detach it).
type reconnectStormSession struct {
	name             string
	tmuxSession      *tmux.TmuxSession
	hub              *streamhub.StreamHub
	subscriberID     streamhub.SubscriberID
	initialTransport *streamhub.MemoryTransport
}

// newReconnectStormSession creates and starts one real tmux session, wires
// it into a real StreamHub via the same ownership-resolution and
// pump-wiring sequence production's HubRegistry.GetOrCreate /
// pumpControlModeOutputIntoHub use (server/services/connectrpc_websocket.go),
// and attaches one initial subscriber standing in for "the browser tab that
// was open before the restart."
func newReconnectStormSession(t *testing.T, index int) *reconnectStormSession {
	t.Helper()

	name := fmt.Sprintf("reconnect-storm-%d-%d-%d", os.Getpid(), time.Now().UnixNano(), index)
	tmuxSession, cleanup := tmux.NewTmuxSessionWithPrefixAndCleanup(name, "sleep 60", "staplersquad_test_")
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Logf("reconnect storm cleanup for %s: %v", name, err)
		}
	})

	if err := tmuxSession.Start(t.TempDir()); err != nil {
		t.Fatalf("failed to start real tmux session %s: %v", name, err)
	}
	if err := tmuxSession.StartControlMode(); err != nil {
		t.Fatalf("failed to start control mode for %s: %v", name, err)
	}
	t.Cleanup(func() { _ = tmuxSession.StopControlMode() })

	controller := &tmuxSessionController{session: tmuxSession}

	// Resolve ownership exactly like production's HubRegistry.GetOrCreate:
	// acquire the sticky per-session lock and assert this session intends
	// PathHubOwned, then run the real OverlapInvariant defense-in-depth
	// check at the real production call site (Epic 3.2).
	if _, err := streamhub.AcquireOwnershipLock(name).ResolveExpecting(true, streamhub.PathHubOwned); err != nil {
		t.Fatalf("failed to resolve PathHubOwned for %s: %v", name, err)
	}
	hub := streamhub.NewStreamHub(name, controller)
	t.Cleanup(func() { _ = hub.ForceTeardown() })
	streamhub.OverlapInvariant(name, 1)

	// Pump raw control-mode output into the hub exactly like production's
	// pumpControlModeOutputIntoHub — the real feed this hub broadcasts from.
	go func() {
		_, updates := controller.SubscribeControlModeUpdates()
		for data := range updates {
			hub.OnRawOutput(data)
		}
	}()

	transport := streamhub.NewMemoryTransport()
	id := hub.AttachSubscriber(transport, streamhub.SubscriberCapability{CanResize: true, CanWrite: false})
	size, err := streamhub.NewTerminalSize(80, 24)
	if err != nil {
		t.Fatalf("NewTerminalSize: %v", err)
	}
	hub.RequestResize(context.Background(), id, size)

	return &reconnectStormSession{
		name:             name,
		tmuxSession:      tmuxSession,
		hub:              hub,
		subscriberID:     id,
		initialTransport: transport,
	}
}

// TestMultiSessionReconnectStorm_should_NeverViolateOverlapInvariant_When_AllSessionsMassReconnectConcurrently
// is plan.md Story 3.3.3, the real-tmux prerequisite gate resolving
// pre-mortem P1 #1: it spins up stormSessionCount real tmux sessions (via
// tmux.NewTmuxSessionWithPrefixAndCleanup, the same real-session pattern
// session/integration_test.go already uses for two concurrent sessions),
// each with a real StreamHub attached, then simulates a
// make-install-service-style restart by detaching every session's
// subscriber and immediately, concurrently re-attaching a fresh one — no
// serialization — and asserts OverlapInvariant never fires and
// streamhub_overlap_invariant_violations_total stays at 0 across the whole
// storm, under -race. This is the one test in this plan that exercises real
// tmux control-mode plumbing across multiple sessions simultaneously,
// rather than a single session or a fake SessionController.
func TestMultiSessionReconnectStorm_should_NeverViolateOverlapInvariant_When_AllSessionsMassReconnectConcurrently(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real multi-tmux-session reconnect storm test in short mode (Task 3.3.3e)")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	// t.Fatal on the very first OverlapInvariant violation, per plan.md's
	// OverlapInvariant Domain Glossary entry: this test never expects or
	// recovers a panic, since OverlapInvariant no longer panics in the
	// running binary — it emits slog.Error and this hook, both non-fatal by
	// themselves, so the test must itself fail loudly on the first
	// occurrence. t.Fatal from a non-test goroutine only fails that
	// goroutine, so a mutex-guarded flag is recorded and asserted after
	// wg.Wait() as well, to guarantee the failure is observed even if it
	// fires from inside a storm goroutine.
	var violationMu sync.Mutex
	var violations []string
	streamhub.SetOverlapInvariantHookForTest(func(sessionName string, ownerCount int) {
		violationMu.Lock()
		violations = append(violations, fmt.Sprintf("%s (owner_count=%d)", sessionName, ownerCount))
		violationMu.Unlock()
	})
	t.Cleanup(func() { streamhub.SetOverlapInvariantHookForTest(nil) })

	violationsBefore := streamhub.OverlapInvariantViolationsTotal()

	sessions := make([]*reconnectStormSession, 0, stormSessionCount)
	for i := 0; i < stormSessionCount; i++ {
		sessions = append(sessions, newReconnectStormSession(t, i))
	}

	// Let the initial resize/quiescence/capture pipeline settle before the
	// storm, so the storm phase below is measuring the reconnect itself,
	// not overlapping with initial-attach negotiation.
	time.Sleep(200 * time.Millisecond)

	// Simulate the actual failure scenario that motivated this project
	// (Task 3.3.3b): every session's subscriber detaches and a fresh one
	// re-attaches, concurrently and immediately, exactly as every open
	// browser tab reconnecting after a `make install-service` restart
	// would. sync.WaitGroup, no serialization across sessions.
	var wg sync.WaitGroup
	reconnectErrs := make([]error, stormSessionCount)
	for idx, s := range sessions {
		wg.Add(1)
		go func(idx int, s *reconnectStormSession) {
			defer wg.Done()

			s.hub.DetachSubscriber(s.subscriberID)

			freshTransport := streamhub.NewMemoryTransport()
			freshID := s.hub.AttachSubscriber(freshTransport, streamhub.SubscriberCapability{CanResize: true, CanWrite: false})
			size, err := streamhub.NewTerminalSize(100, 30)
			if err != nil {
				reconnectErrs[idx] = fmt.Errorf("session %s: NewTerminalSize: %w", s.name, err)
				return
			}
			s.hub.RequestResize(context.Background(), freshID, size)

			// OverlapInvariant is production-reachable on every hub
			// lookup/creation; re-asserting it here for the reconnecting
			// subscriber's session mirrors HubRegistry.GetOrCreate being
			// called again on reconnect in production.
			streamhub.OverlapInvariant(s.name, 1)
		}(idx, s)
	}
	wg.Wait()

	for idx, err := range reconnectErrs {
		if err != nil {
			t.Errorf("reconnect for session index %d failed: %v", idx, err)
		}
	}

	// Give the post-reconnect resize/quiescence/capture pipeline time to
	// complete for every session before asserting on NegotiatedSize.
	time.Sleep(700 * time.Millisecond)

	wantSize, err := streamhub.NewTerminalSize(100, 30)
	if err != nil {
		t.Fatalf("NewTerminalSize: %v", err)
	}
	for _, s := range sessions {
		if got := s.hub.NegotiatedSize(); got != wantSize {
			t.Errorf("session %s: expected resize→quiescence→capture pipeline to complete for the reconnecting subscriber, NegotiatedSize=%v, want %v", s.name, got, wantSize)
		}
		if s.hub.SubscriberCount() != 1 {
			t.Errorf("session %s: expected exactly 1 subscriber after reconnect storm, got %d", s.name, s.hub.SubscriberCount())
		}
	}

	violationMu.Lock()
	gotViolations := append([]string(nil), violations...)
	violationMu.Unlock()
	if len(gotViolations) > 0 {
		t.Fatalf("OverlapInvariant violated during reconnect storm: %v", gotViolations)
	}

	if got := streamhub.OverlapInvariantViolationsTotal() - violationsBefore; got != 0 {
		t.Fatalf("streamhub_overlap_invariant_violations_total increased by %d during the storm, want 0", got)
	}
}
