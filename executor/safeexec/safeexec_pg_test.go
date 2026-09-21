//go:build !windows

package safeexec

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// syncBuffer wraps bytes.Buffer with a mutex so it's safe to use as slog's
// output while a test polls String(): the escalation path deliberately logs
// from a background time.AfterFunc goroutine, concurrently with the test
// goroutine reading the buffer, which a plain bytes.Buffer does not support.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLogs swaps slog's default logger for one writing to an in-memory
// buffer at the given level, restoring the previous default on cleanup.
// This package's production code (safeexec_pg.go) calls stdlib slog.Warn/Debug
// directly rather than this repo's log package, so it must be captured via the
// real slog.Default() — not log.SetSlogDefaultForTest, which only affects the
// injectable seam read by log.Warn/Info/etc.
func captureLogs(t *testing.T, level slog.Level) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

// captureLogsJSON is captureLogs's JSON-handler counterpart: it lets a test
// parse individual log records and assert on attribute *values* (not just
// key substrings), which a text-format buffer can't do reliably.
func captureLogsJSON(t *testing.T, level slog.Level) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

// findJSONLogRecord scans buf's newline-delimited JSON log records for one
// whose "msg" field contains msgSubstring, returning its fields decoded as a
// map (numbers as float64, per encoding/json's default). Returns nil if no
// matching record is found.
func findJSONLogRecord(t *testing.T, buf *syncBuffer, msgSubstring string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("failed to parse JSON log line %q: %v", line, err)
		}
		if msg, _ := record["msg"].(string); strings.Contains(msg, msgSubstring) {
			return record
		}
	}
	return nil
}

// testMetricReader backs the single real MeterProvider installed for this
// test binary (see init below). OTel's global meter provider delegates to
// whatever is installed via otel.SetMeterProvider exactly once per process
// (internal/global/state.go's delegateMeterOnce) — sigkillEscalations is
// built at package-init time against that delegating global meter (see
// safeexec_metrics.go), so only the *first* SetMeterProvider call in the
// binary actually rewires it; every later call just swaps the global
// pointer without re-delegating already-created instruments. A single
// process-lifetime reader shared by all tests (with delta-based assertions
// in sigkillEscalationsDelta) is the correct fit for that one-shot
// semantics — a fresh reader per test would silently stop observing the
// counter after the first test claims the delegation.
var testMetricReader = sdkmetric.NewManualReader()

func init() {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(testMetricReader)))
}

// sigkillEscalationsSum reads the current cumulative summed value of the
// safeexec.sigkill_escalations counter from testMetricReader.
func sigkillEscalationsSum(t *testing.T) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := testMetricReader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "safeexec.sigkill_escalations" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("unexpected data type %T for %s", m.Data, m.Name)
			}
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}
	return total
}

// sigkillEscalationsDelta returns how much the counter has grown since
// baseline. The reader is process-lifetime and cumulative (see
// testMetricReader's doc comment above), so tests that don't run in
// isolation must diff against their own pre-test baseline rather than
// asserting an absolute value.
func sigkillEscalationsDelta(t *testing.T, baseline int64) int64 {
	t.Helper()
	return sigkillEscalationsSum(t) - baseline
}

// withSigkillGrace lowers sigkillGrace for the duration of the test,
// restoring it on cleanup — the escalation path is tested via the actual
// sigkillGrace timer, never a mocked syscall.Kill, so a short grace keeps
// these tests fast without being flaky (per
// the `fix-flaky-tests-dont-defer` skill's timing guidance).
func withSigkillGrace(t *testing.T, grace time.Duration) {
	t.Helper()
	prev := sigkillGrace
	sigkillGrace = grace
	t.Cleanup(func() { sigkillGrace = prev })
}

