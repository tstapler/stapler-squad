package session

// actor_test.go verifies IAC Epic 3: actor goroutine plumbing.
//
// Three invariants tested:
//  1. TestActorNoLeak    — Stop() joins the goroutine; goleak sees no leak.
//  2. TestActorSendSync  — command round-trips through mailbox; snapshot refreshed.
//  3. TestActorStopIdempotent — Stop() is safe to call multiple times.

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// knownBackgroundGoroutines lists process-wide goroutines started by the log
// package and lumberjack at init/first-log time, plus the signal-handler
// goroutine started by TestMain that blocks on a signal channel for the
// lifetime of the test binary.  These are not actor leaks; goleak must be
// told to ignore them so the actor-leak assertions stay tight.
var knownBackgroundGoroutines = []goleak.Option{
	goleak.IgnoreTopFunction("github.com/tstapler/stapler-squad/log.newAsyncWriter.func1"),
	goleak.IgnoreTopFunction("github.com/tstapler/stapler-squad/log.(*AsyncHandler).StartDrain.func1"),
	goleak.IgnoreAnyFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"),
	// TestMain spawns a signal-handler goroutine (integration_test.go) that
	// blocks on <-sigCh for the entire test-binary lifetime.  It cannot be
	// stopped without os.Exit, so we tell goleak to ignore it.
	goleak.IgnoreTopFunction("github.com/tstapler/stapler-squad/session.TestMain.func1"),
	goleak.IgnoreTopFunction("github.com/tstapler/stapler-squad/session.TestMain.func2"),
}

// newActorTestInstance constructs a minimal *Instance suitable for actor tests.
// It uses a real temp directory and a no-op program so NewInstance succeeds
// without needing tmux or git.
func newActorTestInstance(t *testing.T) *Instance {
	t.Helper()
	inst, err := NewInstance(InstanceOptions{
		Title:   "actor-test",
		Path:    t.TempDir(),
		Program: "echo",
	})
	require.NoError(t, err)
	return inst
}

// TestActorNoLeak confirms that Stop() joins the actor goroutine so goleak
// reports no leaked goroutines after the test.
//
// Snapshots the pre-test goroutine set via goleak.IgnoreCurrent() rather than
// asserting a bare process-wide goleak.VerifyNone(): this test runs inside
// the large `session` package's shared test binary, where earlier, unrelated
// tests can leave their own long-lived background goroutines running (e.g. a
// ResponseStream poller, an exec.Cmd wait) well past their own test's return.
// Without the baseline, this test's assertion is really "no goroutine is
// alive anywhere in the process", not its actual claim ("Stop() joins the
// actor goroutine THIS test started") - and fails on those unrelated leaks
// depending on run order/scheduling (observed in CI, not reproducible with
// -run isolating just this test locally).
func TestActorNoLeak(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, append(knownBackgroundGoroutines, baseline)...)

	inst := newActorTestInstance(t)
	li := NewLiveInstance(inst)
	li.Stop()
}

// TestActorSendSync confirms that sendSync enqueues a command, the actor
// executes it, and the atomic snapshot is updated afterwards.
func TestActorSendSync(t *testing.T) {
	inst := newActorTestInstance(t)
	li := NewLiveInstance(inst)
	defer li.Stop()

	var called bool
	err := li.sendSync(func(i *Instance) { called = true })
	require.NoError(t, err)
	require.True(t, called, "command closure must have been executed by the actor")

	// Snapshot must be non-nil after the command (actor calls buildSnapshot after each cmd).
	snap := li.Snapshot()
	require.NotNil(t, snap)
}

// TestActorStopIdempotent confirms that calling Stop() more than once does not
// panic, deadlock, or close the done channel twice.
//
// See TestActorNoLeak for why this baselines via goleak.IgnoreCurrent()
// instead of a bare process-wide goleak.VerifyNone().
func TestActorStopIdempotent(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, append(knownBackgroundGoroutines, baseline)...)

	inst := newActorTestInstance(t)
	li := NewLiveInstance(inst)
	li.Stop()
	li.Stop() // must not panic or deadlock
}
