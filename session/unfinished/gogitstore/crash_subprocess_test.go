// crash_subprocess_test.go provides a shared, bounded way to re-exec the
// test binary for the package's deliberate-crash negative-control tests
// (mmap_truncation_test.go's two TruncateWhileMapped tests and
// mmapindex_test.go's TestMmapIndexHandle_UnmapWhileSliceHeld_CausesCrash).
//
// All three previously called safeexec.CommandContext(context.Background(), ...)
// and blocked on cmd.CombinedOutput() with no bound. safeexec's WaitDelay
// only starts its clock once the passed context is cancelled —
// context.Background() never cancels — so if the subprocess hung instead of
// crashing/exiting (observed on this machine, root cause unconfirmed but
// plausibly an OS crash-reporter intercepting the fault), the parent test
// blocked until Go's own 600s test-binary timeout fired and failed the
// whole package, including every other, unrelated test in it.
//
// A first fix (context.WithTimeout + CommandContextPG + an explicit
// cmd.Cancel process-group SIGKILL) still called cmd.CombinedOutput(),
// which internally spawns goroutines that io.Copy the subprocess's
// stdout/stderr pipes and makes cmd.Wait() block until those goroutines see
// EOF — i.e. until every process holding the pipe's write end open closes
// it. If a detached process outside our process group (e.g. macOS's
// ReportCrash, which by design survives independently of the crashed
// process) inherited a copy of that write end, our own group-wide SIGKILL
// can never make it close, and Wait() — and therefore the whole test —
// keeps blocking past the context deadline regardless.
//
// The fix below avoids CombinedOutput()'s internal wait-for-EOF entirely by
// handing the child raw *os.File pipe ends directly as Stdout/Stderr: with
// an *os.File writer, exec.Cmd dup's the fd straight into the child instead
// of running its own io.Copy goroutine, so cmd.Wait() only blocks on the
// process itself exiting. Output is drained by our own goroutine reading
// the pipe's other end, which we bound independently: after Wait() returns
// we give that goroutine a short grace period to observe EOF, and if it
// hasn't, we forcibly close our read end to unblock it and proceed with
// whatever was captured, explicitly marking the result incomplete.
//
// A third hang, reproduced 2026-08-17 (-count=20, 1 hang in 20 runs, killed
// only by the test binary's own 5m -timeout), showed this still wasn't
// enough: the goroutine dump caught cmd.Wait() itself blocked in wait4(),
// never having returned, well past crashSubprocessTimeout. ctx expiring
// only triggers our cmd.Cancel override (SIGKILL of the process group) —
// it does not bound cmd.Wait() itself. Signaling a process is not the same
// as reaping it: if something (plausibly an OS crash reporter, per the
// original comment above) leaves the child stopped/traced instead of
// exited, wait4() can block forever regardless of how hard we signal it.
// cmd.Wait() below therefore runs in its own goroutine so it can be bounded
// independently, the same way the output-drain goroutine already is.
//
// Validation (2026-08-17): `go test -run TestMmapIndexHandle_TruncateWhileMapped_RecoverableWithProtection
// -count=20` completed cleanly in 551.763s with zero hangs. A full-package
// `-race` run at both 300s and 1200s timeouts instead failed on an unrelated,
// pre-existing hang in this package's git-fixture-building test helpers
// (buildPackedFixtureOnce/gitRunErr in gogitstore_test.go, also stuck in
// cmd.Wait() — the same failure class as this file's own history, but a
// different code path never touched here); that is tracked separately as
// backlog item 9083b3a8-b23a-484f-8940-d8ff7d788ccd. A `-race` run scoped to
// just this file's three tests (-run matching all three
// runBoundedCrashSubprocess callers) passed cleanly in 91.764s: each
// correctly logged its own 30s-timeout inconclusive-SKIP path rather than
// hanging, confirming the bound above holds under race-detector overhead.
package gogitstore

