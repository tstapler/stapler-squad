package tmux

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tstapler/stapler-squad/session/lifecycle"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// testLifecycleMetricReader backs the single real MeterProvider installed for
// this test binary. session/lifecycle's package-level instruments are
// constructed against OTel's global delegating meter at package-init time --
// otel.SetMeterProvider rewires what that delegating meter forwards to, so a
// process-lifetime manual reader is the correct fit here, mirroring
// session/lifecycle/observability_test.go's identical pattern.
var testLifecycleMetricReader = sdkmetric.NewManualReader()

func init() {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(testLifecycleMetricReader)))
}

// collectLifecycleMetric returns the named session_lifecycle_* metric's current
// snapshot, or nil if it hasn't been recorded yet.
func collectLifecycleMetric(t *testing.T, name string) *metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := testLifecycleMetricReader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

// sumTmuxControlModeMetric sums an int64 Sum metric's data points labeled
// subsystem="tmux_control_mode", optionally filtered by reason (pass "" to
// sum across all reasons -- session_lifecycle_active_generations carries no
// "reason" attribute at all).
func sumTmuxControlModeMetric(t *testing.T, m *metricdata.Metrics, reason string) int64 {
	t.Helper()
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("unexpected data type %T for %s", m.Data, m.Name)
	}
	var total int64
	for _, dp := range sum.DataPoints {
		sv, ok := dp.Attributes.Value(attribute.Key("subsystem"))
		if !ok || sv.AsString() != "tmux_control_mode" {
			continue
		}
		if reason != "" {
			rv, ok := dp.Attributes.Value(attribute.Key("reason"))
			if !ok || rv.AsString() != reason {
				continue
			}
		}
		total += dp.Value
	}
	return total
}

// exitRecorder is a concurrency-safe onExit spy: readControlModeOutput's
// reader goroutine invokes it, while tests assert against it from the main
// goroutine.
type exitRecorder struct {
	mu      sync.Mutex
	reasons []string
}

func (r *exitRecorder) record(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reasons = append(r.reasons, reason)
}

func (r *exitRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reasons)
}

func (r *exitRecorder) last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reasons) == 0 {
		return ""
	}
	return r.reasons[len(r.reasons)-1]
}

// newControlModeOutputTestSession builds a TmuxSession wired to an in-memory
// pipe for controlModeStdout, so readControlModeOutput can be run for real
// against fabricated control-mode notification lines, matching the pipe-backed
// convention in control_mode_dispatch_test.go's newDispatchTestSession (which
// wires controlModeStdin the same way for the write side).
func newControlModeOutputTestSession(t *testing.T) (*TmuxSession, io.WriteCloser, *exitRecorder) {
	t.Helper()
	pr, pw := io.Pipe()
	rec := &exitRecorder{}
	sess := &TmuxSession{
		sanitizedName:          "cm_output_test",
		controlModeStdout:      pr,
		controlModeDone:        make(chan struct{}),
		controlModeSubscribers: make(map[string]chan []byte),
		onExit:                 rec.record,
	}
	t.Cleanup(func() {
		_ = pw.Close()
	})
	return sess, pw, rec
}

// startReader runs sess.readControlModeOutput() in a goroutine and returns a
// channel closed once it returns, so tests can deterministically wait for the
// generation to end instead of polling onExit call counts on a timer.
func startReader(sess *TmuxSession) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		sess.readControlModeOutput()
	}()
	return done
}

// waitReaderDone blocks until readerDone closes or fails the test after
// wait.FastTimeout.
func waitReaderDone(t *testing.T, readerDone <-chan struct{}) {
	t.Helper()
	select {
	case <-readerDone:
	case <-time.After(wait.FastTimeout):
		t.Fatal("readControlModeOutput did not return")
	}
}

// newStoppableControlModeTestSession is newControlModeOutputTestSession plus
// the extra fake-process plumbing (mirroring control_mode_refcount_test.go's
// newRefcountTestSession) needed so a real StopControlMode() call proceeds
// past its "not running" nil-check and actually sets intentionalStop.
func newStoppableControlModeTestSession(t *testing.T) (*TmuxSession, io.WriteCloser, *exitRecorder) {
	t.Helper()
	sess, pw, rec := newControlModeOutputTestSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fakeCmd := exec.CommandContext(ctx, "true") //nolint:norawexec // process is immediately Wait()ed; Kill()/second Wait() on the already-dead process is a harmless no-op, per newRefcountTestSession's identical convention.
	if err := fakeCmd.Start(); err != nil {
		t.Skipf("cannot start 'true': %v", err)
	}
	_ = fakeCmd.Wait()

	sess.controlModeCmd = fakeCmd
	sess.controlModeRefCount = 1
	sess.cmSenderExited = make(chan struct{})
	close(sess.cmSenderExited)

	return sess, pw, rec
}

