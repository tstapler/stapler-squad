package session

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/detection/ratelimit"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/headless"
)

// scriptedPool replays steps in order; the last step repeats. A step with err
// set returns that error, a step with wait blocks until ctx is done.
type scriptedPool struct {
	steps []poolStep
	calls atomic.Int32
}

type poolStep struct {
	resp  string
	err   error
	wait  bool
	panic bool
}

func (p *scriptedPool) CallBlocking(ctx context.Context, _ headless.FeatureKey, _, _ string, _ headless.CallOptions, _ headless.CostSink) (string, error) {
	idx := int(p.calls.Add(1)) - 1
	if idx >= len(p.steps) {
		idx = len(p.steps) - 1
	}
	step := p.steps[idx]
	switch {
	case step.panic:
		panic("scripted pool panic")
	case step.wait:
		<-ctx.Done()
		return "", ctx.Err()
	}
	return step.resp, step.err
}

// fakeStopSource is a stopSource returning fixed values.
type fakeStopSource struct {
	status     detection.DetectedStatus
	statusCtx  string
	tmuxAlive  bool
	ctrlActive bool
	paneDead   bool
	exitCode   int
	tail       string
	tailPanics bool
	tailHangs  bool
	tailCalls  atomic.Int32
}

func (f *fakeStopSource) Status() (detection.DetectedStatus, string) { return f.status, f.statusCtx }
func (f *fakeStopSource) LastActivity() time.Time                    { return time.Unix(1700000000, 0) }
func (f *fakeStopSource) TmuxAlive() bool                            { return f.tmuxAlive }
func (f *fakeStopSource) ControllerActive() bool                     { return f.ctrlActive }
func (f *fakeStopSource) PaneExit() (bool, int)                      { return f.paneDead, f.exitCode }
func (f *fakeStopSource) Tail(ctx context.Context) (string, error) {
	f.tailCalls.Add(1)
	switch {
	case f.tailPanics:
		panic("tail exploded")
	case f.tailHangs:
		<-ctx.Done()
		return "", ctx.Err()
	}
	return f.tail, nil
}

type stopHarness struct {
	driver   *AutonomousDriver
	cc       *ClaudeController
	inst     *Instance
	pm       *fakeSendKeysProcessManager
	outcomes chan AutonomousDriverOutcome
	ctx      context.Context
	cancel   context.CancelFunc
}

func newStopHarness(t *testing.T, pool HeadlessPoolClient, maxTurns int, src stopSource) *stopHarness {
	t.Helper()
	inst := &Instance{Title: "stop-harness", UUID: "abcdefgh-stop-harness"}
	cc, _ := NewClaudeController(inst)
	inst.controllerManager.controller.Store(cc)
	pm := &fakeSendKeysProcessManager{}
	inst.processManager = pm
	inst.started.Store(true)

	d := &AutonomousDriver{
		inst:                   inst,
		controller:             cc,
		headlessPool:           pool,
		goal:                   "goal",
		maxTurns:               maxTurns,
		idleSettlePollInterval: testIdleSettlePollInterval,
		idleSettleWindow:       testIdleSettleWindow,
		paneSettlePollInterval: testPaneSettlePollInterval,
		paneSettleMaxWait:      testPaneSettleMaxWait,
		startupTimeout:         10 * time.Second,
		captureTimeout:         50 * time.Millisecond,
		source:                 src,
	}
	h := &stopHarness{driver: d, cc: cc, inst: inst, pm: pm, outcomes: make(chan AutonomousDriverOutcome, 4)}
	d.RegisterCompletionCallback(func(_ string, o AutonomousDriverOutcome) { h.outcomes <- o })
	h.ctx, h.cancel = context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(func() {
		d.Stop()
		d.Wait(context.Background())
		h.cancel()
	})
	return h
}