import (
	"bytes"
	"context"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// crashSubprocessTimeout bounds each deliberate-crash subprocess. Normal
// runs (fixture build + mmap + read) finish in low single-digit seconds on
// this machine; this gives roughly 10x headroom for a loaded CI/dev box
// while staying far under Go's 600s test-binary timeout.
const crashSubprocessTimeout = 30 * time.Second

// crashSubprocessDrainGrace bounds how long, after the subprocess itself
// has exited or been killed, the parent waits for its own output-reader
// goroutine to observe EOF on the pipe before giving up and proceeding with
// whatever was captured. This — not the process-group SIGKILL in Cancel —
// is what bounds the parent if a detached process outside our process
// group (e.g. an OS crash reporter) still holds the pipe's write end open.
const crashSubprocessDrainGrace = 2 * time.Second

// crashSubprocessWaitGrace bounds how long, after ctx's crashSubprocessTimeout
// expires and fires our cmd.Cancel SIGKILL override, the parent additionally
// waits for cmd.Wait() to return before giving up on it too. SIGKILL is a
// request, not a guarantee: if the OS leaves the child stopped/traced
// instead of exited (observed with a hung, never-returning cmd.Wait() during
// -count=20 reproduction), wait4() blocks forever with nothing else to bound
// it. This grace is intentionally short — a successful kill reaps almost
// immediately — so a genuine hang here is classified as timed-out rather
// than silently absorbed into a longer wait.
const crashSubprocessWaitGrace = 5 * time.Second

// crashSubprocessOutcome classifies how a deliberate-crash subprocess ended.
type crashSubprocessOutcome int

const (
	// crashSubprocessExited means the subprocess ran to completion (cleanly
	// or not) within crashSubprocessTimeout — the caller should inspect Err
	// and Output as before.
	crashSubprocessExited crashSubprocessOutcome = iota
	// crashSubprocessTimedOut means the subprocess was killed by
	// crashSubprocessTimeout before it exited. This proves nothing about
	// the crash the test exists to demonstrate and must never be treated
	// as "crashed as expected" or a sentinel-marker pass.
	crashSubprocessTimedOut
)

// crashSubprocessResult is the outcome of runBoundedCrashSubprocess.
type crashSubprocessResult struct {
	Outcome crashSubprocessOutcome
	Output  []byte
	Err     error
	// OutputComplete is false when the output-reader goroutine had to be
	// abandoned (forcibly closed) before it observed EOF — Output may be
	// truncated mid-line in that case.
	OutputComplete bool
}

// runBoundedCrashSubprocess re-execs the current test binary running only
// testName (via -test.run=^testName$), with env appended to the
// subprocess's environment, bounded by crashSubprocessTimeout.
//
// It uses safeexec.CommandContextPG to put the child in its own process
// group, plus an explicit cmd.Cancel override to SIGKILL that whole group:
// CommandContextPG's doc comment claims cancellation SIGTERMs the group,
// but it only sets SysProcAttr.Setpgid — the default Cancel
// (cmd.Process.Kill()) still targets only the direct child's PID. Without
// the override, a timeout firing mid-fixture-build (buildPackedFixture
// shells out to real git) could leave a git grandchild running past the
// kill.
//
// It deliberately does not use cmd.CombinedOutput(): that method blocks
// cmd.Wait() until its own io.Copy goroutines see EOF on the subprocess's
// stdout/stderr pipes, which never happens if some process outside our
// process group (killable via cmd.Cancel above) still holds the pipe's
// write end open. Instead, Stdout/Stderr are raw *os.File pipe ends (so
// exec.Cmd dups them straight into the child and Wait() only blocks on
// process exit), and our own reader goroutine draining the other end is
// bounded separately by crashSubprocessDrainGrace.
func runBoundedCrashSubprocess(t *testing.T, testName string, env string) crashSubprocessResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), crashSubprocessTimeout)
	defer cancel()

	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating output pipe: %v", err)
	}

	cmd := safeexec.CommandContextPG(ctx, os.Args[0], "-test.run=^"+testName+"$", "-test.v")
	cmd.Env = append(os.Environ(), env)
	cmd.Stdout = writeEnd
	cmd.Stderr = writeEnd
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	startErr := cmd.Start()
	// The parent must close its own copy of the write end right away,
	// win or lose: exec.Cmd dup'd it into the child, and as long as our
	// copy stays open here, the read end below can never see EOF even
	// after the child (and every grandchild) closes theirs.
	writeEnd.Close()
	if startErr != nil {
		readEnd.Close()
		t.Fatalf("starting crash subprocess: %v", startErr)
	}

	var (
		mu  sync.Mutex
		buf bytes.Buffer
	)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		var chunk [4096]byte
		for {
			n, rerr := readEnd.Read(chunk[:])
			if n > 0 {
				mu.Lock()
				buf.Write(chunk[:n])
				mu.Unlock()
			}
			if rerr != nil {
				return
			}
		}
	}()

	// cmd.Wait() is bounded independently of ctx: ctx expiring only fires
	// our cmd.Cancel override (a SIGKILL request), it does not itself bound
	// Wait(). If the OS never actually reaps the child — e.g. a crash
	// reporter left it stopped/traced instead of exited, reproduced during
	// -count=20 stress as a Wait() that never returned — wait4() blocks
	// forever with nothing else to stop it.
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	var (
		waitErr      error
		waitTimedOut bool
	)
	select {
	case waitErr = <-waitDone:
	case <-time.After(crashSubprocessTimeout + crashSubprocessWaitGrace):
		// The SIGKILL already fired when ctx expired but the child was
		// never reaped. Give up on Wait() rather than blocking
		// indefinitely; the goroutine above is abandoned and will finish,
		// if ever, sometime after this test returns.
		waitTimedOut = true
	}

	complete := true
	select {
	case <-readDone:
		readEnd.Close()
	case <-time.After(crashSubprocessDrainGrace):
		// Something is still holding the write end open past both the
		// subprocess's own exit/kill and our grace period — most likely a
		// detached process outside our process group, or (if waitTimedOut)
		// the subprocess never actually exited. Force-close our read end to
		// unblock the goroutine's Read() and move on with whatever was
		// captured rather than hanging indefinitely.
		complete = false
		readEnd.Close()
		<-readDone
	}

	mu.Lock()
	out := append([]byte(nil), buf.Bytes()...)
	mu.Unlock()

	if !complete {
		t.Logf("output reader abandoned after %s past subprocess exit — a process outside our group may still hold the output pipe open; output below may be truncated:\n%s", crashSubprocessDrainGrace, out)
	}

	if waitTimedOut {
		t.Logf("cmd.Wait() never returned within %s of this test's own SIGKILL — inconclusive, NOT proof of the crash/recovery this test exists to demonstrate; output captured before giving up:\n%s", crashSubprocessTimeout+crashSubprocessWaitGrace, out)
		return crashSubprocessResult{Outcome: crashSubprocessTimedOut, Output: out, Err: waitErr, OutputComplete: complete}
	}

	if ctx.Err() == context.DeadlineExceeded {
		t.Logf("subprocess killed by this test's own %s timeout — inconclusive, NOT proof of the crash/recovery this test exists to demonstrate; output captured before the kill:\n%s", crashSubprocessTimeout, out)
		return crashSubprocessResult{Outcome: crashSubprocessTimedOut, Output: out, Err: waitErr, OutputComplete: complete}
	}

	return crashSubprocessResult{Outcome: crashSubprocessExited, Output: out, Err: waitErr, OutputComplete: complete}
}
