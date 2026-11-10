package svctest

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// homeEnvMu guards every test in this package that mutates the process-global
// HOME environment variable. HOME is read live (uncached) by
// config.GetConfigDirForDir via os.UserHomeDir() (config/config.go:125), so a
// writer that resolves its path under another test's HOME could plausibly make
// t.TempDir()'s end-of-test os.RemoveAll fail with "directory not empty". That
// mechanism is a hypothesis: the flake has not been reproduced on main.
//
// The lock is belt-and-braces: t.Setenv panics in parallel tests and serial
// tests run one at a time, so two mutators rarely overlap. It does NOT stop a
// goroutine leaked from one test from reading HOME live during another test —
// that reader side takes no lock. Any new test in this package that reads or
// writes HOME should go through withFakeHome/withFakeHomeAsFile;
// TestNoBareEnvMutationInServicesTests enforces that.
var homeEnvMu sync.Mutex

// homeLock / stateLock pair each mutex with the holder record that makes
// acquisition re-entrant per test — see envLock.acquire.
var (
	HomeLock  = &envLock{mu: &homeEnvMu}
	StateLock = &envLock{mu: &stateEnvMu}
)

// withFakeHome points HOME at a fresh temp directory for the duration of t,
// serialized against every other HOME-mutating test in this package via
// homeEnvMu. The lock is acquired before HOME is touched and released via
// t.Cleanup, so it is always released even if t later calls t.Fatal.
func WithFakeHome(t *testing.T) string {
	t.Helper()
	// Lock before t.TempDir() so the dir is created under the lock.
	HomeLock.Acquire(t)
	return WithFakeHomePath(t, t.TempDir())
}

// withFakeHomePath is withFakeHome for tests that already have (or must
// pre-populate) the directory HOME should point at.
func WithFakeHomePath(t *testing.T, home string) string {
	t.Helper()
	HomeLock.Acquire(t)

	t.Setenv("HOME", home)
	return home
}

// stateEnvMu guards every test in this package that mutates the process-global
// STAPLER_SQUAD_TEST_DIR environment variable, which config.GetConfigDirForDir
// reads live (Priority 1). It exists for the same reason as homeEnvMu: a
// background goroutine from one test that re-resolves its config path after a
// different test has reassigned the variable writes into (or expects) a
// directory that other test owns and is tearing down. Use withIsolatedStateDir
// rather than a bare t.Setenv or envtest.NewIsolatedStateDir.
//
// Lock order: by convention withFakeHome before withIsolatedStateDir. It is
// advisory: serial tests never overlap, so no deadlock is possible today.
var stateEnvMu sync.Mutex

// withIsolatedStateDir points STAPLER_SQUAD_TEST_DIR at a fresh temp directory
// for the duration of t, serialized against every other state-dir-mutating test
// in this package via stateEnvMu. The lock is released via t.Cleanup, so it is
// released even if t later calls t.Fatal. Like t.Setenv, it must be called
// before t.Parallel(). Repeat calls on one *testing.T (or its subtests) are
// safe: lock acquisition is re-entrant and the latest value wins.
func WithIsolatedStateDir(t *testing.T) string {
	t.Helper()
	StateLock.Acquire(t)
	return WithIsolatedStateDirPath(t, forgivingTempDir(t))
}

// forgivingTempDir is t.TempDir() whose cleanup retries RemoveAll: a goroutine leaked by an earlier
// test can be mid-MkdirAll in this directory (it resolved the path from the live env var) when cleanup
// runs, which fails t.TempDir()'s single RemoveAll with "directory not empty" and fails the test.
// The retries let that writer finish; once the env var is restored (earlier cleanup) no new writer can start.
func forgivingTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ssq-state-")
	if err != nil {
		t.Fatalf("create state dir: %v", err)
	}
	t.Cleanup(func() {
		var rmErr error
		for attempt := 0; attempt < 10; attempt++ {
			if rmErr = os.RemoveAll(dir); rmErr == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Errorf("remove state dir %s: %v", dir, rmErr)
	})
	return dir
}

// withIsolatedStateDirPath is withIsolatedStateDir for tests that already have
// (or must pre-populate) the directory, e.g. to seed a config.json first.
func WithIsolatedStateDirPath(t *testing.T, dir string) string {
	t.Helper()
	StateLock.Acquire(t)

	t.Setenv("STAPLER_SQUAD_TEST_DIR", dir)
	return dir
}

// envLock is a mutex plus a record of which test holds it, so the holder (and
// its subtests) can re-enter instead of deadlocking on a lock it already owns.
type envLock struct {
	mu     *sync.Mutex
	guard  sync.Mutex // protects holder
	holder string     // t.Name() of the holder; "" when unheld
}

func (l *envLock) heldBy(name string) bool {
	l.guard.Lock()
	defer l.guard.Unlock()
	return l.holder != "" && (name == l.holder || strings.HasPrefix(name, l.holder+"/"))
}

func (l *envLock) setHolder(name string) {
	l.guard.Lock()
	l.holder = name
	l.guard.Unlock()
}

// Acquire takes the mutex for the duration of t, releasing it via t.Cleanup.
// It is re-entrant per test: if t is the current holder, or a subtest of it
// (t.Name() has the holder's name plus "/" as a prefix), it returns without
// locking, so helper-of-helper call chains and subtests that call
// withFakeHome/withIsolatedStateDir again cannot self-deadlock. The holder's
// own t.Cleanup releases the lock, and t.Setenv (called by the caller) restores
// the variable per-t in LIFO order. Only the first acquirer registers cleanup.
//
// Lock order is home, then state; the reverse is not enforced because serial
// tests never overlap, but keep it so a future parallel-capable helper cannot
// deadlock.
func (l *envLock) Acquire(t *testing.T) {
	t.Helper()
	if l.heldBy(t.Name()) {
		return
	}
	l.mu.Lock()
	l.setHolder(t.Name())
	t.Cleanup(func() {
		l.setHolder("")
		l.mu.Unlock()
	})
}
