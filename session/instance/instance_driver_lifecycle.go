package instance

import (
	"errors"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/internal/syncutil"
	"github.com/tstapler/stapler-squad/log"
)

// sessionDriverStopper carries the stop signal for one StartSessionDriver run
// (including any handleDriverFailure restart continuations of that same run,
// which reuse the same stopper — see handleDriverFailure). once guards
// against a racing double-close if StopSessionDriver is ever called
// concurrently.
//
// This previously also carried a `done` channel that StopSessionDriver waited
// on, but that channel was only closed by the *original* run's goroutine
// (StartSessionDriver's defer) — a handleDriverFailure-spawned retry
// goroutine is untracked by it, so StopSessionDriver could observe `done`
// closed and return while a retry continuation was still alive, reintroducing
// the goroutine-outlives-Destroy() bug this type exists to prevent. inst's
// driverWG (Add/Done'd across both the initial run and every retry
// continuation) is the correct join point — see StopSessionDriver and
// JoinSessionDriver, which both wait on it instead.
type sessionDriverStopper struct {
	stop chan struct{}
	once sync.Once
}

// ErrDriverDestroyed and ErrDriverRunning are BeginDriverRun's two refusals, kept
// distinct so StartSessionDriver logs the same message for each as before.
var (
	ErrDriverDestroyed = errors.New("session driver: instance destroyed")
	ErrDriverRunning   = errors.New("session driver: already running")
)

// BeginDriverRun claims the instance's single driver slot for StartSessionDriver
// (session_driver.go) and returns the stop channel the new goroutine must watch. It
// refuses with ErrDriverDestroyed once StopSessionDriver ran and ErrDriverRunning
// while a driver is live. A successful call must be paired with FinishDriverGoroutine.
func (i *Instance) BeginDriverRun() (<-chan struct{}, error) {
	i.driverMu.Lock()
	defer i.driverMu.Unlock()
	if i.driverDestroyed {
		return nil, ErrDriverDestroyed
	}
	if !i.driverRunning.CompareAndSwap(false, true) {
		return nil, ErrDriverRunning
	}
	i.driverGoroutines++
	stopper := &sessionDriverStopper{stop: make(chan struct{})}
	// Add(1) must happen before driverStopper is published (and before driverMu is
	// released): StopSessionDriver reads driverStopper under driverMu and, if
	// non-nil, immediately calls driverWG.Wait() outside the lock. Storing the
	// stopper before Add(1) would let that Wait() race the not-yet-executed Add
	// on a concurrent goroutine — sync.WaitGroup's own contract forbids Add and
	// Wait running concurrently without a happens-before edge, and go test -race
	// flags it. Doing Add under the same critical section as the Store makes the
	// mutex provide that ordering.
	i.driverWG.Add(1)
	i.driverStopper.Store(stopper)
	return stopper.stop, nil
}

// FinishDriverGoroutine ends a driver goroutine started by BeginDriverRun or
// RegisterDriverContinuation: it clears driverRunning when it was the last one, then
// releases the join WaitGroup (that order, so a waiter sees the cleared flag).
func (i *Instance) FinishDriverGoroutine() {
	defer i.driverWG.Done()
	i.driverGoroutineExited()
}

// DriverRunning reports whether a driver goroutine is live, for tests and the
// driver code that stays outside the Instance package.
func (i *Instance) DriverRunning() bool { return i.driverRunning.Load() }

// IsDestroyed reports whether Destroy has run, so a driver goroutine that outlives
// it can stop acting on the instance.
func (i *Instance) IsDestroyed() bool { return i.destroyed.Load() }

// RegisterDriverContinuation registers a continuation driver goroutine spawned by
// restartForRetry and returns the stop channel it must run with; it pairs with
// FinishDriverGoroutine. It refuses (ok=false) once StopSessionDriver has run,
// and does driverWG.Add(1) under driverMu so it cannot race StopSessionDriver's
// Wait — the same ordering StartSessionDriver relies on.
func (i *Instance) RegisterDriverContinuation(stop <-chan struct{}) (driverStop <-chan struct{}, ok bool) {
	i.driverMu.Lock()
	defer i.driverMu.Unlock()
	if i.driverDestroyed {
		return nil, false
	}
	driverStop = stop
	if driverStop == nil {
		if existing := i.driverStopper.Load(); existing != nil {
			driverStop = existing.stop
		} else {
			stopper := &sessionDriverStopper{stop: make(chan struct{})}
			i.driverStopper.Store(stopper)
			driverStop = stopper.stop
		}
	}
	i.driverGoroutines++
	i.driverRunning.Store(true)
	i.driverWG.Add(1)
	return driverStop, true
}