// TestControlMode_StopControlMode_IntentionalStop_DoesNotFireOnExit is
// Task 3.3.1a (REQ-14): a deliberate StopControlMode() followed by the pipe
// closing (StopControlMode's own teardown closes controlModeStdout) must
// never fire onExit.
func TestControlMode_StopControlMode_IntentionalStop_DoesNotFireOnExit(t *testing.T) {
	count, _ := runStopControlModeIntentionalStopScenario(t)
	if count != 0 {
		t.Errorf("onExit call count = %d, want 0", count)
	}
}

// TestControlMode_ScannerEOF_UnilateralExit_FiresOnExit is Task 3.3.1b
// (REQ-14): the pipe closing without StopControlMode ever being called (no
// %exit line either) must fire onExit exactly once with
// "control-mode-pipe-closed".
func TestControlMode_ScannerEOF_UnilateralExit_FiresOnExit(t *testing.T) {
	count, last := runScannerEOFUnilateralScenario(t)
	if count != 1 {
		t.Errorf("onExit call count = %d, want 1", count)
	}
	if last != "control-mode-pipe-closed" {
		t.Errorf("onExit reason = %q, want %q", last, "control-mode-pipe-closed")
	}
}

// TestControlMode_PercentExit_UnilateralExit_FiresOnExitExactlyOnce is Task
// 3.3.1c (REQ-14): a %exit notification immediately followed by scanner EOF
// must fire onExit exactly once (not twice), guarding the 3-call-site-drift
// class of bug this migration exists to close.
func TestControlMode_PercentExit_UnilateralExit_FiresOnExitExactlyOnce(t *testing.T) {
	count, last := runPercentExitUnilateralScenario(t)
	if count != 1 {
		t.Errorf("onExit call count = %d, want exactly 1", count)
	}
	if last != "control-mode-%exit" {
		t.Errorf("onExit reason = %q, want %q", last, "control-mode-%exit")
	}
}

// runStopControlModeIntentionalStopScenario drives Task 3.3.1a's scenario and
// returns the resulting onExit call count and last reason (empty if never
// called). The reader goroutine is started first and synchronized past its
// controlModeDone capture via a matched io.Pipe write/read rendezvous before
// StopControlMode runs concurrently -- avoiding a pre-existing (out-of-scope)
// nil-channel race in readControlModeOutput's post-loop cleanup that would
// otherwise depend on unspecified goroutine scheduling order.
func runStopControlModeIntentionalStopScenario(t *testing.T) (count int, lastReason string) {
	t.Helper()
	sess, pw, rec := newStoppableControlModeTestSession(t)
	readerDone := startReader(sess)

	// io.Pipe's Write blocks until a matching Read fully consumes it, so this
	// call returning is proof the reader goroutine already performed its
	// first Read -- meaning it has already captured controlModeDone (which
	// happens strictly before the scanner is even constructed) and cannot
	// race StopControlMode's concurrent write to that same field below.
	syncWriteControlModeLine(t, pw, "%session-changed $0 sync")

	if err := sess.StopControlMode(); err != nil {
		t.Fatalf("StopControlMode: %v", err)
	}

	waitReaderDone(t, readerDone)
	return rec.count(), rec.last()
}

// runScannerEOFUnilateralScenario drives Task 3.3.1b's scenario: the pipe
// closes (simulating the tmux process being killed) with no preceding %exit
// and no StopControlMode call.
func runScannerEOFUnilateralScenario(t *testing.T) (count int, lastReason string) {
	t.Helper()
	sess, pw, rec := newControlModeOutputTestSession(t)
	readerDone := startReader(sess)

	if err := pw.Close(); err != nil {
		t.Fatalf("closing pipe: %v", err)
	}

	waitReaderDone(t, readerDone)
	return rec.count(), rec.last()
}

// runPercentExitUnilateralScenario drives Task 3.3.1c's scenario: a %exit
// notification arrives, then the pipe closes (scanner EOF) with no
// StopControlMode call -- exercising both the %exit handler and the
// scanner-EOF fallback for the same generation.
func runPercentExitUnilateralScenario(t *testing.T) (count int, lastReason string) {
	t.Helper()
	sess, pw, rec := newControlModeOutputTestSession(t)
	readerDone := startReader(sess)

	syncWriteControlModeLine(t, pw, "%exit")
	if err := pw.Close(); err != nil {
		t.Fatalf("closing pipe: %v", err)
	}

	waitReaderDone(t, readerDone)
	return rec.count(), rec.last()
}

