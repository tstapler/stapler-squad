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
package gogitstore

import (
	"context"
	"os"
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
func runBoundedCrashSubprocess(t *testing.T, testName string, env string) crashSubprocessResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), crashSubprocessTimeout)
	defer cancel()

	cmd := safeexec.CommandContextPG(ctx, os.Args[0], "-test.run=^"+testName+"$", "-test.v")
	cmd.Env = append(os.Environ(), env)
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		t.Logf("subprocess killed by this test's own %s timeout — inconclusive, NOT proof of the crash/recovery this test exists to demonstrate; output captured before the kill:\n%s", crashSubprocessTimeout, out)
		return crashSubprocessResult{Outcome: crashSubprocessTimedOut, Output: out, Err: err}
	}

	return crashSubprocessResult{Outcome: crashSubprocessExited, Output: out, Err: err}
}