// driverGoroutineExited clears driverRunning only when the last driver
// goroutine exits: a driver that handed off to a continuation must not mark the
// instance driverless while the continuation still runs.
func (i *Instance) driverGoroutineExited() {
	i.driverMu.Lock()
	i.driverGoroutines--
	if i.driverGoroutines <= 0 {
		i.driverGoroutines = 0
		i.driverRunning.Store(false)
	}
	i.driverMu.Unlock()
}

// driverStopTimeout bounds how long StopSessionDriver blocks waiting for a
// running driver goroutine to observe the stop signal and exit. The driver
// loop derives a context.Context from stop (see runSessionDriverWithPrompt)
// and polls via Instance.PreviewContext, so closing stop cancels an
// in-flight CapturePaneContent subprocess directly rather than merely
// waiting for one to finish naturally. driverStopTimeout stays above
// execGateAcquireTimeout (5s, session/tmux/exec_gate.go) as a margin for
// process teardown after cancellation, not as the primary wait mechanism.
const DriverStopTimeout = 6 * time.Second

// StopSessionDriver signals a running SessionDriver goroutine (started by
// StartSessionDriver on inst) to exit at its next poll tick, and blocks up to
// driverStopTimeout for it — and any handleDriverFailure-spawned retry
// continuation of it — to actually do so. Called from Instance.Destroy() so
// no driver goroutine can survive past Destroy() and keep polling Preview()
// — which routes through the tmux exec gate's
// appconfig.LoadConfig()/GetConfigDir() (session/tmux/exec_gate.go) — using
// whatever STAPLER_SQUAD_TEST_DIR a later test has since set process-wide.
// Also marks inst as destroyed (under driverMu, alongside the stopper read)
// so a StartSessionDriver call that is still in flight — or arrives later —
// can never spawn a driver goroutine for this instance again; see
// Instance.driverMu's doc comment. A no-op (beyond that marking) if the
// driver was never started.
//
// Waits on inst.driverWG rather than a per-run "done" channel: driverWG is
// Add(1)/Done()'d across both the initial run (StartSessionDriver) and every
// handleDriverFailure retry continuation, so this can't return early while a
// retry goroutine spawned after the stop signal was sent is still running —
// see JoinSessionDriver, which uses the identical wait for the same reason.
func StopSessionDriver(inst *Instance) {
	inst.driverMu.Lock()
	inst.driverDestroyed = true
	stopper := inst.driverStopper.Load()
	inst.driverMu.Unlock()
	if stopper == nil {
		return
	}
	stopper.once.Do(func() { close(stopper.stop) })
	if !syncutil.WaitWithTimeout(&inst.driverWG, DriverStopTimeout) {
		log.Warn("SessionDriver: timed out waiting for driver goroutine to stop",
			"session", inst.Title,
		)
	}
}

// JoinSessionDriver waits for any in-flight SessionDriver goroutine (including
// a handleDriverFailure-spawned restart) to exit, up to stopJoinTimeout. Tests
// should call this before relying on t.TempDir() cleanup, since a driver
// goroutine can otherwise outlive the temp dir it was launched against.
func JoinSessionDriver(inst *Instance) {
	if !syncutil.WaitWithTimeout(&inst.driverWG, StopJoinTimeout) {
		log.Warn("JoinSessionDriver: driver goroutine did not exit within timeout; it may still be running",
			"session", inst.Title, "timeout", StopJoinTimeout)
	}
}

// DriverControl is the seam through which Instance code starts the session driver
// and runs a retry restart without naming session_driver.go, which stays above the
// Instance file set. Two implementations: sessionDriverControl (production) and any
// test fake installed in driverControlHook.
type DriverControl interface {
	Start(inst *Instance, allowedPath string)
	RestartForRetry(inst *Instance, allowedPath, continuationPrompt string, policy RetryPolicy, stop <-chan struct{}) error
}

// driverControlHook is assigned once, in an init() of the staying driver code, so it is
// race-free and never written after startup.
var DriverControlHook DriverControl

func driverControl() DriverControl {
	if DriverControlHook == nil {
		panic("session: DriverControl not registered (the package implementing the session driver must set driverControlHook in init)")
	}
	return DriverControlHook
}