// syncWriteControlModeLine writes line+"\n" to pw, failing the test if the
// write doesn't complete within wait.FastTimeout (the reader goroutine must
// be alive and reading for it to return at all, per io.Pipe's synchronous,
// unbuffered rendezvous semantics).
func syncWriteControlModeLine(t *testing.T, pw io.WriteCloser, line string) {
	t.Helper()
	writeErr := make(chan error, 1)
	go func() {
		_, err := fmt.Fprintf(pw, "%s\n", line)
		writeErr <- err
	}()
	select {
	case err := <-writeErr:
		if err != nil {
			t.Fatalf("writing control-mode line %q: %v", line, err)
		}
	case <-time.After(wait.FastTimeout):
		t.Fatalf("write of control-mode line %q did not complete -- reader not consuming", line)
	}
}

// TestControlMode_LifecycleV2FlagUnset_PreservesLegacyBehavior is REQ-15: all
// 3 of Epic 3.3's scenarios, run with STAPLER_SQUAD_TMUX_LIFECYCLE_V2 unset,
// asserting the exact pre-migration onExit call-count/argument behavior.
func TestControlMode_LifecycleV2FlagUnset_PreservesLegacyBehavior(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TMUX_LIFECYCLE_V2", "")
	assertLifecycleScenarios(t)
}

// TestControlMode_LifecycleV2FlagEnabled_MatchesLegacyOnExitBehavior is
// REQ-15's flag-on counterpart: the same 3 scenarios with
// STAPLER_SQUAD_TMUX_LIFECYCLE_V2=true must produce byte-for-byte identical
// onExit outcomes to the flag-off case.
func TestControlMode_LifecycleV2FlagEnabled_MatchesLegacyOnExitBehavior(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TMUX_LIFECYCLE_V2", "true")
	assertLifecycleScenarios(t)
}

// assertLifecycleScenarios runs Epic 3.3's 3 scenarios under whatever
// STAPLER_SQUAD_TMUX_LIFECYCLE_V2 value the caller has set via t.Setenv,
// asserting the same onExit call-count/argument outcomes regardless of flag
// state -- the parity proof Story 3.1.2 and REQ-15 require.
func assertLifecycleScenarios(t *testing.T) {
	t.Helper()

	t.Run("StopControlMode_IntentionalStop_DoesNotFireOnExit", func(t *testing.T) {
		count, _ := runStopControlModeIntentionalStopScenario(t)
		if count != 0 {
			t.Errorf("onExit call count = %d, want 0", count)
		}
	})

	t.Run("ScannerEOF_UnilateralExit_FiresOnExit", func(t *testing.T) {
		count, last := runScannerEOFUnilateralScenario(t)
		if count != 1 {
			t.Errorf("onExit call count = %d, want 1", count)
		}
		if last != "control-mode-pipe-closed" {
			t.Errorf("onExit reason = %q, want %q", last, "control-mode-pipe-closed")
		}
	})

	t.Run("PercentExit_UnilateralExit_FiresOnExitExactlyOnce", func(t *testing.T) {
		count, last := runPercentExitUnilateralScenario(t)
		if count != 1 {
			t.Errorf("onExit call count = %d, want exactly 1", count)
		}
		if last != "control-mode-%exit" {
			t.Errorf("onExit reason = %q, want %q", last, "control-mode-%exit")
		}
	})
}