func (h *stopHarness) startWithIdle(t *testing.T) {
	t.Helper()
	go pumpIdleSignals(h.ctx, h.cc)
	if err := h.driver.Start(h.ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func (h *stopHarness) outcome(t *testing.T) AutonomousDriverOutcome {
	t.Helper()
	select {
	case o := <-h.outcomes:
		return o
	case <-h.ctx.Done():
		t.Fatal("timed out waiting for the completion callback")
		return AutonomousDriverOutcome{}
	}
}

func (h *stopHarness) noSecondCallback(t *testing.T) {
	t.Helper()
	h.driver.Stop()
	h.driver.Wait(h.ctx)
	select {
	case o := <-h.outcomes:
		t.Fatalf("completion callback fired twice; second outcome %+v", o)
	default:
	}
}

func TestAutonomousDriver_StopReasonPerExit(t *testing.T) {
	idleSrc := &fakeStopSource{status: detection.StatusIdle, tmuxAlive: true, ctrlActive: true}

	t.Run("done", func(t *testing.T) {
		h := newStopHarness(t, &scriptedPool{steps: []poolStep{{resp: "DONE: finished"}}}, 5, idleSrc)
		h.startWithIdle(t)
		o := h.outcome(t)
		if o.StopReason != domain.StopReasonDone || !o.Done || o.Stuck || o.Turns != 1 {
			t.Errorf("got %+v", o)
		}
		if o.Class() != domain.OutcomeClassDone {
			t.Errorf("class = %s", o.Class())
		}
		h.noSecondCallback(t)
	})

	t.Run("rate limit wait exhausted", func(t *testing.T) {
		pool := &scriptedPool{steps: []poolStep{{resp: "DONE: x"}}}
		h := newStopHarness(t, pool, 3, idleSrc)
		m := ratelimit.NewManager("rl", nil)
		m.GetDetector().SetState(ratelimit.StateWaiting)
		h.cc.rateLimitHandler.Store(ratelimit.NewPTYConsumer(nil, m))
		h.driver.rateLimitWaitMax = 20 * time.Millisecond
		h.startWithIdle(t)
		o := h.outcome(t)
		if o.StopReason != domain.StopReasonRateLimitExhausted || o.Turns != 1 || !o.Stuck || o.Done {
			t.Errorf("got %+v", o)
		}
		if pool.calls.Load() != 0 {
			t.Errorf("LLM called %d times while rate limited", pool.calls.Load())
		}
	})

	t.Run("max turns", func(t *testing.T) {
		h := newStopHarness(t, &scriptedPool{steps: []poolStep{{resp: "WAIT: working"}}}, 2, idleSrc)
		h.startWithIdle(t)
		o := h.outcome(t)
		if o.StopReason != domain.StopReasonMaxTurns || o.Turns != 2 || !o.Stuck {
			t.Errorf("got %+v", o)
		}
	})

	t.Run("orchestrator error reports the failing turn, not maxTurns", func(t *testing.T) {
		pool := &scriptedPool{steps: []poolStep{
			{resp: "WAIT: working"},
			{err: errors.New("llm boom")},
		}}
		h := newStopHarness(t, pool, 5, idleSrc)
		h.startWithIdle(t)
		o := h.outcome(t)
		if o.StopReason != domain.StopReasonOrchestratorError || o.Turns != 2 || !o.Stuck {
			t.Errorf("got %+v", o)
		}
	})

	t.Run("submit failure", func(t *testing.T) {
		h := newStopHarness(t, &scriptedPool{steps: []poolStep{{resp: "NEXT_MESSAGE: hello"}}}, 3, idleSrc)
		h.pm.failOn = map[int]bool{1: true, 2: true, 3: true, 4: true}
		h.startWithIdle(t)
		o := h.outcome(t)
		if o.StopReason != domain.StopReasonSubmitFailed || o.Turns != 1 {
			t.Errorf("got %+v", o)
		}
	})

	t.Run("lease timeout", func(t *testing.T) {
		h := newStopHarness(t, &scriptedPool{steps: []poolStep{{resp: "NEXT_MESSAGE: hello"}}}, 3, idleSrc)
		h.driver.turnLeaseWait = 20 * time.Millisecond
		held, err := h.inst.AcquireTerminalWriteLease(h.ctx, LeaseWriterSteer, time.Second)
		if err != nil {
			t.Fatalf("acquire: %v", err)
		}
		defer held.Release()
		h.startWithIdle(t)
		o := h.outcome(t)
		if o.StopReason != domain.StopReasonLeaseTimeout || o.Turns != 1 {
			t.Errorf("got %+v", o)
		}
	})

	t.Run("startup timeout", func(t *testing.T) {
		h := newStopHarness(t, &scriptedPool{steps: []poolStep{{resp: "DONE: x"}}}, 3, idleSrc)
		h.driver.startupTimeout = 30 * time.Millisecond
		if err := h.driver.Start(h.ctx); err != nil { // no idle pump: never idle
			t.Fatal(err)
		}
		o := h.outcome(t)
		if o.StopReason != domain.StopReasonStartupTimeout || o.Turns != 0 || !o.Stuck {
			t.Errorf("got %+v", o)
		}
	})

	t.Run("Stop during startup is cancelled_external", func(t *testing.T) {
		src := &fakeStopSource{status: detection.StatusIdle, tmuxAlive: true, ctrlActive: true}
		h := newStopHarness(t, &scriptedPool{steps: []poolStep{{resp: "DONE: x"}}}, 3, src)
		if err := h.driver.Start(h.ctx); err != nil {
			t.Fatal(err)
		}
		h.driver.Stop()
		o := h.outcome(t)
		if o.StopReason != domain.StopReasonCancelledExternal || o.Stuck || o.Done {
			t.Errorf("got %+v", o)
		}
		if o.Snapshot.CaptureErr != "skipped" || src.tailCalls.Load() != 0 {
			t.Errorf("cancelled stop must not capture; snapshot %+v", o.Snapshot)
		}
	})

	t.Run("parent ctx cancel during startup is cancelled_shutdown", func(t *testing.T) {
		h := newStopHarness(t, &scriptedPool{steps: []poolStep{{resp: "DONE: x"}}}, 3, idleSrc)
		parent, cancel := context.WithCancel(h.ctx)
		if err := h.driver.Start(parent); err != nil {
			t.Fatal(err)
		}
		cancel()
		o := h.outcome(t)
		if o.StopReason != domain.StopReasonCancelledShutdown || o.Stuck {
			t.Errorf("got %+v", o)
		}
	})

	t.Run("LLM error caused by Stop is cancelled_external not orchestrator_error", func(t *testing.T) {
		pool := &scriptedPool{steps: []poolStep{{wait: true}}}
		h := newStopHarness(t, pool, 3, idleSrc)
		h.startWithIdle(t)
		for pool.calls.Load() == 0 {
			select {
			case <-h.ctx.Done():
				t.Fatal("pool never called")
			case <-time.After(time.Millisecond):
			}
		}
		h.driver.Stop()
		o := h.outcome(t)
		if o.StopReason != domain.StopReasonCancelledExternal || o.Turns != 1 {
			t.Errorf("got %+v", o)
		}
	})

	t.Run("panic fires exactly one completion", func(t *testing.T) {
		h := newStopHarness(t, &scriptedPool{steps: []poolStep{{panic: true}}}, 3, idleSrc)
		h.startWithIdle(t)
		o := h.outcome(t)
		if o.StopReason != domain.StopReasonPanic || !o.Stuck {
			t.Errorf("got %+v", o)
		}
		if !h.driver.Wait(h.ctx) {
			t.Error("Wait returned false after panic")
		}
		h.noSecondCallback(t)
	})
}

func TestAutonomousDriver_SnapshotCapturedBeforeCallback(t *testing.T) {
	src := &fakeStopSource{
		status: detection.StatusIdle, statusCtx: "Claude Code readline input prompt",
		tmuxAlive: true, ctrlActive: true, tail: "\x1b[32mhello\x1b[0m\n> ",
	}
	h := newStopHarness(t, &scriptedPool{steps: []poolStep{{err: errors.New("boom")}}}, 3, src)
	h.startWithIdle(t)
	o := h.outcome(t)

	s := o.Snapshot
	if s.CaptureErr != "" {
		t.Fatalf("CaptureErr = %q", s.CaptureErr)
	}
	if s.Bucket != domain.BucketSafeIdle || !s.TmuxAlive || !s.ControllerActive {
		t.Errorf("snapshot %+v", s)
	}
	if s.PaneTail != "hello\n>" {
		t.Errorf("PaneTail = %q", s.PaneTail)
	}
	if got := domain.ClassifyObserved(s); got != domain.ObservedIdlePrompt {
		t.Errorf("observed = %s", got)
	}
}

func TestAutonomousDriver_SnapshotFailureNeverBlocksCompletion(t *testing.T) {
	cases := map[string]*fakeStopSource{
		"tail panics": {tmuxAlive: true, ctrlActive: true, tailPanics: true},
		"tail hangs":  {tmuxAlive: true, ctrlActive: true, tailHangs: true},
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			h := newStopHarness(t, &scriptedPool{steps: []poolStep{{err: errors.New("boom")}}}, 3, src)
			h.startWithIdle(t)
			o := h.outcome(t)
			if o.StopReason != domain.StopReasonOrchestratorError {
				t.Errorf("reason = %s", o.StopReason)
			}
			if o.Snapshot.CaptureErr == "" {
				t.Errorf("CaptureErr empty; snapshot %+v", o.Snapshot)
			}
			if got := domain.ClassifyObserved(o.Snapshot); got == domain.ObservedExited {
				t.Errorf("failed capture classified as %s, want unknown", got)
			}
		})
	}
}

