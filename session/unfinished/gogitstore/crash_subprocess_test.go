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

	waitErr := cmd.Wait()

	complete := true
	select {
	case <-readDone:
		readEnd.Close()
	case <-time.After(crashSubprocessDrainGrace):
		// Something is still holding the write end open past both the
		// subprocess's own exit/kill and our grace period — most likely a
		// detached process outside our process group. Force-close our read
		// end to unblock the goroutine's Read() and move on with whatever
		// was captured rather than hanging indefinitely.
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

	if ctx.Err() == context.DeadlineExceeded {
		t.Logf("subprocess killed by this test's own %s timeout — inconclusive, NOT proof of the crash/recovery this test exists to demonstrate; output captured before the kill:\n%s", crashSubprocessTimeout, out)
		return crashSubprocessResult{Outcome: crashSubprocessTimedOut, Output: out, Err: waitErr, OutputComplete: complete}
	}

	return crashSubprocessResult{Outcome: crashSubprocessExited, Output: out, Err: waitErr, OutputComplete: complete}
}