// TestReadControlModeOutput_LifecycleV2Enabled_EndsGenerationExactlyOnceAcrossThreeCallSites
// is REQ-16: regardless of which of the 3 onExitOnce-guarded sites reaches it
// first, session_lifecycle_ends_total{subsystem="tmux_control_mode"} must
// increment by exactly 1 per generation -- never 0 (missed) or 2+ (double
// counted, the Architecture Review Blocker this migration fixes).
func TestReadControlModeOutput_LifecycleV2Enabled_EndsGenerationExactlyOnceAcrossThreeCallSites(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TMUX_LIFECYCLE_V2", "true")

	// Each case drives one generation to its end via a different one of the 3
	// call sites (or, for %exit/%session-closed, that site racing the
	// scanner-EOF fallback within the same reader goroutine) and asserts the
	// counter moved by exactly 1 -- never 0 (missed) or 2+ (double counted).
	cases := []struct {
		name  string
		lines []string // control-mode lines to write before closing the pipe; nil means none
	}{
		{name: "scanner-EOF fallback only", lines: nil},
		{name: "%exit handler racing scanner-EOF", lines: []string{"%exit"}},
		{name: "%session-closed handler racing scanner-EOF", lines: []string{"%session-closed"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Every case here ends its generation with no StartControlMode/
			// StopControlMode ever setting intentionalStop, so
			// classifyControlModeExit() always reports ReasonTransportDrop
			// ("transport_drop") for these scenarios -- filtering on that specific
			// reason (rather than "") also verifies the recorded label is correct,
			// not just that the counter moved.
			const wantReason = "transport_drop"
			before := sumTmuxControlModeMetric(t, collectLifecycleMetric(t, "session_lifecycle_ends_total"), wantReason)

			sess, pw, _ := newControlModeOutputTestSession(t)
			readerDone := startReader(sess)
			for _, line := range c.lines {
				syncWriteControlModeLine(t, pw, line)
			}
			if err := pw.Close(); err != nil {
				t.Fatalf("closing pipe: %v", err)
			}
			waitReaderDone(t, readerDone)

			after := sumTmuxControlModeMetric(t, collectLifecycleMetric(t, "session_lifecycle_ends_total"), wantReason)
			if after != before+1 {
				t.Errorf("session_lifecycle_ends_total{subsystem=tmux_control_mode,reason=%s} delta = %d, want exactly 1", wantReason, after-before)
			}
		})
	}
}