// startSleepChild starts a plain "sleep" child under CommandContextPG. sleep
// has no signal handling of its own, so it terminates promptly on SIGTERM —
// this exercises the "SIGTERM succeeds" / common fast-exit path.
func startSleepChild(t *testing.T, ctx context.Context) *exec.Cmd {
	t.Helper()
	cmd := CommandContextPG(ctx, "sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep child: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })
	return cmd
}

// startSigtermIgnoringChild re-execs this test binary into
// runSigkillHelperProcess (which ignores SIGTERM and blocks), waiting for it
// to report readiness before returning.
func startSigtermIgnoringChild(t *testing.T, ctx context.Context) *exec.Cmd {
	t.Helper()
	cmd := CommandContextPG(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), sigkillHelperEnvVar+"=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sigterm-ignoring child: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })

	readyCh := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err == nil && strings.TrimSpace(line) != "ready" {
			err = fmt.Errorf("unexpected helper output: %q", line)
		}
		readyCh <- err
	}()
	select {
	case err := <-readyCh:
		if err != nil {
			t.Fatalf("failed waiting for helper readiness: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for sigterm-ignoring helper to report ready")
	}
	return cmd
}

func Test_CommandContextPG_LogsSigtermAtDebug(t *testing.T) {
	// Every test must pin a short grace: sigkillGrace and the AfterFunc timer
	// it schedules are process-global state (like slog's default logger and
	// the OTel meter), so a test that leaves the 5s default in place has its
	// escalation timer fire well after the test returns — into a later
	// test's captureLogs buffer, and on a reused pgid, mid-suite pid
	// collisions can turn a would-be ESRCH into a real (spurious) escalation.
	withSigkillGrace(t, 200*time.Millisecond)
	logs := captureLogs(t, slog.LevelDebug)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := startSleepChild(t, ctx)

	cancel()
	_ = cmd.Wait()

	// Let the escalation timer resolve (it should no-op: sleep already exited
	// on SIGTERM) before returning, so it can't fire during a later test.
	time.Sleep(500 * time.Millisecond)

	if !strings.Contains(logs.String(), "sent SIGTERM to process group") {
		t.Fatalf("expected Debug SIGTERM log, got: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "pgid") {
		t.Fatalf("expected pgid attribute in SIGTERM log, got: %s", logs.String())
	}
}

func Test_CommandContextPG_SigtermSucceeds_NoEscalationSignal(t *testing.T) {
	withSigkillGrace(t, 200*time.Millisecond)
	logs := captureLogs(t, slog.LevelDebug)
	before := sigkillEscalationsSum(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := startSleepChild(t, ctx)

	cancel()
	_ = cmd.Wait()

	// Give the AfterFunc-scheduled escalation timer a chance to fire (it
	// shouldn't do anything, since sleep already exited on SIGTERM) before
	// asserting the negative — generous bound per the flaky-test rule, not a
	// tight race with the timer.
	time.Sleep(500 * time.Millisecond)

	if strings.Contains(logs.String(), "level=WARN") {
		t.Fatalf("expected no Warn-level escalation log when SIGTERM succeeds, got: %s", logs.String())
	}
	if delta := sigkillEscalationsDelta(t, before); delta != 0 {
		t.Fatalf("expected sigkill_escalations delta 0 when SIGTERM succeeds, got %d", delta)
	}
}

func Test_CommandContextPG_EscalatesToSigkill_LogsWarnWithSnapshot(t *testing.T) {
	grace := 200 * time.Millisecond
	withSigkillGrace(t, grace)
	logs := captureLogsJSON(t, slog.LevelDebug)
	before := sigkillEscalationsSum(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := startSigtermIgnoringChild(t, ctx)
	// Captured now, not read from cmd.Process.Pid after cmd.Wait() below:
	// Wait reaps the process, and cmd.Process's Pid field can no longer be
	// trusted to reflect the process this test actually started once that
	// happens.
	pid := cmd.Process.Pid

	cancel()
	_ = cmd.Wait()

	// Wait past the grace period for the escalation to fire, with a
	// generous margin over the deterministic minimum.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logs.String(), "escalated to SIGKILL") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	record := findJSONLogRecord(t, logs, "escalated to SIGKILL")
	if record == nil {
		t.Fatalf("expected Warn escalation log, got: %s", logs.String())
	}
	if level, _ := record["level"].(string); level != slog.LevelWarn.String() {
		t.Fatalf("expected escalation log at Warn level, got level=%v (record: %v)", record["level"], record)
	}

	gotPgid, ok := record["pgid"].(float64)
	if !ok || int(gotPgid) != pid {
		t.Fatalf("expected pgid=%d in escalation log, got %v (record: %v)", pid, record["pgid"], record)
	}

	// grace is logged as a time.Duration, which slog's JSON handler encodes
	// as an int64 count of nanoseconds (see log/slog's json_handler.go) — so
	// this must equal the exact configured grace, not merely "present".
	gotGrace, ok := record["grace"].(float64)
	if !ok || time.Duration(gotGrace) != grace {
		t.Fatalf("expected grace=%s in escalation log, got %v (record: %v)", grace, record["grace"], record)
	}

	gotProcState, ok := record["proc_state"].(string)
	if !ok || gotProcState == "" || gotProcState == "unknown" {
		t.Fatalf("expected a live/zombie proc_state snapshot (not \"unknown\"/empty) in escalation log, got %v (record: %v)", record["proc_state"], record)
	}

	if delta := sigkillEscalationsDelta(t, before); delta != 1 {
		t.Fatalf("expected sigkill_escalations delta 1 after one confirmed escalation, got %d", delta)
	}
}

func Test_CommandContextPG_CancelReturnsPromptly(t *testing.T) {
	grace := 2 * time.Second // large relative to elapsed's 1s bound: proves Cancel doesn't wait on it
	withSigkillGrace(t, grace)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := startSleepChild(t, ctx)

	start := time.Now()
	cancel()
	_ = cmd.Wait()
	elapsed := time.Since(start)

	// Cancel's synchronous work is one syscall.Kill and one Debug log call;
	// well under the grace period is a generous, non-flaky bound.
	if elapsed > time.Second {
		t.Fatalf("cmd.Cancel path took %s, expected it to return promptly without waiting on sigkillGrace", elapsed)
	}

	// Let the escalation timer resolve (sleep already exited on SIGTERM, so
	// this is a no-op ESRCH) before returning, so it can't fire mid-suite
	// against a later test's captured logger or a reused pgid.
	time.Sleep(grace + 500*time.Millisecond)
}

func Test_procStateSnapshot_ReturnsKnownStateForLiveProcess(t *testing.T) {
	state := procStateSnapshot(os.Getpid())
	if state == "unknown" || state == "" {
		t.Fatalf("expected a known process state for the live test process, got %q", state)
	}
}

func Test_procStateSnapshot_ReturnsUnknownForDeadPid(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	if state := procStateSnapshot(pid); state != "unknown" {
		t.Fatalf("expected \"unknown\" for a reaped pid, got %q", state)
	}
}

func Test_procStateSnapshot_ConcurrentCallsDoNotRace(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = procStateSnapshot(os.Getpid())
		}()
	}
	wg.Wait()
}