func TestAutonomousDriver_FinishedAndRunID(t *testing.T) {
	src := &fakeStopSource{tmuxAlive: true, ctrlActive: true}
	h := newStopHarness(t, &scriptedPool{steps: []poolStep{{resp: "DONE: x"}}}, 3, src)
	if h.driver.Finished() {
		t.Error("Finished() true before Start")
	}
	if h.driver.RunID() != 0 {
		t.Errorf("RunID before Start = %d", h.driver.RunID())
	}
	h.startWithIdle(t)
	o := h.outcome(t)
	h.driver.Wait(h.ctx)
	if !h.driver.Finished() {
		t.Error("Finished() false after exit")
	}
	if o.RunID == 0 || o.RunID != h.driver.RunID() {
		t.Errorf("outcome RunID %d vs driver %d", o.RunID, h.driver.RunID())
	}
}

func TestAutonomousDriverOutcome_NormalizedStopReason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   AutonomousDriverOutcome
		want domain.StopReason
		cls  domain.OutcomeClass
	}{
		{"legacy done", AutonomousDriverOutcome{Done: true}, domain.StopReasonDone, domain.OutcomeClassDone},
		{"legacy stuck", AutonomousDriverOutcome{Stuck: true}, "", domain.OutcomeClassStall},
		{"typed wins over legacy flag", AutonomousDriverOutcome{Done: true, StopReason: domain.StopReasonCancelledExternal}, domain.StopReasonCancelledExternal, domain.OutcomeClassCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.in.NormalizedStopReason(); got != tc.want {
				t.Errorf("reason = %q, want %q", got, tc.want)
			}
			if got := tc.in.Class(); got != tc.cls {
				t.Errorf("class = %q, want %q", got, tc.cls)
			}
		})
	}
}

func TestBucketFor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status detection.DetectedStatus
		ctx    string
		want   domain.StatusBucket
	}{
		{"claude prompt is safe idle", detection.StatusIdle, "Claude Code readline input prompt", domain.BucketSafeIdle},
		{"shell prompt idle is not safe", detection.StatusIdle, "command prompt", domain.BucketIdle},
		{"ready is dead code, not safe", detection.StatusReady, "", domain.BucketIdle},
		{"approval", detection.StatusNeedsApproval, "", domain.BucketNeedsInput},
		{"input required", detection.StatusInputRequired, "", domain.BucketNeedsInput},
		{"error", detection.StatusError, "", domain.BucketError},
		{"executing", detection.StatusExecuting, "", domain.BucketActive},
		{"unknown", detection.StatusUnknown, "", domain.BucketUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := bucketFor(tc.status, tc.ctx); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}