// TestClassifyControlModeExit_BothOrigins is a direct table-driven unit test of
// classifyControlModeExit(), covering both intentionalStop origins. Every existing
// test that reaches this function via readControlModeOutput does so with
// intentionalStop==false; the one scenario that does set it true
// (StopControlMode's own doneCh branch) classifies inline without calling this
// function at all, so the intentionalStop==true branch (-> ReasonDeliberateClose)
// had no direct coverage before this test.
func TestClassifyControlModeExit_BothOrigins(t *testing.T) {
	tests := []struct {
		name            string
		intentionalStop bool
		want            lifecycle.Reason
	}{
		{"intentional stop set", true, lifecycle.ReasonDeliberateClose},
		{"intentional stop unset", false, lifecycle.ReasonTransportDrop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := &TmuxSession{}
			sess.intentionalStop.Store(tt.intentionalStop)
			got := sess.classifyControlModeExit()
			if got != tt.want {
				t.Errorf("classifyControlModeExit() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestReadControlModeOutput_GoroutineNeverReachesExitSites_ActiveGenerationsGaugeStaysElevated
// is REQ-16 (mirrors Story 1.3.3 for this subsystem): a generation whose
// goroutine never reaches any of the 3 exit call sites (wedged/abandoned)
// must leave session_lifecycle_active_generations{subsystem="tmux_control_mode"}
// still counting it as active -- this is the intended signal, not a gap.
func TestReadControlModeOutput_GoroutineNeverReachesExitSites_ActiveGenerationsGaugeStaysElevated(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TMUX_LIFECYCLE_V2", "true")

	const gaugeName = "session_lifecycle_active_generations"
	baseline := sumTmuxControlModeMetric(t, collectLifecycleMetric(t, gaugeName), "")

	// Deliberately never write anything and never close the pipe: the reader
	// goroutine blocks forever in scanner.Scan(), simulating a wedged/
	// abandoned generation that never reaches any of the 3 exit sites.
	// t.Cleanup (registered by newControlModeOutputTestSession) closes the
	// pipe at the end of this test so the leaked goroutine can finally exit
	// instead of leaking for the rest of the test binary's run.
	sess, pw, _ := newControlModeOutputTestSession(t)
	readerDone := startReader(sess)

	if err := wait.WaitForCondition(func() bool {
		after := sumTmuxControlModeMetric(t, collectLifecycleMetric(t, gaugeName), "")
		return after == baseline+1
	}, wait.WaitConfig{Timeout: wait.FastTimeout, PollInterval: 5 * time.Millisecond, Description: "gauge reflects the started generation"}); err != nil {
		t.Fatalf("gauge never reflected the started generation: %v", err)
	}

	// Give the (intentionally wedged) goroutine a moment to prove it does
	// NOT end the generation on its own -- the gauge must stay elevated for
	// as long as the goroutine never reaches an exit site.
	time.Sleep(20 * time.Millisecond)
	after := sumTmuxControlModeMetric(t, collectLifecycleMetric(t, gaugeName), "")
	if after != baseline+1 {
		t.Errorf("gauge = baseline+%d, want baseline+1 (still elevated for the abandoned generation)", after-baseline)
	}

	// Unblock the wedged goroutine and join it within this test's own body,
	// rather than leaving it to t.Cleanup's unordered pipe close -- otherwise
	// its delayed endControlModeGenerationV2 decrement of this same
	// process-global gauge can land after this test returns and leak into the
	// NEXT test's before/after delta assertion on the identical gauge.
	if err := pw.Close(); err != nil {
		t.Fatalf("closing pipe: %v", err)
	}
	waitReaderDone(t, readerDone)
}

// closeDoneOnReadReader is a custom io.ReadCloser whose first Read call closes
// doneCh from *inside* the call, before returning line's bytes. That
// establishes a happens-before between doneCh's close and scanner.Scan()
// returning true for that line, deterministically reproducing (rather than
// racing the scheduler for) the scan loop's doneCh branch: a control-mode
// line arriving in the narrow window between StopControlMode() closing
// doneCh and it closing controlModeStdout. Any Read after the first blocks on
// block until Close is called, simulating the pipe staying open a little
// longer -- the fix path must return without needing a second Scan().
type closeDoneOnReadReader struct {
	line    []byte
	doneCh  chan struct{}
	readOne bool
	block   chan struct{}
}

func (r *closeDoneOnReadReader) Read(p []byte) (int, error) {
	if r.readOne {
		<-r.block
		return 0, io.EOF
	}
	r.readOne = true
	close(r.doneCh)
	return copy(p, r.line), nil
}

func (r *closeDoneOnReadReader) Close() error {
	select {
	case <-r.block:
	default:
		close(r.block)
	}
	return nil
}

// TestControlMode_ScanLoopDoneChRace_EndsGenerationWithoutFiringOnExit is the
// regression test for the readControlModeOutput doneCh-return leak: a
// control-mode line is read successfully exactly in the narrow window between
// StopControlMode() closing doneCh and it closing controlModeStdout. Before
// the fix, the scan loop's `case <-doneCh: return` branch returned without
// ever reaching the post-loop cleanup, so with the v2 lifecycle flag enabled
// the span opened at the top of readControlModeOutput was never .End()'d and
// session_lifecycle_active_generations{subsystem="tmux_control_mode"} was
// never decremented -- a false-positive "stuck generation" signal for an
// entirely ordinary shutdown race, not a real wedge. onExit must still never
// fire here (deliberate close), matching every other StopControlMode path.
func TestControlMode_ScanLoopDoneChRace_EndsGenerationWithoutFiringOnExit(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TMUX_LIFECYCLE_V2", "true")

	beforeEnds := sumTmuxControlModeMetric(t, collectLifecycleMetric(t, "session_lifecycle_ends_total"), "deliberate_close")
	beforeActive := sumTmuxControlModeMetric(t, collectLifecycleMetric(t, "session_lifecycle_active_generations"), "")

	rec := &exitRecorder{}
	doneCh := make(chan struct{})
	reader := &closeDoneOnReadReader{
		line:   []byte("%session-changed $0 sync\n"),
		doneCh: doneCh,
		block:  make(chan struct{}),
	}
	sess := &TmuxSession{
		sanitizedName:          "cm_donech_race_test",
		controlModeStdout:      reader,
		controlModeDone:        doneCh,
		controlModeSubscribers: make(map[string]chan []byte),
		onExit:                 rec.record,
	}
	t.Cleanup(func() { _ = reader.Close() })

	readerDone := startReader(sess)
	waitReaderDone(t, readerDone)

	if count := rec.count(); count != 0 {
		t.Errorf("onExit call count = %d, want 0 (deliberate close never fires onExit)", count)
	}

	afterEnds := sumTmuxControlModeMetric(t, collectLifecycleMetric(t, "session_lifecycle_ends_total"), "deliberate_close")
	if afterEnds != beforeEnds+1 {
		t.Errorf("session_lifecycle_ends_total{reason=deliberate_close} delta = %d, want 1 (generation never ended -- the leak this test guards against)", afterEnds-beforeEnds)
	}

	afterActive := sumTmuxControlModeMetric(t, collectLifecycleMetric(t, "session_lifecycle_active_generations"), "")
	if afterActive != beforeActive {
		t.Errorf("session_lifecycle_active_generations delta = %d, want 0 (gauge must return to baseline, not stay elevated by the leaked generation)", afterActive-beforeActive)
	}
}
